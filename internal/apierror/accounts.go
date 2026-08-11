// Package apierror translates errors returned by the generated BTP OpenAPI
// clients into messages that are useful in a managed resource's status and
// events.
package apierror

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sap/crossplane-provider-btp/internal"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

// FromAccounts converts an error returned by the accounts service client into a
// readable error, including the per-field reasons the API reports in "details".
// Errors that did not come from that client are returned unchanged.
func FromAccounts(err error) error {
	genericErr, ok := err.(*accountclient.GenericOpenAPIError)
	if !ok {
		return err
	}

	if accountError, ok := genericErr.Model().(accountclient.ApiExceptionResponseObject); ok {
		general := fmt.Sprintf("API Error: %v, Code %v", internal.Val(accountError.Error.Message), internal.Val(accountError.Error.Code))

		// The API reports the actual cause of a rejected payload in a "details"
		// array rather than in the top-level message. Without it the caller only
		// sees "Request payload is invalid, Code 11000" and has to inspect the
		// raw response to find out which field was wrong.
		if details := detailMessages(accountError.Error.AdditionalProperties); len(details) > 0 {
			return fmt.Errorf("%s; details: %s", general, strings.Join(details, "; "))
		}
		return errors.New(general)
	}

	if genericErr.Body() != nil {
		return fmt.Errorf("API Error: %s", string(genericErr.Body()))
	}

	return err
}

// detailMessages pulls the messages out of the error's "details" array.
//
// The generated ApiExceptionResponseObjectError has no Details field, but its
// UnmarshalJSON collects every key it does not model into AdditionalProperties,
// so the details survive there as plain JSON values. Anything not matching the
// expected shape is skipped rather than reported, which leaves the caller with
// the same general message it would have produced before.
func detailMessages(additionalProperties map[string]interface{}) []string {
	entries, ok := additionalProperties["details"].([]interface{})
	if !ok {
		return nil
	}

	messages := make([]string, 0, len(entries))
	for _, entry := range entries {
		detail, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		message, ok := detail["message"].(string)
		if !ok {
			continue
		}
		if message = strings.TrimSpace(message); message == "" {
			continue
		}
		messages = append(messages, message)
	}

	return messages
}
