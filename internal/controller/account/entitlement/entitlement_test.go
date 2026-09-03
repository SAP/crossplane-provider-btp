package entitlement

import (
	"context"
	"fmt"
	"strings"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	entitlement2 "github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/entitlement/fake"
	"github.com/sap/crossplane-provider-btp/internal/mrstatus"
	test2 "github.com/sap/crossplane-provider-btp/internal/tracking/test"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

const (
	errKubeAPI   = "kube api error"
	errClientAPI = "could not connect to api"
)

var noopStatusUpdate = test.NewMockSubResourceUpdateFn(nil, func(obj client.Object) error {
	return nil
})

func TestObserve(t *testing.T) {
	type args struct {
		cr     *v1alpha1.Entitlement
		client entitlement2.Client
		kube   client.Client
	}

	type want struct {
		o         managed.ExternalObservation
		comparefn func(*v1alpha1.Entitlement) string
		err       error
	}

	var cases = map[string]struct {
		args args
		want want
	}{
		"Error Describing, client returns error": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
				},
				client: fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						return nil, errors.New(errClientAPI)
					}},
				cr: entitlement(),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.Wrap(errors.Wrap(errors.New(errClientAPI), "while describing instance"), "while updating observation"),
			},
		},
		"Error Describing, kube returns error": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate, MockList: test.NewMockListFn(errors.New(errKubeAPI)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: nil,
						Assignment:          nil,
					}, nil
				}},
				cr: entitlement(withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.Wrap(errors.Wrap(errors.Wrap(errors.New(errKubeAPI), "while listing entitlements"), "while finding related entitlements"), "while updating observation"),
			},
		},
		// desired=<unset> here is not a fixture artifact: neither this CR
		// nor sibling "b" sets amount/enable, so Required is a genuinely
		// zero-contribution aggregate reachable in production (see the
		// longer explanation on the "External name valid compound key..."
		// case in TestObserveExternalName below).
		"Simple Case, unique identifier passed": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withServiceName("hana-cloud"), withUniqueServicePlanIdentifier("a")), entitlement(withServiceName("hana-cloud"), withUniqueServicePlanIdentifier("b")))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withServiceName("hana-cloud"), withUniqueServicePlanIdentifier("a"), withExternalName("subaccount-guid/hana-cloud/service-plan-name/a")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=<unset>, observed=1)"},
				err: nil,
			},
		},
		"Simple Case, no additional additional Entitlements, resource does not exist": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate, MockList: test.NewMockListFn(nil, ListEntitlements(entitlement())),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment:          nil,
					}, nil
				}},
				cr: entitlement(withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"Simple Case, resource needs update, amount differs": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate, MockList: test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(2)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withAmount(2), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, Diff: "amount mismatch (desired=2, observed=1)"},
				err: nil,
			},
		},

		"Simple Case, All up-to-date": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate, MockList: test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				comparefn: func(v *v1alpha1.Entitlement) string {
					return cmp.Diff(v.Status.GetCondition(xpv1.Available().Type).Status, xpv1.Available().Status)
				},
				err: nil,
			},
		},
		"Simple Case, All up-to-date, creating condition": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("STARTED"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				comparefn: func(v *v1alpha1.Entitlement) string {
					return cmp.Diff(v.Status.GetCondition(xpv1.Creating().Type).Status, xpv1.Creating().Status)
				},
				err: nil,
			},
		},
		"Simple Case, All up-to-date, processing condition": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("PROCESSING"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				comparefn: func(v *v1alpha1.Entitlement) string {
					return cmp.Diff(v.Status.GetCondition(xpv1.Creating().Type).Status, xpv1.Creating().Status)
				},
				err: nil,
			},
		},
		// upstream issue #280: PROCESSING_FAILED never reports Available, even
		// with an amount still assigned. This asserted Available before.
		"PROCESSING_FAILED with assigned amount -- must not report Available": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("PROCESSING_FAILED"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				comparefn: func(v *v1alpha1.Entitlement) string {
					got := v.Status.GetCondition(xpv1.TypeReady)
					if got.Status != corev1.ConditionFalse {
						return fmt.Sprintf("expected Ready=False for PROCESSING_FAILED, got %s", got.Status)
					}
					if got.Reason != mrstatus.ReasonExternalResourceFailed {
						return fmt.Sprintf("expected reason %q, got %q", mrstatus.ReasonExternalResourceFailed, got.Reason)
					}
					if !strings.Contains(got.Message, "PROCESSING_FAILED") {
						return fmt.Sprintf("expected the condition message to name the platform state, got %q", got.Message)
					}
					return ""
				},
				err: nil,
			},
		},
		// The BTP rejection reason must be readable on the resource; it used to
		// be dropped entirely.
		"PROCESSING_FAILED stateMessage lands on the condition": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:       internal.Ptr(float32(1)),
							EntityState:  internal.Ptr("PROCESSING_FAILED"),
							StateMessage: internal.Ptr("requested quota amount change [0] is lower than the currently consumed quota [2]"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				comparefn: func(v *v1alpha1.Entitlement) string {
					got := v.Status.GetCondition(xpv1.TypeReady)
					if !strings.Contains(got.Message, "requested quota amount change [0] is lower than the currently consumed quota [2]") {
						return fmt.Sprintf("expected the BTP stateMessage on the condition, got %q", got.Message)
					}
					return ""
				},
				err: nil,
			},
		},
		"Assign-time PROCESSING_FAILED with amount=0 (enable-style) -- retries via Create": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withEnabled(true)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:       internal.Ptr(float32(0)),
							EntityState:  internal.Ptr("PROCESSING_FAILED"),
							StateMessage: internal.Ptr("Failed to call Provisioning service to assign quota."),
						},
					}, nil
				}},
				cr: entitlement(withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				// Observe returns ResourceExists=false so the managed reconciler
				// drives Create -> CreateInstance, which re-issues the assign.
				o:   managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"Assign-time PROCESSING_FAILED with nil amount (enable-style) -- retries via Create": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withEnabled(true)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:       nil,
							EntityState:  internal.Ptr("PROCESSING_FAILED"),
							StateMessage: internal.Ptr("Failed to call Provisioning service to assign quota."),
						},
					}, nil
				}},
				cr: entitlement(withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		// Scope note: this pins what Observe itself leaves on the resource. On
		// the amount==0 path Observe reports the external resource as absent,
		// so the managed reconciler goes on to mark Creating() - which replaces
		// Ready - before it persists the status. What an operator reads off
		// this resource while the assignment is being retried is therefore
		// Creating, not ExternalResourceFailed; the rejection resurfaces on the
		// resource once the failed assignment reserves a non-zero amount (the
		// "PROCESSING_FAILED stateMessage lands on the condition" case above,
		// which does persist because Observe reports the resource as existing).
		"Assign-time PROCESSING_FAILED with amount=0 -- Observe must not leave Available behind": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withEnabled(true)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(0)),
							EntityState: internal.Ptr("PROCESSING_FAILED"),
						},
					}, nil
				}},
				cr: entitlement(withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
				comparefn: func(v *v1alpha1.Entitlement) string {
					got := v.Status.GetCondition(xpv1.TypeReady)
					if got.Status != corev1.ConditionFalse {
						return "Ready=True/Available should not be set when BTP reports PROCESSING_FAILED with amount=0"
					}
					if got.Reason != mrstatus.ReasonExternalResourceFailed {
						return fmt.Sprintf("expected reason %q, got %q", mrstatus.ReasonExternalResourceFailed, got.Reason)
					}
					if got.Message == "" {
						return "expected a non-empty condition message naming the platform state"
					}
					return ""
				},
				err: nil,
			},
		},
		"Needs Deletion, assignment gone, noop needed": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1), withConditions(xpv1.Deleting())))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment:          nil,
					}, nil
				}},
				cr: entitlement(withAmount(1), withConditions(xpv1.Deleting()), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		// Fixture artifact, not drift evidence: this CR has no
		// DeletionTimestamp so Required stays the zero-sibling summary
		// (Amount/Enable both nil) even though spec still carries an
		// Amount. desired=<unset> below says nothing about
		// numeric-vs-enable branch selection.
		"Needs Deletion, assignment active, needs update": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1), withConditions(xpv1.Deleting())))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1), withConditions(xpv1.Deleting()), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, Diff: "amount mismatch (desired=<unset>, observed=1)"},
				err: nil,
			},
		},
		"Deletion with siblings, numeric quota, BTP already reduced": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR being deleted — filtered out by UID and Deleting condition
						entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withConditions(xpv1.Deleting())),
						// Sibling CR — remains active
						entitlement(withName("sibling-cr"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							// BTP amount already reduced to sibling sum
							Amount: internal.Ptr(float32(3)),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("a/Alpha/One")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"Deletion with siblings, numeric quota, BTP not yet reduced": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR being deleted — filtered out
						entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withConditions(xpv1.Deleting())),
						// Sibling CR
						entitlement(withName("sibling-cr"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							// BTP still has the full amount (not yet reduced)
							Amount: internal.Ptr(float32(5)),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("a/Alpha/One")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
			},
		},
		"Deletion with no siblings, sole CR, Delete() handles removal": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// Only the CR being deleted — filtered out, no siblings remain
						entitlement(withName("sole-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withConditions(xpv1.Deleting())),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(2)),
						},
					}, nil
				}},
				cr: entitlement(withName("sole-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("a/Alpha/One")),
			},
			want: want{
				// No siblings → deletionComplete returns false → let Delete() fully remove it
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
			},
		},
		"Deletion with siblings, enable-based, deletion complete": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR being deleted — filtered out
						entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(true), withSubaccountGuid("a"), withConditions(xpv1.Deleting())),
						// Sibling CR continues managing the entitlement
						entitlement(withName("sibling-cr"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(true), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(true), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("a/Alpha/One")),
			},
			want: want{
				// Enable-based with siblings → deletionComplete returns true
				o:   managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"Deletion with siblings, findRelatedEntitlements error propagated": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					// First List call (updateObservation) succeeds, second (deletionComplete) also uses this mock.
					// We use a stateful mock that fails on the second call.
					MockList: func() test.MockListFn {
						callCount := 0
						return func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
							callCount++
							if callCount <= 1 {
								// First call in updateObservation — return empty list
								l := obj.(*v1alpha1.EntitlementList)
								l.Items = []v1alpha1.Entitlement{}
								return nil
							}
							// Second call in deletionComplete — return error
							return errors.New(errKubeAPI)
						}
					}(),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(2)),
						},
					}, nil
				}},
				cr: entitlement(withName("err-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("a/Alpha/One")),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.Wrap(errors.Wrap(errors.New(errKubeAPI), errListEntitlements), errFindRelated),
			},
		},
		"Sibling being deleted, active CR computes reduced required amount and triggers update": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR-A: active, being observed — amount=2
						entitlement(withName("cr-a"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
						// CR-B: sibling being deleted — excluded from required sum by Deleting condition
						entitlement(withName("cr-b"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a"), withConditions(xpv1.Deleting())),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							// BTP still has the old combined amount (2+3=5)
							Amount: internal.Ptr(float32(5)),
						},
					}, nil
				}},
				// CR-A: the active CR being observed (no DeletionTimestamp, no Deleting condition)
				cr: entitlement(withName("cr-a"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withExternalName("a/Alpha/One")),
			},
			want: want{
				// Required.Amount should be 2 (only CR-A), not 5 (old combined), triggering an update
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, Diff: "amount mismatch (desired=2, observed=5)"},
				err: nil,
				comparefn: func(cr *v1alpha1.Entitlement) string {
					return cmp.Diff(cr.Status.AtProvider.Required.Amount, internal.Ptr(2))
				},
			},
		},
		"Multiple Entitlements for multiple plans, amount needs update": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// Initial Entitlement with Amount of 1
						entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withAmount(1), withSubaccountGuid("a")),
						// Filter out Different Service Plan
						entitlement(withName("b"), withServiceName("Alpha"), withServicePlan("Two"), withAmount(1), withSubaccountGuid("a")),
						// Add another entitlement with Amount of 2, Expected amount is 3 by now
						entitlement(withName("c"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
						// Filter out different Service Name
						entitlement(withName("d"), withServiceName("Beta"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a")),
						// Filter out objects in deletion
						entitlement(withName("e"), withServiceName("Alpha"), withServicePlan("One"), withAmount(1), withSubaccountGuid("a"), withConditions(xpv1.Deleting())),
						// Filter out for other subaccounts
						entitlement(withName("f"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("b")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withAmount(1), withSubaccountGuid("a"), withExternalName("a/Alpha/One")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, Diff: "amount mismatch (desired=3, observed=1)"},
				err: nil,
				comparefn: func(cr *v1alpha1.Entitlement) string {
					return cmp.Diff(cr.Status.AtProvider.Required.Amount, internal.Ptr(3))
				},
			},
		},
		"Multiple Entitlements for with negative amounts plans, error returned": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// Initial Entitlement with Amount of 1
						entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withAmount(1), withSubaccountGuid("a")),
						// Add another entitlement with Amount of 2, Expected amount is 3 by now
						entitlement(withName("b"), withServiceName("Alpha"), withServicePlan("One"), withAmount(-2), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withAmount(1), withSubaccountGuid("a"), withExternalName("a/Alpha/One")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false, ResourceUpToDate: false},
				err: errors.Wrap(errors.Wrap(errors.New("negative integer not allowed for .Spec.ForProvider.Amount"), "while generating observation"), "while updating observation"),
			},
		},

		"Multiple Entitlements for different Enabled values, error returned": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(true), withSubaccountGuid("a")),
						entitlement(withName("b"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(false), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(true), withSubaccountGuid("a"), withExternalName("a/Alpha/One")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false, ResourceUpToDate: false},
				err: errors.Wrap(errors.Wrap(errors.New("multiple of kind Entitlement have colliding .Spec.ForProvider.Enable"), "while generating observation"), "while updating observation"),
			},
		},

		"Amount differs, but its auto-assigned, All up-to-date": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(2)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:     internal.Ptr(float32(1)),
							AutoAssign: internal.Ptr(true),
						},
					}, nil
				}},
				cr: entitlement(withAmount(2), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=2, observed=1)"},
				err: nil,
			},
		},
		"Amount differs, but its unlimited assigned, All up-to-date": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(2)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:                  internal.Ptr(float32(1)),
							UnlimitedAmountAssigned: internal.Ptr(true),
						},
					}, nil
				}},
				cr: entitlement(withAmount(2), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=2, observed=1)"},
				err: nil,
			},
		},
		"Amount differs, but its system-assigned (AutoAssigned), All up-to-date": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(2)))),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:       internal.Ptr(float32(1)),
							AutoAssigned: internal.Ptr(true),
						},
					}, nil
				}},
				cr: entitlement(withAmount(2), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				// AutoAssigned (system-assigned, AutoAssign=false here) must suppress
				// needsUpdate exactly like AutoAssign/UnlimitedAmountAssigned do, even
				// though the desired amount (2) mismatches the assigned amount (1).
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=2, observed=1)"},
				err: nil,
			},
		},
		"Qualifier-distinct siblings, different qualifiers do not cross-sum": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR under test — qualifier "region-a"
						entitlement(withName("cr-region-a"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withUniqueServicePlanIdentifier("region-a"), withAmount(2), withSubaccountGuid("a")),
						// Same subaccount/service/plan but a different qualifier — must not contribute to our sum
						entitlement(withName("cr-region-b"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withUniqueServicePlanIdentifier("region-b"), withAmount(5), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(2)),
						},
					}, nil
				}},
				cr: entitlement(withName("cr-region-a"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withUniqueServicePlanIdentifier("region-a"), withAmount(2), withSubaccountGuid("a"), withExternalName("a/Alpha/One/region-a")),
			},
			want: want{
				// Required.Amount must be 2 (only the region-a CR), not 7 (2+5
				// cross-summed with the region-b sibling) — proves the qualifier
				// distinguishes otherwise-identical siblings.
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
				comparefn: func(cr *v1alpha1.Entitlement) string {
					return cmp.Diff(internal.Ptr(2), cr.Status.AtProvider.Required.Amount)
				},
			},
		},
		"Qualifier-distinct siblings, both unqualified still sum": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR under test — no qualifier
						entitlement(withName("cr-unqualified-a"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
						// Sibling — also no qualifier, same subaccount/service/plan — must still sum
						entitlement(withName("cr-unqualified-b"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(5)),
						},
					}, nil
				}},
				cr: entitlement(withName("cr-unqualified-a"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withExternalName("a/Alpha/One")),
			},
			want: want{
				// Required.Amount must be 5 (2+3) — a nil qualifier on both sides
				// still matches, so ordinary (non-qualified) siblings keep summing.
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
				comparefn: func(cr *v1alpha1.Entitlement) string {
					return cmp.Diff(internal.Ptr(5), cr.Status.AtProvider.Required.Amount)
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				e := external{client: tc.args.client, kube: tc.args.kube, tracker: test2.NoOpReferenceResolverTracker{}}
				got, err := e.Observe(context.Background(), tc.args.cr)
				if diff := compareErrorMessages(err, tc.want.err); diff != "" {
					t.Errorf("\ne.Observe(...): -want error %s, +got error:\n%s\n", tc.want.err, err)
				}
				/*if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions(), cmpopts.IgnoreTypes(v1alpha1.KymaEnvironmentObservation{})); diff != "" {
					t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
				}*/
				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
				}
				if tc.want.comparefn != nil {
					if diff := tc.want.comparefn(tc.args.cr); diff != "" {
						t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
					}
				}
			},
		)
	}
}

