package btp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"golang.org/x/oauth2/clientcredentials"

	accountsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	entitlementsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

const (
	errCouldNotParseCISSecret      = "CIS Secret seems malformed"
	errCouldNotParseUserCredential = "error while parsing sa-provider-secret JSON"
	errCISBindingCredentialIsNil        = "CIS binding credential is nil"
	errCISBindingMissingRequiredFields  = "CIS binding is missing required fields: %s"
)

type InstanceParameters = map[string]interface{}
type EnvironmentType struct {
	Identifier  string
	ServiceName string
}

type Client struct {
	AccountsServiceClient     *accountsserviceclient.APIClient
	EntitlementsServiceClient *entitlementsserviceclient.ManageAssignedEntitlementsAPIService
	ProvisioningServiceClient provisioningclient.EnvironmentsAPI
	AuthInfo                  runtime.ClientAuthInfoWriter
	Credential                *Credentials
}
type Credentials struct {
	UserCredential *UserCredential
	CISCredential  *CISCredential
}

type UserCredential struct {
	Email    string
	Username string
	Password string
	Idp      string
}

type CISCredential struct {
	Endpoints struct {
		AccountsServiceUrl     string `json:"accounts_service_url"`
		EntitlementsServiceUrl string `json:"entitlements_service_url"`
		ProvisioningServiceUrl string `json:"provisioning_service_url"`
		SaasRegistryServiceUrl string `json:"saas_registry_service_url"`
	} `json:"endpoints"`
	GrantType string `json:"grant_type"`
	Uaa       struct {
		Clientid     string `json:"clientid"`
		Clientsecret string `json:"clientsecret"`
		Url          string `json:"url"`
	} `json:"uaa"`
}

func validateCISCredential(c *CISCredential) error {
	if c == nil {
		return errors.New(errCISBindingCredentialIsNil)

	}
	var missing []string
	if c.Uaa.Clientid == "" {
		missing = append(missing, "uaa.clientid")
	}
	if c.Uaa.Clientsecret == "" {
		missing = append(missing, "uaa.clientsecret")
	}
	if c.Uaa.Url == "" {
		missing = append(missing, "uaa.url")
	}
	if c.Endpoints.AccountsServiceUrl == "" {
		missing = append(missing, "endpoints.accounts_service_url")
	}
	if c.Endpoints.EntitlementsServiceUrl == "" {
		missing = append(missing, "endpoints.entitlements_service_url")
	}
	if c.Endpoints.ProvisioningServiceUrl == "" {
		missing = append(missing, "endpoints.provisioning_service_url")
	}
	if len(missing) > 0 {
		return fmt.Errorf(errCISBindingMissingRequiredFields, strings.Join(missing, ", "))
	}
	return nil
}

const (
	cfenvironmentParameterInstanceName   = "instance_name"
	CfOrgNameParameterName               = "Org Name"
	KymaenvironmentParameterInstanceName = "name"
	grantTypeClientCredentials           = "client_credentials"
	grantTypePassword                    = "password"
	tokenURL                             = "/oauth/token"
)

func NewServiceClientWithCisCredential(credential *Credentials) Client {

	authentication := authenticationParams(credential)

	config := createConfig(credential, tokenURL, authentication)

	client := createClient(credential, config)

	return client
}

func authenticationParams(credential *Credentials) url.Values {
	params := url.Values{}
	if hasClientCredentials(credential) {
		if isGrantTypeClientCredentials(credential) {
			params.Add("username", credential.CISCredential.Uaa.Clientid)
			params.Add("password", credential.CISCredential.Uaa.Clientsecret)
			params.Add("grant_type", grantTypeClientCredentials)
		} else {
			params.Add("username", credential.UserCredential.Email)
			params.Add("password", credential.UserCredential.Password)
			params.Add("grant_type", grantTypePassword)
		}
	} else {
		params.Add("username", credential.UserCredential.Username)
		params.Add("password", credential.UserCredential.Password)
		params.Add("grant_type", grantTypePassword)
	}

	return params
}

func isGrantTypeClientCredentials(credential *Credentials) bool {
	return credential.CISCredential.GrantType == grantTypeClientCredentials
}

