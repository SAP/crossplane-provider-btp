package subaccountdestination

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
)

// mockDestClient is a hand-written mock of DestinationClientI.
type mockDestClient struct {
	getProps  map[string]string
	getEtag   string
	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

func (m *mockDestClient) Get(_ context.Context, _ string) (map[string]string, string, error) {
	return m.getProps, m.getEtag, m.getErr
}
func (m *mockDestClient) Create(_ context.Context, _ map[string]any) error { return m.createErr }
func (m *mockDestClient) Update(_ context.Context, _ map[string]any, _ string) error {
	return m.updateErr
}
func (m *mockDestClient) Delete(_ context.Context, _ string) error { return m.deleteErr }

func newCR(externalName string, params v1alpha1.SubaccountDestinationParameters) *v1alpha1.SubaccountDestination {
	cr := &v1alpha1.SubaccountDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dest",
			Annotations: map[string]string{},
		},
		Spec: v1alpha1.SubaccountDestinationSpec{
			ResourceSpec: xpv1.ResourceSpec{},
			ForProvider:  params,
		},
	}
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	return cr
}

// --- validateExternalName ---

func TestValidateExternalName(t *testing.T) {
	cases := map[string]bool{
		"sub-id/dest-name": true,
		"uuid-123/my-dest": true,
		"":                 false,
		"nodash":           false,
		"a/b/c":            true,
		"/dest":            false,
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

func TestIsUpToDate_Equal(t *testing.T) {
	desired := map[string]any{"Name": "d", "Type": "HTTP", "URL": "https://x.com"}
	observed := map[string]string{"Name": "d", "Type": "HTTP", "URL": "https://x.com"}
	if !isUpToDate(desired, observed) {
		t.Error("isUpToDate = false, want true")
	}
}

func TestIsUpToDate_MissingKey(t *testing.T) {
	desired := map[string]any{"Name": "d", "Type": "HTTP", "URL": "https://x.com"}
	observed := map[string]string{"Name": "d", "Type": "HTTP"}
	if isUpToDate(desired, observed) {
		t.Error("isUpToDate = true, want false (missing URL)")
	}
}

func TestIsUpToDate_DifferentValue(t *testing.T) {
	desired := map[string]any{"Name": "d", "Type": "HTTP", "URL": "https://new.com"}
	observed := map[string]string{"Name": "d", "Type": "HTTP", "URL": "https://old.com"}
	if isUpToDate(desired, observed) {
		t.Error("isUpToDate = true, want false (different URL)")
	}
}

// --- Observe ---

func TestObserve_EmptyExternalName(t *testing.T) {
	subID := "sub-id"
	cr := newCR("", v1alpha1.SubaccountDestinationParameters{Name: "dest", Type: "HTTP", SubaccountID: &subID})
	e := &external{client: &mockDestClient{}}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.ResourceExists {
		t.Error("ResourceExists = true, want false for empty external-name")
	}
}

func TestObserve_NotFound(t *testing.T) {
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{Name: "dest", Type: "HTTP"})
	e := &external{client: &mockDestClient{getErr: destination.NewNotFoundError()}}

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
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", SubaccountID: &subID,
	})
	props := map[string]string{"Name": "dest", "Type": "HTTP"}
	e := &external{client: &mockDestClient{getProps: props, getEtag: "etag1"}}

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
	if cr.Status.AtProvider.ETag == nil || *cr.Status.AtProvider.ETag != "etag1" {
		t.Errorf("ETag not synced, got %v", cr.Status.AtProvider.ETag)
	}
}

func TestObserve_NotUpToDate(t *testing.T) {
	url := "https://new.com"
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", URL: &url,
	})
	props := map[string]string{"Name": "dest", "Type": "HTTP", "URL": "https://old.com"}
	e := &external{client: &mockDestClient{getProps: props}}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !obs.ResourceExists {
		t.Error("ResourceExists = false, want true")
	}
	if obs.ResourceUpToDate {
		t.Error("ResourceUpToDate = true, want false (URL changed)")
	}
}

// --- Create ---

func TestCreate_SetsExternalName(t *testing.T) {
	subID := "sub-id"
	cr := newCR("", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", SubaccountID: &subID,
	})
	e := &external{client: &mockDestClient{}}

	_, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := meta.GetExternalName(cr); got != "sub-id/dest" {
		t.Errorf("external-name = %q, want %q", got, "sub-id/dest")
	}
}

func TestCreate_Conflict(t *testing.T) {
	subID := "sub-id"
	cr := newCR("", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", SubaccountID: &subID,
	})
	e := &external{client: &mockDestClient{createErr: destination.NewConflictError()}}

	_, err := e.Create(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error on 409 conflict, got nil")
	}
}

