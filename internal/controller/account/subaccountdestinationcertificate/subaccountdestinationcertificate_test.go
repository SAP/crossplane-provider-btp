package subaccountdestinationcertificate

import (
	"context"
	"fmt"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination"
	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

// mockCertClient is a hand-written mock of CertificateClientI.
type mockCertClient struct {
	getCert   *destclient.Certificate
	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

func (m *mockCertClient) Get(_ context.Context, _ string) (*destclient.Certificate, error) {
	return m.getCert, m.getErr
}
func (m *mockCertClient) Create(_ context.Context, _ destclient.Certificate) error {
	return m.createErr
}
func (m *mockCertClient) Update(_ context.Context, _ destclient.Certificate) error {
	return m.updateErr
}
func (m *mockCertClient) Delete(_ context.Context, _ string) error { return m.deleteErr }

// capturingMockCertClient is like mockCertClient but records the cert passed to Create.
type capturingMockCertClient struct {
	getCert     *destclient.Certificate
	getErr      error
	createdCert destclient.Certificate
	createErr   error
	updateErr   error
	deleteErr   error
}

func (m *capturingMockCertClient) Get(_ context.Context, _ string) (*destclient.Certificate, error) {
	return m.getCert, m.getErr
}
func (m *capturingMockCertClient) Create(_ context.Context, cert destclient.Certificate) error {
	m.createdCert = cert
	return m.createErr
}
func (m *capturingMockCertClient) Update(_ context.Context, _ destclient.Certificate) error {
	return m.updateErr
}
func (m *capturingMockCertClient) Delete(_ context.Context, _ string) error { return m.deleteErr }

func newCertCR(externalName string, params v1alpha1.SubaccountDestinationCertificateParameters) *v1alpha1.SubaccountDestinationCertificate {
	cr := &v1alpha1.SubaccountDestinationCertificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-cert",
			Annotations: map[string]string{},
		},
		Spec: v1alpha1.SubaccountDestinationCertificateSpec{
			ResourceSpec: xpv1.ResourceSpec{},
			ForProvider:  params,
		},
	}
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	return cr
}

func newObservedCert(name, content, certType string) *destclient.Certificate {
	cert := destclient.NewCertificate(name, content)
	if certType != "" {
		cert.Type = &certType
	}
	return cert
}

// --- validateExternalName ---

func TestValidateExternalName(t *testing.T) {
	cases := map[string]bool{
		"sub-id/cert-name": true,
		"uuid-123/my-cert": true,
		"":                 false,
		"nodash":           false,
		"a/b/c":            true,
		"/cert":            false,
		"sub/":             false,
	}
	for input, wantOK := range cases {
		err := validateExternalName(input)
		if wantOK && err != nil {
			t.Errorf("validateExternalName(%q) = %v, want nil", input, err)
		}
		if !wantOK && err == nil {
			t.Errorf("validateExternalName(%q) = nil, want error", input)
		}
	}
}

// --- isUpToDate ---
// BTP GET returns Content but never Type (omitempty). Drift is detected via Content only.

func TestIsUpToDate_ContentMatches(t *testing.T) {
	observed := newObservedCert("cert", "abc123", "")
	if !isUpToDate("abc123", observed) {
		t.Error("isUpToDate = false, want true (content matches)")
	}
}

func TestIsUpToDate_ContentDiffers(t *testing.T) {
	observed := newObservedCert("cert", "old-content", "")
	if isUpToDate("new-content", observed) {
		t.Error("isUpToDate = true, want false (content changed)")
	}
}

func TestIsUpToDate_TypeInSpecDoesNotTriggerDrift(t *testing.T) {
	// Type is write-only — GET never returns it. A resource with type set in spec
	// must not be considered out-of-date just because observed.Type is nil.
	observed := newObservedCert("cert", "abc123", "") // Type absent in GET response
	if !isUpToDate("abc123", observed) {
		t.Error("isUpToDate = false, want true (type is write-only, not comparable)")
	}
}

func TestIsUpToDate_EmptyCertContent(t *testing.T) {
	// certContent "" (e.g. secret missing) never matches a real observed cert.
	if isUpToDate("", newObservedCert("cert", "abc123", "")) {
		t.Error("isUpToDate = true, want false (empty certContent never matches)")
	}
}

// --- Observe ---

func TestObserve_EmptyExternalName(t *testing.T) {
	subID := "sub-id"
	cr := newCertCR("", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert", SubaccountID: &subID,
	})
	e := &external{client: &mockCertClient{}}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.ResourceExists {
		t.Error("ResourceExists = true, want false for empty external-name")
	}
}

func TestObserve_NotFound(t *testing.T) {
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
	})
	e := &external{client: &mockCertClient{getErr: destination.NewNotFoundError()}}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.ResourceExists {
		t.Error("ResourceExists = true, want false on 404")
	}
}

