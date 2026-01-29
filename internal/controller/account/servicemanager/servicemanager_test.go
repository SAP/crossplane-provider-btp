package servicemanager

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errTracking = errors.New("trackingError")
)

// ====================================================================================
// Resource Tracking Tests
// ====================================================================================

func TestConnect_ResourceTracking(t *testing.T) {
	type fields struct {
		newPlanIdInitializerFn func(ctx context.Context, cr *apisv1beta1.ServiceManager) (ServiceManagerPlanIdInitializer, error)
		newClientInitalizerFn  func() sm.ITfClientInitializer
		resourcetracker        *testutils.ResourceTrackerMock
	}
	type args struct {
		mg resource.Managed
	}

	type want struct {
		err         error
		trackCalled bool
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"TrackError": {
			reason: "should return an error if tracking fails",
			fields: fields{
				resourcetracker: testutils.NewResourceTrackerMockWithError(errTracking),
				newPlanIdInitializerFn: func(ctx context.Context, cr *apisv1beta1.ServiceManager) (ServiceManagerPlanIdInitializer, error) {
					return &PlanIdInitializerMock{}, nil
				},
				newClientInitalizerFn: func() sm.ITfClientInitializer {
					return &TfClientInitializerMock{}
				},
			},
			args: args{
				mg: &apisv1beta1.ServiceManager{
					Spec: apisv1beta1.ServiceManagerSpec{
						ForProvider: apisv1beta1.ServiceManagerParameters{
							SubaccountGuid: "test-guid",
						},
					},
					Status: apisv1beta1.ServiceManagerStatus{
						AtProvider: apisv1beta1.ServiceManagerObservation{
							DataSourceLookup: &apisv1beta1.DataSourceLookup{
								ServiceManagerPlanID: "plan-id",
							},
						},
					},
				},
			},
			want: want{
				err:         errTracking,
				trackCalled: true,
			},
		},
		"TrackingSuccessBeforeInitialization": {
			reason: "should call Track before initializer runs",
			fields: fields{
				resourcetracker: testutils.NewResourceTrackerMock(),
				newPlanIdInitializerFn: func(ctx context.Context, cr *apisv1beta1.ServiceManager) (ServiceManagerPlanIdInitializer, error) {
					return &PlanIdInitializerMock{}, nil
				},
				newClientInitalizerFn: func() sm.ITfClientInitializer {
					return &TfClientInitializerMock{}
				},
			},
			args: args{
				mg: &apisv1beta1.ServiceManager{
					Spec: apisv1beta1.ServiceManagerSpec{
						ForProvider: apisv1beta1.ServiceManagerParameters{
							SubaccountGuid: "test-guid",
						},
					},
					Status: apisv1beta1.ServiceManagerStatus{
						AtProvider: apisv1beta1.ServiceManagerObservation{
							DataSourceLookup: &apisv1beta1.DataSourceLookup{
								ServiceManagerPlanID: "plan-id",
							},
						},
					},
				},
			},
			want: want{
				err:         nil,
				trackCalled: true,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				kube:                   &test.MockClient{},
				resourcetracker:        tc.fields.resourcetracker,
				newPlanIdInitializerFn: tc.fields.newPlanIdInitializerFn,
				newClientInitalizerFn:  tc.fields.newClientInitalizerFn,
			}

			_, err := c.Connect(context.Background(), tc.args.mg)

			if !errors.Is(err, tc.want.err) {
				t.Errorf("expected error %v, got %v", tc.want.err, err)
			}

			// Verify Track was called
			if tc.want.trackCalled != tc.fields.resourcetracker.TrackCalled {
				t.Errorf("expected Track() called=%v, got=%v", tc.want.trackCalled, tc.fields.resourcetracker.TrackCalled)
			}

			// Verify the correct resource was tracked
			if tc.want.trackCalled && tc.fields.resourcetracker.TrackedResource != tc.args.mg {
				t.Errorf("Track() called with wrong resource")
			}
		})
	}
}

// ====================================================================================
// Deletion Blocking Tests
// ====================================================================================

