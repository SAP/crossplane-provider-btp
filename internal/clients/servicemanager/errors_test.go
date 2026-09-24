package servicemanager

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/sap/crossplane-provider-btp/internal"
	smclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
	"github.com/stretchr/testify/assert"
)

// newSMOpenAPIError builds a *smclient.GenericOpenAPIError with model/body set
// via reflection, since the struct fields are unexported in the generated client.
func newSMOpenAPIError(model any, body []byte, msg string) *smclient.GenericOpenAPIError {
	e := &smclient.GenericOpenAPIError{}
	v := reflect.ValueOf(e).Elem()

	if model != nil {
		f := v.FieldByName("model")
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(model))
	}
	if body != nil {
		f := v.FieldByName("body")
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().SetBytes(body)
	}
	if msg != "" {
		f := v.FieldByName("error")
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().SetString(msg)
	}
	return e
}

func TestSpecifyAPIError(t *testing.T) {
	tests := []struct {
		name string
		in   error
		want string
	}{
		{
			name: "nil passes through",
			in:   nil,
			want: "",
		},
		{
			name: "non-OpenAPI error passes through",
			in:   errors.New("plain transport error"),
			want: "plain transport error",
		},
		{
			name: "structured model surfaces error and description",
			in: newSMOpenAPIError(
				smclient.Error{Error: internal.Ptr("BadRequest"), Description: internal.Ptr("plan not found")},
				nil,
				"400 Bad Request",
			),
			want: "API Error: BadRequest, Description: plan not found",
		},
		{
			name: "raw body fallback when no model",
			in:   newSMOpenAPIError(nil, []byte(`{"error":"Unauthorized"}`), "401 Unauthorized"),
			want: `API Error: {"error":"Unauthorized"}`,
		},
		{
			name: "no model, no body returns original",
			in:   newSMOpenAPIError(nil, nil, "503 Service Unavailable"),
			want: "503 Service Unavailable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := specifyAPIError(tc.in)
			if tc.in == nil {
				assert.NoError(t, got)
				return
			}
			assert.EqualError(t, got, tc.want)
		})
	}
}
