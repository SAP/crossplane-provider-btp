package btp

import (
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/sap/crossplane-provider-btp/internal"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func TestFindCFEnvironment(t *testing.T) {
	tests := []struct {
		name         string
		envInstances []provisioningclient.BusinessEnvironmentInstanceResponseObject
		instanceName string
		orgName      string
		wantID       *string
		wantErr      bool
	}{
		{
			name: "Match By ID happy path",
			envInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				{
					Id:              internal.Ptr("env-id-123"),
					EnvironmentType: internal.Ptr("cloudfoundry"),
				},
			},
			instanceName: "env-id-123",
			orgName:      "test-org",
			wantID:       internal.Ptr("env-id-123"),
			wantErr:      false,
		},
		{
			name: "Match by instance name in Parameters",
			envInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				{
					Id:              internal.Ptr("env-456"),
					EnvironmentType: internal.Ptr("cloudfoundry"),
					Parameters:      internal.Ptr(`{"instance_name": "my-cf-env"}`),
				},
			},
			instanceName: "my-cf-env",
			orgName:      "test-org",
			wantID:       internal.Ptr("env-456"),
			wantErr:      false,
		},
		{
			name: "Not found - no match",
			envInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				{
					Id:              internal.Ptr("other-env"),
					EnvironmentType: internal.Ptr("cloudfoundry"),
					Parameters:      internal.Ptr(`{"instance_name": "different-name"}`),
				},
			},
			instanceName: "nonexistent",
			orgName:      "",
			wantID:       nil,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := findCFEnvironment(tt.envInstances, tt.instanceName, tt.orgName)
			if (err != nil) != tt.wantErr {
				t.Errorf("findCFEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Check result
			if tt.wantID == nil {
				if result != nil {
					t.Errorf("findCFEnvironment() expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("findCFEnvironment() expected result with ID %s, got nil", *tt.wantID)
				} else if result.Id == nil || *result.Id != *tt.wantID {
					t.Errorf("findCFEnvironment() got = %v, want %s", result.Id, *tt.wantID)
				}
			}
		})
	}
}

