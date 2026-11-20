package btp

import (
	"context"
	"net/http"

	"golang.org/x/oauth2/clientcredentials"
)

// NewOAuthClient returns an *http.Client that automatically fetches and refreshes
// OAuth2 client_credentials tokens from tokenURL.
func NewOAuthClient(ctx context.Context, tokenURL, clientID, clientSecret string) *http.Client {
	cfg := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
	}
	return cfg.Client(ctx)
}
