package btp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/sap/crossplane-provider-btp/internal"
	accountsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	entitlementsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

const (
	errInstanceDoesNotExist        = "cannot delete instance does not exist"
	errCouldNotParseCISSecret      = "CIS Secret seems malformed"
	errCouldNotParseUserCredential = "error while parsing sa-provider-secret JSON"
)

type InstanceParameters = map[string]interface{}
type EnvironmentType struct {
	Identifier  string
	ServiceName string
}

func KymaEnvironmentType() EnvironmentType {
	return EnvironmentType{
		Identifier:  "kyma",
		ServiceName: "kymaruntime",
	}
}

func CloudFoundryEnvironmentType() EnvironmentType {
	return EnvironmentType{
		Identifier:  "cloudfoundry",
		ServiceName: "cloudfoundry",
	}
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
}

type CISCredential struct {
	Endpoints struct {
		AccountsServiceUrl          string `json:"accounts_service_url"`
		CloudAutomationUrl          string `json:"cloud_automation_url"`
		EntitlementsServiceUrl      string `json:"entitlements_service_url"`
		EventsServiceUrl            string `json:"events_service_url"`
		ExternalProviderRegistryUrl string `json:"external_provider_registry_url"`
		MetadataServiceUrl          string `json:"metadata_service_url"`
		OrderProcessingUrl          string `json:"order_processing_url"`
		ProvisioningServiceUrl      string `json:"provisioning_service_url"`
		SaasRegistryServiceUrl      string `json:"saas_registry_service_url"`
	} `json:"endpoints"`
	GrantType       string `json:"grant_type"`
	SapCloudService string `json:"sap.cloud.service"`
	Uaa             struct {
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
	} `json:"uaa"`
}