func Test_authenticationParams(t *testing.T) {
	type args struct {
		credential *Credentials
	}

	tests := []struct {
		name string
		args args
		want url.Values
	}{
		{
			name: "Grant Type user_token", args: args{
				&Credentials{
					UserCredential: &UserCredential{Email: "my@mail.com", Password: "mypassword"},
					CISCredential: &CISCredential{
						GrantType: "user_token",
						Uaa: struct {
							Clientid     string `json:"clientid"`
							Clientsecret string `json:"clientsecret"`
							Url          string `json:"url"`
						}{
							Clientid: "myclientid",
						},
					},
				},
			}, want: map[string][]string{
				"username":   {"my@mail.com"},
				"password":   {"mypassword"},
				"grant_type": {"password"},
			},
		},
		{
			name: "Grant Type client_credentials", args: args{
				&Credentials{
					CISCredential: &CISCredential{
						GrantType: "client_credentials",
						Uaa: struct {
							Clientid     string `json:"clientid"`
							Clientsecret string `json:"clientsecret"`
							Url          string `json:"url"`
						}{
							Clientid:     "myclientid",
							Clientsecret: "myclientsecret",
						},
					},
				},
			}, want: map[string][]string{
				"username":   {"myclientid"},
				"password":   {"myclientsecret"},
				"grant_type": {"client_credentials"},
			},
		},
		{
			name: "No client credentials", args: args{
				&Credentials{
					UserCredential: &UserCredential{Username: "myusername", Password: "mypassword"},
					CISCredential: &CISCredential{
						GrantType: "user_token",
						Uaa: struct {
							Clientid     string `json:"clientid"`
							Clientsecret string `json:"clientsecret"`
							Url          string `json:"url"`
						}{
							Clientid: "",
						},
					},
				},
			}, want: map[string][]string{
				"username":   {"myusername"},
				"password":   {"mypassword"},
				"grant_type": {"password"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := authenticationParams(tt.args.credential); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("authenticationParams() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func TestCloudFoundryOrgByLabel(t *testing.T) {
	type args struct {
		labels string
	}
	tests := []struct {
		name    string
		args    args
		want    *CloudFoundryOrg
		wantErr bool
	}{
		{
			name: "Happy Path",
			args: args{
				labels: `{"Org ID":"id", "Org Name":"test-name", "API Endpoint":"api-endpoint"}`,
			},
			want: &CloudFoundryOrg{
				Id:          "id",
				Name:        "test-name",
				ApiEndpoint: "api-endpoint",
			},
			wantErr: false,
		},
		{
			name: "Old Format",
			args: args{
				labels: `{"Org ID:":"id", "Org Name":"test-name", "API Endpoint:":"api-endpoint"}`,
			},
			want: &CloudFoundryOrg{
				Id:          "id",
				Name:        "test-name",
				ApiEndpoint: "api-endpoint",
			},
			wantErr: false,
		},
		{
			name: "Invalid JSON",
			args: args{
				labels: `{"Org ID":"id", "Org Name":"test-name", "API Endpoint":}`,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Empty labels",
			args: args{
				labels: ``,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Empty json",
			args: args{
				labels: `{}`,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := NewCloudFoundryOrgByLabel(tt.args.labels)
				if (err != nil) != tt.wantErr {
					t.Errorf("NewCloudFoundryOrgByLabel() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("NewCloudFoundryOrgByLabel() = %+v, want %+v", got, tt.want)
				}
			},
		)
	}
}

func TestValidateCISCredential(t *testing.T) {
	validCredential := func() *CISCredential {
		return &CISCredential{
			GrantType: "client_credentials",
			Uaa: struct {
				Clientid     string `json:"clientid"`
				Clientsecret string `json:"clientsecret"`
				Url          string `json:"url"`
			}{
				Clientid:     "my-client-id",
				Clientsecret: "my-client-secret",
				Url:          "https://my-tenant.authentication.eu10.hana.ondemand.com",
			},
			Endpoints: struct {
				AccountsServiceUrl     string `json:"accounts_service_url"`
				EntitlementsServiceUrl string `json:"entitlements_service_url"`
				ProvisioningServiceUrl string `json:"provisioning_service_url"`
				SaasRegistryServiceUrl string `json:"saas_registry_service_url"`
			}{
				AccountsServiceUrl:     "https://accounts-service.eu10.hana.ondemand.com",
				EntitlementsServiceUrl: "https://entitlements-service.eu10.hana.ondemand.com",
				ProvisioningServiceUrl: "https://provisioning-service.cfapps.eu10.hana.ondemand.com",
				SaasRegistryServiceUrl: "https://saas-manager.cfapps.eu10.hana.ondemand.com",
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*CISCredential)
		useNil  bool
		wantErr bool
		wantMsg string
		wantMsg2 string
	}{
		{
			name:    "valid credential - all required fields present",
			mutate:  func(c *CISCredential) {},
			wantErr: false,
		},
		{
			name:    "missing uaa.clientid",
			mutate:  func(c *CISCredential) { c.Uaa.Clientid = "" },
			wantErr: true,
			wantMsg: "uaa.clientid",
		},
		{
			name:    "missing uaa.clientsecret",
			mutate:  func(c *CISCredential) { c.Uaa.Clientsecret = "" },
			wantErr: true,
			wantMsg: "uaa.clientsecret",
		},
		{
			name:    "missing uaa.url",
			mutate:  func(c *CISCredential) { c.Uaa.Url = "" },
			wantErr: true,
			wantMsg: "uaa.url",
		},
		{
			name:    "missing endpoints.accounts_service_url",
			mutate:  func(c *CISCredential) { c.Endpoints.AccountsServiceUrl = "" },
			wantErr: true,
			wantMsg: "endpoints.accounts_service_url",
		},
		{
			name:    "missing endpoints.entitlements_service_url",
			mutate:  func(c *CISCredential) { c.Endpoints.EntitlementsServiceUrl = "" },
			wantErr: true,
			wantMsg: "endpoints.entitlements_service_url",
		},
		{
			name:    "missing endpoints.provisioning_service_url",
			mutate:  func(c *CISCredential) { c.Endpoints.ProvisioningServiceUrl = "" },
			wantErr: true,
			wantMsg: "endpoints.provisioning_service_url",
		},
		{
			name:     "multiple missing fields reported together",
			mutate:   func(c *CISCredential) { c.Uaa.Clientid = ""; c.Uaa.Url = "" },
			wantErr:  true,
			wantMsg:  "uaa.clientid",
			wantMsg2: "uaa.url",
		},
		{
			name:    "completely empty credential",
			mutate:  func(c *CISCredential) { *c = CISCredential{} },
			wantErr: true,
			wantMsg: "uaa.clientid",
		},
		{
			name:    "nil credential",
			useNil:  true,
			mutate:  func(c *CISCredential) {},
			wantErr: true,
			wantMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cred *CISCredential
			if tt.useNil {
				cred = nil
			} else {
				cred = validCredential()
				tt.mutate(cred)
			}
			err := validateCISCredential(cred)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCISCredential() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("validateCISCredential() error %q does not contain %q", err.Error(), tt.wantMsg)
			}
			if tt.wantErr && tt.wantMsg2 != "" && !strings.Contains(err.Error(), tt.wantMsg2) {
				t.Errorf("validateCISCredential() error %q does not contain %q", err.Error(), tt.wantMsg2)
			}
		})
	}
}
