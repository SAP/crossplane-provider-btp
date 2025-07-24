package kymamodule

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

const (
	errKymaModuleCreateFailed = "Could not create KymaModule"
	errKymaModuleDeleteFailed = "Could not delete KymaModule"
)

var _ Client = &KymaModules{}

type KymaModules struct {
	kube client.Client
}

func NewKymaModule(kube client.Client) *KymaModules {
	return &KymaModules{kube: kube}
}

func (c KymaModules) DescribeInstance(
	ctx context.Context,
	moduleName string,
	moduleChannel string,
) (*v1alpha1.KymaModuleObservation, error) {

	bindings, _, err := c.btp.ProvisioningServiceClient.GetAllEnvironmentInstanceBindings(ctx, kymaInstanceId).Execute()
	if err != nil {
		return make([]provisioningclient.EnvironmentInstanceBindingMetadata, 0), errors.Wrap(err, errKymaModuleCreateFailed)
	}

	if bindings == nil {
		return make([]provisioningclient.EnvironmentInstanceBindingMetadata, 0), nil
	}

	return bindings.Bindings, nil
}

func (c KymaModules) CreateInstance(ctx context.Context, moduleName string, moduleChannel string) error {

	params := make(map[string]interface{})
	params["expiration_seconds"] = ttl
	binding, h, err := c.btp.ProvisioningServiceClient.CreateEnvironmentInstanceBinding(ctx, kymaInstanceId).
		CreateEnvironmentInstanceBindingRequest(provisioningclient.CreateEnvironmentInstanceBindingRequest{Parameters: params}).
		Execute()
	if err != nil {
		return nil, errors.Wrap(specifyAPIError(err), errKymaModuleCreateFailed)
	}
	marshal, err := json.Marshal(binding)
	if err != nil {
		return nil, err
	}
	var bindingMetadata Binding
	err = json.Unmarshal(marshal, &bindingMetadata)
	if err != nil {
		return nil, err
	}
	locationValue := h.Header.Get("Location")
	if locationValue != "" {
		if bindingMetadata.Metadata == nil {
			bindingMetadata.Metadata = &Metadata{}
		}
		bindingMetadata.Metadata.Id = locationValue
	}
	return &bindingMetadata, nil
}

func (c KymaModules) DeleteInstance(ctx context.Context, moduelName string) error {
	for _, binding := range bindings {
		if _, http, err := c.btp.ProvisioningServiceClient.DeleteEnvironmentInstanceBinding(ctx, kymaInstanceId, binding.Id).Execute(); err != nil {
			if http != nil && http.StatusCode != 404 {
				return errors.Wrap(err, errKymaModuleDeleteFailed)
			}
		}
	}

	return nil
}

type Credentials struct {
	Kubeconfig string `json:"kubeconfig,omitempty"`
}
type Metadata struct {
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Id        string    `json:"id,omitempty"`
}

type Binding struct {
	Metadata    *Metadata    `json:"metadata,omitempty"`
	Credentials *Credentials `json:"credentials,omitempty"`
}

func specifyAPIError(err error) error {
	if genericErr, ok := err.(*provisioningclient.GenericOpenAPIError); ok {
		if specific, ok := genericErr.Model().(provisioningclient.ApiExceptionResponseObject); ok {
			return fmt.Errorf("API Error: %v, Code %v", specific.Error.Message, specific.Error.Code)
		}
		if genericErr.Body() != nil {
			return fmt.Errorf("API Error: %s", string(genericErr.Body()))
		}
	}
	return err
}
