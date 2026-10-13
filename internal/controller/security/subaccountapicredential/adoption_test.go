package subaccountapicredential

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ujcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/metrics"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerconfig "github.com/sap/crossplane-provider-btp/config"
)

const (
	adoptionTestSubaccount = "subaccount-868"
	adoptionTestName       = "sac-868-existing-credential"
)

// adoptionTransport models an existing BTP credential. BTP's get response
// intentionally omits client_secret: the secret is write-once and cannot be
// recovered by observing the credential.
type adoptionTransport struct {
	t                   *testing.T
	reads, creates      int
	deletes             int
	gotSubaccount, name string
}

func (a *adoptionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Path, "/login/") {
		return adoptionResponse(req, `{"mail":"test@example.com","issuer":"https://idp.invalid"}`, map[string]string{
			"X-Cpcli-Sessionid": "test-session",
		}), nil
	}
	if !strings.Contains(req.URL.Path, "/security/api-credential") {
		a.t.Errorf("unexpected BTP request: %s", req.URL)
		return nil, fmt.Errorf("unexpected BTP request: %s", req.URL)
	}

	var body struct {
		Params map[string]string `json:"paramValues"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode BTP request: %w", err)
	}
	a.gotSubaccount = body.Params["subaccount"]
	a.name = body.Params["name"]
	if a.gotSubaccount != adoptionTestSubaccount || a.name != adoptionTestName {
		a.t.Errorf("unexpected external identity (subaccount=%q, name=%q)", a.gotSubaccount, a.name)
	}

	switch req.URL.RawQuery {
	case "get":
		a.reads++
		return adoptionResponse(req, fmt.Sprintf(`{"credential-type":"secret","tokenurl":"https://tenant.authentication.example/oauth/token","clientId":"client-868","apiurl":"https://api.authentication.example","name":%q,"read-only":false}`, adoptionTestName), map[string]string{
			"X-Cpcli-Backend-Status":    "200",
			"X-Cpcli-Backend-Mediatype": "application/json",
		}), nil
	case "create":
		a.creates++
		a.t.Error("adoption attempt unexpectedly called BTP create")
		return adoptionResponse(req, `{}`, map[string]string{"X-Cpcli-Backend-Status": "409"}), nil
	case "delete":
		a.deletes++
		a.t.Error("adoption attempt unexpectedly called BTP delete")
		return adoptionResponse(req, `{}`, map[string]string{"X-Cpcli-Backend-Status": "200"}), nil
	default:
		a.t.Errorf("unexpected API credential action %q", req.URL.RawQuery)
		return nil, fmt.Errorf("unexpected API credential action %q", req.URL.RawQuery)
	}
}

func adoptionResponse(req *http.Request, body string, headers map[string]string) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	for key, value := range headers {
		h.Set(key, value)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

// TestRejectsUserSuppliedExternalNameAdoption models the live #868 reproduction:
// a new full-management MR supplies an existing secret-based credential's
// compound external-name and matching explicit spec identity, but has no status,
// Terraform state, or copied connection Secret. The annotation is not ownership
// evidence. BTP's successful get response cannot recover client_secret.
//
// Exercise configured initialization and the real Terraform/Upjet observation
// path. Rejection may happen before any BTP read, but must precede accepting the
// credential as managed or returning partial connection details. A later
// missing-secret error is not adequate adoption prevention. This offline test
// does not model persisted Ready conditions, Secret publication, or finalization;
// the annotated E2E tests cover those reconciliation outcomes and restarts.
func TestRejectsUserSuppliedExternalNameAdoption(t *testing.T) {
	transport := &adoptionTransport{t: t}
	provider := tfprovider.NewWithClient(&http.Client{Transport: transport})
	setup := func(context.Context, client.Client, resource.Managed) (terraform.Setup, error) {
		return terraform.Setup{
			FrameworkProvider: provider,
			Configuration: map[string]any{
				"username":       "test-user",
				"password":       "test-password",
				"globalaccount":  "test-account",
				"cli_server_url": "https://btp.invalid",
			},
		}, nil
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot add core scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot add SubaccountApiCredential scheme: %v", err)
	}

	readOnly := false
	subaccountID := adoptionTestSubaccount
	credentialName := adoptionTestName
	suppliedExternalName := subaccountID + "/" + credentialName
	cr := &v1alpha1.SubaccountApiCredential{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.CRDGroupVersion.String(), Kind: "SubaccountApiCredential"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "new-resource-868",
			UID:  types.UID("new-resource-868-uid"),
			Annotations: map[string]string{
				meta.AnnotationKeyExternalName: suppliedExternalName,
			},
			// Deliberately no previous observation or ownership evidence.
		},
		Spec: v1alpha1.SubaccountApiCredentialSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				DeletionPolicy:     xpv1.DeletionDelete,
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionAll},
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name:      "new-credential-secret",
					Namespace: "default",
				},
			},
			ForProvider: v1alpha1.SubaccountApiCredentialParameters{
				Name:         &credentialName,
				SubaccountID: &subaccountID,
				ReadOnly:     &readOnly,
			},
		},
	}

	// Only the new MR exists in Kubernetes; the Secret destination is fresh.
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	cfg := providerconfig.GetProvider().Resources["btp_subaccount_api_credential"]
	connector := NewGuardedConnector(
		kube,
		setup,
		cfg,
		ujcontroller.NewOperationStore(logging.NewNopLogger()),
		logging.NewNopLogger(),
		&metrics.MetricRecorder{},
		false,
	)
	ctx := context.Background()
	observation, err := func() (managed.ExternalObservation, error) {
		for _, newInitializer := range cfg.InitializerFns {
			if err := newInitializer(kube).Initialize(ctx, cr); err != nil {
				return managed.ExternalObservation{}, err
			}
		}
		external, err := connector.Connect(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		return external.Observe(ctx, cr)
	}()
	if err == nil {
		t.Error("expected user-supplied external-name adoption to be explicitly rejected")
	} else if !strings.Contains(strings.ToLower(err.Error()), "adopt") {
		t.Errorf("expected explicit unsupported-adoption error, not an unrelated initialization/connection/observation error: %v", err)
	}
	if observation.ResourceExists {
		t.Error("rejected adoption must not be reported as managed")
	}
	if diff := cmp.Diff(v1alpha1.SubaccountApiCredentialObservation{}, cr.Status.AtProvider); diff != "" {
		t.Errorf("rejected adoption must not retain public fields from the existing credential (-want +got):\n%s", diff)
	}
	if len(observation.ConnectionDetails) != 0 {
		t.Errorf("rejected adoption must not return connection details (got %d keys)", len(observation.ConnectionDetails))
	}
	if ready := cr.GetCondition(xpv1.TypeReady); ready.Status == corev1.ConditionTrue {
		t.Errorf("rejected adoption must not make the resource Ready: %#v", ready)
	}
	// Keeping the user-supplied annotation does not itself claim ownership;
	// clearing it is also acceptable, but retargeting to another identity is not.
	if got := meta.GetExternalName(cr); got != "" && got != suppliedExternalName {
		t.Errorf("rejected adoption must not retarget external-name, got %q (supplied %q)", got, suppliedExternalName)
	}
	if transport.creates != 0 || transport.deletes != 0 {
		t.Errorf("adoption rejection must not create/delete credentials: creates=%d deletes=%d", transport.creates, transport.deletes)
	}
	if cr.Status.Lifecycle == nil || cr.Status.Lifecycle.Phase != v1alpha1.SubaccountApiCredentialPhaseRejected {
		t.Fatalf("expected persisted terminal Rejected lifecycle state, got %#v", cr.Status.Lifecycle)
	}
	// Reconstruct from persisted status and use a fresh external client to prove
	// normal Delete-policy finalization takes the local no-external-resource path.
	reloaded := &v1alpha1.SubaccountApiCredential{}
	if err := kube.Get(ctx, client.ObjectKeyFromObject(cr), reloaded); err != nil {
		t.Fatalf("reload rejected MR: %v", err)
	}
	now := metav1.Now()
	reloaded.DeletionTimestamp = &now
	cleanup, err := connector.Connect(ctx, reloaded)
	if err != nil {
		t.Fatalf("connect rejected resource during deletion: %v", err)
	}
	cleanupObservation, err := cleanup.Observe(ctx, reloaded)
	if err != nil || cleanupObservation.ResourceExists {
		t.Errorf("rejected-resource cleanup should report local absence (observation=%#v, err=%v)", cleanupObservation, err)
	}
	if _, err := cleanup.Delete(ctx, reloaded); err != nil {
		t.Errorf("rejected-resource local Delete should succeed: %v", err)
	}
	if transport.deletes != 0 {
		t.Errorf("rejected-resource cleanup called BTP Delete %d times", transport.deletes)
	}
	// An early guard may reject before reading. If Observe does reach BTP,
	// verify both components of the annotated identity, not the default name.
	if transport.reads > 0 {
		if diff := cmp.Diff(adoptionTestSubaccount, transport.gotSubaccount); diff != "" {
			t.Errorf("Terraform request used wrong subaccount (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(adoptionTestName, transport.name); diff != "" {
			t.Errorf("Terraform request used wrong credential name (-want +got):\n%s", diff)
		}
	}
}