// TestObserveExternalName exercises keyForObserve's three-branch identity
// resolution: an empty or legacy annotation builds identity from spec,
// a legacy sentinel migrates to the compound key via one kube.Update,
// and any other annotation must parse as a compound key agreeing with
// spec or Observe errors instead.
func TestObserveExternalName(t *testing.T) {
	type want struct {
		o managed.ExternalObservation
		// err is the wanted error, unless isMismatch is set.
		err error
		// isMismatch computes the wanted error from cr's own annotation and
		// spec via ExternalNameKey.Mismatch instead of a fixed err, so the
		// mismatch description text is never duplicated by hand.
		isMismatch          bool
		describeCalled      bool
		updateCalls         int
		updatedExternalName string
		// wantFinalExternalName is the crossplane.io/external-name
		// annotation on cr after Observe returns.
		wantFinalExternalName string
		// wantSentinel, when set, asserts errors.Is(err, wantSentinel) in
		// addition to the message comparison, so a refactor that keeps the
		// wording but drops the sentinel identity still fails.
		wantSentinel error
	}

	var cases = map[string]struct {
		cr        *v1alpha1.Entitlement
		instance  *entitlement2.Instance
		updateErr error
		want      want
	}{
		"External name empty, assignment absent, resource does not exist": {
			cr:       entitlement(),
			instance: &entitlement2.Instance{Assignment: nil},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: false},
				describeCalled:        true,
				wantFinalExternalName: "",
			},
		},
		"External name empty, assignment present, strict adoption error": {
			cr: entitlement(),
			instance: &entitlement2.Instance{
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(5))},
			},
			want: want{
				o:                     managed.ExternalObservation{},
				err:                   errExistingAssignmentRequiresAdoption,
				describeCalled:        true,
				wantFinalExternalName: "",
			},
		},
		// Fixture artifact: this test's kube.MockList is empty, so
		// Required is the zero-sibling aggregate (Amount/Enable both
		// nil) -- reachable in production whenever no contributing
		// sibling sets amount or enable. A live CR in that shape
		// therefore gets a permanent Drift condition, since a nil
		// desired amount always renders as "<unset>" and compares as a
		// mismatch; this is intentional, not a defect.
		"External name valid compound key, assignment present, client receives parsed key": {
			cr: entitlement(withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(5))},
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=<unset>, observed=5)"},
				describeCalled:        true,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"External name valid compound key, assignment absent, resource does not exist": {
			cr: entitlement(withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          nil,
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: false},
				describeCalled:        true,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"External name malformed, wrapped ErrInvalidExternalName, no client call": {
			cr:       entitlement(withExternalName("not-a-compound-key")),
			instance: &entitlement2.Instance{},
			want: want{
				o:                     managed.ExternalObservation{},
				err:                   errors.Wrap(errors.Wrap(entitlement2.ErrInvalidExternalName, errParseExternalName), errUpdateObservation),
				wantFinalExternalName: "not-a-compound-key",
				wantSentinel:          entitlement2.ErrInvalidExternalName,
			},
		},
		"External name empty, invalid spec identity, wrapped errBuildExternalName, no client call": {
			cr:       entitlement(withServiceName("")),
			instance: &entitlement2.Instance{},
			want: want{
				o:                     managed.ExternalObservation{},
				err:                   errors.Wrap(errors.Wrap(entitlement2.ErrEmptyExternalNameSegment, errBuildExternalName), errUpdateObservation),
				wantFinalExternalName: "",
			},
		},
		"External name legacy sentinel, invalid spec identity, wrapped errBuildExternalName, no client call": {
			cr:       entitlement(withName("legacy-bad-spec"), withServiceName(""), withExternalName("legacy-bad-spec")),
			instance: &entitlement2.Instance{},
			want: want{
				o:                     managed.ExternalObservation{},
				err:                   errors.Wrap(errors.Wrap(entitlement2.ErrEmptyExternalNameSegment, errBuildExternalName), errUpdateObservation),
				wantFinalExternalName: "legacy-bad-spec",
			},
		},
		"External name mismatch, subaccountGuid differs": {
			cr:       entitlement(withExternalName("wrong-guid/service-name/service-plan-name")),
			instance: &entitlement2.Instance{},
			want: want{
				o:                     managed.ExternalObservation{},
				isMismatch:            true,
				wantFinalExternalName: "wrong-guid/service-name/service-plan-name",
				wantSentinel:          entitlement2.ErrExternalNameSpecMismatch,
			},
		},
		"External name mismatch, qualifier differs between annotation and spec": {
			cr:       entitlement(withUniqueServicePlanIdentifier("q-spec"), withExternalName("subaccount-guid/service-name/service-plan-name/q-annotation")),
			instance: &entitlement2.Instance{},
			want: want{
				o:                     managed.ExternalObservation{},
				isMismatch:            true,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name/q-annotation",
				wantSentinel:          entitlement2.ErrExternalNameSpecMismatch,
			},
		},
		// Fixture artifact, not drift evidence: same empty-MockList shape as
		// above -- desired=<unset> reflects a zero-sibling aggregate, not a
		// nil Required or anything about branch selection.
		"External name legacy sentinel, assignment present, three-segment migration via one kube.Update": {
			cr: entitlement(withName("legacy-cr"), withExternalName("legacy-cr")),
			instance: &entitlement2.Instance{
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(3))},
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=<unset>, observed=3)"},
				describeCalled:        true,
				updateCalls:           1,
				updatedExternalName:   "subaccount-guid/service-name/service-plan-name",
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"External name legacy sentinel, assignment absent, resource does not exist, annotation unchanged": {
			cr:       entitlement(withName("legacy-cr"), withExternalName("legacy-cr")),
			instance: &entitlement2.Instance{Assignment: nil},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: false},
				describeCalled:        true,
				wantFinalExternalName: "legacy-cr",
			},
		},
		// Fixture artifact, not drift evidence: same empty-MockList shape as
		// above -- desired=<unset> reflects a zero-sibling aggregate, not a
		// nil Required or anything about branch selection.
		"External name legacy sentinel, assignment present, four-segment migration via one kube.Update": {
			cr: entitlement(withName("legacy-cr-q"), withUniqueServicePlanIdentifier("q1"), withExternalName("legacy-cr-q")),
			instance: &entitlement2.Instance{
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(3))},
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=<unset>, observed=3)"},
				describeCalled:        true,
				updateCalls:           1,
				updatedExternalName:   "subaccount-guid/service-name/service-plan-name/q1",
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name/q1",
			},
		},
		"External name legacy sentinel, assignment present, kube.Update fails, wrapped errUpdateExternalName": {
			cr: entitlement(withName("legacy-cr-fail"), withExternalName("legacy-cr-fail")),
			instance: &entitlement2.Instance{
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(3))},
			},
			updateErr: errors.New(errKubeAPI),
			want: want{
				o:                     managed.ExternalObservation{},
				err:                   errors.Wrap(errors.Wrap(errors.New(errKubeAPI), errUpdateExternalName), errUpdateObservation),
				describeCalled:        true,
				updateCalls:           1,
				updatedExternalName:   "subaccount-guid/service-name/service-plan-name",
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var describeCalled bool
				var gotKey entitlement2.ExternalNameKey
				updateCalls := 0
				var updatedName string

				mockClient := fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						describeCalled = true
						gotKey = key
						return tc.instance, nil
					},
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					// Deliberately empty: an empty aggregate excludes cr from
					// its own findRelatedEntitlements sum. The ResourceUpToDate:true
					// asserted below for the externalNameCurrent cases comes from an
					// empty aggregate, not evidence of N:1 amount-aggregation
					// arithmetic (covered separately by TestObserve).
					MockList: test.NewMockListFn(nil, ListEntitlements()),
					MockUpdate: test.NewMockUpdateFn(tc.updateErr, func(obj client.Object) error {
						updateCalls++
						updatedName = meta.GetExternalName(obj.(*v1alpha1.Entitlement))
						return nil
					}),
				}

				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}
				got, err := e.Observe(context.Background(), tc.cr)

				wantErr := tc.want.err
				if tc.want.isMismatch {
					key, perr := entitlement2.ParseExternalName(meta.GetExternalName(tc.cr))
					if perr != nil {
						t.Fatalf("test setup: ParseExternalName(%q): %v", meta.GetExternalName(tc.cr), perr)
					}
					wantErr = errors.Wrap(errors.Wrap(entitlement2.ErrExternalNameSpecMismatch, key.Mismatch(tc.cr)), errUpdateObservation)
				}
				if diff := compareErrorMessages(err, wantErr); diff != "" {
					t.Errorf("\ne.Observe(...): -want error %s, +got error:\n%s\n", wantErr, err)
				}
				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
				}
				if describeCalled != tc.want.describeCalled {
					t.Errorf("DescribeInstance called = %v, want %v", describeCalled, tc.want.describeCalled)
				}
				if describeCalled {
					wantKey, kerr := entitlement2.NewExternalNameKey(tc.cr)
					if kerr != nil {
						t.Fatalf("test setup: NewExternalNameKey: %v", kerr)
					}
					if diff := cmp.Diff(wantKey, gotKey); diff != "" {
						t.Errorf("\nDescribeInstance key: -want +got:\n%s\n", diff)
					}
				}
				if updateCalls != tc.want.updateCalls {
					t.Errorf("kube.Update called %d time(s), want %d", updateCalls, tc.want.updateCalls)
				}
				if tc.want.updateCalls > 0 && updatedName != tc.want.updatedExternalName {
					t.Errorf("kube.Update persisted external-name %q, want %q", updatedName, tc.want.updatedExternalName)
				}
				if got := meta.GetExternalName(tc.cr); got != tc.want.wantFinalExternalName {
					t.Errorf("final external-name annotation = %q, want %q", got, tc.want.wantFinalExternalName)
				}
				if tc.want.wantSentinel != nil && !errors.Is(err, tc.want.wantSentinel) {
					t.Errorf("errors.Is(err, %v) = false, want true (err: %v)", tc.want.wantSentinel, err)
				}
			},
		)
	}
}

