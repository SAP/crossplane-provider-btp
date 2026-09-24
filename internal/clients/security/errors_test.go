package security

import (
	"errors"
	"testing"

	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

func TestSpecifyAPIError(t *testing.T) {
	plain := errors.New("boom")
	tests := map[string]struct {
		in      error
		wantMsg string
	}{
		"non-generic passes through": {in: plain, wantMsg: "boom"},
		"nil passes through":         {in: nil, wantMsg: ""},
		"with body":                  {in: testutils.NewXsuaaAPIError([]byte(`{"error":"bad"}`)), wantMsg: `API Error: {"error":"bad"}`},
		"empty generic returns self": {in: testutils.NewXsuaaAPIError(nil), wantMsg: ""},
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
