package subscription

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/subscription"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	tracking_test "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestObserve(t *testing.T) {
	type args struct {
		cr             resource.Managed
		mockApiHandler *MockApiHandler
		mockTypeMapper *MockTypeMapper
	}
	type want struct {
		err error
		o   managed.ExternalObservation
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotSubscription),
			},
		},
		"NoExternalName": {
			reason: "When externalName isn't in expected format, it has never been created",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("dir-unittests")),
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: false},
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("dir-unittests")),
			},
		},
		"APIErrorOnRead": {
			reason: "When needsCreation can't be determined we can't proceed",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
				mockApiHandler: &MockApiHandler{
					returnGet: nil,
					returnErr: errors.New("internalServerError"),
				},
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New("internalServerError"),
				cr:  NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
			},
		},
		"RequiresCreation": {
			reason: "If client requires it we need to trigger a creation",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
				mockApiHandler: &MockApiHandler{
					returnGet: nil,
					returnErr: nil,
				},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: false},
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
			},
		},
		"RequiresUpdate": {
			reason: "If client requires it we need to trigger an update",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
				mockApiHandler: &MockApiHandler{
					returnGet: &subscription.SubscriptionGet{
						State: internal.Ptr("SUBSCRIBED"),
					},
					returnErr: nil,
				},
				mockTypeMapper: &MockTypeMapper{
					synced:    false,
					available: true,
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: NewSubscription("dir-unittests", WithConditions(xpv1.Available()), WithStatus(v1alpha1.SubscriptionObservation{
					State: internal.Ptr("SUBSCRIBED"),
				}), WithExternalName("name1/plan2")),
			},
		},
		"PendingCreation": {
			reason: "If client determines everything is up to date, but creation is still pending and resource not yet available",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
				mockApiHandler: &MockApiHandler{
					returnGet: &subscription.SubscriptionGet{
						State: internal.Ptr("SUBSCRIBED"),
					},
					returnErr: nil,
				},
				mockTypeMapper: &MockTypeMapper{
					synced:    true,
					available: false,
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: NewSubscription("dir-unittests", WithConditions(xpv1.Unavailable()), WithStatus(v1alpha1.SubscriptionObservation{
					State: internal.Ptr("SUBSCRIBED"),
				}), WithExternalName("name1/plan2")),
			},
		},
		"UpToDate": {
			reason: "If client determines everything is up to date we don't need to do anything",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
				mockApiHandler: &MockApiHandler{
					returnGet: &subscription.SubscriptionGet{
						State: internal.Ptr("SUBSCRIBED"),
					},
					returnErr: nil,
				},
				mockTypeMapper: &MockTypeMapper{
					synced:    true,
					available: true,
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: NewSubscription("dir-unittests", WithConditions(xpv1.Available()), WithStatus(v1alpha1.SubscriptionObservation{
					State: internal.Ptr("SUBSCRIBED"),
				}), WithExternalName("name1/plan2")),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := external{
				tracker:    nil,
				apiHandler: tc.args.mockApiHandler,
				typeMapper: tc.args.mockTypeMapper,
			}
			got, err := ctrl.Observe(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		cr             resource.Managed
		mockApiHandler *MockApiHandler
		kubeUpdateErr  error
	}
	type want struct {
		err error
		o   managed.ExternalCreation
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotSubscription),
			},
		},
		"Failure": {
			reason: "We expect to return error from API client",
			args: args{
				cr:             NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				mockApiHandler: &MockApiHandler{returnErr: errors.New("CreateError")},
			},
			want: want{
				o:   managed.ExternalCreation{},
				cr:  NewSubscription("dir-unittests", WithConditions(xpv1.Creating()), WithStatus(v1alpha1.SubscriptionObservation{})),
				err: errors.New("CreateError"),
			},
		},
		"Success": {
			reason: "We expect a proper externalName and no error being returned here",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				mockApiHandler: &MockApiHandler{
					returnErr:          nil,
					returnExternalName: "name1/plan2",
				}},
			want: want{
				o:   managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}},
				cr:  NewSubscription("dir-unittests", WithConditions(xpv1.Creating()), WithStatus(v1alpha1.SubscriptionObservation{}), WithExternalName("name1/plan2")),
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			mockKube.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return tc.args.kubeUpdateErr
			}
			ctrl := external{
				tracker:    nil,
				kube:       &mockKube,
				apiHandler: tc.args.mockApiHandler,
				typeMapper: &MockTypeMapper{},
			}
			got, err := ctrl.Create(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestRecreateOnFailed(t *testing.T) {
	mockKube := testutils.NewFakeKubeClientBuilder().Build()
	extName := "test-ext-name"
	ctrl := external{
		tracker:    nil,
		kube:       &mockKube,
		apiHandler: &MockApiHandler{
			deleteCounter: 0,
			returnExternalName: extName,
			returnGet: &subscription.SubscriptionGet{
				State: ptr.To(v1alpha1.SubscriptionStateSubscribeFailed),
			},
		},
		typeMapper: &MockTypeMapper{
			synced:    true,
			available: true,
			deletable: true,
		},
	}

	cr := NewSubscription("initial-fail",
		WithExternalName(extName),
		WithRecreateOnSubscriptionFailure())

	// When we observe it, the observed state is SUBSCRIBE_FAILED
	got, err := ctrl.Observe(context.Background(), cr)
	if err != nil {
		t.Errorf("initial observation returned error: %v", err)
	}

	// The controller shall trigger a deletion
	if c := ctrl.apiHandler.(*MockApiHandler).deleteCounter; c != 1 {
		t.Errorf("the initial observation should perform a delete operation (%v)", c)
	}

	// No need to create or update the external resource
	if diff := cmp.Diff(managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},

	}, got); diff != "" {
		t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", "initial observation", diff)
	}

	// The delete operation sets the ready condition
	readyCondition := cr.GetCondition(xpv1.TypeReady)
	if readyCondition.Status != corev1.ConditionFalse {
		t.Errorf("returned CR has wrong ready condition status: %v", readyCondition.Status)
	}
	if readyCondition.Reason != xpv1.ReasonDeleting {
		t.Errorf("returned CR has wrong ready condition reason: %v", readyCondition.Reason)
	}

	// We observe it again
	got, err = ctrl.Observe(context.Background(), cr)
	if err != nil {
		t.Errorf("initial observation returned error: %v", err)
	}

	// The controller shall not trigger deletion, since we're in deleting state
	if c := ctrl.apiHandler.(*MockApiHandler).deleteCounter; c != 1 {
		t.Errorf("the second observation should not perform a delete operation (%v)", c)
	}

	// No need to create or update the external resource
	if diff := cmp.Diff(managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},

	}, got); diff != "" {
		t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", "initial observation", diff)
	}

	// The external resource is deleted
	ctrl.typeMapper = &MockTypeMapper{
			synced:    false,
			available: false,
			deletable: false,
	}
	// The API does not return SUBSCRIBE_FAILED anymore
	ctrl.apiHandler = &MockApiHandler{
		deleteCounter: 0,
		returnExternalName: extName,
		// returnGet: &subscription.SubscriptionGet{
		// 	State: ptr.To(v1alpha1.SubscriptionStateSubscribeFailed),
		// },
	}

	// We observe it again
	got, err = ctrl.Observe(context.Background(), cr)
	if err != nil {
		t.Errorf("initial observation returned error: %v", err)
	}

	// The controller shall not trigger deletion
	if c := ctrl.apiHandler.(*MockApiHandler).deleteCounter; c != 0 {
		t.Errorf("the third observation should not perform a delete operation (%v)", c)
	}

	// The resource shall be created
	if diff := cmp.Diff(managed.ExternalObservation{
		ResourceExists:    false,

	}, got); diff != "" {
		t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", "initial observation", diff)
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr             resource.Managed
		mockApiHandler *MockApiHandler
		mockTypeMapper *MockTypeMapper
	}
	type want struct {
		err error
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotSubscription),
			},
		},
		"Failure": {
			reason: "We expect to return an error if Delete fails",
			args: args{
				cr:             NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				mockApiHandler: &MockApiHandler{returnErr: errors.New("DeleteError")},
				mockTypeMapper: &MockTypeMapper{deletable: true},
			},
			want: want{
				cr:  NewSubscription("dir-unittests", WithConditions(xpv1.Deleting()), WithStatus(v1alpha1.SubscriptionObservation{})),
				err: errors.New("DeleteError"),
			},
		},
		"Success": {
			reason: "We expect to finish gracefully if no error happened during create",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				mockApiHandler: &MockApiHandler{
					returnErr: nil,
				},
				mockTypeMapper: &MockTypeMapper{deletable: true},
			},
			want: want{
				cr:  NewSubscription("dir-unittests", WithConditions(xpv1.Deleting()), WithStatus(v1alpha1.SubscriptionObservation{})),
				err: nil,
			},
		},
		"SkipApiCall": {
			reason: "We expect to skip the API call if external resource is not available (deletion in progress)",
			args: args{
				cr:             NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{State: internal.Ptr(v1alpha1.SubscriptionStateInProcess)})),
				mockTypeMapper: &MockTypeMapper{deletable: false},
			},
			want: want{
				cr:  NewSubscription("dir-unittests", WithConditions(xpv1.Deleting()), WithStatus(v1alpha1.SubscriptionObservation{State: internal.Ptr(v1alpha1.SubscriptionStateInProcess)})),
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			ctrl := external{
				tracker:    nil,
				kube:       &mockKube,
				apiHandler: tc.args.mockApiHandler,
				typeMapper: tc.args.mockTypeMapper,
			}
			err := ctrl.Delete(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		cr             resource.Managed
		mockApiHandler *MockApiHandler
	}
	type want struct {
		err error
		o   managed.ExternalUpdate
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotSubscription),
			},
		},
		"Failure": {
			reason: "",
			args: args{
				cr:             NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				mockApiHandler: &MockApiHandler{returnErr: errors.New("updateError")},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				cr:  NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				err: errors.New("updateError"),
			},
		},
		"Success": {
			reason: "We expect to finish gracefully if no error happened during create",
			args: args{
				cr: NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				mockApiHandler: &MockApiHandler{
					returnErr: nil,
				}},
			want: want{
				o:   managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
				cr:  NewSubscription("dir-unittests", WithStatus(v1alpha1.SubscriptionObservation{})),
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			ctrl := external{
				tracker:    nil,
				kube:       &mockKube,
				apiHandler: tc.args.mockApiHandler,
				typeMapper: &MockTypeMapper{},
			}
			got, err := ctrl.Update(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestConnect(t *testing.T) {
	type args struct {
		cr          resource.Managed
		kubeObjects []client.Object
	}
	type want struct {
		err error
	}
	tests := map[string]struct {
		args args
		want want
	}{
		"NilResource": {
			args: args{
				cr:          nil,
				kubeObjects: []client.Object{},
			},
			want: want{
				err: errors.New("managed resource is not a Subscription custom resource"),
			},
		},
		"NoCISResourceFound": {
			args: args{
				cr:          NewSubscription("unittest-sub1", WithData(v1alpha1.SubscriptionSpec{CloudManagementSecret: ""})),
				kubeObjects: []client.Object{},
			},
			want: want{
				err: errors.New("no Cloud Management Secret Found"),
			},
		},
		"NoCISSecretFound": {
			args: args{
				cr: NewSubscription("unittest-sub1",
					WithData(v1alpha1.SubscriptionSpec{
						CloudManagementSecret:          "cis-test",
						CloudManagementSecretNamespace: "cis-namespace",
					})),
				kubeObjects: []client.Object{},
			},
			want: want{
				err: errors.New("could not get secret of local cloud management"),
			},
		},
		"NewServiceFnError": {
			args: args{
				cr: NewSubscription("unittest-sub1",
					WithData(v1alpha1.SubscriptionSpec{
						CloudManagementSecret:          "cis-test",
						CloudManagementSecretNamespace: "cis-namespace",
					})),
				kubeObjects: []client.Object{
					testutils.NewSecret("cis-test", map[string][]byte{}),
				},
			},
			want: want{
				err: errors.New("secret credentials data not in the expected format"),
			},
		},
		"Successful": {
			args: args{
				cr: NewSubscription("unittest-sub1",
					WithData(v1alpha1.SubscriptionSpec{
						CloudManagementSecret:          "cis-test",
						CloudManagementSecretNamespace: "cis-namespace",
					})),
				kubeObjects: []client.Object{
					testutils.NewSecret("cis-test", map[string][]byte{providerv1alpha1.RawBindingKey: []byte("{}")}),
				},
			},
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			kube := testutils.NewFakeKubeClientBuilder().
				AddResources(tc.args.kubeObjects...).
				Build()
			c := connector{
				kube:         &kube,
				usage:        tracking_test.NoOpReferenceResolverTracker{},
				newServiceFn: newSubscriptionClientFn,
				resourcetracker: tracking.NewDefaultReferenceResolverTracker(&kube),
			}

			connect, err := c.Connect(context.Background(), tc.args.cr)

			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\ne.Connect(...): error \"%v\" not part of \"%v\"", err, tc.want.err)
			}
			if tc.want.err == nil {
				if connect == nil {
					t.Errorf("Expected connector to be != nil")
				}
			}
		})
	}
}

func NewSubscription(name string, m ...SubscriptionModifier) *v1alpha1.Subscription {
	cr := &v1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	meta.SetExternalName(cr, name)
	for _, f := range m {
		f(cr)
	}
	return cr
}

type SubscriptionModifier func(dirEnvironment *v1alpha1.Subscription)

func WithStatus(status v1alpha1.SubscriptionObservation) SubscriptionModifier {
	return func(r *v1alpha1.Subscription) {
		r.Status.AtProvider = status
	}
}

func WithData(data v1alpha1.SubscriptionSpec) SubscriptionModifier {
	return func(r *v1alpha1.Subscription) {
		r.Spec = data
	}
}

func WithConditions(c ...xpv1.Condition) SubscriptionModifier {
	return func(r *v1alpha1.Subscription) { r.Status.ConditionedStatus.Conditions = c }
}

func WithExternalName(externalName string) SubscriptionModifier {
	return func(r *v1alpha1.Subscription) {
		meta.SetExternalName(r, externalName)
	}
}

func WithRecreateOnSubscriptionFailure() SubscriptionModifier {
	return func(r *v1alpha1.Subscription) {
		r.Spec.RecreateOnSubscriptionFailure = true
	}
}