// TestObserveLegacyMigrationSurvivesRevert guards
// persistExternalName's DeepCopy: mockKube.MockUpdate mutates
// cr.Status.AtProvider.Assigned's existing fields in place, the same way
// the real API server's decode does, so a bare pointer-copy regression
// would still pass against a mock that simply swapped pointers.
// stalePreUpgradeObservation differs from the fresh describe below in
// Amount and EntityState, so a reverted fix visibly disagrees on Diff,
// ResourceUpToDate, and the Ready condition instead of silently passing.
func TestObserveLegacyMigrationSurvivesRevert(t *testing.T) {
	cr := entitlement(withName("legacy-migrate"), withExternalName("legacy-migrate"), withAmount(4))

	stalePreUpgradeObservation := &v1alpha1.Assignable{
		Amount:      internal.Ptr(7),
		EntityState: v1alpha1.EntitlementStatusProcessingFailed,
	}

	var updateCalled bool
	mockClient := fake.MockClient{
		MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
			return &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{
					Amount:      internal.Ptr(float32(4)),
					EntityState: internal.Ptr(v1alpha1.EntitlementStatusOk),
				},
			}, nil
		},
	}
	mockKube := &test.MockClient{
		MockStatusUpdate: noopStatusUpdate,
		MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
		MockUpdate: test.NewMockUpdateFn(nil, func(obj client.Object) error {
			updateCalled = true
			// The API server ignores status on a main-resource PUT and
			// returns the stored status; controller-runtime decodes that
			// back into obj, reusing (not replacing) the already-non-nil
			// cr.Status.AtProvider.Assigned that updateObservationFrom set
			// moments ago.
			e := obj.(*v1alpha1.Entitlement)
			e.Status.AtProvider.Assigned.Amount = stalePreUpgradeObservation.Amount
			e.Status.AtProvider.Assigned.EntityState = stalePreUpgradeObservation.EntityState
			return nil
		}),
	}
	e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}

	got, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
	}
	if !updateCalled {
		t.Fatal("test setup: kube.Update was never called -- persistExternalName did not run")
	}

	want := managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: ""}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("\ne.Observe(...): -want, +got:\n%s\n(observing stalePreUpgradeObservation instead of the fresh describe would report Diff \"amount mismatch (desired=4, observed=7)\" and ResourceUpToDate=false)", diff)
	}
	if gotAmount := cr.Status.AtProvider.Assigned.Amount; gotAmount == nil || *gotAmount != 4 {
		t.Errorf("cr.Status.AtProvider.Assigned.Amount = %v, want 4 (this reconcile's fresh describe, not stalePreUpgradeObservation's 7)", internal.Val(gotAmount))
	}
	if gotState := cr.Status.AtProvider.Assigned.EntityState; gotState != v1alpha1.EntitlementStatusOk {
		t.Errorf("cr.Status.AtProvider.Assigned.EntityState = %q, want %q (fresh describe, not stalePreUpgradeObservation's PROCESSING_FAILED)", gotState, v1alpha1.EntitlementStatusOk)
	}
	if readyStatus := cr.Status.GetCondition(xpv1.Available().Type).Status; readyStatus != xpv1.Available().Status {
		t.Errorf("Ready condition status = %v, want %v (EntityState OK from the fresh describe, not stalePreUpgradeObservation's PROCESSING_FAILED)", readyStatus, xpv1.Available().Status)
	}
	assertDriftCondition(t, cr, "", false)
}

func ListEntitlements(v ...*v1alpha1.Entitlement) test.ObjectListFn {
	return func(obj client.ObjectList) error {
		l := obj.(*v1alpha1.EntitlementList)
		l.Items = []v1alpha1.Entitlement{}
		for _, e := range v {
			l.Items = append(l.Items, *e)
		}
		return nil
	}
}

// listOnceThenErr returns a MockListFn that succeeds once via populate,
// then returns err on every later call, so the first
// (updateObservationFrom) list succeeds and only a later caller (like
// mayAdopt) observes the failure.
func listOnceThenErr(populate test.ObjectListFn, err error) test.MockListFn {
	calls := 0
	return func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
		calls++
		if calls == 1 {
			return populate(obj)
		}
		return err
	}
}

func compareErrorMessages(is error, target error) string {
	var isMsg, targetMsg string
	if is != nil {
		isMsg = is.Error()
	}
	if target != nil {
		targetMsg = target.Error()
	}
	return cmp.Diff(isMsg, targetMsg)
}

type entitlementModifier func(*v1alpha1.Entitlement)

func withName(name string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.Name = name }
}
func withServiceName(name string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.Spec.ForProvider.ServiceName = name }
}

func withServicePlan(plan string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.Spec.ForProvider.ServicePlanName = plan }
}

func withUniqueServicePlanIdentifier(plan string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.Spec.ForProvider.ServicePlanUniqueIdentifier = &plan }
}

func withSubaccountGuid(guid string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.Spec.ForProvider.SubaccountGuid = guid }
}

func withAmount(amount int) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.Spec.ForProvider.Amount = &amount }
}

func withEnabled(enabled bool) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.Spec.ForProvider.Enable = &enabled }
}

func withUID(uid string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { r.UID = types.UID(uid) }
}

func withExternalName(name string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) { meta.SetExternalName(r, name) }
}

func withDeletionTimestamp() entitlementModifier {
	return func(r *v1alpha1.Entitlement) {
		now := metav1.Now()
		r.DeletionTimestamp = &now
	}
}

func withAssignedStatus(amount *int, entityState string) entitlementModifier {
	return func(r *v1alpha1.Entitlement) {
		if r.Status.AtProvider == nil {
			r.Status.AtProvider = &v1alpha1.EntitlementObservation{}
		}
		r.Status.AtProvider.Assigned = &v1alpha1.Assignable{
			Amount:      amount,
			EntityState: entityState,
		}
	}
}

// withRequiredAssigned directly sets Status.AtProvider.Required/Assigned,
// bypassing MergeRelatedEntitlements/GenerateObservation, for
// TestCalculateDiff's fixtures needing independent control over both sides.
func withRequiredAssigned(required *v1alpha1.EntitlementSummary, assigned *v1alpha1.Assignable) entitlementModifier {
	return func(r *v1alpha1.Entitlement) {
		if r.Status.AtProvider == nil {
			r.Status.AtProvider = &v1alpha1.EntitlementObservation{}
		}
		r.Status.AtProvider.Required = required
		r.Status.AtProvider.Assigned = assigned
	}
}

func withConditions(c ...xpv1.Condition) entitlementModifier {
	return func(r *v1alpha1.Entitlement) {
		r.Status.SetConditions(c...)
	}
}
func entitlement(m ...entitlementModifier) *v1alpha1.Entitlement {
	cr := &v1alpha1.Entitlement{
		ObjectMeta: metav1.ObjectMeta{
			Name: "entitlement",
		},
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				SubaccountGuid:  "subaccount-guid",
				ServiceName:     "service-name",
				ServicePlanName: "service-plan-name",
			},
		},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func TestObserveWithDifferentType(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client entitlement2.Client
	}

	type want struct {
		o   managed.ExternalObservation
		err error
	}
	var cases = map[string]struct {
		args args
		want want
	}{
		"Non Entitlement Type, returns error": {
			args: args{
				client: fake.MockClient{},
				cr:     nil,
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New(errNotEntitlement),
			},
		},
	}
	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				e := external{client: tc.args.client, tracker: test2.NoOpReferenceResolverTracker{}}
				got, err := e.Observe(context.Background(), tc.args.cr)
				if diff := compareErrorMessages(err, tc.want.err); diff != "" {
					t.Errorf("\ne.Observe(...): -want error %s, +got error:\n%s\n", tc.want.err, err)
				}
				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
				}
			},
		)
	}
}

func TestObserveSoftvalidation(t *testing.T) {
	type args struct {
		cr     *v1alpha1.Entitlement
		client entitlement2.Client
	}

	type want struct {
		containsMessage *[]string
	}
	var cases = map[string]struct {
		args args
		want want
	}{
		"Could not check if entitled": {
			args: args{
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{}, nil
				}},
				cr: entitlement(withAmount(1), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				containsMessage: internal.Ptr([]string{"Could not find service to be entitled. Check if Global Account is entitled for usage (Control Center)."}),
			},
		},
		"Non Numeric Service entitled, Cr using amount": {
			args: args{
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Unlimited: internal.Ptr(true),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{},
					}, nil
				}},
				cr: entitlement(withAmount(1), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				containsMessage: internal.Ptr([]string{"This serviceplan is non numeric, please use .Spec.ForProvider.Enable and omit the use of .Spec.ForProvider.Amount to configure the entitlement"}),
			},
		},
		"Amount and enable is used": {
			args: args{
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{},
					}, nil
				}},
				cr: entitlement(withAmount(1), withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				containsMessage: internal.Ptr([]string{".Spec.ForProvider.Amount & .Spec.ForProvider.Enable set. Only one value is supported. This depends on the type of service"}),
			},
		},
	}
	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				e := external{client: tc.args.client, kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(tc.args.cr)),
				}, tracker: test2.NoOpReferenceResolverTracker{}}
				_, err := e.Observe(context.Background(), tc.args.cr)
				if diff := compareErrorMessages(err, nil); diff != "" {
					t.Errorf("\ne.Observe(...): -want error nil, +got error:\n%s\n", err)
				}
				condition := tc.args.cr.Status.GetCondition(v1alpha1.SoftValidationCondition)
				if tc.want.containsMessage != nil {
					for _, msg := range *tc.want.containsMessage {
						if !strings.Contains(condition.Message, msg) {
							t.Errorf("\ne.Observe(...): -want-substring %s\n, +got:\n%s\n", msg, condition.Message)
						}

					}

				}

			},
		)
	}
}

