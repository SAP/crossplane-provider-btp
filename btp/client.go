package btp

import (
	"fmt"

	"github.com/pkg/errors"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func NewBTPClient(cisSecretData []byte, serviceAccountSecretData []byte) (*Client, error) {

	accountsServiceClient, err := ServiceClientFromSecret(cisSecretData, serviceAccountSecretData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get BTP accounts service client.")
	}
	return &accountsServiceClient, nil
}

func specifyAPIError(err error) error {
	if genericErr, ok := err.(*provisioningclient.GenericOpenAPIError); ok {
		if provisionErr, ok := genericErr.Model().(provisioningclient.ApiExceptionResponseObject); ok {
			return errors.New(fmt.Sprintf("API Error: %v, Code %v", provisionErr.Error.Message, provisionErr.Error.Code))
		}
		if genericErr.Body() != nil {
			return errors.Wrap(err, string(genericErr.Body()))
		}
	}
	return err
}
