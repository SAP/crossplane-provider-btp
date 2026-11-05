package btp

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseCISCredentials(t *testing.T) {
	type args struct {
		encoded string
	}
	type want struct {
		creds *CISCredentials
		err   error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidFullInput": {
			reason: "all fields present and valid — returns populated CISCredentials",
			args:   args{encoded: mustEncodeCredentials(t, validCredentialsJSON())},
			want: want{
				creds: &CISCredentials{
					UAA: struct {
						ClientID     string `json:"clientid"`
						ClientSecret string `json:"clientsecret"`
						URL          string `json:"url"`
						SubaccountID string `json:"subaccountid"`
					}{
						ClientID:     "client-id",
						ClientSecret: "client-secret",
						URL:          "https://uaa.example.com",
						SubaccountID: "sub-1234",
					},
					Endpoints: struct {
						AccountsServiceURL     string `json:"accounts_service_url"`     //nolint:tagliatelle // BTP API uses snake_case
						ProvisioningServiceURL string `json:"provisioning_service_url"` //nolint:tagliatelle // BTP API uses snake_case
					}{
						AccountsServiceURL:     "https://accounts.example.com",
						ProvisioningServiceURL: "https://provisioning.example.com",
					},
				},
			},
		},
		"ProvisioningURLOptional": {
			reason: "provisioning_service_url absent — not required, no error",
			args: args{
				encoded: func() string {
					m := validCredentialsJSON()
					delete(m["endpoints"].(map[string]any), "provisioning_service_url")
					return mustEncodeCredentials(t, m)
				}(),
			},
			want: want{
				creds: &CISCredentials{
					UAA: struct {
						ClientID     string `json:"clientid"`
						ClientSecret string `json:"clientsecret"`
						URL          string `json:"url"`
						SubaccountID string `json:"subaccountid"`
					}{
						ClientID:     "client-id",
						ClientSecret: "client-secret",
						URL:          "https://uaa.example.com",
						SubaccountID: "sub-1234",
					},
					Endpoints: struct {
						AccountsServiceURL     string `json:"accounts_service_url"`     //nolint:tagliatelle // BTP API uses snake_case
						ProvisioningServiceURL string `json:"provisioning_service_url"` //nolint:tagliatelle // BTP API uses snake_case
					}{
						AccountsServiceURL: "https://accounts.example.com",
					},
				},
			},
		},
		"InvalidBase64": {
			reason: "non-base64 input — decode error returned",
			args:   args{encoded: "not-valid-base64!!!"},
			want:   want{err: cmpopts.AnyError},
		},
		"MalformedJSON": {
			reason: "valid base64 but not JSON — parse error returned",
			args:   args{encoded: base64.StdEncoding.EncodeToString([]byte("not json {{{"))},
			want:   want{err: cmpopts.AnyError},
		},
		"MissingClientID": {
			reason: "uaa.clientid empty — ErrMissingClientID returned",
			args: args{
				encoded: func() string {
					m := validCredentialsJSON()
					m["uaa"].(map[string]any)["clientid"] = ""
					return mustEncodeCredentials(t, m)
				}(),
			},
			want: want{err: ErrMissingClientID},
		},
		"MissingClientSecret": {
			reason: "uaa.clientsecret empty — ErrMissingClientSecret returned",
			args: args{
				encoded: func() string {
					m := validCredentialsJSON()
					m["uaa"].(map[string]any)["clientsecret"] = ""
					return mustEncodeCredentials(t, m)
				}(),
			},
			want: want{err: ErrMissingClientSecret},
		},
		"MissingUAAURL": {
			reason: "uaa.url empty — ErrMissingUAAURL returned",
			args: args{
				encoded: func() string {
					m := validCredentialsJSON()
					m["uaa"].(map[string]any)["url"] = ""
					return mustEncodeCredentials(t, m)
				}(),
			},
			want: want{err: ErrMissingUAAURL},
		},
		"MissingAccountsServiceURL": {
			reason: "endpoints.accounts_service_url empty — ErrMissingAccountsServiceURL returned",
			args: args{
				encoded: func() string {
					m := validCredentialsJSON()
					m["endpoints"].(map[string]any)["accounts_service_url"] = ""
					return mustEncodeCredentials(t, m)
				}(),
			},
			want: want{err: ErrMissingAccountsServiceURL},
		},
		"MissingSubaccountID": {
			reason: "uaa.subaccountid empty — ErrMissingSubaccountID returned",
			args: args{
				encoded: func() string {
					m := validCredentialsJSON()
					m["uaa"].(map[string]any)["subaccountid"] = ""
					return mustEncodeCredentials(t, m)
				}(),
			},
			want: want{err: ErrMissingSubaccountID},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseCISCredentials(tc.args.encoded)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nParseCISCredentials(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.creds, got); diff != "" {
				t.Errorf("%s\nParseCISCredentials(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func mustEncodeCredentials(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustEncodeCredentials: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func validCredentialsJSON() map[string]any {
	return map[string]any{
		"uaa": map[string]any{
			"clientid":     "client-id",
			"clientsecret": "client-secret",
			"url":          "https://uaa.example.com",
			"subaccountid": "sub-1234",
		},
		"endpoints": map[string]any{
			"accounts_service_url":     "https://accounts.example.com",
			"provisioning_service_url": "https://provisioning.example.com",
		},
	}
}