// TestUpdate exercises Update's annotation-driven identity resolution:
// currentExternalNameKey parses cr's external-name annotation and
// rejects an empty annotation or a mismatch against spec before ever
// calling UpdateInstance, and Update never rewrites the annotation
// itself (only Observe's persistExternalName does). Error messages are
// hard-coded rather than recomputed, so a wrong message is actually
// caught instead of trivially matching itself.
func TestUpdate(t *testing.T) {
	type want struct {
		err          error
		wantSentinel error
		updateCalled bool
	}

	cases := map[string]struct {
		cr   *v1alpha1.Entitlement
		want want
	}{
		"annotation matches spec identity, fake receives the parsed annotation key": {
			cr: entitlement(
				withExternalName("subaccount-guid/service-name/service-plan-name"),
				withAssignedStatus(internal.Ptr(5), "OK"),
			),
			want: want{
				updateCalled: true,
			},
		},
		"annotation disagrees with spec identity, wrapped ErrExternalNameSpecMismatch, zero client calls": {
			cr: entitlement(
				withExternalName("wrong-guid/service-name/service-plan-name"),
				withAssignedStatus(internal.Ptr(5), "OK"),
			),
			want: want{
				err:          errors.New(`while resolving entitlement identity: subaccountGuid mismatch (annotation="wrong-guid", spec="subaccount-guid"): external-name does not match immutable spec identity`),
				wantSentinel: entitlement2.ErrExternalNameSpecMismatch,
				updateCalled: false,
			},
		},
		"empty annotation, Update keeps the strict rejection Delete no longer applies, zero client calls": {
			cr: entitlement(
				withAssignedStatus(internal.Ptr(5), "OK"),
			),
			want: want{
				err:          errors.New(`while resolving entitlement identity: cannot parse external-name: external-name must be in format 'subaccountGuid/serviceName/servicePlanName[/servicePlanUniqueIdentifier]'`),
				wantSentinel: entitlement2.ErrInvalidExternalName,
				updateCalled: false,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var updateCalled bool
				var gotKey entitlement2.ExternalNameKey
				mockClient := fake.MockClient{
					MockUpdateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						updateCalled = true
						gotKey = key
						return nil
					},
				}
				beforeExternalName := meta.GetExternalName(tc.cr)

				e := external{client: mockClient, tracker: test2.NoOpReferenceResolverTracker{}}
				_, err := e.Update(context.Background(), tc.cr)

				if diff := compareErrorMessages(err, tc.want.err); diff != "" {
					t.Errorf("\ne.Update(...): -want error %s, +got error:\n%s\n", tc.want.err, err)
				}
				if tc.want.wantSentinel != nil && !errors.Is(err, tc.want.wantSentinel) {
					t.Errorf("errors.Is(err, %v) = false, want true (err: %v)", tc.want.wantSentinel, err)
				}

				if updateCalled != tc.want.updateCalled {
					t.Errorf("UpdateInstance called = %v, want %v", updateCalled, tc.want.updateCalled)
				}
				if updateCalled {
					wantKey, kerr := entitlement2.ParseExternalName(meta.GetExternalName(tc.cr))
					if kerr != nil {
						t.Fatalf("test setup: ParseExternalName(%q): %v", meta.GetExternalName(tc.cr), kerr)
					}
					if diff := cmp.Diff(wantKey, gotKey); diff != "" {
						t.Errorf("\nUpdateInstance key: -want +got:\n%s\n", diff)
					}
				}
				if got := meta.GetExternalName(tc.cr); got != beforeExternalName {
					t.Errorf("Update(...) changed external-name annotation from %q to %q; Update must never call meta.SetExternalName", beforeExternalName, got)
				}
			},
		)
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr     *v1alpha1.Entitlement
		client fake.MockClient
		kube   client.Client
	}

	type want struct {
		err                error
		wantSentinel       error
		requiredAmount     *int
		wantDescribeCalled bool
		wantDeleteCalled   bool
		wantKey            entitlement2.ExternalNameKey
	}

	var cases = map[string]struct {
		args args
		want want
	}{
		"Delete with sibling, numeric quota, amount sent is sibling sum not zero": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR being deleted — filtered out by UID
						entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
						// Sibling CR with amount=3 — should be the remaining sum
						entitlement(withName("sibling-cr"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(5)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withExternalName("a/Alpha/One"), withAssignedStatus(internal.Ptr(5), "OK")),
			},
			want: want{
				err:                nil,
				requiredAmount:     internal.Ptr(3), // sibling sum
				wantDescribeCalled: true,
				wantDeleteCalled:   true,
				wantKey:            entitlement2.ExternalNameKey{SubaccountGUID: "a", ServiceName: "Alpha", ServicePlanName: "One"},
			},
		},
		"Delete with multiple siblings, numeric quota, amount is sum of all siblings": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR being deleted
						entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
						// Sibling 1
						entitlement(withName("sibling-1"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a")),
						// Sibling 2
						entitlement(withName("sibling-2"), withUID("uid-3"), withServiceName("Alpha"), withServicePlan("One"), withAmount(4), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(9)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withExternalName("a/Alpha/One"), withAssignedStatus(internal.Ptr(9), "OK")),
			},
			want: want{
				err:                nil,
				requiredAmount:     internal.Ptr(7), // 3 + 4
				wantDescribeCalled: true,
				wantDeleteCalled:   true,
				wantKey:            entitlement2.ExternalNameKey{SubaccountGUID: "a", ServiceName: "Alpha", ServicePlanName: "One"},
			},
		},
		"Delete sole CR, no siblings, amount set to zero for full removal": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// Only the CR being deleted — filtered out by UID, no siblings remain
						entitlement(withName("sole-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(2)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withName("sole-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withExternalName("a/Alpha/One"), withAssignedStatus(internal.Ptr(2), "OK")),
			},
			want: want{
				err:            nil,
				requiredAmount: nil, // no siblings → MergeRelatedEntitlements returns nil Amount; the
				// real client fills the wire amount to zero -- see
				// TestDeleteInstanceSoleNumericSendsZero, which asserts that
				// directly against the actual SetServicePlans payload.
				wantDescribeCalled: true,
				wantDeleteCalled:   true,
				wantKey:            entitlement2.ExternalNameKey{SubaccountGUID: "a", ServiceName: "Alpha", ServicePlanName: "One"},
			},
		},
		"Delete with four-segment key including qualifier, qualifier-distinct sibling excluded from sum": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// Same subaccount/service/plan but no qualifier -- must
						// NOT be summed into this four-segment CR's aggregate;
						// proves findRelatedEntitlements' qualifier filter on
						// the Delete path, not just Observe's.
						entitlement(withName("other-qualifier-sibling"), withUID("uid-2"), withAmount(99)),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(4)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withUniqueServicePlanIdentifier("plan-q"), withAmount(4), withExternalName("subaccount-guid/service-name/service-plan-name/plan-q"), withAssignedStatus(internal.Ptr(4), "OK")),
			},
			want: want{
				err:                nil,
				requiredAmount:     nil, // qualifier-distinct sibling excluded -- MergeRelatedEntitlements never sees it
				wantDescribeCalled: true,
				wantDeleteCalled:   true,
				wantKey:            entitlement2.ExternalNameKey{SubaccountGUID: "subaccount-guid", ServiceName: "service-name", ServicePlanName: "service-plan-name", ServicePlanUniqueIdentifier: internal.Ptr("plan-q")},
			},
		},
		"Route A: deleting CR, empty annotation, non-deleting sibling already carries the compound key, sibling sum below assigned amount, spec-derived key issues the reduction": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList: test.NewMockListFn(nil, ListEntitlements(
						// CR being deleted, filtered out by UID; empty
						// annotation since a deleting CR skips persistExternalName.
						entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
						// Non-deleting sibling already proves the aggregate via
						// its own persisted compound key; its amount is the
						// remaining sum, below BTP's current assignment.
						entitlement(withName("sibling-cr"), withUID("uid-2"), withServiceName("Alpha"), withServicePlan("One"), withAmount(3), withSubaccountGuid("a"), withExternalName("a/Alpha/One")),
					)),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(5)), // 2 (cr) + 3 (sibling); sibling sum (3) is below this
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withAssignedStatus(internal.Ptr(5), "OK")),
			},
			want: want{
				err:                nil,
				requiredAmount:     internal.Ptr(3), // sibling sum -- Delete must issue the reduction via the spec-derived key instead of erroring
				wantDescribeCalled: true,
				wantDeleteCalled:   true,
				wantKey:            entitlement2.ExternalNameKey{SubaccountGUID: "a", ServiceName: "Alpha", ServicePlanName: "One"},
			},
		},
		"External name empty, no siblings, spec-derived key, DeleteInstance succeeds": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements()),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(2)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withName("no-annotation-cr"), withAmount(2), withAssignedStatus(internal.Ptr(2), "OK")),
			},
			want: want{
				err:                nil,
				requiredAmount:     nil, // no siblings → MergeRelatedEntitlements returns nil Amount
				wantDescribeCalled: true,
				wantDeleteCalled:   true,
				wantKey:            entitlement2.ExternalNameKey{SubaccountGUID: "subaccount-guid", ServiceName: "service-name", ServicePlanName: "service-plan-name"},
			},
		},
		"External name metadata-name sentinel, legacy unmigrated CR reaching Delete, spec-derived key, DeleteInstance succeeds": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements()),
				},
				client: fake.MockClient{MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(4)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withName("legacy-cr"), withAmount(4), withExternalName("legacy-cr"), withAssignedStatus(internal.Ptr(4), "OK")),
			},
			want: want{
				err:                nil,
				requiredAmount:     nil, // no siblings → MergeRelatedEntitlements returns nil Amount
				wantDescribeCalled: true,
				wantDeleteCalled:   true,
				wantKey:            entitlement2.ExternalNameKey{SubaccountGUID: "subaccount-guid", ServiceName: "service-name", ServicePlanName: "service-plan-name"},
			},
		},
		"External name mismatches spec identity, wrapped ErrExternalNameSpecMismatch, no BTP write": {
			args: args{
				cr: entitlement(withName("mismatch-cr"), withExternalName("wrong-guid/service-name/service-plan-name"), withAssignedStatus(internal.Ptr(5), "OK")),
			},
			want: want{
				err:          errors.New(`while resolving entitlement identity: subaccountGuid mismatch (annotation="wrong-guid", spec="subaccount-guid"): external-name does not match immutable spec identity`),
				wantSentinel: entitlement2.ErrExternalNameSpecMismatch,
			},
		},
		"External name structurally unparseable, wrapped ErrInvalidExternalName, no client call": {
			args: args{
				cr: entitlement(withName("unparseable-cr"), withExternalName("a/b")),
			},
			want: want{
				err:          errors.New(`while resolving entitlement identity: cannot parse external-name: external-name must be in format 'subaccountGuid/serviceName/servicePlanName[/servicePlanUniqueIdentifier]'`),
				wantSentinel: entitlement2.ErrInvalidExternalName,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var capturedAmount *int
				deleteCalled := false
				describeCalled := false
				var gotDescribeKey, gotDeleteKey entitlement2.ExternalNameKey

				origDescribe := tc.args.client.MockDescribeInstanceFn
				tc.args.client.MockDescribeInstanceFn = func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
					describeCalled = true
					gotDescribeKey = key
					if origDescribe != nil {
						return origDescribe(ctx, key)
					}
					return &entitlement2.Instance{}, nil
				}
				tc.args.client.MockDeleteInstanceFn = func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
					deleteCalled = true
					gotDeleteKey = key
					if cr.Status.AtProvider != nil && cr.Status.AtProvider.Required != nil {
						capturedAmount = cr.Status.AtProvider.Required.Amount
					}
					return nil
				}
				beforeExternalName := meta.GetExternalName(tc.args.cr)

				e := external{client: tc.args.client, kube: tc.args.kube, tracker: test2.NoOpReferenceResolverTracker{}}
				_, err := e.Delete(context.Background(), tc.args.cr)
				if diff := compareErrorMessages(err, tc.want.err); diff != "" {
					t.Errorf("\ne.Delete(...): -want error %v, +got error:\n%s\n", tc.want.err, err)
				}
				if tc.want.wantSentinel != nil && !errors.Is(err, tc.want.wantSentinel) {
					t.Errorf("errors.Is(err, %v) = false, want true (err: %v)", tc.want.wantSentinel, err)
				}

				if got := meta.GetExternalName(tc.args.cr); got != beforeExternalName {
					t.Errorf("Delete(...) changed external-name annotation from %q to %q; Delete must never call meta.SetExternalName", beforeExternalName, got)
				}

				if describeCalled != tc.want.wantDescribeCalled {
					t.Errorf("\ne.Delete(...): DescribeInstance called = %v, want %v", describeCalled, tc.want.wantDescribeCalled)
				}
				if deleteCalled != tc.want.wantDeleteCalled {
					t.Errorf("\ne.Delete(...): DeleteInstance called = %v, want %v", deleteCalled, tc.want.wantDeleteCalled)
				}
				if !tc.want.wantDeleteCalled {
					return
				}

				// wantKey is an explicit expectation, not re-derived from
				// keyForObserve: deriving it the same way would pass
				// regardless of how keyForObserve classified the
				// annotation, including a wrongly spec-derived key for a
				// compound annotation.
				if diff := cmp.Diff(tc.want.wantKey, gotDescribeKey); diff != "" {
					t.Errorf("\nDescribeInstance key: -want +got:\n%s\n", diff)
				}
				if diff := cmp.Diff(tc.want.wantKey, gotDeleteKey); diff != "" {
					t.Errorf("\nDeleteInstance key: -want +got:\n%s\n", diff)
				}

				if diff := cmp.Diff(tc.want.requiredAmount, capturedAmount); diff != "" {
					t.Errorf("\ne.Delete(...) Required.Amount passed to DeleteInstance: -want, +got:\n%s\n", diff)
				}
			},
		)
	}
}

// recorderFake captures every event.Event recorded through it, letting
// tests assert both that the deletion carve-out emits exactly the expected
// preservation event and that no other path emits one.
type recorderFake struct{ events []event.Event }

func (r *recorderFake) Event(_ runtime.Object, e event.Event) {
	r.events = append(r.events, e)
}
func (r *recorderFake) WithAnnotations(_ ...string) event.Recorder { return r }

// TestObserveEmptyNameAdoption exercises Observe's aggregate adoption
// guard for an empty external-name annotation: an unowned assignment
// refuses adoption, but a non-deleting same-key sibling already carrying
// the compound key or legacy sentinel proves the aggregate is
// provider-managed, and assignFailedNoQuota or AutoAssigned never needs proof.
func TestObserveEmptyNameAdoption(t *testing.T) {
	type want struct {
		o              managed.ExternalObservation
		wantGuardError bool
		// wantErr, when set, is compared instead of the wantGuardError
		// sentinel -- for errors that aren't the adoption refusal itself,
		// e.g. mayAdopt's own kube.List failure propagating unwrapped.
		wantErr               error
		wantFinalExternalName string
		wantRequiredAmount    *int
	}

	var cases = map[string]struct {
		cr       *v1alpha1.Entitlement
		instance *entitlement2.Instance
		siblings []*v1alpha1.Entitlement
		// mayAdoptListErr, when set, makes the *second* kube.List call
		// fail (the first, from updateObservationFrom, still succeeds),
		// isolating mayAdopt's own findRelatedEntitlements call.
		mayAdoptListErr error
		want            want
	}{
		"ordinary assignment, no sibling proof, guard error, no join": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			want: want{
				o:                     managed.ExternalObservation{},
				wantGuardError:        true,
				wantFinalExternalName: "",
			},
		},
		"ordinary assignment, sibling carries exact compound key, joins and aggregates bare siblings": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(9)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("proving-sibling"), withUID("cr-2"), withAmount(3), withExternalName("subaccount-guid/service-name/service-plan-name")),
				entitlement(withName("bare-sibling"), withUID("cr-3"), withAmount(4)),
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
				wantRequiredAmount:    internal.Ptr(9), // 2 (cr) + 3 (proving sibling) + 4 (previously bare sibling)
			},
		},
		"ordinary assignment, sibling carries legacy sentinel, joins": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(5)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("legacy-sibling"), withUID("cr-2"), withAmount(3), withExternalName("legacy-sibling")),
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
				wantRequiredAmount:    internal.Ptr(5),
			},
		},
		"ordinary assignment, sibling annotation malformed or unrelated, guard error": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("unrelated-sibling"), withUID("cr-2"), withAmount(3), withExternalName("not-a-compound-key")),
			},
			want: want{
				o:                     managed.ExternalObservation{},
				wantGuardError:        true,
				wantFinalExternalName: "",
			},
		},
		"ordinary assignment, proving sibling is itself deleting, guard error": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("deleting-sibling"), withUID("cr-2"), withAmount(3), withExternalName("subaccount-guid/service-name/service-plan-name"), withConditions(xpv1.Deleting())),
			},
			want: want{
				o:                     managed.ExternalObservation{},
				wantGuardError:        true,
				wantFinalExternalName: "",
			},
		},
		"ordinary assignment, qualifier-distinct sibling annotation ignored, guard error": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				// Same subaccount/service/plan but a different qualifier:
				// findRelatedEntitlements excludes it before
				// siblingProvesOwnership ever sees its (deliberately
				// coincidental) matching annotation string.
				entitlement(withName("qualifier-sibling"), withUID("cr-2"), withAmount(3), withUniqueServicePlanIdentifier("region-b"), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				o:                     managed.ExternalObservation{},
				wantGuardError:        true,
				wantFinalExternalName: "",
			},
		},
		"assignFailedNoQuota assignment, no sibling needed, resource does not exist": {
			cr: entitlement(withUID("cr-1"), withEnabled(true)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(0)), EntityState: internal.Ptr("PROCESSING_FAILED")},
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: false},
				wantFinalExternalName: "",
			},
		},
		"AutoAssigned assignment, no sibling needed, joins without a client write": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(10)), EntityState: internal.Ptr("OK"), AutoAssigned: internal.Ptr(true)},
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: "amount mismatch (desired=2, observed=10)"},
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		// mayAdopt's own findRelatedEntitlements call fails after
		// updateObservationFrom's earlier call already succeeded; the
		// error must propagate, not be swallowed into an empty sibling list.
		"ordinary assignment, mayAdopt's kube.List fails, error propagated not swallowed": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			mayAdoptListErr: errors.New(errKubeAPI),
			want: want{
				o:                     managed.ExternalObservation{},
				wantErr:               errors.Wrap(errors.Wrap(errors.Wrap(errors.New(errKubeAPI), errListEntitlements), errFindRelated), errUpdateObservation),
				wantFinalExternalName: "",
			},
		},
		// A joined aggregate whose sibling sum (2 + 3 + 10 = 15) exceeds
		// what BTP reports as assigned (9) must surface as
		// ResourceUpToDate:false, proving drift is reachable through the
		// join, not just through an already-current annotation.
		"ordinary assignment, joined aggregate exceeds assigned amount, resource needs update": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(9)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("proving-sibling"), withUID("cr-2"), withAmount(3), withExternalName("subaccount-guid/service-name/service-plan-name")),
				entitlement(withName("bare-sibling"), withUID("cr-3"), withAmount(10)),
			},
			want: want{
				o:                     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, Diff: "amount mismatch (desired=15, observed=9)"},
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
				wantRequiredAmount:    internal.Ptr(15), // 2 (cr) + 3 (proving sibling) + 10 (bare sibling), exceeds assigned 9
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var createCalled, updateCalled, deleteCalled bool
				var kubeUpdateCalled bool
				var persistedExternalName string
				mockClient := fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						return tc.instance, nil
					},
					MockCreateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						createCalled = true
						return nil
					},
					MockUpdateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						updateCalled = true
						return nil
					},
					MockDeleteInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						deleteCalled = true
						return nil
					},
				}
				all := append([]*v1alpha1.Entitlement{tc.cr}, tc.siblings...)
				listFn := test.NewMockListFn(nil, ListEntitlements(all...))
				if tc.mayAdoptListErr != nil {
					listFn = listOnceThenErr(ListEntitlements(all...), tc.mayAdoptListErr)
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         listFn,
					MockUpdate: test.NewMockUpdateFn(nil, func(obj client.Object) error {
						kubeUpdateCalled = true
						persistedExternalName = meta.GetExternalName(obj.(*v1alpha1.Entitlement))
						return nil
					}),
				}

				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}
				got, err := e.Observe(context.Background(), tc.cr)

				switch {
				case tc.want.wantGuardError:
					if diff := compareErrorMessages(err, errExistingAssignmentRequiresAdoption); diff != "" {
						t.Errorf("\ne.Observe(...): -want error %s, +got error:\n%s\n", errExistingAssignmentRequiresAdoption, err)
					}
					if !errors.Is(err, errExistingAssignmentRequiresAdoption) {
						t.Errorf("errors.Is(err, errExistingAssignmentRequiresAdoption) = false, want true (err: %v)", err)
					}
				case tc.want.wantErr != nil:
					if diff := compareErrorMessages(err, tc.want.wantErr); diff != "" {
						t.Errorf("\ne.Observe(...): -want error %s, +got error:\n%s\n", tc.want.wantErr, err)
					}
				case err != nil:
					t.Errorf("e.Observe(...) returned unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
				}
				if got := meta.GetExternalName(tc.cr); got != tc.want.wantFinalExternalName {
					t.Errorf("final external-name annotation = %q, want %q", got, tc.want.wantFinalExternalName)
				}
				if createCalled || updateCalled || deleteCalled {
					t.Errorf("Observe must never write to BTP: create=%v update=%v delete=%v", createCalled, updateCalled, deleteCalled)
				}
				// Asserting kube.Update was actually called (not just the
				// in-memory annotation) catches a regression that skips
				// persistExternalName's kube.Update while leaving
				// SetExternalName's in-memory mutation intact.
				wantKubeUpdateCalled := tc.want.wantFinalExternalName != ""
				if kubeUpdateCalled != wantKubeUpdateCalled {
					t.Errorf("kube.Update called = %v, want %v", kubeUpdateCalled, wantKubeUpdateCalled)
				}
				if wantKubeUpdateCalled && persistedExternalName != tc.want.wantFinalExternalName {
					t.Errorf("kube.Update persisted external-name %q, want %q", persistedExternalName, tc.want.wantFinalExternalName)
				}
				if tc.want.wantRequiredAmount != nil {
					if tc.cr.Status.AtProvider == nil || tc.cr.Status.AtProvider.Required == nil {
						t.Fatalf("cr.Status.AtProvider.Required is nil, want Amount=%d", *tc.want.wantRequiredAmount)
					}
					if diff := cmp.Diff(tc.want.wantRequiredAmount, tc.cr.Status.AtProvider.Required.Amount); diff != "" {
						t.Errorf("\ncr.Status.AtProvider.Required.Amount: -want, +got:\n%s\n", diff)
					}
				}
			},
		)
	}
}

