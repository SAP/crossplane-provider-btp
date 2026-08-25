package destination

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2/clientcredentials"
)

// DestinationCredential holds the OAuth2 binding fields from a
// Destination Service instance binding JSON.
type DestinationCredential struct {
	ClientID     string `json:"clientid"`
	ClientSecret string `json:"clientsecret"`
	TokenURL     string `json:"tokenurl"`
	URI          string `json:"uri"`
}

// ParseCredential unmarshals a Destination Service binding JSON blob.
func ParseCredential(raw []byte) (DestinationCredential, error) {
	var c DestinationCredential
	return c, json.Unmarshal(raw, &c)
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
	httpClient *http.Client
	baseURL    string
}

// NewDestinationClient creates a client authenticating via OAuth2 client credentials.
func NewDestinationClient(cred DestinationCredential) (DestinationClientI, error) {
	cfg := &clientcredentials.Config{
		ClientID:     cred.ClientID,
		ClientSecret: cred.ClientSecret,
		TokenURL:     cred.TokenURL,
	}
	return &destinationClient{
		httpClient: cfg.Client(context.Background()),
		baseURL:    cred.URI,
	}, nil
}

func (c *destinationClient) Get(ctx context.Context, name string) (map[string]string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/destination-configuration/v1/subaccountDestinations/"+name, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", &destError{code: http.StatusNotFound, message: string(body)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", &destError{code: resp.StatusCode, message: string(body)}
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, "", err
	}
	props := make(map[string]string, len(raw))
	for k, v := range raw {
		props[k] = fmt.Sprintf("%v", v)
	}
	return props, resp.Header.Get("ETag"), nil
}

func (c *destinationClient) Create(ctx context.Context, props map[string]any) error {
	body, err := json.Marshal(props)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/destination-configuration/v1/subaccountDestinations",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusConflict:
		return &destError{code: http.StatusConflict, message: string(respBody)}
	case http.StatusCreated, http.StatusOK:
		return nil
	default:
		return &destError{code: resp.StatusCode, message: string(respBody)}
	}
}

func (c *destinationClient) Update(ctx context.Context, props map[string]any, etag string) error {
	body, err := json.Marshal(props)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/destination-configuration/v1/subaccountDestinations",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return &destError{code: resp.StatusCode, message: string(respBody)}
	}
	return nil
}

func (c *destinationClient) Delete(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/destination-configuration/v1/subaccountDestinations/"+name, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil // already deleted — not an error
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return &destError{code: resp.StatusCode, message: string(body)}
	}
	return nil
}
