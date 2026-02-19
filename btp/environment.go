package btp

import (
	"context"
	"encoding/json"
	"net/http"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

// First tries to get the environment by id, if not found, it tries to get it by name and type
func (c *Client) GetEnvironment(
	ctx context.Context, Id string, instanceName string, environmentType EnvironmentType,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	// Try to get the environment by id first
	environmentInstance, _, err := c.GetEnvironmentInstanceByID(ctx, Id)
	if err != nil {
		return nil, err
	}

	if environmentInstance != nil {
		return environmentInstance, nil
	}

	// If not found by id, try to get it by name and type
	return c.GetEnvironmentByNameAndType(ctx, instanceName, environmentType)
}

func (c *Client) GetEnvironmentInstanceByID(ctx context.Context, instanceID string) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
	response, resp, err := c.ProvisioningServiceClient.GetEnvironmentInstance(ctx, instanceID).Execute()

	if err != nil {
		return nil, resp.StatusCode == 404, specifyAPIError(err)
	}

	return response, false, nil
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

// GetEnvironmentById retrieves environment using its ID. It performs a list and filters client-side.
// Deprecated: use GetEnvironmentsByID instead.
func (c *Client) GetEnvironmentById(
	ctx context.Context, Id string,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {

	var environmentInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	response, _, err := c.ProvisioningServiceClient.GetEnvironmentInstances(ctx).Authorization("").Execute()

	if err != nil {
		return nil, specifyAPIError(err)
	}

	for _, instance := range response.EnvironmentInstances {

		var parameters string
		var parameterList map[string]interface{}
		if instance.Parameters != nil {
			parameters = *instance.Parameters
		}
		err := json.Unmarshal([]byte(parameters), &parameterList)
		if err != nil {
			return nil, err
		}
		if instance.Id != nil && *instance.Id == Id {
			environmentInstance = &instance
			break
		}

	}
	return environmentInstance, err

}

func (c *Client) DeleteEnvironmentInstanceByID(ctx context.Context, instanceID string) (*http.Response, error) {
	_, raw, err := c.ProvisioningServiceClient.DeleteEnvironmentInstance(ctx, instanceID).Execute()
	if err != nil {
		return raw, specifyAPIError(err)
	}
	return raw, nil
}