// TestObserveAssignFailedDeletingGate proves the
// empty/legacy branch's assignFailedNoQuota shortcut must not
// short-circuit to ResourceExists:false for a deleting CR whose
// assignment is still reserved (assignmentStillReserved); it defers to
// adoptionGuardApplies instead, which carries the same exception so an
// unproven CR is still refused via errUnownedAssignmentBlocksFinalize.
// The non-deleting and nothing-reserved cases must keep taking the
// shortcut unchanged.
func TestObserveAssignFailedDeletingGate(t *testing.T) {
	cases := map[string]struct {
		cr           *v1alpha1.Entitlement
		instance     *entitlement2.Instance
		siblings     []*v1alpha1.Entitlement
		wantShortcut bool
		wantErr      error
	}{
		"non-deleting, assignFailedNoQuota: shortcut still taken": {
			cr: entitlement(withUID("cr-1"), withEnabled(true)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: nil, EntityState: internal.Ptr("PROCESSING_FAILED")},
			},
			wantShortcut: true,
		},
		"deleting, assignFailedNoQuota, nothing actually reserved: shortcut still taken": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(0)), EntityState: internal.Ptr("PROCESSING_FAILED")},
			},
			wantShortcut: true,
		},
		"deleting, assignFailedNoQuota, nothing actually reserved (nil amount), no sibling proof: shortcut still taken": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: nil, EntityState: internal.Ptr("PROCESSING_FAILED")},
			},
			wantShortcut: true,
		},
		"deleting, assignFailedNoQuota, UnlimitedAmountAssigned still reserved, no sibling proof: shortcut skipped, refused not finalized": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: nil, EntityState: internal.Ptr("PROCESSING_FAILED"), UnlimitedAmountAssigned: internal.Ptr(true)},
			},
			wantErr: errUnownedAssignmentBlocksFinalize,
		},
		"deleting, assignFailedNoQuota, UnlimitedAmountAssigned still reserved, sibling proves ownership: shortcut skipped, joins and falls through": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: nil, EntityState: internal.Ptr("PROCESSING_FAILED"), UnlimitedAmountAssigned: internal.Ptr(true)},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("sibling-cr"), withUID("cr-2"), withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			wantShortcut: false,
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				all := append([]*v1alpha1.Entitlement{tc.cr}, tc.siblings...)
				mockClient := fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						return tc.instance, nil
					},
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(all...)),
				}
				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}

				obs, err := e.observeExternalName(context.Background(), tc.cr)
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("observeExternalName error = %v, want %v", err, tc.wantErr)
					}
					if obs != nil {
						t.Errorf("observeExternalName returned observation %v alongside terminal error, want nil", obs)
					}
					return
				}
				if err != nil {
					t.Fatalf("observeExternalName returned unexpected error: %v", err)
				}
				gotShortcut := obs != nil
				if gotShortcut != tc.wantShortcut {
					t.Errorf("observeExternalName returned a terminal observation = %v (obs=%v), want %v", gotShortcut, obs, tc.wantShortcut)
				}
				if gotShortcut && obs.ResourceExists {
					t.Errorf("shortcut observation ResourceExists = true, want false")
				}
			},
		)
	}
}

// TestObserveNeedsCreateDeletingReservedGate exercises Observe()
// end-to-end: needsCreate's own "not created yet" short-circuit must not
// fire for a deleting CR whose assignment is still reserved
// (assignFailedNoQuota misses UnlimitedAmountAssigned). The clearest case
// is an externalNameCurrent CR, since observeExternalName always returns
// (nil, nil) for it regardless of assignFailedNoQuota, making the
// needsCreate call site the only place that can still decide correctly;
// an empty-annotation CR is covered too, though it already resolves
// inside observeExternalName via adoptionGuardApplies.
func TestObserveNeedsCreateDeletingReservedGate(t *testing.T) {
	cases := map[string]struct {
		cr       *v1alpha1.Entitlement
		instance *entitlement2.Instance
		siblings []*v1alpha1.Entitlement
		wantErr  error
		wantO    managed.ExternalObservation
	}{
		"already-established solo CR, no siblings, still reserved: falls through to Delete() instead of silent finalize": {
			cr: entitlement(withUID("cr-1"), withAmount(4), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{
					Amount:                  nil,
					EntityState:             internal.Ptr("PROCESSING_FAILED"),
					UnlimitedAmountAssigned: internal.Ptr(true),
				},
			},
			wantO: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
		},
		"already-established solo CR, no siblings, nothing actually reserved (nil amount): finalizes silently": {
			cr: entitlement(withUID("cr-1"), withAmount(4), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{
					Amount:      nil,
					EntityState: internal.Ptr("PROCESSING_FAILED"),
				},
			},
			wantO: managed.ExternalObservation{ResourceExists: false},
		},
		"empty annotation, bare, no sibling proof, still reserved: refused via errUnownedAssignmentBlocksFinalize": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{
					Amount:                  nil,
					EntityState:             internal.Ptr("PROCESSING_FAILED"),
					UnlimitedAmountAssigned: internal.Ptr(true),
				},
			},
			wantErr: errUnownedAssignmentBlocksFinalize,
			wantO:   managed.ExternalObservation{},
		},
		"empty annotation, sibling proves ownership, still reserved: joins and falls through, not silently finalized": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{
					Amount:                  nil,
					EntityState:             internal.Ptr("PROCESSING_FAILED"),
					UnlimitedAmountAssigned: internal.Ptr(true),
				},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("sibling-cr"), withUID("cr-2"), withAmount(3), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			// A numeric sibling is required, not incidental: an
			// enable-based sibling would make both the correct and a
			// buggy blind-shortcut path land on the same
			// ResourceExists:false, unable to discriminate between them.
			wantO: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				all := append([]*v1alpha1.Entitlement{tc.cr}, tc.siblings...)
				mockClient := fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						return tc.instance, nil
					},
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(all...)),
				}
				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}

				got, err := e.Observe(context.Background(), tc.cr)
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("Observe error = %v, want %v", err, tc.wantErr)
					}
				} else if err != nil {
					t.Fatalf("Observe returned unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.wantO, got); diff != "" {
					t.Errorf("Observe(...): -want, +got:\n%s\n", diff)
				}
			},
		)
	}
}