type CloudFoundryOrg struct {
	Id          string `json:"Org Id,"`
	Name        string `json:"Org Name,"`
	ApiEndpoint string `json:"API Endpoint,"`
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

func (c *Client) CreateKymaEnvironment(ctx context.Context, instanceName string, planeName string, parameters InstanceParameters, resourceUID string, serviceAccountEmail string) (string, error) {
	envType := KymaEnvironmentType()
	payload := provisioningclient.CreateEnvironmentInstanceRequestPayload{
		Description:     internal.Ptr("created via crossplane-provider-btp-account"),
		EnvironmentType: envType.Identifier,
		Name:            &instanceName,
		Origin:          nil,
		Parameters:      parameters,
		PlanName:        planeName,
		ServiceName:     envType.ServiceName,
		TechnicalKey:    nil,
		User:            &serviceAccountEmail,
	}
	obj, _, err := c.ProvisioningServiceClient.CreateEnvironmentInstance(ctx).CreateEnvironmentInstanceRequestPayload(payload).Execute()

	if err != nil {
		return "", specifyAPIError(err)
	}

	return *obj.Id, nil
}

func (c *Client) UpdateKymaEnvironment(ctx context.Context, environmentInstanceId string, planeName string, instanceParameters InstanceParameters, resourceUID string) error {
	payload := provisioningclient.UpdateEnvironmentInstanceRequestPayload{
		Parameters: instanceParameters,
		PlanName:   planeName,
	}

	_, _, err := c.ProvisioningServiceClient.UpdateEnvironmentInstance(ctx, environmentInstanceId).UpdateEnvironmentInstanceRequestPayload(payload).Execute()
	if err != nil {
		return specifyAPIError(err)
	}

	return nil
}

func (c *Client) CreateCloudFoundryOrg(
	ctx context.Context, serviceAccountEmail string, resourceUID string,
	landscape string, orgName string, environmentName string,
) (createdOrg string, err error) {
	parameters := map[string]interface{}{
		cfenvironmentParameterInstanceName: orgName, v1alpha1.SubaccountOperatorLabel: resourceUID,
	}
	cloudFoundryPlanName := "standard"
	envType := CloudFoundryEnvironmentType()

	var envName *string = nil
	if environmentName != "" {
		envName = &environmentName
	}
	payload := provisioningclient.CreateEnvironmentInstanceRequestPayload{
		Description:     internal.Ptr("created via crossplane-btp-account-provider"),
		EnvironmentType: envType.Identifier,
		LandscapeLabel:  &landscape,
		Name:            envName,
		Origin:          nil,
		Parameters:      parameters,
		PlanName:        cloudFoundryPlanName,
		ServiceName:     envType.ServiceName,
		TechnicalKey:    nil,
		User:            &serviceAccountEmail,
	}
	localReturnValue, _, err := c.ProvisioningServiceClient.CreateEnvironmentInstance(ctx).CreateEnvironmentInstanceRequestPayload(payload).Execute()
	if err != nil {
		return "", specifyAPIError(err)
	}
	createdOrg = *localReturnValue.Id
	return createdOrg, nil
}

func (c *Client) CreateCloudFoundryOrgIfNotExists(
	ctx context.Context, instanceName string, serviceAccountEmail string, resourceUID string,
	landscape string, orgName string, environmentName string,
) (*CloudFoundryOrg, error) {
	cfEnvironment, err := c.GetCFEnvironmentByNameAndOrg(ctx, instanceName, orgName)
	if err != nil {
		return nil, err
	}
	var orgId string
	if cfEnvironment == nil {
		orgId, err = c.CreateCloudFoundryOrg(ctx, serviceAccountEmail, resourceUID, landscape, orgName, environmentName)
		if err != nil {
			return nil, err
		}
	} else {
		orgId = *cfEnvironment.Id
	}
	cfOrg, err := c.GetCloudFoundryOrg(ctx, orgId)
	if err != nil {
		return nil, err
	}
	return cfOrg, err
}

func (c *Client) GetCloudFoundryOrg(
	ctx context.Context, orgId string,
) (*CloudFoundryOrg, error) {
	cfEnvironment, err := c.GetCFEnvironmentByOrgId(ctx, orgId)
	if err != nil {
		return nil, err
	}
	return c.ExtractOrg(cfEnvironment)
}

func (c *Client) getCloudFoundryEnvironmentId(ctx context.Context, instanceName string, orgName string) (
	string, error,
) {
	environment, err := c.GetCFEnvironmentByNameAndOrg(ctx, instanceName, orgName)

	if err != nil {
		return "", err
	}
	cloudFoundryOrgId := ""
	if environment != nil && environment.Id != nil {
		cloudFoundryOrgId = *environment.Id
	}
	return cloudFoundryOrgId, nil
}

func (c *Client) DeleteEnvironmentById(ctx context.Context, environmentId string) error {
	_, _, err := c.ProvisioningServiceClient.DeleteEnvironmentInstance(ctx, environmentId).Execute()
	if err != nil {
		return specifyAPIError(err)
	}
	return nil
}

func (c *Client) DeleteCloudFoundryEnvironment(ctx context.Context, instanceName string, orgName string) error {
	environmentId, getErr := c.getCloudFoundryEnvironmentId(ctx, instanceName, orgName)
	if getErr != nil {
		return specifyAPIError(getErr)
	}
	delErr := c.DeleteEnvironmentById(ctx, environmentId)
	if delErr != nil {
		return specifyAPIError(delErr)
	}
	return nil
}

// First tries to get the environment by external name, if not found, it tries to get it by name and type
func (c *Client) GetEnvironment(
	ctx context.Context, Id string, instanceName string, environmentType EnvironmentType,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	// Try to get the environment by external name first
	environmentInstance, err := c.GetEnvironmentById(ctx, Id)
	if err != nil {
		return nil, err
	}

	if environmentInstance != nil {
		return environmentInstance, nil
	}

	// If not found by external name, try to get it by name and type
	return c.GetEnvironmentByNameAndType(ctx, instanceName, environmentType)
}

// Legacy function to get environment by name and type
func (c *Client) GetEnvironmentByNameAndType(
	ctx context.Context, instanceName string, environmentType EnvironmentType,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	var environmentInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	response, _, err := c.ProvisioningServiceClient.GetEnvironmentInstances(ctx).Authorization("").Execute()

	if err != nil {
		return nil, specifyAPIError(err)
	}

	for _, instance := range response.EnvironmentInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != environmentType.Identifier {
			continue
		}

		var parameters string
		var parameterList map[string]interface{}
		if instance.Parameters != nil {
			parameters = *instance.Parameters
		}
		err := json.Unmarshal([]byte(parameters), &parameterList)
		if err != nil {
			return nil, err
		}
		if parameterList[cfenvironmentParameterInstanceName] == instanceName {
			environmentInstance = &instance
			break
		}
		if parameterList[KymaenvironmentParameterInstanceName] == instanceName {
			environmentInstance = &instance
			break
		}
	}
	return environmentInstance, err
}

