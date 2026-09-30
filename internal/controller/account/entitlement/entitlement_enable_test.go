package entitlement

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	managedfake "github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	entitlementclient "github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/entitlement/fake"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	trackingtest "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Exercise the managed reconciler's choice to call Update, then observe the
// applied aggregate to ensure the update stops once BTP converges.
func TestReconcileEnableDrift(t *testing.T) {
	cases := map[string]struct {
		enable       bool
		assigned     bool
		autoAssign   bool
		autoAssigned bool
		wantUpdates  int
	}{
		"EnableFailedAssignment":     {enable: true, assigned: false, wantUpdates: 1},
		"DisableUnlimitedAssignment": {enable: false, assigned: true, wantUpdates: 1},
		"AlreadyEnabled":             {enable: true, assigned: true},
		"AlreadyDisabled":            {enable: false, assigned: false},
		"AutoAssign":                 {enable: true, assigned: false, autoAssign: true},
		"AutoAssigned":               {enable: true, assigned: false, autoAssigned: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := apis.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			cr := entitlement(withName("cis-enable"), withUID("cis-enable"), withEnabled(tc.enable), withExternalName("subaccount-guid/service-name/service-plan-name"))
			cr.Spec.ManagementPolicies = xpv1.ManagementPolicies{xpv1.ManagementActionAll}
			kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.Entitlement{}).WithObjects(cr).Build()
			assigned := tc.assigned
			state := "PROCESSING_FAILED"
			updates := 0
			client := fake.MockClient{
				MockDescribeInstanceFn: func(context.Context, entitlementclient.ExternalNameKey) (*entitlementclient.Instance, error) {
					return &entitlementclient.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(2000000000)), EntityState: &state,
							UnlimitedAmountAssigned: &assigned, AutoAssign: &tc.autoAssign, AutoAssigned: &tc.autoAssigned,
						},
					}, nil
				},
				MockUpdateInstanceFn: func(_ context.Context, key entitlementclient.ExternalNameKey, got *v1alpha1.Entitlement) error {
					updates++
					if key.String() != meta.GetExternalName(cr) || got.Status.AtProvider.Required.Enable == nil || *got.Status.AtProvider.Required.Enable != tc.enable || got.Status.AtProvider.Required.Amount != nil {
						t.Fatalf("update must use the aggregate enable and adopted identity: key=%s, aggregate=%#v", key.String(), got.Status.AtProvider.Required)
					}
					assigned = *got.Status.AtProvider.Required.Enable
					state = "OK"
					return nil
				},
				MockCreateInstanceFn: func(context.Context, entitlementclient.ExternalNameKey, *v1alpha1.Entitlement) error {
					t.Fatal("existing positive-quota assignment must not be recreated")
					return nil
				},
			}
			e := &external{client: client, kube: kube, tracker: trackingtest.NoOpReferenceResolverTracker{}}
			r := managed.NewReconciler(&managedfake.Manager{Client: kube, Scheme: scheme}, resource.ManagedKind(v1alpha1.EntitlementGroupVersionKind),
				managed.WithExternalConnector(managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) { return e, nil })),
				managed.WithInitializers(), managed.WithRecorder(event.NewNopRecorder()))
			key := types.NamespacedName{Name: cr.Name}
			for range 3 {
				if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
					t.Fatal(err)
				}
			}
			if updates != tc.wantUpdates {
				t.Errorf("want %d update calls, got %d", tc.wantUpdates, updates)
			}
			got := &v1alpha1.Entitlement{}
			if err := kube.Get(context.Background(), key, got); err != nil {
				t.Fatal(err)
			}
			if tc.wantUpdates == 1 {
				assertDriftCondition(t, got, "", false)
				if got.Status.AtProvider.Assigned.UnlimitedAmountAssigned != tc.enable {
					t.Errorf("assignment did not converge: %#v", got.Status.AtProvider.Assigned)
				}
			}
		})
	}
}

func TestNeedsUpdateUsesAggregateShape(t *testing.T) {
	cases := map[string]struct {
		specAmount *int
		required   *v1alpha1.EntitlementSummary
		assigned   *v1alpha1.Assignable
		want       bool
	}{
		"SiblingEnable":    {required: &v1alpha1.EntitlementSummary{Enable: internal.Ptr(true)}, assigned: &v1alpha1.Assignable{}, want: true},
		"NumericMismatch":  {specAmount: internal.Ptr(3), required: &v1alpha1.EntitlementSummary{Amount: internal.Ptr(5)}, assigned: &v1alpha1.Assignable{Amount: internal.Ptr(3)}, want: true},
		"NumericMatch":     {specAmount: internal.Ptr(3), required: &v1alpha1.EntitlementSummary{Amount: internal.Ptr(5)}, assigned: &v1alpha1.Assignable{Amount: internal.Ptr(5)}},
		"NumericUnlimited": {specAmount: internal.Ptr(3), required: &v1alpha1.EntitlementSummary{Amount: internal.Ptr(5)}, assigned: &v1alpha1.Assignable{Amount: internal.Ptr(3), UnlimitedAmountAssigned: true}},
		"NoContribution":   {required: &v1alpha1.EntitlementSummary{}, assigned: &v1alpha1.Assignable{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := entitlement(withRequiredAssigned(tc.required, tc.assigned))
			cr.Spec.ForProvider.Amount = tc.specAmount
			if got := (&external{}).needsUpdate(cr); got != tc.want {
				t.Errorf("want needsUpdate=%t, got %t", tc.want, got)
			}
		})
	}
}
