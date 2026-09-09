package destination

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2/clientcredentials"

	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

// CertificateClientI is the interface used by the controller.
type CertificateClientI interface {
	// Get returns the certificate by name.
	// Returns a not-found error (IsNotFound == true) when the API responds 404.
	Get(ctx context.Context, name string) (*destclient.Certificate, error)
	// Create uploads a new certificate. Returns a conflict error (IsConflict == true) on 409.
	Create(ctx context.Context, cert destclient.Certificate) error
	// Update replaces an existing certificate.
	Update(ctx context.Context, cert destclient.Certificate) error
	// Delete removes a certificate. 404 responses are silently ignored.
	Delete(ctx context.Context, name string) error
}

type certificateClient struct {
	api destclient.CertificatesOnSubaccountLevelAPI
}

// NewCertificateClient creates a client authenticating via OAuth2 client credentials.
func NewCertificateClient(cred DestinationCredential) (CertificateClientI, error) {
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
	return &certificateClient{
		api: apiClient.CertificatesOnSubaccountLevelAPI,
	}, nil
}

func (c *certificateClient) Get(ctx context.Context, name string) (*destclient.Certificate, error) {
	cert, resp, err := c.api.V1SubaccountCertificatesCertificateNameGet(ctx, name).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, &destError{code: http.StatusNotFound, message: err.Error()}
		}
		return nil, err
	}
	return cert, nil
}

func enrichErr(err error) error {
	var apiErr destclient.GenericOpenAPIError
	if ok := asGenericOpenAPIError(err, &apiErr); ok && len(apiErr.Body()) > 0 {
		return fmt.Errorf("%w; response body: %s", err, string(apiErr.Body()))
	}
	return err
}

// asGenericOpenAPIError checks whether err is a GenericOpenAPIError via errors.As-style unwrap.
func asGenericOpenAPIError(err error, target *destclient.GenericOpenAPIError) bool {
	if e, ok := err.(destclient.GenericOpenAPIError); ok {
		*target = e
		return true
	}
	return false
}

func (c *certificateClient) Create(ctx context.Context, cert destclient.Certificate) error {
	req := destclient.CertificateAsV1SubaccountCertificatesPutRequest(&cert)
	_, resp, err := c.api.V1SubaccountCertificatesPost(ctx).Certificate(req).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusConflict {
			return &destError{code: http.StatusConflict, message: err.Error()}
		}
		return enrichErr(err)
	}
	return nil
}

func (c *certificateClient) Update(ctx context.Context, cert destclient.Certificate) error {
	req := destclient.CertificateAsV1SubaccountCertificatesPutRequest(&cert)
	_, resp, err := c.api.V1SubaccountCertificatesPut(ctx).Certificate(req).Execute()
	if err != nil {
		if resp != nil {
			return &destError{code: resp.StatusCode, message: err.Error()}
		}
		return err
	}
	return nil
}

func (c *certificateClient) Delete(ctx context.Context, name string) error {
	_, resp, err := c.api.V1SubaccountCertificatesCertificateNameDelete(ctx, name).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}
