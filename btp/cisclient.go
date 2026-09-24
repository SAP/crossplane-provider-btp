package btp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"golang.org/x/oauth2"
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
	// Cache the fully-built Client per credential hash so the oauth2 token
	// cache survives across reconciles. Without this, the providerconfig
	// connector would rebuild all 3 sub-clients + 3 token sources on every
	// reconcile, producing ~1.7 token POSTs per Observe.
	key := credentialCacheKey(credential)
	if cached, ok := clientCache.Load(key); ok {
		return cached.(Client)
	}

	authentication := authenticationParams(credential)
	config := createConfig(credential, tokenURL, authentication)
	client := createClient(credential, config)

	actual, _ := clientCache.LoadOrStore(key, client)
	return actual.(Client)
}

// clientCache: process-wide btp.Client cache keyed by credential hash.
// Credential rotation produces a new key automatically; old entries leak
// until process restart. Add a TTL/LRU if rotation churn becomes an issue.
var clientCache sync.Map

// credentialCacheKey builds a stable string key from the credential bundle.
// We need ALL credential fields to differentiate cache entries (including the
// secret, so that a credential rotation produces a new entry), but we do not
// need cryptographic hashing — this is an in-process map key, not a password
// verifier. NUL separator can't appear in any of the input strings.
func credentialCacheKey(c *Credentials) string {
	var parts []string
	if c.CISCredential != nil {
		parts = append(parts,
			c.CISCredential.Uaa.Clientid,
			c.CISCredential.Uaa.Clientsecret,
			c.CISCredential.Uaa.Url,
			c.CISCredential.Endpoints.AccountsServiceUrl,
			c.CISCredential.Endpoints.EntitlementsServiceUrl,
			c.CISCredential.Endpoints.ProvisioningServiceUrl,
			c.CISCredential.GrantType,
		)
	}
	if c.UserCredential != nil {
		parts = append(parts,
			c.UserCredential.Email,
			c.UserCredential.Username,
			c.UserCredential.Password,
			c.UserCredential.Idp,
		)
	}
	return strings.Join(parts, "\x00")
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
	// One shared oauth2 *http.Client across all 3 sub-clients so the token
	// cache is shared. Without this, each createXxxServiceClient builds its
	// own *http.Client → 3 independent token caches → extra token POSTs per
	// Observe.
	sharedHTTPClient := sharedOAuthClient(config)
	client := Client{
		AccountsServiceClient:     createAccountsServiceClient(credential, sharedHTTPClient),
		EntitlementsServiceClient: createEntitlementsServiceClient(credential, sharedHTTPClient),
		ProvisioningServiceClient: createProvisioningServiceClient(credential, sharedHTTPClient),
		AuthInfo:                  GetBasicAuth(credential),
		Credential:                credential,
	}
	return client
}

// sharedOAuthClient builds a single *http.Client backed by the debug-aware
// HTTP transport and the oauth2 transport with a reused token source.
// Sharing this client across the 3 sub-clients collapses 3 token caches into 1.
func sharedOAuthClient(config *clientcredentials.Config) *http.Client {
	baseHTTPClient := DebugPrintHTTPClient()
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, baseHTTPClient)
	// config.TokenSource already wraps in oauth2.ReuseTokenSource internally.
	return &http.Client{
		Transport: &oauth2.Transport{
			Source: config.TokenSource(ctx),
			Base:   baseHTTPClient.Transport,
		},
	}
}

func createProvisioningServiceClient(
	credential *Credentials, sharedHTTPClient *http.Client,
) provisioningclient.EnvironmentsAPI {
	provisioningServiceUrl, err := url.Parse(credential.CISCredential.Endpoints.ProvisioningServiceUrl)
	if err != nil {
		return nil
	}

	c := provisioningclient.NewConfiguration()

	c.HTTPClient = sharedHTTPClient
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
	cisCredential *Credentials, sharedHTTPClient *http.Client,
) *entitlementsserviceclient.ManageAssignedEntitlementsAPIService {
	entitlementsServiceUrl, err := url.Parse(cisCredential.CISCredential.Endpoints.EntitlementsServiceUrl)
	if err != nil {
		return nil
	}

	c := entitlementsserviceclient.NewConfiguration()

	c.HTTPClient = sharedHTTPClient
	c.Servers = []entitlementsserviceclient.ServerConfiguration{{URL: entitlementsServiceUrl.String()}}

	client := entitlementsserviceclient.NewAPIClient(c)

	return client.ManageAssignedEntitlementsAPI
}

func createAccountsServiceClient(
	cisCredential *Credentials, sharedHTTPClient *http.Client,
) *accountsserviceclient.APIClient {
	accountServiceUrl, err := url.Parse(cisCredential.CISCredential.Endpoints.AccountsServiceUrl)
	if err != nil {
		return nil
	}

	c := accountsserviceclient.NewConfiguration()

	c.HTTPClient = sharedHTTPClient
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
