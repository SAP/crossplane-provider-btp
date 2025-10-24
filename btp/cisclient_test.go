package btp

import (
	"net/url"
	"reflect"
	"testing"
)

// TODO: Add unit tests for GetCFEnvironmentByNameAndOrg
// Currently cannot be tested without refactoring due to hardcoded getCFEnvironments() dependency.
// Proposed approaches:
//   1. Interface-based DI: Add EnvironmentFetcher interface to Client
//   2. Function field injection: Add getCFEnvironmentsFunc field to Client
// This would enable testing the ID matching logic added in PR #362 without hitting real BTP API.
// Related to: CloudFoundry environment import bug fix

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
