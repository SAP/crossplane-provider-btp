package btp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestLegacyUserSecretCannotEnableWorkload(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, key := range []string{"TokenFile", "tokenFile", "TOKENFILE", "IASURL", "iasUrl", "iasClientId", "IASCLIENTID", "IASResource", "workloadIdentity"} {
		for _, value := range []any{server.URL, "", nil} {
			data, _ := json.Marshal(map[string]any{"Email": "user@example.com", "Username": "user", "Password": "secret", key: value})
			if _, err := ParseUserCredential(data); err == nil {
				t.Fatalf("accepted forbidden key %s", key)
			}
			cis := []byte(`{"grant_type":"user_token","uaa":{"clientid":"client","clientsecret":"secret","url":"` + server.URL + `"},"endpoints":{"accounts_service_url":"` + server.URL + `","entitlements_service_url":"` + server.URL + `","provisioning_service_url":"` + server.URL + `"}}`)
			if _, err := ServiceClientFromSecret(cis, data); err == nil {
				t.Fatal("Secret activated client")
			}
		}
	}
	if requests.Load() != 0 {
		t.Fatal("rejected Secret caused network traffic")
	}
}

func TestLegacyCredentialParsingCompatibility(t *testing.T) {
	for _, data := range []string{`{"email":"user@example.com","username":"user","password":"secret","idp":"custom","ignored":"compatible"}`, `{"EMAIL":"user@example.com","Username":"user","Password":"secret","Idp":"custom"}`} {
		user, err := ParseUserCredential([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		if user.Email != "user@example.com" || user.Username != "user" || user.Password != "secret" || user.Idp != "custom" {
			t.Fatal("legacy parsing changed")
		}
	}
	// Only immediate configuration keys are rejected, preserving unrelated legacy metadata.
	if _, err := ParseUserCredential([]byte(`{"metadata":{"TokenFile":"ignored"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyServiceClientKeepsAuthenticationModes(t *testing.T) {
	for _, grant := range []string{"user_token", "client_credentials"} {
		cis := []byte(`{"grant_type":"` + grant + `","uaa":{"clientid":"manual-client","clientsecret":"manual-secret","url":"https://uaa.example"},"endpoints":{"accounts_service_url":"https://api.example","entitlements_service_url":"https://api.example","provisioning_service_url":"https://api.example"}}`)
		c, err := ServiceClientFromSecret(cis, []byte(`{"email":"user@example.com","username":"legacy-user","password":"legacy-password","idp":"custom-origin"}`))
		if err != nil {
			t.Fatal(err)
		}
		if c.Credential.WorkloadIdentity != nil {
			t.Fatal("legacy credentials enabled workload mode")
		}
		params := authenticationParams(c.Credential)
		if grant == "client_credentials" {
			if params.Get("grant_type") != "client_credentials" || params.Get("username") != "manual-client" || params.Get("password") != "manual-secret" {
				t.Fatal("client credentials mode changed")
			}
		} else if params.Get("grant_type") != "password" || params.Get("username") != "user@example.com" || params.Get("password") != "legacy-password" {
			t.Fatal("legacy password mode changed")
		}
	}
}