// TestObserveDeletingUnjoined exercises resolveUnjoinedDeletion's decision
// table for a deleting CR with no sibling ownership proof: zero remaining
// siblings finalize silently only when assignmentStillReserved reports
// nothing reserved, else refuse via errUnownedAssignmentBlocksFinalize;
// with remaining siblings, finalize silently when their own need already
// covers BTP's current assignment, else refuse the same way, since no
// sibling in an unproven list can justify shrinking an assignment none of
// them is proven to own.
func TestObserveDeletingUnjoined(t *testing.T) {
	type want struct {
		o                   managed.ExternalObservation
		wantDeletionRefused bool
		wantErr             error
	}

	var cases = map[string]struct {
		cr       *v1alpha1.Entitlement
		instance *entitlement2.Instance
		siblings []*v1alpha1.Entitlement
		listErr  error
		want     want
	}{
		"leak shape: no remaining siblings, share still reserved, must not finalize silently": {
			cr: entitlement(withUID("cr-1"), withAmount(4), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				// CR-A (the only sibling that ever proved ownership) has
				// already finalized and is gone: BTP's assigned amount
				// (4) is exactly this CR's own never-annotated share,
				// which MergeRelatedEntitlements had already folded into
				// CR-A's write while CR-A was still around.
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(4)), EntityState: internal.Ptr("OK")},
			},
			want: want{
				o:                   managed.ExternalObservation{},
				wantDeletionRefused: true,
			},
		},
		"leak shape: enable-based, no remaining siblings, share still reserved, must not finalize silently": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				// Amount nil + UnlimitedAmountAssigned is this codebase's
				// enable-based representation of a reserved assignment
				// (see assignmentStillReserved): CR-A already finalized
				// through the enable-based branch with no write, so this
				// is the identical leak shape as the numeric case above,
				// just for an "enable: true" plan.
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: nil, EntityState: internal.Ptr("OK"), UnlimitedAmountAssigned: internal.Ptr(true)},
			},
			want: want{
				o:                   managed.ExternalObservation{},
				wantDeletionRefused: true,
			},
		},
		"leak shape: enable-based with zero amount, no remaining siblings, share still reserved, must not finalize silently": {
			cr: entitlement(withUID("cr-1"), withEnabled(true), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				// Amount present and non-positive (0) alongside
				// UnlimitedAmountAssigned: true -- the shape
				// assignmentStillReserved's pre-fix Amount==nil-only test
				// missed entirely, since a non-nil Amount short-circuited
				// straight to the numeric comparison and never consulted
				// the flag.
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(0)), EntityState: internal.Ptr("OK"), UnlimitedAmountAssigned: internal.Ptr(true)},
			},
			want: want{
				o:                   managed.ExternalObservation{},
				wantDeletionRefused: true,
			},
		},
		"no remaining siblings, nothing reserved (zero amount), finalizes silently": {
			cr: entitlement(withUID("cr-1"), withAmount(2), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(0)), EntityState: internal.Ptr("OK")},
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		"leak shape: remaining sibling exists but unproven, share still carried, must not finalize silently": {
			cr: entitlement(withUID("cr-1"), withAmount(2), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				// BTP still has the combined amount (2+3=5): the sibling's
				// own required sum (3) has not absorbed this CR's
				// reduction, and the sibling never proved ownership, so
				// writing the reduction would touch an assignment adoption
				// was denied for.
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(5)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("sibling-cr"), withUID("cr-2"), withAmount(3)),
			},
			want: want{
				o:                   managed.ExternalObservation{},
				wantDeletionRefused: true,
			},
		},
		"remaining sibling proves ownership, share still carried, falls through to ordinary deletion path": {
			cr: entitlement(withUID("cr-1"), withAmount(2), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				// Identical BTP shape to the leak shape above (2+3=5), but
				// this sibling carries the compound key, so mayAdopt
				// proves the aggregate and cr joins instead of reaching
				// resolveUnjoinedDeletion.
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(5)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("sibling-cr"), withUID("cr-2"), withAmount(3), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				// ResourceExists:true, ResourceUpToDate:true lets Delete()
				// run next and issue the reduction, not a silent finalize.
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
			},
		},
		"remaining sibling exists, BTP already reduced to sibling sum, finalizes silently": {
			cr: entitlement(withUID("cr-1"), withAmount(2), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				// BTP already reduced to exactly the sibling's own sum (3).
				Assignment: &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(3)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("sibling-cr"), withUID("cr-2"), withAmount(3)),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		// resolveUnjoinedDeletion issues its own findRelatedEntitlements
		// call (a third kube.List, after updateObservationFrom's and
		// mayAdopt's); listErr lets the first two succeed and fails only
		// the third, so the error is attributed to this call specifically.
		"empty name, resolveUnjoinedDeletion's own kube.List fails, error propagated not swallowed": {
			cr: entitlement(withUID("cr-1"), withAmount(4), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(4)), EntityState: internal.Ptr("OK")},
			},
			listErr: errors.New(errKubeAPI),
			want: want{
				o:       managed.ExternalObservation{},
				wantErr: errors.Wrap(errors.Wrap(errors.Wrap(errors.New(errKubeAPI), errListEntitlements), errFindRelated), errUpdateObservation),
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var createCalled, updateCalled, deleteCalled, kubeUpdateCalled bool
				mockClient := fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						return tc.instance, nil
					},
					MockCreateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						createCalled = true
						return nil
					},
					MockUpdateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						updateCalled = true
						return nil
					},
					MockDeleteInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						deleteCalled = true
						return nil
					},
				}
				all := append([]*v1alpha1.Entitlement{tc.cr}, tc.siblings...)
				listFn := test.NewMockListFn(nil, ListEntitlements(all...))
				if tc.listErr != nil {
					calls := 0
					listFn = func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
						calls++
						if calls <= 2 {
							return ListEntitlements(all...)(obj)
						}
						return tc.listErr
					}
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         listFn, // no sibling proves ownership
					MockUpdate: test.NewMockUpdateFn(nil, func(obj client.Object) error {
						kubeUpdateCalled = true
						return nil
					}),
				}

				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}
				got, err := e.Observe(context.Background(), tc.cr)

				switch {
				case tc.want.wantDeletionRefused:
					if diff := compareErrorMessages(err, errUnownedAssignmentBlocksFinalize); diff != "" {
						t.Errorf("\ne.Observe(...): -want error %s, +got error:\n%s\n", errUnownedAssignmentBlocksFinalize, err)
					}
					if !errors.Is(err, errUnownedAssignmentBlocksFinalize) {
						t.Errorf("errors.Is(err, errUnownedAssignmentBlocksFinalize) = false, want true (err: %v)", err)
					}
					// The deleting-CR refusal must never be (or resemble)
					// the annotation-instructing adoption error: following
					// that instruction on a deleting CR would flip
					// keyForObserve to externalNameCurrent, skip this
					// guard entirely, and let Delete() fully remove the
					// very assignment this refusal exists to protect.
					if errors.Is(err, errExistingAssignmentRequiresAdoption) {
						t.Errorf("a deleting CR must never surface errExistingAssignmentRequiresAdoption (its annotation remedy is destructive here): err = %v", err)
					}
					if err != nil && strings.Contains(err.Error(), "adopt") {
						t.Errorf("deleting-CR refusal must not instruct annotation-based adoption, got: %v", err)
					}
					if err != nil && !strings.Contains(err.Error(), "finalizer") {
						t.Errorf("deleting-CR refusal must point at removing the finalizer as the non-destructive remedy, got: %v", err)
					}
					if err != nil && !strings.Contains(err.Error(), "crossplane.io/external-name") {
						t.Errorf("deleting-CR refusal must name the ownership-fork remedy (setting crossplane.io/external-name) for a genuinely owned assignment, got: %v", err)
					}
				case tc.want.wantErr != nil:
					if diff := compareErrorMessages(err, tc.want.wantErr); diff != "" {
						t.Errorf("\ne.Observe(...): -want error %s, +got error:\n%s\n", tc.want.wantErr, err)
					}
				case err != nil:
					t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
				}
				if createCalled || updateCalled || deleteCalled {
					t.Errorf("Observe must never write to BTP: create=%v update=%v delete=%v", createCalled, updateCalled, deleteCalled)
				}
				if kubeUpdateCalled {
					t.Errorf("a deleting, unjoined CR must not persist an external-name")
				}
				if got := meta.GetExternalName(tc.cr); got != "" {
					t.Errorf("final external-name annotation = %q, want empty", got)
				}
			},
		)
	}
}

// TestResolveUnjoinedDeletionMergeRelatedError proves
// resolveUnjoinedDeletion's >=1-sibling branch wraps
// MergeRelatedEntitlements' failure as errMergeRelated. It calls
// resolveUnjoinedDeletion directly rather than through Observe(), since
// the function only issues one findRelatedEntitlements lookup.
func TestResolveUnjoinedDeletionMergeRelatedError(t *testing.T) {
	cr := entitlement(withUID("cr-1"), withAmount(4), withDeletionTimestamp(), withConditions(xpv1.Deleting()))
	mockKube := &test.MockClient{
		MockStatusUpdate: noopStatusUpdate,
		MockList: test.NewMockListFn(nil, ListEntitlements(
			entitlement(withName("sibling-enable-true"), withUID("cr-2"), withEnabled(true)),
			entitlement(withName("sibling-enable-false"), withUID("cr-3"), withEnabled(false)),
		)),
	}
	e := external{kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}

	obs, err := e.resolveUnjoinedDeletion(context.Background(), cr)

	wantErr := errors.Wrap(errors.New("multiple of kind Entitlement have colliding .Spec.ForProvider.Enable"), errMergeRelated)
	if diff := compareErrorMessages(err, wantErr); diff != "" {
		t.Errorf("\ne.resolveUnjoinedDeletion(...): -want error %s, +got error:\n%s\n", wantErr, err)
	}
	if obs != nil {
		t.Errorf("e.resolveUnjoinedDeletion(...) observation = %v, want nil", obs)
	}
}

// TestObserveAutoAssignedDeleting proves a deleting AutoAssigned
// entitlement reports ResourceExists:false with a preservation Event and
// never attempts a BTP write, whether or not it already joined via a
// persisted compound key. Without this carve-out, deletionComplete never
// reports true for a solo AutoAssigned CR, since the client-level choke
// point silently suppresses the write Delete() would issue. The
// PROCESSING_FAILED cases confirm the Event still fires ahead of
// observeExternalName's assignFailedNoQuota shortcut (see
// deletingAutoAssigned).
func TestObserveAutoAssignedDeleting(t *testing.T) {
	okInstance := &entitlement2.Instance{
		EntitledServicePlan: &entclient.ServicePlanResponseObject{},
		Assignment: &entclient.AssignedServicePlanSubaccountDTO{
			Amount:       internal.Ptr(float32(5)),
			EntityState:  internal.Ptr("OK"),
			AutoAssigned: internal.Ptr(true),
		},
	}
	// processingFailedInstance reports nothing actually reserved
	// (PROCESSING_FAILED, nil amount) alongside AutoAssigned.
	processingFailedInstance := &entitlement2.Instance{
		EntitledServicePlan: &entclient.ServicePlanResponseObject{},
		Assignment: &entclient.AssignedServicePlanSubaccountDTO{
			Amount:       nil,
			EntityState:  internal.Ptr("PROCESSING_FAILED"),
			AutoAssigned: internal.Ptr(true),
		},
	}

	cases := map[string]struct {
		cr       *v1alpha1.Entitlement
		instance *entitlement2.Instance
	}{
		"already-adopted current-key solo AutoAssigned CR": {
			cr:       entitlement(withUID("cr-1"), withAmount(5), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: okInstance,
		},
		"bare empty-name AutoAssigned CR, never previously observed": {
			cr:       entitlement(withUID("cr-1"), withAmount(5), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: okInstance,
		},
		// observeExternalName's assignFailedNoQuota shortcut (see
		// deletingAutoAssigned) used to return ResourceExists:false for this
		// shape, ahead of ever emitting the Event below.
		"bare empty-name AutoAssigned CR, PROCESSING_FAILED with nothing reserved": {
			cr:       entitlement(withUID("cr-1"), withAmount(5), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
			instance: processingFailedInstance,
		},
		// needsCreate already returns false unconditionally once
		// Assigned.AutoAssigned is true, so this shape never reaches that
		// shortcut; this case pins that the Event still fires anyway.
		"already-adopted current-key AutoAssigned CR, PROCESSING_FAILED with nothing reserved": {
			cr:       entitlement(withUID("cr-1"), withAmount(5), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: processingFailedInstance,
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var createCalled, updateCalled, deleteCalled bool
				mockClient := fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						return tc.instance, nil
					},
					MockCreateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						createCalled = true
						return nil
					},
					MockUpdateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						updateCalled = true
						return nil
					},
					MockDeleteInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						deleteCalled = true
						return nil
					},
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(tc.cr)),
					MockUpdate:       test.NewMockUpdateFn(nil),
				}
				recorder := &recorderFake{}

				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

				// Observe twice: crossplane-runtime only clears the
				// finalizer after a reconcile that reports
				// ResourceExists:false, so a real loop would call Observe
				// repeatedly until it does. Calling it twice here shows the
				// carve-out is deterministic and idempotent rather than a
				// one-shot fluke -- which is what lets the runtime stop
				// calling Delete instead of looping it forever.
				for attempt := 1; attempt <= 2; attempt++ {
					got, err := e.Observe(context.Background(), tc.cr)
					if err != nil {
						t.Fatalf("attempt %d: e.Observe(...) returned unexpected error: %v", attempt, err)
					}
					if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: false}, got); diff != "" {
						t.Errorf("attempt %d: e.Observe(...): -want, +got:\n%s\n", attempt, diff)
					}
				}

				if createCalled || updateCalled || deleteCalled {
					t.Errorf("an AutoAssigned CR must never write to BTP: create=%v update=%v delete=%v", createCalled, updateCalled, deleteCalled)
				}
				if len(recorder.events) != 2 {
					t.Fatalf("recorded %d events across 2 Observe calls, want 2 (one preservation Normal event each)", len(recorder.events))
				}
				for i, ev := range recorder.events {
					if ev.Type != event.TypeNormal {
						t.Errorf("event[%d].Type = %v, want %v", i, ev.Type, event.TypeNormal)
					}
					if ev.Reason != event.Reason(reasonAutoAssignedPreserved) {
						t.Errorf("event[%d].Reason = %v, want %v", i, ev.Reason, reasonAutoAssignedPreserved)
					}
					if ev.Message != "BTP auto-assigned entitlement remains available and was not modified" {
						t.Errorf("event[%d].Message = %q, want the preservation message", i, ev.Message)
					}
				}
			},
		)
	}
}

// TestObserveAutoAssignedConverges proves a
// PROCESSING_FAILED, nil/zero-amount AutoAssigned entitlement converges
// (ResourceExists:true) instead of looping Create forever, which
// happened before needsCreate special-cased AutoAssigned.
func TestObserveAutoAssignedConverges(t *testing.T) {
	cases := map[string]struct {
		amount   *float32
		wantDiff string
	}{
		"nil amount": {amount: nil, wantDiff: "enable mismatch (desired=true, observed=false)"},
		// Same enable mismatch as "nil amount": a stray BTP amount
		// alongside UnlimitedAmountAssigned must not route this
		// enable-based CR through the numeric diff branch.
		"zero amount": {amount: internal.Ptr(float32(0)), wantDiff: "enable mismatch (desired=true, observed=false)"},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				cr := entitlement(withUID("cr-1"), withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name"))
				mockClient := fake.MockClient{
					MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						return &entitlement2.Instance{
							EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
							Assignment: &entclient.AssignedServicePlanSubaccountDTO{
								Amount:       tc.amount,
								EntityState:  internal.Ptr("PROCESSING_FAILED"),
								AutoAssigned: internal.Ptr(true),
							},
						}, nil
					},
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
				}

				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}
				got, err := e.Observe(context.Background(), cr)

				if err != nil {
					t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
				}
				// ResourceExists:true is exactly the signal that stops the
				// managed reconciler from calling Create; ResourceExists:false
				// (the pre-fix behavior) is what looped Create forever.
				if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: tc.wantDiff}, got); diff != "" {
					t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
				}
				if status := cr.Status.GetCondition(xpv1.Available().Type).Status; status == xpv1.Available().Status {
					t.Errorf("Ready=True/Available should not be set when BTP reports PROCESSING_FAILED with nothing reserved")
				}
			},
		)
	}
}

