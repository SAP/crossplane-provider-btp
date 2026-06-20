package security

import (
	"fmt"

	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

// SpecifyAPIError unwraps an XSUAA OpenAPI error so that the response body
// (or parsed model) is surfaced instead of just the HTTP status string.
// It returns err unchanged when err is not an *xsuaa.GenericOpenAPIError.
func SpecifyAPIError(err error) error {
	genericErr, ok := err.(*xsuaa.GenericOpenAPIError)
	if !ok {
		return err
	}
	if model := genericErr.Model(); model != nil {
		return fmt.Errorf("API Error: %v", model)
	}
	if body := genericErr.Body(); len(body) > 0 {
		return fmt.Errorf("API Error: %s", string(body))
	}
	return err
}
