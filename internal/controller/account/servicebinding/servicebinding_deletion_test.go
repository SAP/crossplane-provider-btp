package servicebinding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	ujcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/metrics"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/config"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
)

const (
	deletionTestBindingID  = "00000000-0000-4000-8000-000000000001"
	deletionTestActiveID   = "00000000-0000-4000-8000-000000000002"
	deletionTestAccountID  = "00000000-0000-4000-8000-000000000003"
	deletionTestInstanceID = "00000000-0000-4000-8000-000000000004"
)

// Exercise the pinned BTP provider and Upjet framework client. Only HTTP is
// substituted: the CLI proxy reports backend status in a header on HTTP 200.
// This covers the conversion of a backend 404 to a Delete diagnostic and of
// a Read 404 to null Terraform state, without credentials or external calls.
type bindingDeletionTransport struct {
	t              *testing.T
	exists         bool
	readStatus     int
	deleteStatus   int
	readErr        error
	deleteErr      error
	vanishOnDelete bool
	deleteBody     string
	deleteSucceeds bool
	verifyExists   bool
	verifyStatus   int
	verifyErr      error
	reads, deletes int
}

func (b *bindingDeletionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Path, "/login/") {
		return bindingDeletionResponse(req, 200, `{"mail":"test@example.com","issuer":"https://idp.invalid"}`), nil
	}
	if !strings.HasSuffix(req.URL.Path, "/services/binding") {
		b.t.Errorf("unexpected BTP command: %s", req.URL)
		return nil, fmt.Errorf("unexpected BTP command")
	}
	var body struct {
		Params map[string]string `json:"paramValues"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Params["id"] != deletionTestBindingID || body.Params["subaccount"] != deletionTestAccountID {
		b.t.Errorf("wrong binding target: %v", body.Params)
		return nil, fmt.Errorf("wrong binding target")
	}
	switch req.URL.RawQuery {
	case "get":
		b.reads++
		if b.deleteSucceeds && b.reads >= 3 {
			b.exists = b.verifyExists
			b.readStatus = b.verifyStatus
			b.readErr = b.verifyErr
		}
		if b.readErr != nil {
			response := bindingDeletionResponse(req, 200, "")
			response.Body = io.NopCloser(bindingDeletionErrorReader{b.readErr})
			return response, nil
		}
		if b.readStatus != 0 {
			return bindingDeletionResponse(req, b.readStatus, `{"error":"Forbidden","description":"insufficient permissions"}`), nil
		}
		if !b.exists {
			return bindingDeletionResponse(req, 404, `{"error":"NotFound","description":"could not find such /v1/service_bindings"}`), nil
		}
		payload := fmt.Sprintf(`{"id":%q,"subaccount_id":%q,"service_instance_id":%q,"name":"retired-binding","ready":true,"last_operation":{"state":"succeeded"},"credentials":{},"context":{},"labels":{}}`, deletionTestBindingID, deletionTestAccountID, deletionTestInstanceID)
		return bindingDeletionResponse(req, 200, payload), nil
	case "delete":
		b.deletes++
		if b.deleteErr != nil {
			response := bindingDeletionResponse(req, 200, "")
			response.Body = io.NopCloser(bindingDeletionErrorReader{b.deleteErr})
			return response, nil
		}
		if b.vanishOnDelete {
			b.exists = false
		}
		if b.deleteSucceeds {
			b.exists = false
			return bindingDeletionResponse(req, 200, `{}`), nil
		}
		if b.deleteStatus != 0 {
			return bindingDeletionResponse(req, b.deleteStatus, b.deleteBody), nil
		}
		if !b.exists {
			return bindingDeletionResponse(req, 404, `{"error":"NotFound","description":"could not find such /v1/service_bindings"}`), nil
		}
		b.t.Error("test did not configure a delete result")
		return nil, fmt.Errorf("unexpected delete")
	default:
		b.t.Errorf("unexpected binding action: %s", req.URL.RawQuery)
		return nil, fmt.Errorf("unexpected binding action")
	}
}

// Fail during response-body delivery, after the retrying HTTP client has
// received the headers. This avoids waiting through its network retry backoff.
type bindingDeletionErrorReader struct{ err error }

func (r bindingDeletionErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestRetiredBindingCleanup_BackendErrors(t *testing.T) {
	for _, mode := range []string{"expired", "retired"} {
		for _, tc := range []struct {
			name                     string
			exists                   bool
			readStatus, deleteStatus int
			readErr, deleteErr       error
			wantError                bool
			wantDeletes              int
		}{
			{name: "AlreadyAbsent"},
			{name: "ReadForbidden", readStatus: 403, wantError: true},
			{name: "ReadTransportFailure", readErr: errors.New("read connection failed"), wantError: true},
			{name: "DeleteForbidden", exists: true, deleteStatus: 403, wantError: true, wantDeletes: 1},
			{name: "DeleteTransportFailure", exists: true, deleteErr: errors.New("delete connection failed"), wantError: true, wantDeletes: 1},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				transport := &bindingDeletionTransport{
					t: t, exists: tc.exists, readStatus: tc.readStatus, deleteStatus: tc.deleteStatus,
					readErr: tc.readErr, deleteErr: tc.deleteErr,
					deleteBody: `{"error":"Forbidden","description":"insufficient permissions"}`,
				}
				e := bindingDeletionExternal(t, transport)
				cr := bindingDeletionCR()
				original := cr.DeepCopy()
				var err error
				var remaining []*v1alpha1.RetiredSBResource
				if mode == "expired" {
					remaining, err = e.keyRotator.DeleteExpiredKeys(context.Background(), cr)
				} else {
					err = e.keyRotator.DeleteRetiredKeys(context.Background(), cr)
					remaining = cr.Status.RetiredKeys
				}
				if (err != nil) != tc.wantError {
					t.Fatalf("unexpected cleanup error: %v", err)
				}
				if transport.reads != 1 || transport.deletes != tc.wantDeletes {
					t.Errorf("got %d reads and %d deletes, want 1 read and %d deletes", transport.reads, transport.deletes, tc.wantDeletes)
				}
				if tc.wantError {
					if len(remaining) != 1 || remaining[0].DeletionAttempts != 4 || remaining[0].LastDeletionError == original.Status.RetiredKeys[0].LastDeletionError {
						t.Fatalf("failed deletion must retain key and record failure: %+v", remaining)
					}
				} else {
					if mode == "expired" && len(remaining) != 0 {
						t.Errorf("confirmed absent expired binding must be removed from cleanup result: %+v", remaining)
					}
					if diff := cmp.Diff(original.Status.RetiredKeys, cr.Status.RetiredKeys); diff != "" {
						t.Errorf("successful cleanup changed failure bookkeeping (-want +got):\n%s", diff)
					}
				}
				// Neither the native resource nor its active binding can be mutated
				// by building/observing/deleting a retired binding's TF shadow.
				original.Status.RetiredKeys = cr.DeepCopy().Status.RetiredKeys
				if diff := cmp.Diff(original, cr); diff != "" {
					t.Errorf("active ServiceBinding changed (-want +got):\n%s", diff)
				}
			})
		}
	}
}

func TestDeleteBinding_ConcurrentRemovalConverges(t *testing.T) {
	transport := &bindingDeletionTransport{t: t, exists: true, vanishOnDelete: true}
	e := bindingDeletionExternal(t, transport)
	cr := bindingDeletionCR()
	remaining, err := e.keyRotator.DeleteExpiredKeys(context.Background(), cr)
	if err == nil || len(remaining) != 1 || !strings.Contains(err.Error(), "NotFound") {
		t.Fatalf("first reconcile must retain the key after a Delete error: keys=%v, err=%v", remaining, err)
	}
	remaining, err = e.keyRotator.DeleteExpiredKeys(context.Background(), cr)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("next reconcile must confirm absence and remove the key: keys=%v, err=%v", remaining, err)
	}
	if transport.reads != 2 || transport.deletes != 1 {
		t.Errorf("got %d reads and %d deletes, want 2 reads and 1 delete", transport.reads, transport.deletes)
	}
}

func TestRetiredBindingCleanup_DeletionVerification(t *testing.T) {
	for _, mode := range []string{"expired", "retired"} {
		for _, tc := range []struct {
			name                     string
			verifyExists             bool
			verifyStatus             int
			verifyErr                error
			wantError, wantTransient bool
		}{
			{name: "ConfirmedAbsent"},
			{name: "StillPresent", verifyExists: true, wantError: true},
			{name: "VerificationForbidden", verifyStatus: 403, wantError: true, wantTransient: true},
			{name: "VerificationTransportFailure", verifyErr: errors.New("verification connection failed"), wantError: true, wantTransient: true},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				transport := &bindingDeletionTransport{
					t: t, exists: true, deleteSucceeds: true,
					verifyExists: tc.verifyExists, verifyStatus: tc.verifyStatus, verifyErr: tc.verifyErr,
				}
				e := bindingDeletionExternal(t, transport)
				cr := bindingDeletionCR()
				original := cr.DeepCopy()
				var err error
				var remaining []*v1alpha1.RetiredSBResource
				if mode == "expired" {
					remaining, err = e.keyRotator.DeleteExpiredKeys(context.Background(), cr)
				} else {
					err = e.keyRotator.DeleteRetiredKeys(context.Background(), cr)
					remaining = cr.Status.RetiredKeys
				}
				if (err != nil) != tc.wantError || errors.Is(err, servicebindingclient.ErrVerifyTransient) != tc.wantTransient {
					t.Fatalf("unexpected verification result: %v", err)
				}
				if transport.reads != 3 || transport.deletes != 1 {
					t.Errorf("got %d reads and %d deletes, want pre-read, TF polling read, verification read, and 1 delete", transport.reads, transport.deletes)
				}
				if tc.wantError {
					wantAttempts := int32(4)
					if tc.wantTransient {
						wantAttempts = 3
					}
					if len(remaining) != 1 || remaining[0].DeletionAttempts != wantAttempts {
						t.Fatalf("unverified deletion must retain key with %d attempts: %+v", wantAttempts, remaining)
					}
					if tc.wantTransient && remaining[0].LastDeletionError != original.Status.RetiredKeys[0].LastDeletionError {
						t.Error("verification failure should preserve existing LastDeletionError")
					}
				} else if mode == "expired" && len(remaining) != 0 {
					t.Errorf("verified absent binding must be removed: %+v", remaining)
				}
				original.Status.RetiredKeys = cr.DeepCopy().Status.RetiredKeys
				if diff := cmp.Diff(original, cr); diff != "" {
					t.Errorf("active ServiceBinding changed (-want +got):\n%s", diff)
				}
			})
		}
	}
}

func bindingDeletionResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":              {"application/json"},
			"X-Cpcli-Sessionid":         {"test-session"},
			"X-Cpcli-Backend-Status":    {strconv.Itoa(status)},
			"X-Cpcli-Backend-Mediatype": {"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}
}

func bindingDeletionExternal(t *testing.T, transport *bindingDeletionTransport) *external {
	t.Helper()
	provider := tfprovider.NewWithClient(&http.Client{Transport: transport})
	setup := func(context.Context, kubeclient.Client, resource.Managed) (terraform.Setup, error) {
		return terraform.Setup{
			FrameworkProvider: provider,
			Configuration: map[string]any{
				"username": "test-user", "password": "test-password",
				"globalaccount": "test-account", "cli_server_url": "https://btp.invalid",
			},
		}, nil
	}
	kube := fake.NewClientBuilder().Build()
	cfg := config.GetProvider().Resources["btp_subaccount_service_binding"]
	connector := ujcontroller.NewTerraformPluginFrameworkConnector(kube, setup, cfg, ujcontroller.NewOperationStore(logging.NewNopLogger()), ujcontroller.WithTerraformPluginFrameworkMetricRecorder(&metrics.MetricRecorder{}), ujcontroller.WithTerraformPluginFrameworkLogger(logging.NewNopLogger()))
	e := &external{clientFactory: newServiceBindingClientFactory(kube, connector)}
	e.keyRotator = servicebindingclient.NewSBKeyRotator(e)
	return e
}

func bindingDeletionCR() *v1alpha1.ServiceBinding {
	cr := &v1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "active-binding", UID: "test-uid"},
		Spec: v1alpha1.ServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{ProviderConfigReference: &xpv1.Reference{Name: "test-provider"}},
			ForProvider: v1alpha1.ServiceBindingParameters{
				Name: "active-binding", SubaccountID: internal.Ptr(deletionTestAccountID),
				ServiceInstanceID: internal.Ptr(deletionTestInstanceID),
			},
		},
		Status: v1alpha1.ServiceBindingStatus{
			AtProvider: v1alpha1.ServiceBindingObservation{
				ID: deletionTestActiveID, Name: "active-binding", Ready: internal.Ptr(true), State: internal.Ptr("succeeded"),
			},
			RetiredKeys: []*v1alpha1.RetiredSBResource{{
				ID: deletionTestBindingID, Name: "retired-binding",
				DeletionDate:     internal.Ptr(metav1.NewTime(time.Now().Add(-time.Hour))),
				DeletionAttempts: 3, LastDeletionError: "previous NotFound",
			}},
		},
	}
	meta.SetExternalName(cr, deletionTestActiveID)
	return cr
}

func TestDeleteBinding_BackendNotFound(t *testing.T) {
	transport := &bindingDeletionTransport{t: t}
	e := bindingDeletionExternal(t, transport)
	cr := bindingDeletionCR()
	original := cr.DeepCopy()

	// Reproduce the backend error through the actual Delete and diagnostic path.
	client, err := e.clientFactory.CreateClient(context.Background(), cr, "retired-binding", deletionTestBindingID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Delete(context.Background()); err == nil || !strings.Contains(err.Error(), "API Error Deleting Resource Service Binding (Subaccount)") {
		t.Fatalf("expected the backend Delete diagnostic, got %v", err)
	}
	transport.deletes = 0
	if err := e.DeleteBinding(context.Background(), cr, "retired-binding", deletionTestBindingID); err != nil {
		t.Fatalf("already absent binding should complete deletion: %v", err)
	}
	if transport.reads != 1 || transport.deletes != 0 {
		t.Errorf("got %d reads and %d deletes, want 1 read and no delete", transport.reads, transport.deletes)
	}
	if diff := cmp.Diff(original, cr); diff != "" {
		t.Errorf("original ServiceBinding changed (-want +got):\n%s", diff)
	}
}

func TestUpdate_PersistsConfirmedAbsentRetiredKeyCleanup(t *testing.T) {
	cr := bindingDeletionCR()
	cr.SetConditions(xpv1.Available())
	cr.Status.RetiredKeys = append(cr.Status.RetiredKeys, &v1alpha1.RetiredSBResource{
		ID: "00000000-0000-4000-8000-000000000005", Name: "not-yet-expired",
		DeletionDate: internal.Ptr(metav1.NewTime(time.Now().Add(time.Hour).Truncate(time.Second))),
	})
	original := cr.DeepCopy()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	transport := &bindingDeletionTransport{t: t}
	e := bindingDeletionExternal(t, transport)
	e.kube = kube
	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	got := &v1alpha1.ServiceBinding{}
	if err := kube.Get(context.Background(), types.NamespacedName{Name: cr.Name}, got); err != nil {
		t.Fatal(err)
	}
	original.Status.RetiredKeys = original.Status.RetiredKeys[1:]
	original.ResourceVersion = got.ResourceVersion
	if diff := cmp.Diff(original, got); diff != "" {
		t.Errorf("cleanup must remove only the confirmed absent expired key (-want +got):\n%s", diff)
	}
	if transport.reads != 1 || transport.deletes != 0 {
		t.Errorf("got %d reads and %d deletes, want 1 read and no delete", transport.reads, transport.deletes)
	}
}