func TestCreate_ImportScenario(t *testing.T) {
	subID := "sub-id"
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", SubaccountID: &subID,
	})
	// Resource exists — Get returns props, Create should not be called
	e := &external{client: &mockDestClient{
		getProps:  map[string]string{"Name": "dest", "Type": "HTTP"},
		createErr: destination.NewConflictError(), // would fail if called
	}}

	_, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("import scenario should not error, got: %v", err)
	}
}

// --- Delete ---

func TestDelete_Success(t *testing.T) {
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{Name: "dest", Type: "HTTP"})
	e := &external{client: &mockDestClient{}}

	_, err := e.Delete(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_EmptyExternalName(t *testing.T) {
	cr := newCR("", v1alpha1.SubaccountDestinationParameters{Name: "dest", Type: "HTTP"})
	e := &external{client: &mockDestClient{deleteErr: destination.NewNotFoundError()}}

	_, err := e.Delete(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error on delete with empty external-name: %v", err)
	}
}

// --- buildPropertyBag ---

func TestBuildPropertyBag_TypedFields(t *testing.T) {
	url := "https://example.com"
	auth := "NoAuthentication"
	subID := "sub-id"
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", URL: &url, Authentication: &auth, SubaccountID: &subID,
		AdditionalProperties: map[string]string{"ProxyType": "Internet"},
	})

	props, err := buildPropertyBag(context.Background(), nil, cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props["Name"] != "dest" {
		t.Errorf("Name = %q, want %q", props["Name"], "dest")
	}
	if props["URL"] != "https://example.com" {
		t.Errorf("URL = %q, want %q", props["URL"], "https://example.com")
	}
	if props["Authentication"] != "NoAuthentication" {
		t.Errorf("Authentication = %q, want %q", props["Authentication"], "NoAuthentication")
	}
	if props["ProxyType"] != "Internet" {
		t.Errorf("ProxyType = %q, want %q", props["ProxyType"], "Internet")
	}
}

func TestBuildPropertyBag_AdditionalOverridesTyped(t *testing.T) {
	url := "https://original.com"
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", URL: &url,
		AdditionalProperties: map[string]string{"URL": "https://override.com"},
	})

	props, err := buildPropertyBag(context.Background(), nil, cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props["URL"] != "https://override.com" {
		t.Errorf("URL = %q, want override value", props["URL"])
	}
}

// --- Update ---

func TestUpdate_Success(t *testing.T) {
	etag := "etag-42"
	url := "https://updated.example.com"
	subID := "sub-id"
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", URL: &url, SubaccountID: &subID,
	})
	cr.Status.AtProvider.ETag = &etag

	var capturedEtag string
	var capturedProps map[string]any
	mock := &capturingUpdateClient{
		onUpdate: func(props map[string]any, e string) error {
			capturedProps = props
			capturedEtag = e
			return nil
		},
	}
	e := &external{client: mock}

	_, err := e.Update(context.Background(), cr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedEtag != etag {
		t.Errorf("Update called with etag %q, want %q", capturedEtag, etag)
	}
	if capturedProps["URL"] != url {
		t.Errorf("Update props URL = %q, want %q", capturedProps["URL"], url)
	}
}

func TestUpdate_PropagatesError(t *testing.T) {
	subID := "sub-id"
	cr := newCR("sub-id/dest", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP", SubaccountID: &subID,
	})
	e := &external{client: &mockDestClient{updateErr: fmt.Errorf("server error")}}

	_, err := e.Update(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// capturingUpdateClient captures Update arguments for assertion.
type capturingUpdateClient struct {
	mockDestClient
	onUpdate func(props map[string]any, etag string) error
}

func (c *capturingUpdateClient) Update(_ context.Context, props map[string]any, etag string) error {
	return c.onUpdate(props, etag)
}

// --- Delete with malformed external-name ---

func TestDelete_MalformedExternalName(t *testing.T) {
	cr := newCR("nodash", v1alpha1.SubaccountDestinationParameters{Name: "dest", Type: "HTTP"})
	e := &external{client: &mockDestClient{}}

	_, err := e.Delete(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error for malformed external-name, got nil")
	}
}

// --- Create with nil SubaccountID ---

func TestCreate_NilSubaccountIDReturnsError(t *testing.T) {
	cr := newCR("", v1alpha1.SubaccountDestinationParameters{
		Name: "dest", Type: "HTTP",
		// SubaccountID deliberately nil (ref not yet resolved)
	})
	e := &external{client: &mockDestClient{}}

	_, err := e.Create(context.Background(), cr)
	if err == nil {
		t.Fatal("expected error when SubaccountID is nil, got nil")
	}
}

// Verify interface compliance at compile time.
var _ managed.ExternalClient = &external{}