// TestCreate exercises Create's guarded fresh read: since Observe's
// decision to call Create can rely on a TTL-cached read, Create
// re-describes with DescribeInstanceFresh and re-runs the same ownership
// guard immediately before writing, for every externalNameState.
func TestCreate(t *testing.T) {
	type want struct {
		err            error
		wantGuardError bool
		// isMismatch computes the wanted error from cr's own annotation
		// and spec via ExternalNameKey.Mismatch instead of a fixed err
		// (mirrors TestObserveExternalName), and additionally asserts
		// DescribeInstanceFresh is never called -- keyForObserve failing
		// must short-circuit before any client call.
		isMismatch            bool
		createCalled          bool
		wantKey               *entitlement2.ExternalNameKey
		wantFinalExternalName string
		wantRequiredAmount    *int
	}

	compoundKey := entitlement2.ExternalNameKey{SubaccountGUID: "subaccount-guid", ServiceName: "service-name", ServicePlanName: "service-plan-name"}

	var cases = map[string]struct {
		cr        *v1alpha1.Entitlement
		instance  *entitlement2.Instance
		siblings  []*v1alpha1.Entitlement
		createErr error
		// describeErr, when set, makes DescribeInstanceFresh itself fail.
		describeErr error
		// mayAdoptListErr, when set, makes the *second* kube.List call
		// fail (the first, from updateObservationFrom, still succeeds),
		// isolating mayAdopt's own findRelatedEntitlements call.
		mayAdoptListErr error
		want            want
	}{
		"empty name, fresh absence, create called, key set only after success": {
			cr:       entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{Assignment: nil},
			want: want{
				createCalled:          true,
				wantKey:               &compoundKey,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"empty name, assignFailedNoQuota, create called": {
			cr: entitlement(withUID("cr-1"), withEnabled(true)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{Category: internal.Ptr("ELASTIC_SERVICE")},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(0)), EntityState: internal.Ptr("PROCESSING_FAILED")},
			},
			want: want{
				createCalled:          true,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"empty name, fresh unmanaged ordinary assignment, guard error, no create, annotation stays empty": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(5)), EntityState: internal.Ptr("OK")},
			},
			want: want{
				wantGuardError:        true,
				createCalled:          false,
				wantFinalExternalName: "",
			},
		},
		"empty name, sibling-managed ordinary assignment, aggregate upsert with full Required, key set on success": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(5)), EntityState: internal.Ptr("OK")},
			},
			siblings: []*v1alpha1.Entitlement{
				entitlement(withName("proving-sibling"), withUID("cr-2"), withAmount(3), withExternalName("subaccount-guid/service-name/service-plan-name")),
			},
			want: want{
				createCalled:          true,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
				wantRequiredAmount:    internal.Ptr(5),
			},
		},
		"empty name, AutoAssigned assignment, no BTP write, key set, succeeds": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(10)), EntityState: internal.Ptr("OK"), AutoAssigned: internal.Ptr(true)},
			},
			want: want{
				createCalled:          false,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"current compound key, fresh absence, create called with parsed key, annotation unchanged": {
			cr:       entitlement(withUID("cr-1"), withAmount(2), withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: &entitlement2.Instance{Assignment: nil},
			want: want{
				createCalled:          true,
				wantKey:               &compoundKey,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"current compound key, fresh ordinary assignment, aggregate upsert allowed as explicit intent": {
			cr: entitlement(withUID("cr-1"), withAmount(2), withExternalName("subaccount-guid/service-name/service-plan-name")),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			want: want{
				createCalled:          true,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"legacy sentinel, fresh ordinary assignment, aggregate upsert allowed as sentinel proof, key set after success": {
			cr: entitlement(withName("legacy-cr"), withUID("cr-1"), withAmount(2), withExternalName("legacy-cr")),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			want: want{
				createCalled:          true,
				wantFinalExternalName: "subaccount-guid/service-name/service-plan-name",
			},
		},
		"create/upsert error, annotation remains unchanged": {
			cr:        entitlement(withUID("cr-1"), withAmount(2)),
			instance:  &entitlement2.Instance{Assignment: nil},
			createErr: errors.New(errClientAPI),
			want: want{
				err:                   errors.Wrap(errors.New(errClientAPI), errCreateInstance),
				createCalled:          true,
				wantFinalExternalName: "",
			},
		},
		// DescribeInstanceFresh failing must wrap as the single
		// errDescribeInstance, not double-wrapped with errUpdateObservation,
		// since updateObservationFrom is never reached on this path.
		"empty name, DescribeInstanceFresh fails, single-wrapped errDescribeInstance, no create": {
			cr:          entitlement(withUID("cr-1"), withAmount(2)),
			describeErr: errors.New(errClientAPI),
			want: want{
				err:                   errors.Wrap(errors.New(errClientAPI), errDescribeInstance),
				createCalled:          false,
				wantFinalExternalName: "",
			},
		},
		// A keyForObserve spec-mismatch must wrap as errResolveIdentity,
		// not errUpdateObservation: DescribeInstanceFresh is never called
		// at this point.
		"current compound key mismatch, keyForObserve error wrapped as errResolveIdentity, no describe call": {
			cr: entitlement(withUID("cr-1"), withAmount(2), withExternalName("wrong-guid/service-name/service-plan-name")),
			want: want{
				isMismatch:            true,
				createCalled:          false,
				wantFinalExternalName: "wrong-guid/service-name/service-plan-name",
			},
		},
		// mayAdopt's own findRelatedEntitlements call fails after
		// updateObservationFrom's earlier call already succeeded; Create
		// must propagate the error unwrapped, not swallow it into an
		// empty sibling list.
		"empty name, mayAdopt's kube.List fails, error propagated not swallowed": {
			cr: entitlement(withUID("cr-1"), withAmount(2)),
			instance: &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			},
			mayAdoptListErr: errors.New(errKubeAPI),
			want: want{
				err:                   errors.Wrap(errors.Wrap(errors.New(errKubeAPI), errListEntitlements), errFindRelated),
				createCalled:          false,
				wantFinalExternalName: "",
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var createCalled, describeCalled bool
				var gotKey entitlement2.ExternalNameKey
				var gotCR *v1alpha1.Entitlement
				var kubeUpdateCalled bool

				mockClient := fake.MockClient{
					MockDescribeInstanceFreshFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
						describeCalled = true
						return tc.instance, tc.describeErr
					},
					MockCreateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
						createCalled = true
						gotKey = key
						gotCR = cr
						return tc.createErr
					},
				}
				all := append([]*v1alpha1.Entitlement{tc.cr}, tc.siblings...)
				listFn := test.NewMockListFn(nil, ListEntitlements(all...))
				if tc.mayAdoptListErr != nil {
					listFn = listOnceThenErr(ListEntitlements(all...), tc.mayAdoptListErr)
				}
				mockKube := &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         listFn,
					MockUpdate: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						kubeUpdateCalled = true
						return nil
					},
				}

				e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}
				_, err := e.Create(context.Background(), tc.cr)

				wantErr := tc.want.err
				if tc.want.isMismatch {
					key, perr := entitlement2.ParseExternalName(meta.GetExternalName(tc.cr))
					if perr != nil {
						t.Fatalf("test setup: ParseExternalName(%q): %v", meta.GetExternalName(tc.cr), perr)
					}
					wantErr = errors.Wrap(errors.Wrap(entitlement2.ErrExternalNameSpecMismatch, key.Mismatch(tc.cr)), errResolveIdentity)
				}
				if tc.want.wantGuardError {
					if !errors.Is(err, errExistingAssignmentRequiresAdoption) {
						t.Errorf("errors.Is(err, errExistingAssignmentRequiresAdoption) = false, want true (err: %v)", err)
					}
				} else if diff := compareErrorMessages(err, wantErr); diff != "" {
					t.Errorf("\ne.Create(...): -want error %s, +got error:\n%s\n", wantErr, err)
				}
				if tc.want.isMismatch && describeCalled {
					t.Errorf("DescribeInstanceFresh must not be called when keyForObserve fails")
				}
				if createCalled != tc.want.createCalled {
					t.Errorf("CreateInstance called = %v, want %v", createCalled, tc.want.createCalled)
				}
				if createCalled && tc.want.wantKey != nil {
					if diff := cmp.Diff(*tc.want.wantKey, gotKey); diff != "" {
						t.Errorf("\nCreateInstance key: -want +got:\n%s\n", diff)
					}
				}
				if kubeUpdateCalled {
					t.Errorf("Create must never call kube.Update")
				}
				if got := meta.GetExternalName(tc.cr); got != tc.want.wantFinalExternalName {
					t.Errorf("final external-name annotation = %q, want %q", got, tc.want.wantFinalExternalName)
				}
				if tc.want.wantRequiredAmount != nil {
					if gotCR == nil || gotCR.Status.AtProvider == nil || gotCR.Status.AtProvider.Required == nil {
						t.Fatalf("CreateInstance's cr.Status.AtProvider.Required is nil, want Amount=%d", *tc.want.wantRequiredAmount)
					}
					if diff := cmp.Diff(tc.want.wantRequiredAmount, gotCR.Status.AtProvider.Required.Amount); diff != "" {
						t.Errorf("\nCreateInstance cr.Status.AtProvider.Required.Amount: -want, +got:\n%s\n", diff)
					}
				}
			},
		)
	}
}

// TestEmptyAnnotationCreateThenObserveConverges drives a full
// Observe -> Create -> Observe sequence for a freshly created
// Entitlement: the first Observe reports absent, Create stamps the
// compound external-name, and the second Observe resolves via
// externalNameCurrent with no adoption error.
func TestEmptyAnnotationCreateThenObserveConverges(t *testing.T) {
	cr := entitlement(withUID("cr-1"), withAmount(2))

	var created bool
	mockClient := fake.MockClient{
		MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
			if !created {
				return &entitlement2.Instance{Assignment: nil}, nil
			}
			return &entitlement2.Instance{
				EntitledServicePlan: &entclient.ServicePlanResponseObject{},
				Assignment:          &entclient.AssignedServicePlanSubaccountDTO{Amount: internal.Ptr(float32(2)), EntityState: internal.Ptr("OK")},
			}, nil
		},
		MockDescribeInstanceFreshFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
			return &entitlement2.Instance{Assignment: nil}, nil
		},
		MockCreateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
			created = true
			return nil
		},
	}
	mockKube := &test.MockClient{
		MockStatusUpdate: noopStatusUpdate,
		MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
	}
	e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}}

	firstObs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("first e.Observe(...) returned unexpected error: %v", err)
	}
	if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: false}, firstObs); diff != "" {
		t.Fatalf("first e.Observe(...): -want, +got:\n%s\n", diff)
	}
	if got := meta.GetExternalName(cr); got != "" {
		t.Fatalf("external-name before Create = %q, want empty", got)
	}

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("e.Create(...) returned unexpected error: %v", err)
	}
	if got, want := meta.GetExternalName(cr), "subaccount-guid/service-name/service-plan-name"; got != want {
		t.Fatalf("external-name after Create = %q, want %q", got, want)
	}

	secondObs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("second e.Observe(...) returned unexpected error (this is exactly the break-1 loop): %v", err)
	}
	if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, secondObs); diff != "" {
		t.Errorf("second e.Observe(...): -want, +got:\n%s\n", diff)
	}
}

// TestCalculateDiff exercises calculateDiff in isolation from Observe:
// the aggregate desired state (status.atProvider.required) against what
// BTP reports (status.atProvider.assigned). Fixtures use
// withRequiredAssigned to set both sides directly, since calculateDiff
// never reads spec.forProvider.
func TestCalculateDiff(t *testing.T) {
	cases := map[string]struct {
		cr   *v1alpha1.Entitlement
		want string
	}{
		"two siblings summing to 5 (this CR contributes 2), assigned 5, empty diff": {
			cr: entitlement(withAmount(2), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Amount: internal.Ptr(5)},
				&v1alpha1.Assignable{Amount: internal.Ptr(5)},
			)),
			want: "",
		},
		"same aggregate, assigned 4, identical non-empty diff (this CR contributes 2)": {
			cr: entitlement(withAmount(2), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Amount: internal.Ptr(5)},
				&v1alpha1.Assignable{Amount: internal.Ptr(4)},
			)),
			want: "amount mismatch (desired=5, observed=4)",
		},
		"numeric nil desired vs non-nil observed, mismatch": {
			cr: entitlement(withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Amount: nil},
				&v1alpha1.Assignable{Amount: internal.Ptr(3)},
			)),
			want: "amount mismatch (desired=<unset>, observed=3)",
		},
		"numeric non-nil desired vs nil observed, mismatch": {
			cr: entitlement(withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Amount: internal.Ptr(3)},
				&v1alpha1.Assignable{Amount: nil},
			)),
			want: "amount mismatch (desired=3, observed=<unset>)",
		},
		"enable true vs UnlimitedAmountAssigned false, mismatch": {
			cr: entitlement(withEnabled(true), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Enable: internal.Ptr(true)},
				&v1alpha1.Assignable{UnlimitedAmountAssigned: false},
			)),
			want: "enable mismatch (desired=true, observed=false)",
		},
		"enable match, empty diff": {
			cr: entitlement(withEnabled(true), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Enable: internal.Ptr(true)},
				&v1alpha1.Assignable{UnlimitedAmountAssigned: true},
			)),
			want: "",
		},
		// An enable-based aggregate (Required.Amount nil) whose Enable
		// agrees with UnlimitedAmountAssigned must report no diff even
		// when BTP's assignment also carries a stray numeric Amount.
		"enable-based aggregate agrees despite assigned amount present, empty diff": {
			cr: entitlement(withEnabled(true), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Enable: internal.Ptr(true)},
				&v1alpha1.Assignable{UnlimitedAmountAssigned: true, Amount: internal.Ptr(0)},
			)),
			want: "",
		},
		// Same shape as above but disagreeing: must still report the enable
		// mismatch, not an amount mismatch, regardless of the stray amount.
		"enable-based aggregate disagrees despite assigned amount present, enable-mismatch message": {
			cr: entitlement(withEnabled(true), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Enable: internal.Ptr(true)},
				&v1alpha1.Assignable{UnlimitedAmountAssigned: false, Amount: internal.Ptr(0)},
			)),
			want: "enable mismatch (desired=true, observed=false)",
		},
		"nil desired enable never matches, even against a false observed flag": {
			cr: entitlement(withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Enable: nil},
				&v1alpha1.Assignable{UnlimitedAmountAssigned: false},
			)),
			want: "enable mismatch (desired=<unset>, observed=false)",
		},
		"AutoAssigned mismatch still returns diff even though writes are suppressed": {
			cr: entitlement(withAmount(5), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Amount: internal.Ptr(5)},
				&v1alpha1.Assignable{Amount: internal.Ptr(3), AutoAssigned: true},
			)),
			want: "amount mismatch (desired=5, observed=3)",
		},
		"absent assignment returns empty diff": {
			cr: entitlement(withAmount(5), withRequiredAssigned(
				&v1alpha1.EntitlementSummary{Amount: internal.Ptr(5)},
				nil,
			)),
			want: "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := calculateDiff(tc.cr)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("calculateDiff(...): -want, +got:\n%s", diff)
			}
		})
	}
}

// assertDriftCondition asserts cr's Drift condition matches
// DriftDetected(wantMessage) when wantDetected, or NoDrift() otherwise,
// comparing Status/Reason/Message individually so LastTransitionTime
// never needs to be neutralized.
func assertDriftCondition(t *testing.T, cr *v1alpha1.Entitlement, wantMessage string, wantDetected bool) {
	t.Helper()
	want := v1alpha1.NoDrift()
	if wantDetected {
		want = v1alpha1.DriftDetected(wantMessage)
	}
	got := cr.Status.GetCondition(v1alpha1.DriftConditionType)
	if got.Status != want.Status || got.Reason != want.Reason || got.Message != want.Message {
		t.Errorf("Drift condition = %+v, want Status=%v Reason=%v Message=%q", got, want.Status, want.Reason, want.Message)
	}
}

