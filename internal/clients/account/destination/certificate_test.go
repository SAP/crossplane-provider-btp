package destination

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

func newTestCertClient(t *testing.T, srv *httptest.Server) *certificateClient {
	t.Helper()
	cfg := destclient.NewConfiguration()
	cfg.HTTPClient = srv.Client()
	cfg.Servers = destclient.ServerConfigurations{{URL: srv.URL}}
	return &certificateClient{api: destclient.NewAPIClient(cfg).CertificatesOnSubaccountLevelAPI}
}

func TestPublicKeys(t *testing.T) {
	srv := testServer(t, http.StatusOK, `[{"Content":"-----BEGIN CERTIFICATE-----\nAA==\n-----END CERTIFICATE-----\n","SubjectDN":"CN=x"}]`, "")
	defer srv.Close()
	keys, err := newTestCertClient(t, srv).PublicKeys(context.Background(), "c.pem")
	if err != nil || len(keys) != 1 || keys[0].GetSubjectDN() != "CN=x" {
		t.Fatalf("PublicKeys() = %+v, %v", keys, err)
	}
}

func TestPublicKeys_NotFound(t *testing.T) {
	srv := testServer(t, http.StatusNotFound, `{}`, "")
	defer srv.Close()
	_, err := newTestCertClient(t, srv).PublicKeys(context.Background(), "c.pem")
	if !IsNotFound(err) {
		t.Fatalf("want not-found, got %v", err)
	}
}

func TestGenerate_SendsOnlySetAttributes(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	a := destclient.NewAttributesAttributes()
	if err := newTestCertClient(t, srv).Generate(context.Background(), *destclient.NewAttributes("c.pem", *a)); err != nil {
		t.Fatal(err)
	}
	attrs, _ := body["Attributes"].(map[string]any)
	if body["Name"] != "c.pem" || attrs["AutomaticRenew"] != false {
		t.Errorf("body = %v", body)
	}
	for _, k := range []string{"CN", "Validity", "Password"} {
		if _, ok := attrs[k]; ok {
			t.Errorf("unset attribute %s was sent", k)
		}
	}
	if _, ok := body["Type"]; ok {
		t.Error("Type sent for .pem")
	}
}

func TestGenerate_Conflict(t *testing.T) {
	srv := testServer(t, http.StatusConflict, `{}`, "")
	defer srv.Close()
	a := destclient.NewAttributesAttributes()
	err := newTestCertClient(t, srv).Generate(context.Background(), *destclient.NewAttributes("c.pem", *a))
	if !IsConflict(err) {
		t.Fatalf("want conflict, got %v", err)
	}
}
