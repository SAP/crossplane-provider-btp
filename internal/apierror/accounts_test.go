package apierror

import (
	"errors"
	"testing"

	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

func TestFromAccounts(t *testing.T) {
	plain := errors.New("boom")

	tests := map[string]struct {
		in error
		// want is the expected message. wantSame additionally asserts the
		// original error is handed back untouched, rather than merely
		// formatting to the same text.
		want     string
		wantSame bool
	}{
		"non-generic error passes through": {
			in:       plain,
			want:     "boom",
			wantSame: true,
		},
		"nil passes through": {
			in: nil,
		},

		// The case from the issue: the top-level message says only that the
		// payload was invalid, and the reason lives in details.
		"payload invalid, detail is surfaced": {
			in: testutils.NewAccountAPIErrorFromBody([]byte(`{
				"error": {
					"code": 11000,
					"message": "Request payload is invalid",
					"target": "/accounts/v1/subaccounts",
					"correlationID": "bb8d3f4b-c9f8-4126-4813-8265e34d687a",
					"details": [
						{"code": 11001, "message": "displayName: Display name can't contain / "}
					]
				}
			}`), "400 Bad Request"),
			want: `API Error: Request payload is invalid, Code 11000; details: displayName: Display name can't contain /`,
		},
		"several details are all surfaced": {
			in: testutils.NewAccountAPIErrorFromBody([]byte(`{
				"error": {
					"code": 11000,
					"message": "Request payload is invalid",
					"details": [
						{"code": 11001, "message": "displayName: Display name can't contain / "},
						{"code": 11002, "message": "subdomain: Subdomain is already taken"}
					]
				}
			}`), "400 Bad Request"),
			want: `API Error: Request payload is invalid, Code 11000; details: displayName: Display name can't contain /; subdomain: Subdomain is already taken`,
		},

		// Errors that carry no details must keep producing exactly what they
		// produced before this change.
		"409 conflict from the typed constructor is unchanged": {
			in:   testutils.NewAccountAPIError(409, "directory has child resources", "409 Conflict"),
			want: "API Error: directory has child resources, Code 409",
		},
		"409 conflict from a raw body is unchanged": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":409,"message":"Subaccount with the same subdomain already exists"}}`), "409 Conflict"),
			want: "API Error: Subaccount with the same subdomain already exists, Code 409",
		},
		"500 with an empty message is unchanged": {
			in:   testutils.NewAccountAPIError(500, "", "500 Internal Server Error"),
			want: "API Error: , Code 500",
		},

		// Anything not shaped like a details array falls back to the general
		// message instead of failing or leaking a Go-rendered value.
		"details is not an array": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":11000,"message":"Request payload is invalid","details":"nope"}}`), "400 Bad Request"),
			want: "API Error: Request payload is invalid, Code 11000",
		},
		"details entry is not an object": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":11000,"message":"Request payload is invalid","details":["nope"]}}`), "400 Bad Request"),
			want: "API Error: Request payload is invalid, Code 11000",
		},
		"details entry has no message": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":11000,"message":"Request payload is invalid","details":[{"code":11001}]}}`), "400 Bad Request"),
			want: "API Error: Request payload is invalid, Code 11000",
		},
		"details message is not a string": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":11000,"message":"Request payload is invalid","details":[{"message":123}]}}`), "400 Bad Request"),
			want: "API Error: Request payload is invalid, Code 11000",
		},
		"details message is null": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":11000,"message":"Request payload is invalid","details":[{"message":null}]}}`), "400 Bad Request"),
			want: "API Error: Request payload is invalid, Code 11000",
		},
		"details message is blank": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":11000,"message":"Request payload is invalid","details":[{"message":"   "}]}}`), "400 Bad Request"),
			want: "API Error: Request payload is invalid, Code 11000",
		},
		"details array is empty": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"code":11000,"message":"Request payload is invalid","details":[]}}`), "400 Bad Request"),
			want: "API Error: Request payload is invalid, Code 11000",
		},
		"unusable details are skipped, usable ones kept": {
			in: testutils.NewAccountAPIErrorFromBody([]byte(`{
				"error": {
					"code": 11000,
					"message": "Request payload is invalid",
					"details": ["nope", {"code": 11002}, {"message": "subdomain: Subdomain is already taken"}]
				}
			}`), "400 Bad Request"),
			want: `API Error: Request payload is invalid, Code 11000; details: subdomain: Subdomain is already taken`,
		},
		"code and message are both absent": {
			in:   testutils.NewAccountAPIErrorFromBody([]byte(`{"error":{"target":"/accounts/v1/subaccounts"}}`), "400 Bad Request"),
			want: "API Error: , Code 0",
		},

		// No decodable model, so the raw body is the best available detail.
		"body only falls back to the raw body": {
			in:   testutils.NewAccountAPIErrorBodyOnly([]byte(`{"unexpected":"shape"}`), "502 Bad Gateway"),
			want: `API Error: {"unexpected":"shape"}`,
		},
		"neither model nor body returns the original error": {
			in:       testutils.NewAccountAPIErrorBodyOnly(nil, "503 Service Unavailable"),
			want:     "503 Service Unavailable",
			wantSame: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FromAccounts(tc.in)

			if tc.in == nil {
				if got != nil {
					t.Fatalf("FromAccounts(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("FromAccounts() = nil, want an error")
			}
			if tc.wantSame && got != tc.in {
				t.Errorf("FromAccounts() returned a new error %v, want the original untouched", got)
			}
			if got.Error() != tc.want {
				t.Errorf("FromAccounts() = %q, want %q", got.Error(), tc.want)
			}
		})
	}
}
