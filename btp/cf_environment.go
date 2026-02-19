package btp

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func CloudFoundryEnvironmentType() EnvironmentType {
	return EnvironmentType{
		Identifier:  "cloudfoundry",
		ServiceName: "cloudfoundry",
	}
}

type CloudFoundryOrg struct {
	Id          string `json:"Org Id,"`
	Name        string `json:"Org Name,"`
	ApiEndpoint string `json:"API Endpoint,"`
}

// CreateCloudFoundryEnvironment creates a Cloud Foundry environment instance with the given parameters. It returns the instance ID of the created environment.
func (c *Client) CreateCloudFoundryEnvironment(
	ctx context.Context, serviceAccountEmail string, resourceUID string,
	landscape string, orgName string, environmentName string,
) (instanceId string, err error) {
	parameters := map[string]interface{}{
		cfenvironmentParameterInstanceName: orgName, v1alpha1.SubaccountOperatorLabel: resourceUID,
	}
	cloudFoundryPlanName := "standard"
	envType := CloudFoundryEnvironmentType()

	var envName *string = nil
	if environmentName != "" {
		envName = &environmentName
	}
	payload := provisioningclient.CreateEnvironmentInstanceRequestPayload{
		Description:     internal.Ptr("created via crossplane-btp-account-provider"),
		EnvironmentType: envType.Identifier,
		LandscapeLabel:  &landscape,
		Name:            envName,
		Origin:          nil,
		Parameters:      parameters,
		PlanName:        cloudFoundryPlanName,
		ServiceName:     envType.ServiceName,
		TechnicalKey:    nil,
		User:            &serviceAccountEmail,
	}
	localReturnValue, _, err := c.ProvisioningServiceClient.CreateEnvironmentInstance(ctx).CreateEnvironmentInstanceRequestPayload(payload).Execute()
	if err != nil {
		return "", specifyAPIError(err)
	}
	instanceId = *localReturnValue.Id
	return instanceId, nil
}

func (c *Client) CreateCloudFoundryEnvironmentAndGetOrg(
	ctx context.Context, instanceName string, serviceAccountEmail string, resourceUID string,
	landscape string, orgName string, environmentName string,
) (string, *CloudFoundryOrg, error) {

	instanceId, err := c.CreateCloudFoundryEnvironment(ctx, serviceAccountEmail, resourceUID, landscape, orgName, environmentName)
	if err != nil {
		return "", nil, err
	}

	cfOrg, err := c.GetCloudFoundryOrg(ctx, instanceId)
	if err != nil {
		return "", nil, err
	}
	return instanceId, cfOrg, err
}

func (c *Client) GetCloudFoundryOrg(
	ctx context.Context, instanceId string,
) (*CloudFoundryOrg, error) {
	cfEnvironment, _, err := c.GetEnvironmentInstanceByID(ctx, instanceId)
	if err != nil {
		return nil, err
	}
	return c.ExtractOrg(cfEnvironment)
}

func (c *Client) GetCFEnvironmentByNameAndOrg(
	ctx context.Context, instanceName string, orgName string,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	envInstances, err := c.ListCFEnvironments(ctx)
	if err != nil {
		return nil, err
	}

	return findCFEnvironment(envInstances, instanceName, orgName)
}

// GetCFEnvironmentByOrgId retrieves CF environment using orgId by doing a list with client side filtering
func (c *Client) GetCFEnvironmentByOrgId(ctx context.Context, orgId string) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	var environmentInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	envInstances, err := c.ListCFEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range envInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != CloudFoundryEnvironmentType().Identifier {
			continue
		}
		if instance.Id != nil && *instance.Id == orgId {
			environmentInstance = &instance
			break
		}
	}
	return environmentInstance, err
}

// findCFEnvironment tries to find a Cloud Foundry environment instance by matching either the ID or the instance name/org name in parameters
func findCFEnvironment(envInstances []provisioningclient.BusinessEnvironmentInstanceResponseObject, instanceName string, orgName string) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	// First check ID match
	for _, instance := range envInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != CloudFoundryEnvironmentType().Identifier {
			continue
		}
		if instance.Id != nil && *instance.Id == instanceName {
			return &instance, nil
		}
	}
	// Then check instance name and org name match
	for _, instance := range envInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != CloudFoundryEnvironmentType().Identifier {
			continue
		}
		var parameters string
		if instance.Parameters != nil {
			parameters = *instance.Parameters
		}
		var parameterList map[string]interface{}
		if err := json.Unmarshal([]byte(parameters), &parameterList); err != nil {
			continue // skip invalid JSON
		}
		if parameterList[cfenvironmentParameterInstanceName] == instanceName || parameterList[cfenvironmentParameterInstanceName] == orgName {
			return &instance, nil
		}
	}
	return nil, nil
}

func (c *Client) ListCFEnvironments(ctx context.Context) ([]provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	response, _, err := c.ProvisioningServiceClient.GetEnvironmentInstances(ctx).Authorization("").Execute()
	if err != nil {
		return nil, specifyAPIError(err)
	}
	return response.EnvironmentInstances, nil
}

func (c *Client) ExtractOrg(cfEnvironment *provisioningclient.BusinessEnvironmentInstanceResponseObject) (*CloudFoundryOrg, error) {
	if cfEnvironment == nil {
		return nil, nil
	}

	var label string
	if cfEnvironment.Labels != nil {
		label = *cfEnvironment.Labels
	}

	return NewCloudFoundryOrgByLabel(label)
}

// NewCloudFoundryOrgByLabel creates a CloudFoundryOrg from a JSON-formatted labels string.
// In legacy format, the keys have a trailing colon (:), while in the new format they do not.
// The function handles both formats.
func NewCloudFoundryOrgByLabel(rawLabels string) (*CloudFoundryOrg, error) {
	if rawLabels == "" {
		return nil, errors.New("labels string is empty")
	}
	labels := make(map[string]string)
	err := json.Unmarshal([]byte(rawLabels), &labels)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling labels format")
	}

	if len(labels) == 0 {
		return nil, errors.New("no labels found in the provided string")
	}

	oldOrgId, oldOrgIdExists := labels["Org ID:"]
	oldApiEndpoint, oldApiEndpointExist := labels["API Endpoint:"]

	if oldOrgIdExists || oldApiEndpointExist {
		//labels are in the old format
		return &CloudFoundryOrg{
			Id:          oldOrgId,
			Name:        labels["Org Name"],
			ApiEndpoint: oldApiEndpoint,
		}, nil
	}

	//use the new format, having empty values will be handled by the caller
	return &CloudFoundryOrg{
		Id:          labels["Org ID"],
		Name:        labels["Org Name"],
		ApiEndpoint: labels["API Endpoint"],
	}, nil
}