// TestObserveDrift exercises Observe's aggregate Drift condition/Event
// wiring end to end: placement relative to needsCreate/the AutoAssigned-
// deleting carve-out/the generic deletion branch/needsUpdate, the
// dedupe-against-previously-persisted-message rule, and the two skip
// conditions (assignment absent, cr deleting).
func TestObserveDrift(t *testing.T) {
	t.Run("actionable drift: needsUpdate=true returns ResourceUpToDate=false with Diff set, condition and event recorded before that early return", func(t *testing.T) {
		cr := entitlement(withUID("cr-1"), withAmount(5), withExternalName("subaccount-guid/service-name/service-plan-name"))
		mockClient := fake.MockClient{
			MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
				return &entitlement2.Instance{
					EntitledServicePlan: &entclient.ServicePlanResponseObject{},
					Assignment: &entclient.AssignedServicePlanSubaccountDTO{
						Amount:      internal.Ptr(float32(4)),
						EntityState: internal.Ptr("OK"),
					},
				}, nil
			},
		}
		mockKube := &test.MockClient{
			MockStatusUpdate: noopStatusUpdate,
			MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
		}
		recorder := &recorderFake{}
		e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

		const wantDiff = "amount mismatch (desired=5, observed=4)"
		got, err := e.Observe(context.Background(), cr)
		if err != nil {
			t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
		}
		if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, Diff: wantDiff}, got); diff != "" {
			t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
		}
		assertDriftCondition(t, cr, wantDiff, true)
		if len(recorder.events) != 1 {
			t.Fatalf("recorded %d events, want 1", len(recorder.events))
		}
		if ev := recorder.events[0]; ev.Type != event.TypeWarning || ev.Reason != event.Reason(v1alpha1.DriftDetectedReason) || ev.Message != wantDiff {
			t.Errorf("event = %+v, want Warning/DriftDetected/%q", ev, wantDiff)
		}
	})

	t.Run("suppressed drift: enable-based CR (needsUpdate always false) still returns ResourceUpToDate=true with Diff set, no BTP write, and the Ready switch does not clear the Drift condition", func(t *testing.T) {
		cr := entitlement(withUID("cr-1"), withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name"))
		var createCalled, updateCalled, deleteCalled bool
		mockClient := fake.MockClient{
			MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
				return &entitlement2.Instance{
					EntitledServicePlan: &entclient.ServicePlanResponseObject{},
					Assignment: &entclient.AssignedServicePlanSubaccountDTO{
						UnlimitedAmountAssigned: internal.Ptr(false),
						EntityState:             internal.Ptr("OK"),
					},
				}, nil
			},
			MockCreateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
				createCalled = true
				return nil
			},
			MockUpdateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
				updateCalled = true
				return nil
			},
			MockDeleteInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
				deleteCalled = true
				return nil
			},
		}
		mockKube := &test.MockClient{
			MockStatusUpdate: noopStatusUpdate,
			MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
		}
		recorder := &recorderFake{}
		e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

		const wantDiff = "enable mismatch (desired=true, observed=false)"
		got, err := e.Observe(context.Background(), cr)
		if err != nil {
			t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
		}
		if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: wantDiff}, got); diff != "" {
			t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
		}
		if createCalled || updateCalled || deleteCalled {
			t.Errorf("Observe must never write to BTP: create=%v update=%v delete=%v", createCalled, updateCalled, deleteCalled)
		}
		assertDriftCondition(t, cr, wantDiff, true)
		if readyStatus := cr.Status.GetCondition(xpv1.Available().Type).Status; readyStatus != xpv1.Available().Status {
			t.Errorf("Ready condition status = %v, want %v -- setting Drift must not suppress the normal status switch", readyStatus, xpv1.Available().Status)
		}
		if len(recorder.events) != 1 {
			t.Fatalf("recorded %d events, want 1", len(recorder.events))
		}
	})

	t.Run("agreeing enable-based aggregate whose BTP assignment also carries an amount: needsCreate/assignFailedNoQuota do not block reaching drift, calculateDiff still prefers the enable comparison, so ResourceUpToDate=true with an empty Diff, NoDrift, and zero events", func(t *testing.T) {
		// Mirrors the "suppressed drift" fixture above, but the desired
		// enable AGREES with UnlimitedAmountAssigned even though the
		// assignment also reports a numeric Amount. Pins that such a CR
		// still reaches calculateDiff and produces no condition churn or
		// event, not just an empty diff string in isolation.
		cr := entitlement(withUID("cr-1"), withEnabled(true), withExternalName("subaccount-guid/service-name/service-plan-name"))
		var createCalled, updateCalled, deleteCalled bool
		mockClient := fake.MockClient{
			MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
				return &entitlement2.Instance{
					EntitledServicePlan: &entclient.ServicePlanResponseObject{},
					Assignment: &entclient.AssignedServicePlanSubaccountDTO{
						UnlimitedAmountAssigned: internal.Ptr(true),
						Amount:                  internal.Ptr(float32(0)),
						EntityState:             internal.Ptr("OK"),
					},
				}, nil
			},
			MockCreateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
				createCalled = true
				return nil
			},
			MockUpdateInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
				updateCalled = true
				return nil
			},
			MockDeleteInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey, cr *v1alpha1.Entitlement) error {
				deleteCalled = true
				return nil
			},
		}
		mockKube := &test.MockClient{
			MockStatusUpdate: noopStatusUpdate,
			MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
		}
		recorder := &recorderFake{}
		e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

		got, err := e.Observe(context.Background(), cr)
		if err != nil {
			t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
		}
		if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: ""}, got); diff != "" {
			t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
		}
		if createCalled || updateCalled || deleteCalled {
			t.Errorf("Observe must never write to BTP: create=%v update=%v delete=%v", createCalled, updateCalled, deleteCalled)
		}
		assertDriftCondition(t, cr, "", false)
		if len(recorder.events) != 0 {
			t.Errorf("recorded %d events, want 0 -- an agreeing enable-based aggregate must never emit", len(recorder.events))
		}
	})

	t.Run("event dedupe across polls: first diff emits once, an unchanged diff does not re-emit, clearing sets NoDrift with an empty message and no event, and the identical diff reappearing after a clear emits again", func(t *testing.T) {
		cr := entitlement(withUID("cr-1"), withAmount(5), withExternalName("subaccount-guid/service-name/service-plan-name"))
		assignedAmount := float32(4)
		mockClient := fake.MockClient{
			MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
				return &entitlement2.Instance{
					EntitledServicePlan: &entclient.ServicePlanResponseObject{},
					Assignment: &entclient.AssignedServicePlanSubaccountDTO{
						Amount:      internal.Ptr(assignedAmount),
						EntityState: internal.Ptr("OK"),
					},
				}, nil
			},
		}
		mockKube := &test.MockClient{
			MockStatusUpdate: noopStatusUpdate,
			MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
		}
		recorder := &recorderFake{}
		e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

		const driftMessage = "amount mismatch (desired=5, observed=4)"

		// Call 1: assigned=4 vs desired=5 -- first diff ever seen, emits.
		if _, err := e.Observe(context.Background(), cr); err != nil {
			t.Fatalf("call 1: e.Observe(...) returned unexpected error: %v", err)
		}
		assertDriftCondition(t, cr, driftMessage, true)
		if len(recorder.events) != 1 {
			t.Fatalf("after call 1: recorded %d events, want 1", len(recorder.events))
		}

		// Call 2: assigned still 4 -- identical diff, must not re-emit.
		if _, err := e.Observe(context.Background(), cr); err != nil {
			t.Fatalf("call 2: e.Observe(...) returned unexpected error: %v", err)
		}
		assertDriftCondition(t, cr, driftMessage, true)
		if len(recorder.events) != 1 {
			t.Fatalf("after call 2 (unchanged diff): recorded %d events, want still 1", len(recorder.events))
		}

		// Call 3: assigned now matches desired (5) -- clears to NoDrift
		// with an empty message; clearing itself never emits.
		assignedAmount = 5
		if _, err := e.Observe(context.Background(), cr); err != nil {
			t.Fatalf("call 3: e.Observe(...) returned unexpected error: %v", err)
		}
		assertDriftCondition(t, cr, "", false)
		if len(recorder.events) != 1 {
			t.Fatalf("after call 3 (cleared): recorded %d events, want still 1 (clearing never emits)", len(recorder.events))
		}

		// Call 4: assigned drops back to 4 -- text is identical to calls
		// 1-2, but the immediately prior persisted message was "" (set by
		// call 3's NoDrift), so this must emit again: dedupe compares
		// against the last persisted message only, never a historical log
		// of every diff ever seen.
		assignedAmount = 4
		if _, err := e.Observe(context.Background(), cr); err != nil {
			t.Fatalf("call 4: e.Observe(...) returned unexpected error: %v", err)
		}
		assertDriftCondition(t, cr, driftMessage, true)
		if len(recorder.events) != 2 {
			t.Fatalf("after call 4 (diff reappears after a clear): recorded %d events, want 2", len(recorder.events))
		}
	})

	t.Run("absent assignment: Drift condition never acquired, no event emitted", func(t *testing.T) {
		cr := entitlement(withUID("cr-1"), withAmount(5), withExternalName("subaccount-guid/service-name/service-plan-name"))
		mockClient := fake.MockClient{
			MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
				return &entitlement2.Instance{EntitledServicePlan: &entclient.ServicePlanResponseObject{}, Assignment: nil}, nil
			},
		}
		mockKube := &test.MockClient{
			MockStatusUpdate: noopStatusUpdate,
			MockList:         test.NewMockListFn(nil, ListEntitlements(cr)),
		}
		recorder := &recorderFake{}
		e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

		got, err := e.Observe(context.Background(), cr)
		if err != nil {
			t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
		}
		if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: false}, got); diff != "" {
			t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
		}
		if reason := cr.Status.GetCondition(v1alpha1.DriftConditionType).Reason; reason != "" {
			t.Errorf("Drift condition acquired with reason %q, want absent (never set) when the assignment itself is absent", reason)
		}
		if len(recorder.events) != 0 {
			t.Errorf("recorded %d events, want 0", len(recorder.events))
		}
	})

	t.Run("deleting CR with an actionable amount mismatch: Drift condition never acquired, no event emitted", func(t *testing.T) {
		cr := entitlement(withUID("cr-1"), withAmount(5), withDeletionTimestamp(), withConditions(xpv1.Deleting()), withExternalName("subaccount-guid/service-name/service-plan-name"))
		mockClient := fake.MockClient{
			MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
				return &entitlement2.Instance{
					EntitledServicePlan: &entclient.ServicePlanResponseObject{},
					Assignment: &entclient.AssignedServicePlanSubaccountDTO{
						// Mismatches the CR's own desired 5 -- exactly the
						// shape that would be actionable drift if this CR
						// were not deleting.
						Amount:      internal.Ptr(float32(4)),
						EntityState: internal.Ptr("OK"),
					},
				}, nil
			},
		}
		mockKube := &test.MockClient{
			MockStatusUpdate: noopStatusUpdate,
			// No siblings: deletionComplete's zero-item shortcut reports
			// "not complete" unconditionally, so this falls to "let
			// Delete() handle it" -- inside the deletion branch, strictly
			// before drift is ever computed.
			MockList: test.NewMockListFn(nil, ListEntitlements()),
		}
		recorder := &recorderFake{}
		e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

		got, err := e.Observe(context.Background(), cr)
		if err != nil {
			t.Fatalf("e.Observe(...) returned unexpected error: %v", err)
		}
		if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, got); diff != "" {
			t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
		}
		if reason := cr.Status.GetCondition(v1alpha1.DriftConditionType).Reason; reason != "" {
			t.Errorf("Drift condition acquired with reason %q, want absent (never set) for a deleting CR", reason)
		}
		if len(recorder.events) != 0 {
			t.Errorf("recorded %d events, want 0", len(recorder.events))
		}
	})

	t.Run("two amount-bearing siblings whose aggregate equals the assignment produce an empty diff and no event on either CR", func(t *testing.T) {
		crA := entitlement(withName("cr-a"), withUID("cr-a"), withServiceName("Alpha"), withServicePlan("One"), withSubaccountGuid("a"), withAmount(2), withExternalName("a/Alpha/One"))
		crB := entitlement(withName("cr-b"), withUID("cr-b"), withServiceName("Alpha"), withServicePlan("One"), withSubaccountGuid("a"), withAmount(3), withExternalName("a/Alpha/One"))
		mockClient := fake.MockClient{
			MockDescribeInstanceFn: func(ctx context.Context, key entitlement2.ExternalNameKey) (*entitlement2.Instance, error) {
				return &entitlement2.Instance{
					EntitledServicePlan: &entclient.ServicePlanResponseObject{},
					Assignment: &entclient.AssignedServicePlanSubaccountDTO{
						Amount:      internal.Ptr(float32(5)), // 2 (crA) + 3 (crB), exactly the aggregate
						EntityState: internal.Ptr("OK"),
					},
				}, nil
			},
		}
		mockKube := &test.MockClient{
			MockStatusUpdate: noopStatusUpdate,
			MockList:         test.NewMockListFn(nil, ListEntitlements(crA, crB)),
		}
		recorder := &recorderFake{}
		e := external{client: mockClient, kube: mockKube, tracker: test2.NoOpReferenceResolverTracker{}, recorder: recorder}

		for _, cr := range []*v1alpha1.Entitlement{crA, crB} {
			got, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("e.Observe(%s, ...) returned unexpected error: %v", cr.Name, err)
			}
			if diff := cmp.Diff(managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, Diff: ""}, got); diff != "" {
				t.Errorf("\ne.Observe(%s, ...): -want, +got:\n%s\n", cr.Name, diff)
			}
			assertDriftCondition(t, cr, "", false)
		}
		if len(recorder.events) != 0 {
			t.Errorf("recorded %d events across both CRs, want 0 -- a matching aggregate must never emit", len(recorder.events))
		}
	})
}
