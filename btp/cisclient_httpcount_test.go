package btp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sap/crossplane-provider-btp/internal/httpcount"
)

// TestClientCache_SharedTokenSource_OnePOSTAcrossManyGETs proves the
// second half of PR #744 §1: a single oauth2.ReuseTokenSource is shared
// across the 3 sub-clients built by createClient. Firing N GETs against
// the accounts / entitlements / provisioning endpoints must produce
// exactly ONE POST to /oauth/token — not one per call, not one per
// sub-client.
//
// We wire an httptest server that answers both /oauth/token and the API
// endpoints, inject an httpcount.RoundTripper as the oauth2 base
// transport, and count.
func TestClientCache_SharedTokenSource_OnePOSTAcrossManyGETs(t *testing.T) {
	// Shares package-level newBaseHTTPClientFn + clientCache — non-parallel.
	r := require.New(t)

	clientCache.Range(func(k, _ any) bool { clientCache.Delete(k); return true })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token-abc",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			// Every other path: return an empty 200 so the openapi clients
			// don't hit a parse error path unrelated to what we're testing.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	rt := httpcount.New(nil)
	orig := newBaseHTTPClientFn
	newBaseHTTPClientFn = func() *http.Client { return rt.Client() }
	t.Cleanup(func() { newBaseHTTPClientFn = orig })

	cred := &Credentials{
		CISCredential: &CISCredential{GrantType: grantTypeClientCredentials},
	}
	cred.CISCredential.Uaa.Clientid = "cid-tok"
	cred.CISCredential.Uaa.Clientsecret = "cs"
	cred.CISCredential.Uaa.Url = srv.URL
	cred.CISCredential.Endpoints.AccountsServiceUrl = srv.URL
	cred.CISCredential.Endpoints.EntitlementsServiceUrl = srv.URL
	cred.CISCredential.Endpoints.ProvisioningServiceUrl = srv.URL

	client := NewServiceClientWithCisCredential(cred)

	// Fire many GETs across all 3 sub-clients. Any of them requires a
	// bearer token — under a shared oauth2 source, only the first triggers
	// the /oauth/token POST; the rest reuse the cached token.
	const rounds = 10
	for range rounds {
		_, _, _ = client.EntitlementsServiceClient.GetDirectoryAssignments(t.Context()).Execute()
	}

	tokenPOSTs := rt.CountFor("POST", "/oauth/token")
	r.Equal(uint64(1), tokenPOSTs,
		"shared oauth2.ReuseTokenSource must produce exactly 1 token POST across %d GETs; got %d",
		rounds, tokenPOSTs)

	// Sanity: the GETs did leave the process.
	entGETs := rt.CountFor("GET", "/entitlements/v1/assignments")
	r.Equal(uint64(rounds), entGETs, "expected %d GETs on entitlements path; got %d", rounds, entGETs)
}
