package btp

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"

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
	Id          string `json:"Org Id"`
	Name        string `json:"Org Name"`
	ApiEndpoint string `json:"API Endpoint"`
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

	// The instance exists in CIS from here on, so the ID is returned even if the org lookup fails.
	cfOrg, err := c.GetCloudFoundryOrg(ctx, instanceId)
	if err != nil {
		return instanceId, nil, err
	}
	return instanceId, cfOrg, nil
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

// Label keys BTP has been observed to use for the Cloud Foundry org details. Keys may or may not carry a trailing
// colon (:), and the variants can be mixed within a single labels object, so each key is resolved independently.
var (
	cfOrgIdLabelKeys       = []string{"Org ID", "Org ID:"}
	cfOrgNameLabelKeys     = []string{"Org Name", "Org Name:"}
	cfApiEndpointLabelKeys = []string{"API Endpoint", "API Endpoint:"}
)

// NewCloudFoundryOrgByLabel creates a CloudFoundryOrg from a JSON-formatted labels string.
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

	//having empty values will be handled by the caller
	org := CloudFoundryOrgFromLabels(labels)
	return &org, nil
}

// CloudFoundryOrgFromLabels extracts the Cloud Foundry org details from already unmarshalled environment labels.
// Every field is looked up on its own, accepting all known key variants. Fields without a matching label are empty.
func CloudFoundryOrgFromLabels(labels map[string]string) CloudFoundryOrg {
	return CloudFoundryOrg{
		Id:          lookupLabel(labels, cfOrgIdLabelKeys),
		Name:        lookupLabel(labels, cfOrgNameLabelKeys),
		ApiEndpoint: lookupLabel(labels, cfApiEndpointLabelKeys),
	}
}

func lookupLabel(labels map[string]string, keys []string) string {
	for _, key := range keys {
		if value, ok := labels[key]; ok {
			return value
		}
	}
	return ""
}

// GetCFEnvironmentByResourceUID finds the Cloud Foundry environment instance that was created for the resource with the
// given UID. Create stores the UID in the instance parameters, so it identifies environments created by this resource.
func (c *Client) GetCFEnvironmentByResourceUID(
	ctx context.Context, resourceUID string,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	if resourceUID == "" {
		return nil, nil
	}
	envInstances, err := c.ListCFEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range envInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != CloudFoundryEnvironmentType().Identifier {
			continue
		}
		if instance.Id == nil || instance.Parameters == nil {
			continue
		}
		var parameters map[string]interface{}
		if err := json.Unmarshal([]byte(*instance.Parameters), &parameters); err != nil {
			continue // skip invalid JSON
		}
		if parameters[v1alpha1.SubaccountOperatorLabel] == resourceUID {
			return &instance, nil
		}
	}
	return nil, nil
}
