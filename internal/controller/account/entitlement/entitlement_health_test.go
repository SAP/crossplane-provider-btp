package entitlement

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	managerfake "github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	entclient "github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
	entfake "github.com/sap/crossplane-provider-btp/internal/controller/account/entitlement/fake"
	api "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	trackingfake "github.com/sap/crossplane-provider-btp/internal/tracking/test"
)

// Exercise the managed reconciler's status persistence as well as Observe:
// quota retry paths replace Ready with Creating, but successful polling or
// updating must retain the external failure without turning Synced into an error.
func TestReconcileEntitlementProcessingFailed(t *testing.T) {
	cases := map[string]struct {
		amount       *int
		desired      int
		enable       bool
		autoAssigned bool
		wantDrift    bool
		wantCreate   bool
		wantUpdate   bool
	}{
		"historical failed reduction with correct quota":           {amount: internal.Ptr(2), desired: 2},
		"historical failed removal with correct environment quota": {amount: internal.Ptr(1), desired: 1},
		"rejected amount change retries update":                    {amount: internal.Ptr(2), desired: 1, wantDrift: true, wantUpdate: true},
		"enable drift with positive quota":                         {amount: internal.Ptr(2000000000), enable: true, wantDrift: true},
		"zero quota retries create":                                {amount: internal.Ptr(0), enable: true, wantCreate: true},
		"unset quota retries create":                               {enable: true, wantCreate: true},
		"auto-assigned zero quota is not recreated":                {amount: internal.Ptr(0), desired: 0, autoAssigned: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			if err := apis.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			cr := entitlement(withUID("entitlement"), withExternalName("subaccount-guid/service-name/service-plan-name"))
			cr.Generation = 2
			cr.Spec.ManagementPolicies = xpv1.ManagementPolicies{xpv1.ManagementActionAll}
			if tc.enable {
				cr.Spec.ForProvider.Enable = internal.Ptr(true)
			} else {
				cr.Spec.ForProvider.Amount = internal.Ptr(tc.desired)
			}
			cr.SetConditions(xpv1.Available(), xpv1.ReconcileSuccess())
			kube := kubefake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&v1alpha1.Entitlement{}).WithObjects(cr).Build()
			message := "Request to reduce quota to 0 rejected because an instance still consumes quota."
			assignment := &api.AssignedServicePlanSubaccountDTO{
				EntityState:  internal.Ptr("PROCESSING_FAILED"),
				StateMessage: internal.Ptr(message),
				AutoAssigned: internal.Ptr(tc.autoAssigned),
			}
			if tc.amount != nil {
				assignment.Amount = internal.Ptr(float32(*tc.amount))
			}
			creates, updates := 0, 0
			describe := func(context.Context, entclient.ExternalNameKey) (*entclient.Instance, error) {
				return &entclient.Instance{EntitledServicePlan: &api.ServicePlanResponseObject{}, Assignment: assignment}, nil
			}
			apiClient := entfake.MockClient{
				MockDescribeInstanceFn:      describe,
				MockDescribeInstanceFreshFn: describe,
				MockCreateInstanceFn: func(_ context.Context, _ entclient.ExternalNameKey, observed *v1alpha1.Entitlement) error {
					creates++
					if !reflect.DeepEqual(observed.Status.AtProvider.Assigned.Amount, tc.amount) {
						t.Error("Create did not receive the observed failed quota")
					}
					return nil
				},
				MockUpdateInstanceFn: func(context.Context, entclient.ExternalNameKey, *v1alpha1.Entitlement) error {
					updates++
					return nil
				},
				MockDeleteInstanceFn: func(context.Context, entclient.ExternalNameKey, *v1alpha1.Entitlement) error {
					t.Fatal("unexpected delete")
					return nil
				},
			}
			e := &external{kube: kube, client: apiClient, tracker: trackingfake.NoOpReferenceResolverTracker{}}
			r := managed.NewReconciler(&managerfake.Manager{Client: kube, Scheme: scheme},
				resource.ManagedKind(v1alpha1.EntitlementGroupVersionKind),
				managed.WithExternalConnector(managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
					return e, nil
				})), managed.WithInitializers(), managed.WithRecorder(event.NewNopRecorder()))
			key := types.NamespacedName{Name: cr.Name}
			reconcileAndGet := func() *v1alpha1.Entitlement {
				t.Helper()
				// The first reconcile may only persist the managed finalizer.
				for range 3 {
					if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key}); err != nil {
						t.Fatal(err)
					}
				}
				got := &v1alpha1.Entitlement{}
				if err := kube.Get(ctx, key, got); err != nil {
					t.Fatal(err)
				}
				return got
			}
			got := reconcileAndGet()
			ready, synced := got.GetCondition(xpv1.TypeReady), got.GetCondition(xpv1.TypeSynced)
			wantReason := xpv1.ConditionReason("ExternalResourceFailed")
			if tc.wantCreate {
				wantReason = xpv1.ReasonCreating
			} else if !strings.Contains(ready.Message, message) || ready.ObservedGeneration != cr.Generation {
				t.Errorf("Ready lost the observed platform failure: %+v", ready)
			}
			if ready.Status != xpv1.Unavailable().Status || ready.Reason != wantReason {
				t.Errorf("Ready = %+v, want False/%s", ready, wantReason)
			}
			if synced.Status != xpv1.ReconcileSuccess().Status || synced.Reason != xpv1.ReasonReconcileSuccess {
				t.Errorf("Synced = %+v, want successful reconcile", synced)
			}
			if !tc.wantCreate && !reflect.DeepEqual(got.Status.AtProvider.Assigned.Amount, tc.amount) {
				t.Errorf("reported quota changed: got %v, want %v", got.Status.AtProvider.Assigned.Amount, tc.amount)
			}
			if (creates > 0) != tc.wantCreate || (updates > 0) != tc.wantUpdate {
				t.Errorf("creates=%d updates=%d, want create=%t update=%t", creates, updates, tc.wantCreate, tc.wantUpdate)
			}
			if !tc.wantCreate {
				drift := got.GetCondition(v1alpha1.DriftConditionType)
				if (drift.Status == xpv1.Available().Status) != tc.wantDrift {
					t.Errorf("Drift = %+v, want detected=%t", drift, tc.wantDrift)
				}
			}

			// Recovery requires a new BTP observation, not clearing the condition
			// merely because the desired quota agrees with the failed assignment.
			assignment.EntityState = internal.Ptr("OK")
			assignment.StateMessage = nil
			assignment.Amount = internal.Ptr(float32(tc.desired))
			assignment.UnlimitedAmountAssigned = internal.Ptr(tc.enable)
			got = reconcileAndGet()
			ready = got.GetCondition(xpv1.TypeReady)
			if ready.Status != xpv1.Available().Status || ready.Reason != xpv1.ReasonAvailable || ready.Message != "" {
				t.Errorf("recovered Ready = %+v, want Available with no stale failure", ready)
			}
		})
	}
}