func hasClientCredentials(credential *Credentials) bool {
	return credential.CISCredential.Uaa.Clientid != ""
}

func createClient(credential *Credentials, config *clientcredentials.Config) Client {
	client := Client{
		AccountsServiceClient:     createAccountsServiceClient(credential, config),
		EntitlementsServiceClient: createEntitlementsServiceClient(credential, config),
		ProvisioningServiceClient: createProvisioningServiceClient(credential, config),
		AuthInfo:                  GetBasicAuth(credential),
		Credential:                credential,
	}
	return client
}

func createProvisioningServiceClient(
	credential *Credentials, config *clientcredentials.Config,
) provisioningclient.EnvironmentsAPI {
	provisioningServiceUrl, err := url.Parse(credential.CISCredential.Endpoints.ProvisioningServiceUrl)
	if err != nil {
		return nil
	}

	c := provisioningclient.NewConfiguration()

	c.HTTPClient = config.Client(NewBackgroundContextWithDebugPrintHTTPClient())
	c.Servers = []provisioningclient.ServerConfiguration{{URL: provisioningServiceUrl.String()}}

	client := provisioningclient.NewAPIClient(c)

	return client.EnvironmentsAPI
}

func createConfig(credential *Credentials, tokenURL string, endPointParams url.Values) *clientcredentials.Config {
	uaa := credential.CISCredential.Uaa
	config := &clientcredentials.Config{
		ClientID:       uaa.Clientid,
		ClientSecret:   uaa.Clientsecret,
		TokenURL:       uaa.Url + tokenURL,
		EndpointParams: endPointParams,
	}
	return config
}

func createEntitlementsServiceClient(
	cisCredential *Credentials, config *clientcredentials.Config,
) *entitlementsserviceclient.ManageAssignedEntitlementsAPIService {
	entitlementsServiceUrl, err := url.Parse(cisCredential.CISCredential.Endpoints.EntitlementsServiceUrl)
	if err != nil {
		return nil
	}

	c := entitlementsserviceclient.NewConfiguration()

	c.HTTPClient = config.Client(NewBackgroundContextWithDebugPrintHTTPClient())
	c.Servers = []entitlementsserviceclient.ServerConfiguration{{URL: entitlementsServiceUrl.String()}}

	client := entitlementsserviceclient.NewAPIClient(c)

	return client.ManageAssignedEntitlementsAPI
}

func createAccountsServiceClient(
	cisCredential *Credentials, config *clientcredentials.Config,
) *accountsserviceclient.APIClient {
	accountServiceUrl, err := url.Parse(cisCredential.CISCredential.Endpoints.AccountsServiceUrl)
	if err != nil {
		return nil
	}

	c := accountsserviceclient.NewConfiguration()

	c.HTTPClient = config.Client(NewBackgroundContextWithDebugPrintHTTPClient())
	c.Servers = []accountsserviceclient.ServerConfiguration{{URL: accountServiceUrl.String()}}

	client := accountsserviceclient.NewAPIClient(c)

	return client

}

func GetBasicAuth(cisCredentials *Credentials) runtime.ClientAuthInfoWriter {
	return httptransport.BasicAuth(
		cisCredentials.CISCredential.Uaa.Clientid, cisCredentials.CISCredential.Uaa.Clientsecret,
	)
}

func ServiceClientFromSecret(cisSecret []byte, userSecret []byte) (Client, error) {
	var cisCredential CISCredential
	if err := json.Unmarshal(cisSecret, &cisCredential); err != nil {
		return Client{}, errors.Wrap(err, errCouldNotParseCISSecret)
	}

	if err := validateCISCredential(&cisCredential); err != nil {
		return Client{}, err
	}

	var userCredential UserCredential

	if err := json.Unmarshal(userSecret, &userCredential); err != nil {
		return Client{}, errors.Wrap(err, errCouldNotParseUserCredential)

	}

	credential := &Credentials{
		UserCredential: &userCredential,
		CISCredential:  &cisCredential,
	}

	return NewServiceClientWithCisCredential(credential), nil
}
