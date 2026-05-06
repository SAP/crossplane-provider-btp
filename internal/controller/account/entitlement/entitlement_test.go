package entitlement

import (
	"context"
	"strings"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	entitlement2 "github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/entitlement/fake"
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
					MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: nil,
						Assignment:          nil,
					}, nil
				}},
				cr: entitlement(),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.Wrap(errors.Wrap(errors.Wrap(errors.New(errKubeAPI), "while listing entitlements"), "while finding related entitlements"), "while updating observation"),
			},
		},
		"Simple Case, unique identifier passed": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withServiceName("hana-cloud"), withUniqueServicePlanIdentifier("a")), entitlement(withServiceName("hana-cloud"), withUniqueServicePlanIdentifier("b")))),
				},
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withServiceName("hana-cloud"), withUniqueServicePlanIdentifier("a")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
			},
		},
		"Simple Case, no additional additional Entitlements, resource does not exist": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate, MockList: test.NewMockListFn(nil, ListEntitlements(entitlement())),
				},
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment:          nil,
					}, nil
				}},
				cr: entitlement(),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withAmount(2)),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
				err: nil,
			},
		},

		"Simple Case, All up-to-date": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate, MockList: test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1)))),
				},
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("OK"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1)),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("STARTED"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1)),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("PROCESSING"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1)),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				comparefn: func(v *v1alpha1.Entitlement) string {
					return cmp.Diff(v.Status.GetCondition(xpv1.Creating().Type).Status, xpv1.Creating().Status)
				},
				err: nil,
			},
		},
		"Simple Case, All up-to-date, unavailable condition": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1)))),
				},
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:      internal.Ptr(float32(1)),
							EntityState: internal.Ptr("PROCESSING_FAILED"),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1)),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				comparefn: func(v *v1alpha1.Entitlement) string {
					return cmp.Diff(v.Status.GetCondition(xpv1.Available().Type).Status, xpv1.Available().Status)
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment:          nil,
					}, nil
				}},
				cr: entitlement(withAmount(1), withConditions(xpv1.Deleting())),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"Needs Deletion, assignment active, needs update": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(1), withConditions(xpv1.Deleting())))),
				},
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withAmount(1), withConditions(xpv1.Deleting())),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							// BTP amount already reduced to sibling sum
							Amount: internal.Ptr(float32(3)),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							// BTP still has the full amount (not yet reduced)
							Amount: internal.Ptr(float32(5)),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(2)),
						},
					}, nil
				}},
				cr: entitlement(withName("sole-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(true), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(2)),
						},
					}, nil
				}},
				cr: entitlement(withName("err-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withDeletionTimestamp(), withConditions(xpv1.Deleting())),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							// BTP still has the old combined amount (2+3=5)
							Amount: internal.Ptr(float32(5)),
						},
					}, nil
				}},
				// CR-A: the active CR being observed (no DeletionTimestamp, no Deleting condition)
				cr: entitlement(withName("cr-a"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a")),
			},
			want: want{
				// Required.Amount should be 2 (only CR-A), not 5 (old combined), triggering an update
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withAmount(1), withSubaccountGuid("a")),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withAmount(1), withSubaccountGuid("a")),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount: internal.Ptr(float32(1)),
						},
					}, nil
				}},
				cr: entitlement(withName("a"), withServiceName("Alpha"), withServicePlan("One"), withEnabled(true), withSubaccountGuid("a")),
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:     internal.Ptr(float32(1)),
							AutoAssign: internal.Ptr(true),
						},
					}, nil
				}},
				cr: entitlement(withAmount(2)),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
			},
		},
		"Amount differs, but its unlimited assigned, All up-to-date": {
			args: args{
				kube: &test.MockClient{
					MockStatusUpdate: noopStatusUpdate,
					MockList:         test.NewMockListFn(nil, ListEntitlements(entitlement(withAmount(2)))),
				},
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{
							Amount:                  internal.Ptr(float32(1)),
							UnlimitedAmountAssigned: internal.Ptr(true),
						},
					}, nil
				}},
				cr: entitlement(withAmount(2)),
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
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

func compareErrorMessages(is error, target error) string {
	if is == nil && target == nil {
		return ""
	}
	return cmp.Diff(is.Error(), target.Error())
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{}, nil
				}},
				cr: entitlement(withAmount(1)),
			},
			want: want{
				containsMessage: internal.Ptr([]string{"Could not find service to be entitled. Check if Global Account is entitled for usage (Control Center)."}),
			},
		},
		"Non Numeric Service entitled, Cr using amount": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Unlimited: internal.Ptr(true),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{},
					}, nil
				}},
				cr: entitlement(withAmount(1)),
			},
			want: want{
				containsMessage: internal.Ptr([]string{"This serviceplan is non numeric, please use .Spec.ForProvider.Enable and omit the use of .Spec.ForProvider.Amount to configure the entitlement"}),
			},
		},
		"Amount and enable is used": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
					return &entitlement2.Instance{
						EntitledServicePlan: &entclient.ServicePlanResponseObject{
							Category: internal.Ptr("ELASTIC_SERVICE"),
						},
						Assignment: &entclient.AssignedServicePlanSubaccountDTO{},
					}, nil
				}},
				cr: entitlement(withAmount(1), withEnabled(true)),
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

func TestDelete(t *testing.T) {
	type args struct {
		cr     *v1alpha1.Entitlement
		client fake.MockClient
		kube   client.Client
	}

	type want struct {
		err            error
		requiredAmount *int
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
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
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withAssignedStatus(internal.Ptr(5), "OK")),
			},
			want: want{
				err:            nil,
				requiredAmount: internal.Ptr(3), // sibling sum
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
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
				cr: entitlement(withName("deleting-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withAssignedStatus(internal.Ptr(9), "OK")),
			},
			want: want{
				err:            nil,
				requiredAmount: internal.Ptr(7), // 3 + 4
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input v1alpha1.Entitlement) (*entitlement2.Instance, error) {
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
				cr: entitlement(withName("sole-cr"), withUID("uid-1"), withServiceName("Alpha"), withServicePlan("One"), withAmount(2), withSubaccountGuid("a"), withAssignedStatus(internal.Ptr(2), "OK")),
			},
			want: want{
				err:            nil,
				requiredAmount: nil, // no siblings → MergeRelatedEntitlements returns nil Amount
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var capturedAmount *int
				deleteCalled := false

				tc.args.client.MockDeleteInstanceFn = func(ctx context.Context, cr *v1alpha1.Entitlement) error {
					deleteCalled = true
					if cr.Status.AtProvider != nil && cr.Status.AtProvider.Required != nil {
						capturedAmount = cr.Status.AtProvider.Required.Amount
					}
					return nil
				}

				e := external{client: tc.args.client, kube: tc.args.kube, tracker: test2.NoOpReferenceResolverTracker{}}
				_, err := e.Delete(context.Background(), tc.args.cr)
				if diff := compareErrorMessages(err, tc.want.err); diff != "" {
					t.Errorf("\ne.Delete(...): -want error %v, +got error:\n%s\n", tc.want.err, err)
				}

				if !deleteCalled {
					t.Errorf("\ne.Delete(...): expected DeleteInstance to be called, but it wasn't")
					return
				}

				if diff := cmp.Diff(tc.want.requiredAmount, capturedAmount); diff != "" {
					t.Errorf("\ne.Delete(...) Required.Amount passed to DeleteInstance: -want, +got:\n%s\n", diff)
				}
			},
		)
	}
}
