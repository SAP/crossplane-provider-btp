package destination

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2/clientcredentials"

	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

// DestinationCredential holds the OAuth2 binding fields from a
// Destination Service instance binding JSON.
type DestinationCredential struct {
	ClientID     string `json:"clientid"`
	ClientSecret string `json:"clientsecret"`
	TokenURL     string `json:"tokenurl"`
	URI          string `json:"uri"`
	// URL is an alias for URI used by some BTP landscapes.
	URL string `json:"url,omitempty"`
	// UAA holds nested credentials written by some BTP landscapes.
	UAA *struct {
		ClientID     string `json:"clientid"`
		ClientSecret string `json:"clientsecret"`
		URL          string `json:"url"`
	} `json:"uaa,omitempty"`
}

// ParseCredential unmarshals a Destination Service binding JSON blob.
func ParseCredential(raw []byte) (DestinationCredential, error) {
	var c DestinationCredential
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	// Normalize URI: if empty, fall back to url.
	// If adding a new alias here, update assembleDestinationCredJSON in internal/controller/providerconfig/config.go too.
	if c.URI == "" && c.URL != "" {
		c.URI = c.URL
	}
	// Normalize from nested uaa structure when flat fields are absent.
	if c.UAA != nil {
		if c.ClientID == "" {
			c.ClientID = c.UAA.ClientID
		}
		if c.ClientSecret == "" {
			c.ClientSecret = c.UAA.ClientSecret
		}
		if c.TokenURL == "" {
			c.TokenURL = c.UAA.URL
		}
	}
	// If TokenURL is still absent, derive it from the UAA base URL (url field).
	// Some BTP landscapes write a bare UAA base URL under "url" rather than a
	// full token endpoint under "tokenurl".
	if c.TokenURL == "" && c.URL != "" {
		c.TokenURL = strings.TrimRight(c.URL, "/") + "/oauth/token"
	}
	return c, nil
}

// DestinationClientI is the interface the controller uses — isolates HTTP concerns.
type DestinationClientI interface {
	// Get returns the full property bag, ETag header value, and error.
	// Returns a not-found error (IsNotFound == true) when the API responds 404.
	Get(ctx context.Context, name string) (map[string]string, string, error)
	// Create POSTs a new destination. Returns a conflict error (IsConflict == true) on 409.
	Create(ctx context.Context, props map[string]any) error
	// Update PUTs a destination, supplying the etag for optimistic concurrency.
	Update(ctx context.Context, props map[string]any, etag string) error
	// Delete removes a destination. 404 responses are silently ignored.
	Delete(ctx context.Context, name string) error
}

type destError struct {
	code    int
	message string
}

func (e *destError) Error() string {
	return fmt.Sprintf("destination API error %d: %s", e.code, e.message)
}

// IsNotFound reports whether err is a 404 from the Destination Service API.
func IsNotFound(err error) bool {
	e, ok := err.(*destError)
	return ok && e.code == http.StatusNotFound
}

// IsConflict reports whether err is a 409 from the Destination Service API.
func IsConflict(err error) bool {
	e, ok := err.(*destError)
	return ok && e.code == http.StatusConflict
}

// NewNotFoundError returns a 404 error, for use in mocks and tests.
func NewNotFoundError() error {
	return &destError{code: http.StatusNotFound, message: "not found"}
}

// NewConflictError returns a 409 error, for use in mocks and tests.
func NewConflictError() error {
	return &destError{code: http.StatusConflict, message: "conflict"}
}

type destinationClient struct {
	api destclient.DestinationsOnSubaccountLevelAPI
}

// NewDestinationClient creates a client authenticating via OAuth2 client credentials.
func NewDestinationClient(cred DestinationCredential) (DestinationClientI, error) {
	oauthHTTP := (&clientcredentials.Config{
		ClientID:     cred.ClientID,
		ClientSecret: cred.ClientSecret,
		TokenURL:     cred.TokenURL,
	}).Client(context.Background())

	cfg := destclient.NewConfiguration()
	cfg.HTTPClient = oauthHTTP
	cfg.Servers = destclient.ServerConfigurations{
		{URL: strings.TrimRight(cred.URI, "/") + "/destination-configuration"},
	}

	apiClient := destclient.NewAPIClient(cfg)
	return &destinationClient{
		api: apiClient.DestinationsOnSubaccountLevelAPI,
	}, nil
}

func (c *destinationClient) Get(ctx context.Context, name string) (map[string]string, string, error) {
	dest, resp, err := c.api.V1SubaccountDestinationsDestinationNameGet(ctx, name).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, "", &destError{code: http.StatusNotFound, message: err.Error()}
		}
		return nil, "", err
	}

	raw, err := dest.ToMap()
	if err != nil {
		return nil, "", err
	}
	props := make(map[string]string, len(raw))
	for k, v := range raw {
		props[k] = fmt.Sprintf("%v", v)
	}
	return props, resp.Header.Get("ETag"), nil
}

func (c *destinationClient) Create(ctx context.Context, props map[string]any) error {
	dest := propsToDestination(props)
	req := destclient.DestinationAsV1SubaccountDestinationsPutRequest(&dest)
	_, resp, err := c.api.V1SubaccountDestinationsPost(ctx).Destination(req).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusConflict {
			return &destError{code: http.StatusConflict, message: err.Error()}
		}
		return err
	}
	return nil
}

func (c *destinationClient) Update(ctx context.Context, props map[string]any, etag string) error {
	dest := propsToDestination(props)
	req := destclient.DestinationAsV1SubaccountDestinationsPutRequest(&dest)
	call := c.api.V1SubaccountDestinationsPut(ctx).Destination(req)
	if etag != "" {
		call = call.IfMatch(etag)
	}
	_, resp, err := call.Execute()
	if err != nil {
		if resp != nil {
			return &destError{code: resp.StatusCode, message: err.Error()}
		}
		return err
	}
	return nil
}

func (c *destinationClient) Delete(ctx context.Context, name string) error {
	_, resp, err := c.api.V1SubaccountDestinationsDestinationNameDelete(ctx, name).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil // already deleted — not an error
		}
		return err
	}
	return nil
}

// propsToDestination converts a flat property map into the generated Destination type.
// Name and Type are required fields; all other keys go into AdditionalProperties.
func propsToDestination(props map[string]any) destclient.Destination {
	name, _ := props["Name"].(string)
	typ, _ := props["Type"].(string)
	dest := destclient.NewDestination(name, typ)
	dest.AdditionalProperties = make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "Name" || k == "Type" {
			continue
		}
		dest.AdditionalProperties[k] = v
	}
	return *dest
}
