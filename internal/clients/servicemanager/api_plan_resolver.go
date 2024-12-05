package servicemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicemanager "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
	"golang.org/x/oauth2/clientcredentials"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const ErrUnmarshalToBindingCredentials = "JSON can not be unmarshalled to BindingCredentials"
const ErrJsonMarshal = "Error marshalling secret data"
const ErrMissingClientId = "Client ID (clientid) is missing"
const ErrMissingClientSecret = "Client Secret (clientsecret) is missing"
const ErrMissingSmUrl = "Service Manager URL (sm_url) is missing"
const ErrMissingUrl = "Token URL (tokenurl) is missing"
const ErrMissingXsappname = "Xsappname (xsappname) is missing"

// PlanIdResolver used as its own resolval implementation downstream
type PlanIdResolver interface {
	PlanIDByName(ctx context.Context, offeringName string, servicePlanName string) (string, error)
}

// NewCredsFromOperatorSecret creates a new BindingCredentials from a secret data
// of a btp service operator secret, which is slightly different in structure then
// the creds of a regular servicebinding
func NewCredsFromOperatorSecret(secretData map[string][]byte) (BindingCredentials, error) {
	// to make sure we are not using encoded data we need to convert to a plain map first
	var plain = make(map[string]string)
	for k, v := range secretData {
		plain[k] = string(v)
	}

	bytes, err := json.Marshal(plain)
	if err != nil {
		return BindingCredentials{}, errors.New(ErrJsonMarshal)
	}

	var binding BindingCredentials
	err = json.Unmarshal(bytes, &binding)
	if err != nil {
		return BindingCredentials{}, errors.New(ErrUnmarshalToBindingCredentials)
	}

	if plain[apisv1alpha1.ResourceCredentialsXsuaaUrl] == "" {
		return BindingCredentials{}, errors.New(ErrMissingUrl)
	}

	binding.Url = internal.Ptr(plain[apisv1alpha1.ResourceCredentialsXsuaaUrl])

	if binding.Clientid == nil {
		return BindingCredentials{}, errors.New(ErrMissingClientId)
	}
	if binding.Clientsecret == nil {
		return BindingCredentials{}, errors.New(ErrMissingClientSecret)
	}
	if binding.SmUrl == nil {
		return BindingCredentials{}, errors.New(ErrMissingSmUrl)
	}
	if binding.Xsappname == nil {
		return BindingCredentials{}, errors.New(ErrMissingXsappname)
	}

	return binding, nil
}

type BindingCredentials struct {
	Clientid     *string `json:"clientid,omitempty"`
	Clientsecret *string `json:"clientsecret,omitempty"`
	SmUrl        *string `json:"sm_url,omitempty"`
	Url          *string `json:"url,omitempty"`
	Xsappname    *string `json:"xsappname,omitempty"`
}

// ServiceManagerClient is a client for looking up serviceplanID over the service manager API, it requires
// a service manager instance binding
type ServiceManagerClient struct {
	servicemanager.ServiceOfferingsAPI
	servicemanager.ServicePlansAPI
}

func NewServiceManagerClient(ctx context.Context, creds *BindingCredentials) (*ServiceManagerClient, error) {
	const oauthTokenUrlPath = "/oauth/token"

	log := log.FromContext(ctx)

	endPointParams := url.Values{}
	endPointParams.Add("grant_type", "client_credentials")

	config := clientcredentials.Config{
		ClientID:       internal.Val(creds.Clientid),
		ClientSecret:   internal.Val(creds.Clientsecret),
		TokenURL:       internal.Val(creds.Url) + oauthTokenUrlPath,
		EndpointParams: endPointParams,
	}

	smURL, err := url.Parse(internal.Val(creds.SmUrl))
	if err != nil {
		newErr := errors.Wrapf(err, "Cannot parse serviceManagerUrl: %s", internal.Val(creds.SmUrl))
		log.Error(newErr, "")
		return nil, newErr
	}
	apiClientConfig := servicemanager.NewConfiguration()
	apiClientConfig.Host = smURL.Host
	apiClientConfig.Scheme = smURL.Scheme
	apiClientConfig.HTTPClient = config.Client(ctx)

	apiClient := servicemanager.NewAPIClient(apiClientConfig)

	return &ServiceManagerClient{
		apiClient.ServiceOfferingsAPI,
		apiClient.ServicePlansAPI,
	}, nil
}

func (sm *ServiceManagerClient) PlanIDByName(ctx context.Context, offeringName, planName string) (string, error) {
	offeringQuery := fmt.Sprintf("catalog_name eq '%s'", offeringName)
	execute, _, err := sm.GetServiceOfferings(ctx).FieldQuery(offeringQuery).Execute()
	if err != nil {
		return "", err
	}
	if len(execute.Items) == 0 {
		return "", errors.Errorf("API returned no service plan for offering %s", offeringName)
	}

	planQuery := fmt.Sprintf("catalog_name eq '%s' and service_offering_id eq '%s'", planName, *execute.Items[0].Id)
	object, _, err := sm.GetAllServicePlans(ctx).FieldQuery(planQuery).Execute()
	if err != nil {
		return "", err
	}

	if len(object.Items) == 0 {
		return "", errors.Errorf("No service plan '%s' found for offering '%s'", planName, offeringName)
	}

	servicePlanID := *object.Items[0].Id
	return servicePlanID, nil
}
