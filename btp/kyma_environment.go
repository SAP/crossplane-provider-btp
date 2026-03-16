package btp

import (
	"context"

	"github.com/sap/crossplane-provider-btp/internal"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func KymaEnvironmentType() EnvironmentType {
	return EnvironmentType{
		Identifier:  "kyma",
		ServiceName: "kymaruntime",
	}
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
