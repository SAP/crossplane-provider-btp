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
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
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
	b, _ := json.Marshal(cfg)
	key := string(b)

	mu.Lock()
	defer mu.Unlock()
	if c, ok := cache[key]; ok {
		return c
	}
	// The client outlives this call; a reconcile ctx would be cancelled and break
	// every later token refresh. WithoutCancel keeps values (oauth2.HTTPClient) but
	// also drops the deadline, so add a client with timeout for token fetches unless
	// a client was already provided in the context
	base := context.WithoutCancel(ctx)
	if _, ok := base.Value(oauth2.HTTPClient).(*http.Client); !ok {
		base = context.WithValue(base, oauth2.HTTPClient, &http.Client{Timeout: 30 * time.Second})
	}
	c := cfg.Client(base)
	cache[key] = c
	return c
}