func TestDelete_DeletionBlocking(t *testing.T) {
	type fields struct {
		tfClient *TfClientFake
		tracker  *testutils.ResourceTrackerMock
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err                 error
		setConditionsCalled bool
		deleteAttempted     bool
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"BlockedByResourceUsage": {
			reason: "should block deletion when resource is still in use",
			fields: fields{
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return nil
					},
				},
				tracker: testutils.NewResourceTrackerMockBlocking(),
			},
			args: args{
				mg: &apisv1beta1.ServiceManager{},
			},
			want: want{
				err:                 errors.New(providerv1alpha1.ErrResourceInUse),
				setConditionsCalled: true,
				deleteAttempted:     false,
			},
		},
		"AllowedWhenNotInUse": {
			reason: "should allow deletion when resource is not in use",
			fields: fields{
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return nil
					},
				},
				tracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &apisv1beta1.ServiceManager{},
			},
			want: want{
				err:                 nil,
				setConditionsCalled: true,
				deleteAttempted:     true,
			},
		},
		"DeleteAPIError": {
			reason: "should return API error even when not blocked",
			fields: fields{
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return errors.New("deleteError")
					},
				},
				tracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &apisv1beta1.ServiceManager{},
			},
			want: want{
				err:                 errors.New("deleteError"),
				setConditionsCalled: true,
				deleteAttempted:     true,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				kube:     &test.MockClient{MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil)},
				tracker:  tc.fields.tracker,
				tfClient: tc.fields.tfClient,
			}

			_, err := e.Delete(context.Background(), tc.args.mg)

			// Check error
			if tc.want.err != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tc.want.err)
				} else if !errors.Is(err, tc.want.err) && err.Error() != tc.want.err.Error() {
					t.Errorf("expected error %v, got %v", tc.want.err, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			// Verify SetConditions was called
			if tc.want.setConditionsCalled != tc.fields.tracker.SetConditionsCalled {
				t.Errorf("expected SetConditions() called=%v, got=%v",
					tc.want.setConditionsCalled, tc.fields.tracker.SetConditionsCalled)
			}

			// Verify delete was attempted (or not)
			if tc.want.deleteAttempted != tc.fields.tfClient.DeleteCalled {
				t.Errorf("expected DeleteResources() called=%v, got=%v",
					tc.want.deleteAttempted, tc.fields.tfClient.DeleteCalled)
			}

			// Verify Deleting condition was set
			cr, ok := tc.args.mg.(*apisv1beta1.ServiceManager)
			if !ok {
				t.Fatalf("expected *apisv1beta1.ServiceManager, got %T", tc.args.mg)
			}

			deletingCondition := cr.GetCondition(xpv1.TypeReady)
			if deletingCondition.Reason != xpv1.ReasonDeleting {
				t.Errorf("expected Deleting condition, got %v", deletingCondition.Reason)
			}
		})
	}
}

