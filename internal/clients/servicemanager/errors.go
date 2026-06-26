package servicemanager

import (
	"fmt"

	"github.com/sap/crossplane-provider-btp/internal"
	smclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

// specifyAPIError unwraps a *smclient.GenericOpenAPIError so the BTP-side message
// (structured smclient.Error model when present, raw body otherwise) is surfaced
// in the returned error. Non-OpenAPI errors are returned unchanged.
func specifyAPIError(err error) error {
	genericErr, ok := err.(*smclient.GenericOpenAPIError)
	if !ok {
		return err
	}
	if smErr, ok := genericErr.Model().(smclient.Error); ok {
		return fmt.Errorf("API Error: %s, Description: %s", internal.Val(smErr.Error), internal.Val(smErr.Description))
	}
	if body := genericErr.Body(); len(body) > 0 {
		return fmt.Errorf("API Error: %s", string(body))
	}
	return err
}
