package btp

import (
	"context"
	"encoding/json"

	"github.com/sap/crossplane-provider-btp/internal"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func KymaEnvironmentType() EnvironmentType {
	return EnvironmentType{
		Identifier:  "kyma",
		ServiceName: "kymaruntime",
	}
}

// GetKymaEnvironment retrieves a Kyma environment using either UUID-based lookup (new format, >= v1.2.2)
// or name-based lookup (legacy format, < v1.2.2).
//
// For UUID external-names: directly retrieves by ID, returns nil if not found (drift scenario).
// For legacy name external-names: falls back to GetEnvironmentByNameAndType which lists and filters client-side.
func (c *Client) GetKymaEnvironment(
	ctx context.Context, Id string, instanceName string, environmentType EnvironmentType,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	if internal.IsValidUUID(Id) {
		// New format (>= v1.2.2): external-name is a UUID, use direct ID lookup
		environmentInstance, notFound, err := c.GetEnvironmentInstanceByID(ctx, Id)
		if notFound {
			// 404 is not an error - resource doesn't exist (drift scenario)
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return environmentInstance, nil
	}

	// Legacy format (< v1.2.2): external-name is the instance name, use name-based lookup
	return c.GetEnvironmentByNameAndType(ctx, instanceName, environmentType)
}

func (c *Client) CreateKymaEnvironment(ctx context.Context, instanceName string, planeName string, parameters InstanceParameters, resourceUID string, serviceAccountEmail string) (string, error) {
	envType := KymaEnvironmentType()
	payload := provisioningclient.CreateEnvironmentInstanceRequestPayload{
		Description:     internal.Ptr("created via crossplane-provider-btp-account"),
		EnvironmentType: envType.Identifier,
		Name:            &instanceName,
		Origin:          nil,
		Parameters:      parameters,
		PlanName:        planeName,
		ServiceName:     envType.ServiceName,
		TechnicalKey:    nil,
		User:            &serviceAccountEmail,
	}
	obj, _, err := c.ProvisioningServiceClient.CreateEnvironmentInstance(ctx).CreateEnvironmentInstanceRequestPayload(payload).Execute()

	if err != nil {
		return "", specifyAPIError(err)
	}

	return *obj.Id, nil
}

func (c *Client) UpdateKymaEnvironment(ctx context.Context, environmentInstanceId string, planeName string, instanceParameters InstanceParameters, resourceUID string) error {
	payload := provisioningclient.UpdateEnvironmentInstanceRequestPayload{
		Parameters: instanceParameters,
		PlanName:   planeName,
	}

	_, _, err := c.ProvisioningServiceClient.UpdateEnvironmentInstance(ctx, environmentInstanceId).UpdateEnvironmentInstanceRequestPayload(payload).Execute()
	if err != nil {
		return specifyAPIError(err)
	}

	return nil
}

// GetEnvironmentByNameAndType retrieves environment using its name and type. It performs a list and filters client-side.
// Deprecated: use GetEnvironmentsByID instead.
func (c *Client) GetEnvironmentByNameAndType(
	ctx context.Context, instanceName string, environmentType EnvironmentType,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	var environmentInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	response, _, err := c.ProvisioningServiceClient.GetEnvironmentInstances(ctx).Authorization("").Execute()

	if err != nil {
		return nil, specifyAPIError(err)
	}

	for _, instance := range response.EnvironmentInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != environmentType.Identifier {
			continue
		}

		var parameters string
		var parameterList map[string]interface{}
		if instance.Parameters != nil {
			parameters = *instance.Parameters
		}
		err := json.Unmarshal([]byte(parameters), &parameterList)
		if err != nil {
			return nil, err
		}
		// keeping the old parameter to not potentially break systems, function is deprecated and should not be used anymore
		if parameterList[cfenvironmentParameterInstanceName] == instanceName {
			environmentInstance = &instance
			break
		}
		if parameterList[KymaenvironmentParameterInstanceName] == instanceName {
			environmentInstance = &instance
			break
		}
	}
	return environmentInstance, err
}
