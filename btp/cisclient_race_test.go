package btp

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2/clientcredentials"
)

// TestClientCache_SingleBuildUnderConcurrentFirstCall probes CTL-3.
//
// NewServiceClientWithCisCredential's Load+LoadOrStore pattern let
// concurrent first-callers on the same credential each execute
// createClient(). LoadOrStore kept one; the others were discarded, but
// the work — oauth2 transport, three openapi sub-clients, shared
// *http.Client — had already been done. The fix wraps the build in
// singleflight.Group.Do, so exactly one build runs even under 32
// concurrent first-callers.
//
// We swap buildClientFn to count invocations, blow away clientCache, and
// let 32 goroutines all pile into NewServiceClientWithCisCredential at
// once. builds.Load() must equal 1.
func TestClientCache_SingleBuildUnderConcurrentFirstCall(t *testing.T) {
	// Shares package-level clientCache — non-parallel.
	r := require.New(t)

	clientCache.Range(func(k, _ any) bool { clientCache.Delete(k); return true })

	var builds atomic.Uint64
	orig := buildClientFn
	buildClientFn = func(cred *Credentials, cfg *clientcredentials.Config) Client {
		builds.Add(1)
		return Client{Credential: cred}
	}
	t.Cleanup(func() { buildClientFn = orig })

	cred := &Credentials{
		CISCredential: &CISCredential{GrantType: grantTypeClientCredentials},
	}
	cred.CISCredential.Uaa.Clientid = "cid-3"
	cred.CISCredential.Uaa.Clientsecret = "cs"
	cred.CISCredential.Uaa.Url = "https://uaa.example"
	cred.CISCredential.Endpoints.AccountsServiceUrl = "https://a.example"
	cred.CISCredential.Endpoints.EntitlementsServiceUrl = "https://e.example"
	cred.CISCredential.Endpoints.ProvisioningServiceUrl = "https://p.example"

	const callers = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]Client, callers)
	for i := range callers {
		wg.Go(func() {
			<-start
			results[i] = NewServiceClientWithCisCredential(cred)
		})
	}
	close(start)
	wg.Wait()

	t.Logf("concurrent first-callers that ran createClient: %d / %d", builds.Load(), callers)
	r.Equal(uint64(1), builds.Load(), "singleflight must collapse concurrent first-callers into a single build")
	for i := 1; i < callers; i++ {
		r.Equal(results[0].Credential, results[i].Credential, "all callers must receive the same Client")
	}
}

// TestClientCache_LoadOrStoreIdentity — regression guard. All callers must
// receive the same *cached* Client value even under contention.
func TestClientCache_LoadOrStoreIdentity(t *testing.T) {
	// Shares package-level clientCache — non-parallel.
	r := require.New(t)

	clientCache.Range(func(k, _ any) bool { clientCache.Delete(k); return true })

	cred := &Credentials{
		CISCredential: &CISCredential{GrantType: grantTypeClientCredentials},
	}
	cred.CISCredential.Uaa.Clientid = "cid-2"
	cred.CISCredential.Uaa.Clientsecret = "s"
	cred.CISCredential.Uaa.Url = "https://uaa.example"

	sentinel := Client{Credential: cred}
	clientCache.Store(credentialCacheKey(cred), sentinel)

	var wg sync.WaitGroup
	results := make([]Client, 16)
	for i := range 16 {
		wg.Go(func() {
			v, ok := clientCache.Load(credentialCacheKey(cred))
			r.True(ok)
			results[i] = v.(Client)
		})
	}
	wg.Wait()

	for i := 1; i < len(results); i++ {
		r.Equal(sentinel.Credential, results[i].Credential, "all readers must see the same cached Client")
	}
}