func TestObserve(t *testing.T) {
	type want struct {
		err error
		obs managed.ExternalObservation
		cr  *apisv1beta1.ServiceManager
	}
	type args struct {
		cr       *apisv1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "InstanceObserveError",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					observeFn: func() (sm.ResourcesStatus, error) {
						return sm.ResourcesStatus{}, errors.New("observeError")
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{},
				err: errors.New("observeError"),
				cr: NewServiceManager("test",
					WithStatus(apisv1beta1.ServiceManagerObservation{
						Status: apisv1beta1.ServiceManagerUnbound,
					}),
					WithConditions(xpv1.Unavailable())),
			},
		},
		{
			name: "NotAvailable",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					observeFn: func() (sm.ResourcesStatus, error) {
						// Doesn't matter what observe is returned exactly, as long as its passed through and IDs are persisted
						return sm.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{ResourceExists: false},
							InstanceID:          "someID",
						}, nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: false},
				err: nil,
				cr: NewServiceManager("test",
					WithStatus(apisv1beta1.ServiceManagerObservation{
						Status:            apisv1beta1.ServiceManagerUnbound,
						ServiceInstanceID: "someID",
					}),
					WithConditions(xpv1.Unavailable()),
				),
			},
		},
		{
			name: "IsAvailable",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					observeFn: func() (sm.ResourcesStatus, error) {
						// Doesn't matter if updated or not
						return sm.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{
								ResourceExists:    true,
								ResourceUpToDate:  true,
								ConnectionDetails: map[string][]byte{"key": []byte("value")},
							},
							InstanceID: "someID",
							BindingID:  "anotherID",
						}, nil

					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: map[string][]byte{"key": []byte("value")}},
				err: nil,
				cr: NewServiceManager("test",
					WithStatus(apisv1beta1.ServiceManagerObservation{
						Status:            apisv1beta1.ServiceManagerBound,
						ServiceInstanceID: "someID",
						ServiceBindingID:  "anotherID",
					}),
					WithConditions(xpv1.Available())),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tfClient: tc.args.tfClient,
				kube: &test.MockClient{
					MockStatusUpdate: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						return nil
					},
				},
			}
			obs, err := uua.Observe(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(obs, tc.want.obs); diff != "" {
				t.Errorf("\ne.Observe(): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.args.cr, tc.want.cr); diff != "" {
				t.Errorf("\ne.Observe(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type want struct {
		err error
		cr  *apisv1beta1.ServiceManager
	}
	type args struct {
		cr       *apisv1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "CreateError",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					createFn: func() (string, string, error) {
						return "", "", errors.New("createError")
					},
				},
			},
			want: want{
				err: errors.New("createError"),
				cr:  NewServiceManager("test", WithConditions(xpv1.Creating())),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					createFn: func() (string, string, error) {
						return "someID", "anotherID", nil
					},
				},
			},
			want: want{
				err: nil,
				cr: NewServiceManager("test",
					WithExternalName("someID/anotherID"),
					WithConditions(xpv1.Creating()),
				),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tfClient: tc.args.tfClient,
			}
			_, err := uua.Create(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Create(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Create(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type want struct {
		err error
	}
	type args struct {
		cr       *apisv1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "UpdateError",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID")),
				tfClient: &TfClientFake{
					updateFn: func() error {
						return errors.New("updateError")
					},
				},
			},
			want: want{
				err: errors.New("updateError"),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID")),
				tfClient: &TfClientFake{
					updateFn: func() error {
						return nil
					},
				},
			},
			want: want{
				err: nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tfClient: tc.args.tfClient,
			}
			_, err := uua.Update(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Update(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type want struct {
		err error
		cr  *apisv1beta1.ServiceManager
	}
	type args struct {
		cr       *apisv1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "DeleteError",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID/anotherID")),
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return errors.New("deleteError")
					},
				},
			},
			want: want{
				err: errors.New("deleteError"),
				cr:  NewServiceManager("test", WithExternalName("someID/anotherID"), WithConditions(xpv1.Deleting())),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID/anotherID")),
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return nil
					},
				},
			},
			want: want{
				err: nil,
				cr:  NewServiceManager("test", WithExternalName("someID/anotherID"), WithConditions(xpv1.Deleting())),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tracker:  testutils.NewResourceTrackerMock(),
				tfClient: tc.args.tfClient,
			}
			_, err := uua.Delete(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Delete(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Delete(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

// ====================================================================================
// Mock Implementations
// ====================================================================================

var _ sm.ITfClientInitializer = &TfClientInitializerMock{}

type TfClientInitializerMock struct {
	client sm.ITfClient
	err    error
}

func (t *TfClientInitializerMock) ConnectResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (sm.ITfClient, error) {
	if t.err != nil {
		return nil, t.err
	}
	if t.client != nil {
		return t.client, nil
	}
	return &TfClientFake{}, nil
}

var _ ServiceManagerPlanIdInitializer = &PlanIdInitializerMock{}

type PlanIdInitializerMock struct {
	planID string
	err    error
}

func (p *PlanIdInitializerMock) ServiceManagerPlanIDByName(ctx context.Context, subaccountId string, servicePlanName string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	if p.planID != "" {
		return p.planID, nil
	}
	return "default-plan-id", nil
}

// ====================================================================================
// Test Utilities
// ====================================================================================

func NewServiceManager(name string, m ...ServiceManagerModifier) *apisv1beta1.ServiceManager {
	cr := &apisv1beta1.ServiceManager{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	meta.SetExternalName(cr, name)
	for _, f := range m {
		f(cr)
	}
	return cr
}

// this pattern can be potentially auto generated, its quite useful to write expressive unittests
type ServiceManagerModifier func(dirEnvironment *apisv1beta1.ServiceManager)

func WithStatus(status apisv1beta1.ServiceManagerObservation) ServiceManagerModifier {
	return func(r *apisv1beta1.ServiceManager) {
		r.Status.AtProvider = status
	}
}

func WithData(data apisv1beta1.ServiceManagerParameters) ServiceManagerModifier {
	return func(r *apisv1beta1.ServiceManager) {
		r.Spec.ForProvider = data
	}
}

func WithConditions(c ...xpv1.Condition) ServiceManagerModifier {
	return func(r *apisv1beta1.ServiceManager) { r.Status.Conditions = c }
}

func WithExternalName(externalName string) ServiceManagerModifier {
	return func(r *apisv1beta1.ServiceManager) {
		meta.SetExternalName(r, externalName)
	}
}

// Fakes
var _ sm.ITfClient = &TfClientFake{}

type TfClientFake struct {
	observeFn    func() (sm.ResourcesStatus, error)
	createFn     func() (string, string, error)
	updateFn     func() error
	deleteFn     func() error
	DeleteCalled bool
}

func (t *TfClientFake) ObserveResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (sm.ResourcesStatus, error) {
	return t.observeFn()
}

func (t *TfClientFake) CreateResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (string, string, error) {
	return t.createFn()
}

func (t *TfClientFake) UpdateResources(ctx context.Context, cr *apisv1beta1.ServiceManager) error {
	return t.updateFn()
}

func (t *TfClientFake) DeleteResources(ctx context.Context, cr *apisv1beta1.ServiceManager) error {
	t.DeleteCalled = true
	return t.deleteFn()
}