func (c *Client) GetEnvironmentById(
	ctx context.Context, Id string,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {

	var environmentInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	response, _, err := c.ProvisioningServiceClient.GetEnvironmentInstances(ctx).Authorization("").Execute()

	if err != nil {
		return nil, specifyAPIError(err)
	}

	for _, instance := range response.EnvironmentInstances {

		var parameters string
		var parameterList map[string]interface{}
		if instance.Parameters != nil {
			parameters = *instance.Parameters
		}
		err := json.Unmarshal([]byte(parameters), &parameterList)
		if err != nil {
			return nil, err
		}
		if instance.Id != nil && *instance.Id == Id {
			environmentInstance = &instance
			break
		}

	}
	return environmentInstance, err

}

func (c *Client) GetCFEnvironmentByNameAndOrg(
	ctx context.Context, instanceName string, orgName string,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	var environmentInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	envInstances, err := c.getCFEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range envInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != CloudFoundryEnvironmentType().Identifier {
			continue
		}

		var parameters string
		var parameterList map[string]interface{}
		if instance.Parameters != nil {
			parameters = *instance.Parameters
		}
		err := json.Unmarshal([]byte(parameters), &parameterList)
		if err != nil {
			return nil, err
		}
		if parameterList[cfenvironmentParameterInstanceName] == instanceName {
			environmentInstance = &instance
			break
		}
		if parameterList[cfenvironmentParameterInstanceName] == orgName {
			environmentInstance = &instance
			break
		}
	}
	return environmentInstance, err
}

func (c *Client) GetCFEnvironmentByOrgId(ctx context.Context, orgId string) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	var environmentInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	envInstances, err := c.getCFEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range envInstances {
		if instance.EnvironmentType != nil && *instance.EnvironmentType != CloudFoundryEnvironmentType().Identifier {
			continue
		}
		if *instance.Id == orgId {
			environmentInstance = &instance
			break
		}
	}
	return environmentInstance, err
}

func (c *Client) getCFEnvironments(ctx context.Context) ([]provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	// additional Authorization param needs to be set != nil to avoid client blocking the call due to mandatory condition in specs
	response, _, err := c.ProvisioningServiceClient.GetEnvironmentInstances(ctx).Authorization("").Execute()
	if err != nil {
		return nil, specifyAPIError(err)
	}
	return response.EnvironmentInstances, nil
}

func (c *Client) ExtractOrg(cfEnvironment *provisioningclient.BusinessEnvironmentInstanceResponseObject) (*CloudFoundryOrg, error) {
	if cfEnvironment == nil {
		return nil, nil
	}

	var label string
	if cfEnvironment.Labels != nil {
		label = *cfEnvironment.Labels
	}

	return c.NewCloudFoundryOrgByLabel(label)
}

func (c *Client) NewCloudFoundryOrgByLabel(label string) (*CloudFoundryOrg, error) {
	var cloudFoundryOrg *CloudFoundryOrg
	err := json.Unmarshal([]byte(label), &cloudFoundryOrg)
	return cloudFoundryOrg, err
}

func (c *Client) GetBTPSubaccount(
	ctx context.Context, subaccountGUID string,
) (*accountsserviceclient.SubaccountResponseObject, error) {
	btpSubaccount, _, err := c.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccount(ctx, subaccountGUID).Execute()
	return btpSubaccount, err
}

func specifyAPIError(err error) error {
	if genericErr, ok := err.(*provisioningclient.GenericOpenAPIError); ok {
		if provisionErr, ok := genericErr.Model().(provisioningclient.ApiExceptionResponseObject); ok {
			return errors.New(fmt.Sprintf("API Error: %v, Code %v", provisionErr.Error.Message, provisionErr.Error.Code))
		}
		if genericErr.Body() != nil {
			return errors.Wrap(err, string(genericErr.Body()))
		}
	}
	return err
}
