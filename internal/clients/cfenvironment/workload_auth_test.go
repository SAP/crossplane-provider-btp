package environments

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/sap/crossplane-provider-btp/btp"
)

func cfTestAssertion(subject string) string {
	body, _ := json.Marshal(map[string]any{"iss": "https://issuer.example", "sub": subject, "aud": []string{"https://ias.example"}, "exp": time.Now().Add(time.Hour).Unix()})
	return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(body) + ".c2ln"
}

func TestCFWorkloadLoginRotationAndRecovery(t *testing.T) {
	workloadCFMu.Lock()
	workloadCFCache = map[string]*config.Config{}
	workloadCFMu.Unlock()
	file := filepath.Join(t.TempDir(), "token")
	first := cfTestAssertion("first")
	os.WriteFile(file, []byte(first), 0600)
	var mu sync.Mutex
	var assertions []string
	reject := false
	shortExpiry := false
	noRefresh := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			r.ParseForm()
			mu.Lock()
			defer mu.Unlock()
			grant := r.Form.Get("grant_type")
			if grant == "refresh_token" {
				w.WriteHeader(400)
				io.WriteString(w, `{"error":"invalid_grant"}`)
				return
			}
			if grant != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.Form.Get("password") != "" || r.URL.Query().Get("login_hint") != `{"origin":"origin-test"}` {
				t.Errorf("unexpected workload authentication parameters")
			}
			assertions = append(assertions, r.Form.Get("assertion"))
			response := map[string]any{"access_token": "at", "token_type": "bearer", "expires_in": 3600}
			if shortExpiry {
				response["expires_in"] = 1
				shortExpiry = false
			}
			if !noRefresh {
				response["refresh_token"] = "rt"
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		if r.URL.Path == "/probe" {
			mu.Lock()
			defer mu.Unlock()
			if reject {
				reject = false
				w.WriteHeader(401)
				io.WriteString(w, `{"errors":[]}`)
				return
			}
			io.WriteString(w, `{}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"links": map[string]any{"login": map[string]string{"href": "http://" + r.Host}, "uaa": map[string]string{"href": "http://" + r.Host}, "app_ssh": map[string]any{"meta": map[string]string{"oauth_client": "ssh"}}}})
	}))
	defer server.Close()
	org := &btp.CloudFoundryOrg{Name: "org", Id: "guid", ApiEndpoint: server.URL}
	user := &btp.UserCredential{Email: "workload@example.com", Idp: "origin-test", TokenFile: file}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := newWorkloadOrganizationClient(org, user); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(assertions) != 1 {
		t.Fatalf("concurrent logins=%d", len(assertions))
	}
	// Same principal, rotated signature: cache stays stable.
	rotated := first[:len(first)-4] + "c2lnMg"
	os.WriteFile(file, []byte(rotated), 0600)
	if _, err := newWorkloadOrganizationClient(org, user); err != nil {
		t.Fatal(err)
	}
	if len(assertions) != 1 {
		t.Fatal("rotation caused login churn")
	}
	var cfg *config.Config
	for _, c := range workloadCFCache {
		cfg = c
	}
	transport := cfg.HTTPAuthClient().Transport.(*workloadCFTransport)
	// Session recycle must read current projection.
	transport.created = time.Now().Add(-16 * time.Minute)
	resp, err := cfg.HTTPAuthClient().Get(server.URL + "/probe")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(assertions) != 2 || assertions[1] != rotated {
		t.Fatal("recycle did not read rotated assertion")
	}
	// CF's refresh reauthentication retains the original assertion; our wrapper
	// renews from disk after it fails, including the no-refresh-token case.
	mu.Lock()
	reject = true
	mu.Unlock()
	resp, err = cfg.HTTPAuthClient().Get(server.URL + "/probe")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(assertions) != 3 || assertions[2] != rotated {
		t.Fatal("401 recovery did not read fresh assertion")
	}
	// Expired access token, rejected refresh grant: fresh assertion must recover.
	mu.Lock()
	shortExpiry = true
	mu.Unlock()
	transport.created = time.Now().Add(-16 * time.Minute)
	resp, err = cfg.HTTPAuthClient().Get(server.URL + "/probe")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(assertions) != 5 {
		t.Fatalf("refresh rejection recovery logins=%d", len(assertions))
	}
	// No refresh token: same fresh-assertion recovery instead of stale replay.
	mu.Lock()
	shortExpiry = true
	noRefresh = true
	mu.Unlock()
	transport.created = time.Now().Add(-16 * time.Minute)
	resp, err = cfg.HTTPAuthClient().Get(server.URL + "/probe")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(assertions) != 7 {
		t.Fatalf("no-refresh recovery logins=%d", len(assertions))
	}
	os.WriteFile(file, []byte(cfTestAssertion("other")), 0600)
	transport.created = time.Now().Add(-16 * time.Minute)
	if _, err = cfg.HTTPAuthClient().Get(server.URL + "/probe"); err == nil {
		t.Fatal("accepted changed principal in old session")
	}
	if _, err = newWorkloadOrganizationClient(org, user); err != nil {
		t.Fatal(err)
	}
	if len(assertions) != 8 {
		t.Fatal("new principal reused old session")
	}
}
