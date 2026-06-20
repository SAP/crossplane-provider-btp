package security

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

// newGenericErr builds a *xsuaa.GenericOpenAPIError with the given body
// using reflect/unsafe because the field is unexported.
func newGenericErr(body []byte) *xsuaa.GenericOpenAPIError {
	e := &xsuaa.GenericOpenAPIError{}
	if body != nil {
		f := reflect.ValueOf(e).Elem().FieldByName("body")
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(body))
	}
	return e
}

func TestSpecifyAPIError(t *testing.T) {
	plain := errors.New("boom")
	tests := map[string]struct {
		in      error
		wantMsg string
	}{
		"non-generic passes through": {in: plain, wantMsg: "boom"},
		"nil passes through":         {in: nil, wantMsg: ""},
		"with body":                  {in: newGenericErr([]byte(`{"error":"bad"}`)), wantMsg: `API Error: {"error":"bad"}`},
		"empty generic returns self": {in: newGenericErr(nil), wantMsg: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := SpecifyAPIError(tc.in)
			if tc.in == nil {
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want non-nil error")
			}
			if got.Error() != tc.wantMsg {
				t.Errorf("want %q, got %q", tc.wantMsg, got.Error())
			}
		})
	}
}
