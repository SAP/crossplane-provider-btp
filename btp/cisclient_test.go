package btp

import (
	"net/url"
	"reflect"
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
							Apiurl          string `json:"apiurl"`
							Clientid        string `json:"clientid"`
							Clientsecret    string `json:"clientsecret"`
							CredentialType  string `json:"credential-type"`
							Identityzone    string `json:"identityzone"`
							Identityzoneid  string `json:"identityzoneid"`
							Sburl           string `json:"sburl"`
							Subaccountid    string `json:"subaccountid"`
							Tenantid        string `json:"tenantid"`
							Tenantmode      string `json:"tenantmode"`
							Uaadomain       string `json:"uaadomain"`
							Url             string `json:"url"`
							Verificationkey string `json:"verificationkey"`
							Xsappname       string `json:"xsappname"`
							Xsmasterappname string `json:"xsmasterappname"`
							Zoneid          string `json:"zoneid"`
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
							Apiurl          string `json:"apiurl"`
							Clientid        string `json:"clientid"`
							Clientsecret    string `json:"clientsecret"`
							CredentialType  string `json:"credential-type"`
							Identityzone    string `json:"identityzone"`
							Identityzoneid  string `json:"identityzoneid"`
							Sburl           string `json:"sburl"`
							Subaccountid    string `json:"subaccountid"`
							Tenantid        string `json:"tenantid"`
							Tenantmode      string `json:"tenantmode"`
							Uaadomain       string `json:"uaadomain"`
							Url             string `json:"url"`
							Verificationkey string `json:"verificationkey"`
							Xsappname       string `json:"xsappname"`
							Xsmasterappname string `json:"xsmasterappname"`
							Zoneid          string `json:"zoneid"`
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
							Apiurl          string `json:"apiurl"`
							Clientid        string `json:"clientid"`
							Clientsecret    string `json:"clientsecret"`
							CredentialType  string `json:"credential-type"`
							Identityzone    string `json:"identityzone"`
							Identityzoneid  string `json:"identityzoneid"`
							Sburl           string `json:"sburl"`
							Subaccountid    string `json:"subaccountid"`
							Tenantid        string `json:"tenantid"`
							Tenantmode      string `json:"tenantmode"`
							Uaadomain       string `json:"uaadomain"`
							Url             string `json:"url"`
							Verificationkey string `json:"verificationkey"`
							Xsappname       string `json:"xsappname"`
							Xsmasterappname string `json:"xsmasterappname"`
							Zoneid          string `json:"zoneid"`
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
				labels: `{"Org ID:":"id", "Org Name:":"test-name", "API Endpoint:":"api-endpoint"}`,
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
