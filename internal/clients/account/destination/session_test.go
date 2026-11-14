package destination

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// jwtWith builds an unsigned three-part JWT around the given JSON payload.
func jwtWith(payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(payload)) + ".sig"
}

func tokenServer(t *testing.T, accessToken string, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":3600}`, accessToken)
	}))
}

// The binding check must not cost an extra token request per reconcile.
func TestSession_ClaimAndClientsShareOneToken(t *testing.T) {
	var hits int32
	tokSrv := tokenServer(t, jwtWith(`{"ext_attr":{"subaccountid":"sub-1"}}`), &hits)
	defer tokSrv.Close()
	apiSrv := testServer(t, http.StatusOK, `{"Name":"d","Type":"HTTP","Content":"AA=="}`, "")
	defer apiSrv.Close()

	s := NewSession(DestinationCredential{ClientID: "id", ClientSecret: "secret", TokenURL: tokSrv.URL, URI: apiSrv.URL})
	got, err := s.TokenSubaccount()
	if err != nil || got != "sub-1" {
		t.Fatalf("TokenSubaccount() = %q, %v; want sub-1, nil", got, err)
	}
	if _, _, err := s.DestinationClient().Get(context.Background(), "d"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.CertificateClient().Get(context.Background(), "c.pem"); err != nil {
		t.Fatalf("certificate Get: %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("token requests = %d, want 1", n)
	}
}
