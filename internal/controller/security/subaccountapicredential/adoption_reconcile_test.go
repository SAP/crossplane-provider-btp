package subaccountapicredential

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	btpprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	resourcefake "github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ujcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/metrics"
	ujterraform "github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/sap/crossplane-provider-btp/apis"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerconfig "github.com/sap/crossplane-provider-btp/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	adoptionTestSubaccountID = "subaccount-test"
	adoptionTestCredential   = "credential-test"
	adoptionTestClientID     = "client-id-test"
	adoptionTestClientSecret = "client-secret-test"
	adoptionTestAPIURL       = "https://api.example.test"
	adoptionTestTokenURL     = "https://token.example.test"
)

type failOnceConnectionSecretPatchClient struct {
	client.Client
	secretName string
	failed     bool
}

func (c *failOnceConnectionSecretPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if secret, ok := obj.(*corev1.Secret); ok && secret.Name == c.secretName && !c.failed {
		c.failed = true
		return errors.New("synthetic connection Secret publication failure")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

// TestCredentialAdoptionReconcile exercises a Framework-created state followed by
// the real managed reconciler, resource-specific decorator, synthetic BTP transport,
// and connection Secret publisher. No live BTP calls are made. Secret material is
// only compared in memory and never logged.
func TestCredentialAdoptionReconcile(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		includeClientSecret  bool
		coldStart            bool
		newAdopter           bool
		emptyConnection      bool
		driftedSubaccount    bool
		missingDestination   bool
		noCreatePolicy       bool
		failPublicationOnce  bool
		managementPolicies   xpv2.ManagementPolicies
		policyFeatureEnabled bool
		wantReady            corev1.ConditionStatus
		wantSynced           corev1.ConditionStatus
	}{
		{name: "complete state retries after publication failure", includeClientSecret: true, failPublicationOnce: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
		{name: "complete state is accepted with Observe-only policy", includeClientSecret: true, managementPolicies: xpv2.ManagementPolicies{xpv2.ManagementActionObserve}, policyFeatureEnabled: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
		{name: "complete state is accepted with mutation-capable policy", includeClientSecret: true, managementPolicies: xpv2.ManagementPolicies{xpv2.ManagementActionAll}, policyFeatureEnabled: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
		{name: "incomplete state is rejected", includeClientSecret: false, wantReady: corev1.ConditionFalse, wantSynced: corev1.ConditionFalse},
		{name: "absent target is not created under Observe-only policy", managementPolicies: xpv2.ManagementPolicies{xpv2.ManagementActionObserve}, policyFeatureEnabled: true, noCreatePolicy: true, wantReady: corev1.ConditionUnknown, wantSynced: corev1.ConditionFalse},
		{name: "fresh Create proceeds without a destination Secret", includeClientSecret: true, missingDestination: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
		{name: "cold state is reconstructed from complete connection Secret", includeClientSecret: true, coldStart: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
		{name: "cold state without client secret is rejected", includeClientSecret: false, coldStart: true, wantReady: corev1.ConditionFalse, wantSynced: corev1.ConditionFalse},
		{name: "new Observe-only adopter with only client_secret", includeClientSecret: true, coldStart: true, newAdopter: true, managementPolicies: xpv2.ManagementPolicies{xpv2.ManagementActionObserve}, policyFeatureEnabled: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
		{name: "new mutation-capable adopter with only client_secret", includeClientSecret: true, coldStart: true, newAdopter: true, managementPolicies: xpv2.ManagementPolicies{xpv2.ManagementActionAll}, policyFeatureEnabled: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
		{name: "new Observe-only adopter with public fields only is rejected", coldStart: true, newAdopter: true, managementPolicies: xpv2.ManagementPolicies{xpv2.ManagementActionObserve}, policyFeatureEnabled: true, wantReady: corev1.ConditionFalse, wantSynced: corev1.ConditionFalse},
		{name: "new Observe-only adopter with empty destination is rejected", coldStart: true, newAdopter: true, emptyConnection: true, managementPolicies: xpv2.ManagementPolicies{xpv2.ManagementActionObserve}, policyFeatureEnabled: true, wantReady: corev1.ConditionFalse, wantSynced: corev1.ConditionFalse},
		{name: "established cold identity survives spec subaccount drift", includeClientSecret: true, coldStart: true, driftedSubaccount: true, wantReady: corev1.ConditionTrue, wantSynced: corev1.ConditionTrue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var creates, reads int
			externalExists := tc.coldStart
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Cpcli-Backend-Status", "200")
				if strings.HasPrefix(r.URL.Path, "/login/") {
					w.Header().Set("X-Cpcli-Sessionid", "synthetic-session")
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"mail":"test@example.test","issuer":"test-issuer"}`))
					return
				}
				if !strings.Contains(r.URL.Path, "/command/") {
					t.Errorf("unexpected synthetic BTP request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
					http.Error(w, "unexpected request", http.StatusInternalServerError)
					return
				}
				var request struct {
					ParamValues map[string]string `json:"paramValues"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode synthetic BTP request: %v", err)
				}
				if request.ParamValues["subaccount"] != adoptionTestSubaccountID || request.ParamValues["name"] != adoptionTestCredential {
					t.Errorf("request targeted subaccount/name %q/%q, want %q/%q", request.ParamValues["subaccount"], request.ParamValues["name"], adoptionTestSubaccountID, adoptionTestCredential)
				}
				// Check the target even on the absence probe before fresh Create.
				switch r.URL.RawQuery {
				case "create":
					creates++
					externalExists = true
				case "get":
					reads++
					// Do not let a permissive synthetic server hide a malformed Read.
					if !externalExists || request.ParamValues["subaccount"] == "" {
						w.Header().Set("X-Cpcli-Backend-Status", "404")
						return
					}
				default:
					t.Errorf("unexpected synthetic BTP action %q", r.URL.RawQuery)
					http.Error(w, "unexpected action", http.StatusInternalServerError)
					return
				}
				response := map[string]any{
					"tenantmode": "dedicated", "subaccountid": adoptionTestSubaccountID,
					"credential-type": "SECRET", "clientid": adoptionTestClientID,
					"tokenurl": adoptionTestTokenURL, "apiurl": adoptionTestAPIURL,
					"name": adoptionTestCredential,
				}
				if r.URL.RawQuery == "create" && tc.includeClientSecret {
					response["clientsecret"] = adoptionTestClientSecret
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			scheme := runtime.NewScheme()
			if err := apis.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}

			connectionSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "credential-connection", Namespace: "crossplane-system"},
				Type:       resource.SecretTypeConnection,
				Data:       map[string][]byte{"unrelated": []byte("preserved")},
			}
			if tc.coldStart {
				connectionSecret.Data = map[string][]byte{
					"unrelated":           []byte("preserved"),
					"attribute.client_id": []byte(adoptionTestClientID),
					"attribute.api_url":   []byte(adoptionTestAPIURL),
					"attribute.token_url": []byte(adoptionTestTokenURL),
				}
				if tc.includeClientSecret {
					connectionSecret.Data["attribute.client_secret"] = []byte(adoptionTestClientSecret)
				}
			}
			cr := &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{Name: "credential-mr", UID: "adoption-test-uid", Generation: 1},
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
						ProviderConfigReference: &xpv2.Reference{Name: "synthetic"},
						WriteConnectionSecretToReference: &xpv2.SecretReference{
							Name: connectionSecret.Name, Namespace: connectionSecret.Namespace,
						},
					},
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
						SubaccountID: strptr(adoptionTestSubaccountID), Name: strptr(adoptionTestCredential),
					},
				},
			}
			if tc.noCreatePolicy {
				cr.Spec.ManagementPolicies = tc.managementPolicies
			}
			if tc.coldStart && !tc.newAdopter {
				cr.Status.AtProvider = securityv1alpha1.SubaccountApiCredentialObservation{
					Name:           strptr(adoptionTestCredential),
					SubaccountID:   strptr(adoptionTestSubaccountID),
					CredentialType: strptr("SECRET"),
				}
			}
			if tc.coldStart {
				meta.SetExternalName(cr, adoptionTestSubaccountID+"/"+adoptionTestCredential)
			}
			if tc.newAdopter && (tc.includeClientSecret || tc.emptyConnection) {
				connectionSecret.Data = map[string][]byte{"unrelated": []byte("preserved")}
				if tc.includeClientSecret {
					connectionSecret.Data["attribute.client_secret"] = []byte(adoptionTestClientSecret)
				}
			}
			if tc.driftedSubaccount {
				cr.Spec.ForProvider.SubaccountID = strptr("drifted-subaccount")
			}
			objects := []client.Object{cr}
			if !tc.missingDestination {
				objects = append(objects, connectionSecret)
			}
			kube := kubefake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&securityv1alpha1.SubaccountApiCredential{}).
				WithObjects(objects...).Build()

			resourceConfig := providerconfig.GetProvider().Resources["btp_subaccount_api_credential"]
			frameworkProvider := btpprovider.NewWithClient(server.Client())
			setup := func(context.Context, client.Client, resource.Managed) (ujterraform.Setup, error) {
				return ujterraform.Setup{
					FrameworkProvider: frameworkProvider,
					Configuration: map[string]any{
						"username": "synthetic-user", "password": "synthetic-password",
						"globalaccount": "synthetic-account", "cli_server_url": server.URL,
					},
				}, nil
			}
			store := ujcontroller.NewOperationStore(logging.NewNopLogger())
			connectorOptions := []ujcontroller.TerraformPluginFrameworkConnectorOption{
				ujcontroller.WithTerraformPluginFrameworkMetricRecorder(&metrics.MetricRecorder{}),
				ujcontroller.WithTerraformPluginFrameworkLogger(logging.NewNopLogger()),
			}
			if tc.noCreatePolicy {
				connectorOptions = append(connectorOptions, ujcontroller.WithTerraformPluginFrameworkManagementPolicies(true))
			}
			connector := ujcontroller.NewTerraformPluginFrameworkConnector(kube, setup, resourceConfig, store, connectorOptions...)

			initializers := make(managed.InitializerChain, 0, len(resourceConfig.InitializerFns))
			for _, makeInitializer := range resourceConfig.InitializerFns {
				initializers = append(initializers, makeInitializer(kube))
			}
			newReconciler := func(connector managed.ExternalConnector, managementPoliciesEnabled bool, managerClient client.Client) *managed.Reconciler {
				opts := []managed.ReconcilerOption{
					managed.WithExternalConnector(connector),
					managed.WithInitializers(initializers...),
					managed.WithRecorder(event.NewNopRecorder()),
					managed.WithPollInterval(time.Hour),
				}
				if managementPoliciesEnabled {
					opts = append(opts, managed.WithManagementPolicies())
				}
				return managed.NewReconciler(
					&resourcefake.Manager{Client: managerClient, Scheme: scheme},
					resource.ManagedKind(securityv1alpha1.SubaccountApiCredential_GroupVersionKind), opts...,
				)
			}

			key := types.NamespacedName{Name: cr.Name}
			// Warm cases genuinely verify absence, Create the synthetic BTP
			// credential, and persist complete Framework state in the tracker. The
			// cold cases instead start from the fixture's status and destination;
			// new adopters deliberately have no observed status at all.
			if !tc.coldStart {
				if _, err := newReconciler(newCredentialAdoptionConnector(connector, kube), tc.noCreatePolicy, kube).Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
					t.Fatalf("initial managed reconcile failed: %v", err)
				}
				if tc.noCreatePolicy {
					if creates != 0 || reads == 0 {
						t.Fatalf("synthetic BTP calls under Observe-only policy: Create=%d Read=%d; want no Create and at least one Read", creates, reads)
					}
				} else if creates != 1 || reads == 0 {
					freshState := &securityv1alpha1.SubaccountApiCredential{}
					_ = kube.Get(context.Background(), key, freshState)
					t.Fatalf("synthetic BTP calls after fresh reconcile: Create=%d Read=%d; want one Create and a confirming Read; conditions=%#v", creates, reads, freshState.Status.Conditions)
				}
			}

			// Exercise normal framework policy semantics on an existing target.
			if tc.policyFeatureEnabled && !tc.noCreatePolicy {
				updated := &securityv1alpha1.SubaccountApiCredential{}
				if err := kube.Get(context.Background(), key, updated); err != nil {
					t.Fatal(err)
				}
				updated.Spec.ManagementPolicies = tc.managementPolicies
				if err := kube.Update(context.Background(), updated); err != nil {
					t.Fatalf("set test management policy: %v", err)
				}
			}

			// Simulate an incomplete destination Secret while the Framework state is
			// still warm, as after a transient publication problem.
			if tc.includeClientSecret && !tc.coldStart {
				partial := &corev1.Secret{}
				if err := kube.Get(context.Background(), client.ObjectKeyFromObject(connectionSecret), partial); err != nil {
					t.Fatal(err)
				}
				partial.Data = map[string][]byte{"unrelated": []byte("preserved")}
				if err := kube.Update(context.Background(), partial); err != nil {
					t.Fatalf("make destination Secret partial: %v", err)
				}
			}

			// The next reconciliation adopts/observes the existing synthetic target
			// through the real completeness decorator and normal Secret publisher.
			adoptionConnector := connector
			if tc.policyFeatureEnabled {
				adoptionConnector = ujcontroller.NewTerraformPluginFrameworkConnector(
					kube, setup, resourceConfig, store,
					append(connectorOptions, ujcontroller.WithTerraformPluginFrameworkManagementPolicies(true))...,
				)
			}
			publicationClient := client.Client(kube)
			var publicationFailure *failOnceConnectionSecretPatchClient
			if tc.failPublicationOnce {
				publicationFailure = &failOnceConnectionSecretPatchClient{Client: kube, secretName: connectionSecret.Name}
				publicationClient = publicationFailure
			}
			adoptionReconciler := newReconciler(newCredentialAdoptionConnector(adoptionConnector, publicationClient), tc.policyFeatureEnabled, publicationClient)
			if _, err := adoptionReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
				t.Fatalf("adoption reconciliation failed: %v", err)
			}
			if tc.coldStart && tc.includeClientSecret {
				if _, err := adoptionReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
					t.Fatalf("second cold-state reconcile failed: %v", err)
				}
			}
			if tc.failPublicationOnce {
				if !publicationFailure.failed {
					t.Fatal("synthetic connection Secret publication failure was not injected")
				}
				failedAttempt := &securityv1alpha1.SubaccountApiCredential{}
				if err := kube.Get(context.Background(), key, failedAttempt); err != nil {
					t.Fatal(err)
				}
				if got := failedAttempt.GetCondition(xpv2.TypeSynced).Status; got != corev1.ConditionFalse {
					t.Fatalf("publication failure Synced status = %q, want False", got)
				}
				if message := failedAttempt.GetCondition(xpv2.TypeSynced).Message; !strings.Contains(message, "synthetic connection Secret publication failure") {
					t.Fatalf("publication failure diagnostic %q does not contain injected failure", message)
				}
				if _, err := adoptionReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
					t.Fatalf("publication retry reconciliation failed: %v", err)
				}
			}
			wantCreates := 1
			wantReads := 2
			if tc.coldStart || tc.noCreatePolicy {
				wantCreates = 0
				wantReads = 1
				if tc.noCreatePolicy {
					wantReads = 2
				}
				if tc.includeClientSecret {
					wantReads = 2
				}
			}
			if reads < wantReads || creates != wantCreates {
				failedState := &securityv1alpha1.SubaccountApiCredential{}
				_ = kube.Get(context.Background(), key, failedState)
				t.Fatalf("synthetic BTP calls after adoption reconcile: Create=%d Read=%d; want Create=%d and at least %d Reads; conditions=%#v", creates, reads, wantCreates, wantReads, failedState.Status.Conditions)
			}

			got := &securityv1alpha1.SubaccountApiCredential{}
			if err := kube.Get(context.Background(), key, got); err != nil {
				t.Fatal(err)
			}
			if tc.driftedSubaccount && value(got.Spec.ForProvider.SubaccountID) != "drifted-subaccount" {
				t.Error("identity pin changed the persisted user spec")
			}
			ready := got.GetCondition(xpv2.TypeReady)
			synced := got.GetCondition(xpv2.TypeSynced)
			if ready.Status != tc.wantReady {
				t.Errorf("Ready status = %q, want %q (reason %q, message %q; conditions=%#v)", ready.Status, tc.wantReady, ready.Reason, ready.Message, got.Status.Conditions)
			}
			if synced.Status != tc.wantSynced {
				t.Errorf("Synced status = %q, want %q (reason %q, message %q)", synced.Status, tc.wantSynced, synced.Reason, synced.Message)
			}
			if tc.includeClientSecret {
				if gotName := meta.GetExternalName(got); gotName != adoptionTestSubaccountID+"/"+adoptionTestCredential {
					t.Errorf("external identity = %q, want %q", gotName, adoptionTestSubaccountID+"/"+adoptionTestCredential)
				}
			}
			if !tc.includeClientSecret && !tc.noCreatePolicy && !strings.Contains(synced.Message, errCredentialAdoptionIncomplete) {
				t.Errorf("incomplete adoption diagnostic missing from Synced message: %q", synced.Message)
			}

			published := &corev1.Secret{}
			if err := kube.Get(context.Background(), client.ObjectKeyFromObject(connectionSecret), published); err != nil {
				t.Fatal(err)
			}
			if tc.includeClientSecret {
				for key, want := range map[string][]byte{
					"attribute.client_id":     []byte(adoptionTestClientID),
					"attribute.client_secret": []byte(adoptionTestClientSecret),
					"attribute.api_url":       []byte(adoptionTestAPIURL),
					"attribute.token_url":     []byte(adoptionTestTokenURL),
				} {
					if string(published.Data[key]) != string(want) {
						t.Errorf("published Secret key %q did not match expected in-memory value", key)
					}
				}
				if !tc.missingDestination && string(published.Data["unrelated"]) != "preserved" {
					t.Error("publication did not preserve unrelated destination Secret data")
				}
			} else if got := published.Data["attribute.client_secret"]; len(got) != 0 {
				t.Error("incomplete state unexpectedly published a client secret")
			}
		})
	}
}