func TestObserve_UpToDate(t *testing.T) {
	subID := "sub-id"
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert", Type: "PEM", SubaccountID: &subID,
	})
	// API returns Content; Type is absent (write-only).
	e := &external{client: &mockCertClient{getCert: newObservedCert("cert", "abc123", "")}, certContent: "abc123"}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !obs.ResourceExists {
		t.Error("ResourceExists = false, want true")
	}
	if !obs.ResourceUpToDate {
		t.Error("ResourceUpToDate = false, want true")
	}
	if cr.Status.AtProvider.Name == nil || *cr.Status.AtProvider.Name != "cert" {
		t.Errorf("AtProvider.Name = %v, want %q", cr.Status.AtProvider.Name, "cert")
	}
}

func TestObserve_NotUpToDate(t *testing.T) {
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
	})
	// API returns old content — drift detected.
	e := &external{client: &mockCertClient{getCert: newObservedCert("cert", "old-content", "")}, certContent: "new-content"}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !obs.ResourceExists {
		t.Error("ResourceExists = false, want true")
	}
	if obs.ResourceUpToDate {
		t.Error("ResourceUpToDate = true, want false (content changed)")
	}
}

func TestObserve_GetError(t *testing.T) {
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
	})
	e := &external{client: &mockCertClient{getErr: fmt.Errorf("api error")}}

	_, err := e.Observe(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Create ---

func TestCreate_SetsExternalName(t *testing.T) {
	subID := "sub-id"
	cr := newCertCR("", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert", SubaccountID: &subID,
	})
	e := &external{client: &mockCertClient{}, certContent: "abc123"}

	_, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := meta.GetExternalName(cr); got != "sub-id/cert" {
		t.Errorf("external-name = %q, want %q", got, "sub-id/cert")
	}
}

func TestCreate_NilSubaccountIDReturnsError(t *testing.T) {
	cr := newCertCR("", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
		// SubaccountID deliberately nil
	})
	e := &external{client: &mockCertClient{}}

	_, err := e.Create(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error when SubaccountID is nil, got nil")
	}
}

func TestCreate_Conflict(t *testing.T) {
	subID := "sub-id"
	cr := newCertCR("", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert", SubaccountID: &subID,
	})
	e := &external{client: &mockCertClient{createErr: destination.NewConflictError()}, certContent: "abc123"}

	_, err := e.Create(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error on 409 conflict, got nil")
	}
}

func TestCreate_ImportScenario(t *testing.T) {
	subID := "sub-id"
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert", SubaccountID: &subID,
	})
	// Resource exists — Get returns a cert; Create must not be called.
	e := &external{client: &mockCertClient{
		getCert:   newObservedCert("cert", "abc123", ""),
		createErr: destination.NewConflictError(), // would fail if called
	}, certContent: "abc123"}

	_, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("import scenario should not error, got: %v", err)
	}
	if got := meta.GetExternalName(cr); got != "sub-id/cert" {
		t.Errorf("external-name = %q, want %q after import", got, "sub-id/cert")
	}
}

func TestCreate_PresetExternalNameUsedForCertName(t *testing.T) {
	subID := "sub-id"
	// User pre-set external-name with a different cert name than spec.forProvider.name.
	cr := newCertCR("sub-id/preset-name.pem", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "spec-name.pem", SubaccountID: &subID,
	})
	// cert does not exist yet (404), so Create will be called.
	var createdCert destclient.Certificate
	mock := &capturingMockCertClient{getErr: destination.NewNotFoundError()}
	e := &external{client: mock, certContent: "abc123"}

	_, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	createdCert = mock.createdCert
	if createdCert.Name != "preset-name.pem" {
		t.Errorf("created cert name = %q, want %q (should use pre-set external-name, not spec.forProvider.name)", createdCert.Name, "preset-name.pem")
	}
	if got := meta.GetExternalName(cr); got != "sub-id/preset-name.pem" {
		t.Errorf("external-name = %q, want %q", got, "sub-id/preset-name.pem")
	}
}

// --- Update ---

func TestUpdate_Success(t *testing.T) {
	subID := "sub-id"
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert", SubaccountID: &subID,
	})
	e := &external{client: &mockCertClient{}, certContent: "new-content"}

	_, err := e.Update(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdate_PropagatesError(t *testing.T) {
	subID := "sub-id"
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert", SubaccountID: &subID,
	})
	e := &external{client: &mockCertClient{updateErr: fmt.Errorf("server error")}}

	_, err := e.Update(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Delete ---

func TestDelete_Success(t *testing.T) {
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
	})
	e := &external{client: &mockCertClient{}}

	_, err := e.Delete(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_EmptyExternalName(t *testing.T) {
	cr := newCertCR("", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
	})
	e := &external{client: &mockCertClient{}}

	_, err := e.Delete(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error on delete with empty external-name: %v", err)
	}
}

func TestDelete_MalformedExternalName(t *testing.T) {
	cr := newCertCR("nodash", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
	})
	e := &external{client: &mockCertClient{}}

	_, err := e.Delete(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error for malformed external-name, got nil")
	}
}

func TestDelete_PropagatesError(t *testing.T) {
	cr := newCertCR("sub-id/cert", v1alpha1.SubaccountDestinationCertificateParameters{
		Name: "cert",
	})
	e := &external{client: &mockCertClient{deleteErr: fmt.Errorf("server error")}}

	_, err := e.Delete(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// Verify interface compliance at compile time.
var _ managed.ExternalClient = &external{}