func TestEntitlementFailureConditionMessage(t *testing.T) {
	const prefix = "BTP reports the entitlement assignment as PROCESSING_FAILED"
	cases := map[string]string{
		"missing message": "",
		"platform detail": "The quota change was rejected.",
		"long ASCII":      strings.Repeat("a", 5000),
		"long UTF-8":      strings.Repeat("界", 2000),
	}
	for name, detail := range cases {
		t.Run(name, func(t *testing.T) {
			cr := entitlement(withAssignedStatus(internal.Ptr(2), "PROCESSING_FAILED"))
			cr.Generation = 2
			cr.Status.AtProvider.Assigned.StateMessage = detail
			got := entitlementFailedCondition(cr)
			if len(got.Message) > 4096 || !utf8.ValidString(got.Message) || !strings.HasPrefix(got.Message, prefix) {
				t.Fatalf("invalid failure message: bytes=%d validUTF8=%t", len(got.Message), utf8.ValidString(got.Message))
			}
			if len(detail) > 4096 {
				if !strings.HasSuffix(got.Message, "...") || len(got.Message) < 4093 {
					t.Errorf("long message was not truncated at the UTF-8 boundary: bytes=%d", len(got.Message))
				}
			} else {
				want := prefix
				if detail != "" {
					want += ": " + detail
				}
				if got.Message != want {
					t.Errorf("message = %q, want %q", got.Message, want)
				}
			}
		})
	}
}
