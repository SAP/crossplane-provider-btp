package btp

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"golang.org/x/oauth2"
)

func cisTestJWT(claims map[string]interface{}) string {
	b, _ := json.Marshal(claims)
	return "e30." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
}

func TestNativeCISWorkloadExchangeRefreshAndPrincipal(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	assertion := cisTestJWT(map[string]interface{}{"iss": "https://issuer.example", "sub": "sa", "aud": []string{"ias"}, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix()})
	require.NoError(t, os.WriteFile(file, []byte(assertion), 0600))
	var mu sync.Mutex
	var got []string
	badUser := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("password") != "" || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Error("unexpected password/grant")
		}
		email := "workload@example.com"
		mu.Lock()
		defer mu.Unlock()
		if badUser {
			email = "other@example.com"
		}
		claims := map[string]interface{}{"sub": "user", "mail": email, "user_name": email, "origin": "test-origin", "exp": time.Now().Add(time.Hour).Unix()}
		if r.URL.Path == "/oauth2/token" {
			got = append(got, r.Form.Get("assertion"))
			if r.Form.Get("client_assertion") != r.Form.Get("assertion") || r.Form.Get("resource") != "test-dependency" || r.Form.Get("client_secret") != "" {
				t.Error("bad IAS client authentication")
			}
		} else if r.Form.Get("client_secret") != "manual-binding-secret" {
			t.Error("missing manual CIS secret")
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{"access_token": cisTestJWT(claims), "token_type": "bearer", "expires_in": 3600}))
	}))
	defer server.Close()
	credentials := &Credentials{WorkloadIdentity: &WorkloadIdentityConfiguration{UserEmail: "workload@example.com", IdentityProvider: "test-origin", TokenFile: file, IASURL: server.URL, IASClientID: "consumer", IASResource: "test-dependency"}, CISCredential: &CISCredential{GrantType: "user_token"}}
	credentials.CISCredential.Uaa.Url = server.URL
	credentials.CISCredential.Uaa.Clientid = "manual"
	credentials.CISCredential.Uaa.Clientsecret = "manual-binding-secret"
	source := &workloadCISTokenSource{credentials: credentials, http: server.Client()}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := source.Token(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(got) != 1 {
		t.Fatalf("login stampede=%d", len(got))
	}
	rotated := assertion[:len(assertion)-4] + "c2lnMg"
	require.NoError(t, os.WriteFile(file, []byte(rotated), 0600))
	source.acquired = time.Now().Add(-16 * time.Minute)
	if _, err := source.Token(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1] != rotated {
		t.Fatal("renewal did not reread projection")
	}
	source.token.Expiry = time.Now().Add(-time.Minute)
	if _, err := source.Token(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatal("expired access token was not replaced")
	}
	source.acquired = time.Now().Add(-16 * time.Minute)
	badUser = true
	if _, err := source.Token(); err == nil {
		t.Fatal("accepted different IAS user")
	}
	badUser = false
	require.NoError(t, os.WriteFile(file, []byte(cisTestJWT(map[string]interface{}{"iss": "other", "sub": "other", "exp": time.Now().Add(time.Hour).Unix()})), 0600))
	if _, err := source.Token(); err == nil {
		t.Fatal("accepted different workload principal")
	}
}

func TestWorkloadCISInvalidatesUnauthorizedToken(t *testing.T) {
	source := &workloadCISTokenSource{token: &oauth2.Token{AccessToken: "cached"}}
	transport := &workloadCISTransport{source: source, inner: cisRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	req, _ := http.NewRequest(http.MethodGet, "https://api.example", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, resp.Body.Close())
	if source.token != nil {
		t.Fatal("401 did not invalidate native user token")
	}
}

type cisRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cisRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
