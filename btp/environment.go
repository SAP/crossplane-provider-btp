package btp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func (c *Client) GetEnvironmentInstanceByID(ctx context.Context, instanceID string) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
	response, resp, err := c.ProvisioningServiceClient.GetEnvironmentInstance(ctx, instanceID).Execute()

	if err != nil {
		return nil, resp.StatusCode == 404, specifyAPIError(err)
	}

	return response, false, nil
}

// FindEnvironment returns the single environment instance of envType whose
// name, as carried in envType.InstanceNameParameter, equals name. found is
// false when none matches; more than one match is an error, never resolved by
// picking one. Instances whose parameters cannot be parsed cannot carry the
// name and are skipped.
func (c *Client) FindEnvironment(ctx context.Context, envType EnvironmentType, name string) (provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
	named := func(instance provisioningclient.BusinessEnvironmentInstanceResponseObject) bool {
		var parameters map[string]any
		if err := json.Unmarshal([]byte(instance.GetParameters()), &parameters); err != nil {
			return false
		}
		return parameters[envType.InstanceNameParameter] == name
	}
	return c.findOneEnvironment(ctx, envType, named, fmt.Sprintf(" named %q", name))
}

// FindCloudFoundryEnvironment returns the subaccount's Cloud Foundry
// environment. BTP allows at most one per subaccount, so it is found by type
// alone; finding more than one is an error.
func (c *Client) FindCloudFoundryEnvironment(ctx context.Context) (provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
	everyInstance := func(provisioningclient.BusinessEnvironmentInstanceResponseObject) bool { return true }
	return c.findOneEnvironment(ctx, CloudFoundryEnvironmentType(), everyInstance, "")
}

// findOneEnvironment returns the single environment instance of envType that
// match accepts. desc qualifies the environments in the ambiguity error, e.g.
// ` named "x"`.
func (c *Client) findOneEnvironment(ctx context.Context, envType EnvironmentType, match func(provisioningclient.BusinessEnvironmentInstanceResponseObject) bool, desc string) (provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
	instances, err := c.ListCFEnvironments(ctx)
	if err != nil {
		return provisioningclient.BusinessEnvironmentInstanceResponseObject{}, false, err
	}

	var matches []provisioningclient.BusinessEnvironmentInstanceResponseObject
	for _, instance := range instances {
		if instance.GetEnvironmentType() == envType.Identifier && match(instance) {
			matches = append(matches, instance)
		}
	}

	switch len(matches) {
	case 0:
		return provisioningclient.BusinessEnvironmentInstanceResponseObject{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return provisioningclient.BusinessEnvironmentInstanceResponseObject{}, false, errors.Errorf(
			"found %d %s environments%s in this subaccount", len(matches), envType.Identifier, desc)
	}
}

// GetEnvironmentById retrieves environment using its ID. It performs a list and filters client-side.
// Deprecated: use GetEnvironmentInstanceByID instead.
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
