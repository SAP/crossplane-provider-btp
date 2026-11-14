// Package destinationtest fakes a Destination service binding for controller tests.
package destinationtest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BindingKey is the key under which BindingSecret stores the credentials.
const BindingKey = "credentials"

// Service serves a binding's token endpoint, issuing tokens for one
// subaccount, and a Destination service API that answers 404 to every call.
type Service struct {
	srv    *httptest.Server
	tokens atomic.Int32
}

// New starts a Service whose tokens carry subaccount in ext_attr.subaccountid.
func New(t *testing.T, subaccount string) *Service {
	t.Helper()
	enc := base64.RawURLEncoding.EncodeToString
	token := enc([]byte(`{"alg":"none"}`)) + "." + enc(fmt.Appendf(nil, `{"ext_attr":{"subaccountid":%q}}`, subaccount)) + ".sig"

	s := &Service{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		s.tokens.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":3600}`, token)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

// TokenRequests returns how many tokens were issued.
func (s *Service) TokenRequests() int { return int(s.tokens.Load()) }

// BindingSecret returns a binding Secret pointing at s, with the credentials
// as JSON under BindingKey.
func (s *Service) BindingSecret(name, namespace string) *corev1.Secret {
	cred, _ := json.Marshal(map[string]string{
		"clientid":     "id",
		"clientsecret": "secret",
		"tokenurl":     s.srv.URL + "/oauth/token",
		"uri":          s.srv.URL,
	})
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       map[string][]byte{BindingKey: cred},
	}
}
