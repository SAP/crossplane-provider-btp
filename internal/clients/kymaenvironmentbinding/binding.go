package kymaenvironmentbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

const (
	errKymaBindingCreateFailed = "Could not create KymaEnvironmentBinding"
	errKymaBindingDeleteFailed = "Could not delete KymaEnvironmentBinding"
)

var _ Client = &KymaBindings{}

type KymaBindings struct {
	btp btp.Client
}

func NewKymaBindings(btp btp.Client) *KymaBindings {
	return &KymaBindings{btp: btp}
}

func (c KymaBindings) DescribeInstance(
	ctx context.Context,
	kymaInstanceId string,
) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {

	bindings, _, err := c.btp.ProvisioningServiceClient.GetAllEnvironmentInstanceBindings(ctx, kymaInstanceId).Execute()
	if err != nil {
		return make([]provisioningclient.EnvironmentInstanceBindingMetadata, 0), errors.Wrap(err, errKymaBindingCreateFailed)
	}

	return bindings.Bindings, nil
}

func (c KymaBindings) CreateInstance(ctx context.Context, kymaInstanceId string, ttl int) (*Binding, error) {

	params := make(map[string]interface{})
	params["expiration_seconds"] = ttl
	binding, h, err := c.btp.ProvisioningServiceClient.CreateEnvironmentInstanceBinding(ctx, kymaInstanceId).
		CreateEnvironmentInstanceBindingRequest(provisioningclient.CreateEnvironmentInstanceBindingRequest{Parameters: params}).
		Execute()
	if err != nil {
		return nil, errors.Wrap(specifyAPIError(err), errKymaBindingCreateFailed)
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

func (c KymaBindings) DeleteInstances(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
	for _, binding := range bindings {
		if _, http, err := c.btp.ProvisioningServiceClient.DeleteEnvironmentInstanceBinding(ctx, kymaInstanceId, binding.Id).Execute(); err != nil {
			if http != nil && http.StatusCode != 404 {
				return errors.Wrap(err, errKymaBindingDeleteFailed)
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
