// Package cccache reuses OAuth2 client_credentials HTTP clients across reconciles.
//
// Several native clients (security xsuaa role-collection maintainer + user/group
// assigners, subscription saas) build a fresh clientcredentials.Config and call
// config.Client(ctx) on every reconcile. That client's token source is only reused
// within that throwaway instance, so each reconcile fetches a new token — a login
// per reconcile on the client identity. Concurrent per-identity logins are the
// lockout trigger analysed in issue #1036.
//
// HTTPClient caches the *http.Client (and its reused token source) per credential
// bundle, so the token is fetched once per identity and reused across reconciles.
package cccache

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/oauth2/clientcredentials"
)

var (
	mu    sync.Mutex
	cache = map[string]*http.Client{}
)

// HTTPClient returns a cached *http.Client for the given client_credentials config,
// keyed by the credential bundle. The underlying oauth2 token source is reused
// across calls, so the client identity logs in once per token lifetime instead of
// once per reconcile. config.Client builds lazily (no login until first request),
// so caching it is cheap and safe.
func HTTPClient(ctx context.Context, cfg *clientcredentials.Config) *http.Client {
	key := strings.Join([]string{cfg.ClientID, cfg.ClientSecret, cfg.TokenURL, strings.Join(cfg.Scopes, ",")}, "\x00")

	mu.Lock()
	defer mu.Unlock()
	if c, ok := cache[key]; ok {
		return c
	}
	c := cfg.Client(ctx)
	cache[key] = c
	return c
}
