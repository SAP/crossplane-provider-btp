package servicemanager

import (
	"context"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
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
				err:         errors.Wrap(errTracking, "cannot track resource usage"),
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
				if err != nil && tc.want.err != nil {
					if !strings.Contains(err.Error(), tc.want.err.Error()) {
						t.Errorf("expected error to contain %q, got %q", tc.want.err.Error(), err.Error())
					}
				} else {
					t.Errorf("expected error %v, got %v", tc.want.err, err)
				}
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
// InitializeServicePlanId / PendingAdminBindingCleanup Tests
// ====================================================================================

func TestInitializeServicePlanId(t *testing.T) {
	errCleanup := errors.New("delete binding failed")
	errInitializer := errors.New("initializer build failed")

	statusUpdateOK := test.NewMockSubResourceUpdateFn(nil)
	statusUpdateErr := test.NewMockSubResourceUpdateFn(errors.New("status update failed"))

	cases := map[string]struct {
		reason         string
		cr             *apisv1beta1.ServiceManager
		mock           *PlanIdInitializerMock
		initializerErr error
		statusUpdateFn func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error
		wantErr        bool
		wantPending    bool
		wantPlanID     string
	}{
		"FreshLookupSucceeds": {
			reason: "no prior DataSourceLookup: creates binding, resolves plan ID, deletes binding, saves without pending flag",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
			},
			mock:           &PlanIdInitializerMock{planID: "plan-abc"},
			statusUpdateFn: statusUpdateOK,
			wantErr:        false,
			wantPending:    false,
			wantPlanID:     "plan-abc",
		},
		"FreshLookupDeleteFails_SetsPendingFlag": {
			reason: "plan ID resolved but binding delete fails: saves plan ID with pendingAdminBindingCleanup=true, returns nil (CR stays healthy)",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
			},
			mock:           &PlanIdInitializerMock{planID: "plan-abc", cleanupErr: errCleanup},
			statusUpdateFn: statusUpdateOK,
			wantErr:        false,
			wantPending:    true,
			wantPlanID:     "plan-abc",
		},
		"FreshLookupBothFail_ReturnsError": {
			reason: "plan ID lookup fails and id is empty: returns error, no status saved",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
			},
			mock:           &PlanIdInitializerMock{err: errors.New("lookup failed")},
			statusUpdateFn: statusUpdateOK,
			wantErr:        true,
		},
		"AlreadyInitialized_NoPendingFlag_Noop": {
			reason: "DataSourceLookup already set and pendingAdminBindingCleanup=false: no-op",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
				Status: apisv1beta1.ServiceManagerStatus{
					AtProvider: apisv1beta1.ServiceManagerObservation{
						DataSourceLookup: &apisv1beta1.DataSourceLookup{
							ServiceManagerPlanID:       "existing-plan",
							PendingAdminBindingCleanup: false,
						},
					},
				},
			},
			mock:           &PlanIdInitializerMock{},
			statusUpdateFn: statusUpdateOK,
			wantErr:        false,
			wantPending:    false,
			wantPlanID:     "existing-plan",
		},
		"AlreadyInitialized_PendingFlag_DeleteSucceeds_ClearsFlag": {
			reason: "pendingAdminBindingCleanup=true and delete succeeds: clears flag, updates status",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
				Status: apisv1beta1.ServiceManagerStatus{
					AtProvider: apisv1beta1.ServiceManagerObservation{
						DataSourceLookup: &apisv1beta1.DataSourceLookup{
							ServiceManagerPlanID:       "existing-plan",
							PendingAdminBindingCleanup: true,
						},
					},
				},
			},
			mock:           &PlanIdInitializerMock{deleteBindErr: nil},
			statusUpdateFn: statusUpdateOK,
			wantErr:        false,
			wantPending:    false,
			wantPlanID:     "existing-plan",
		},
		"AlreadyInitialized_PendingFlag_DeleteFails_StaysHealthy": {
			reason: "pendingAdminBindingCleanup=true and delete still fails: returns nil, flag unchanged",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
				Status: apisv1beta1.ServiceManagerStatus{
					AtProvider: apisv1beta1.ServiceManagerObservation{
						DataSourceLookup: &apisv1beta1.DataSourceLookup{
							ServiceManagerPlanID:       "existing-plan",
							PendingAdminBindingCleanup: true,
						},
					},
				},
			},
			mock:           &PlanIdInitializerMock{deleteBindErr: errCleanup},
			statusUpdateFn: statusUpdateOK,
			wantErr:        false,
			wantPending:    true,
			wantPlanID:     "existing-plan",
		},
		"AlreadyInitialized_PendingFlag_StatusUpdateFails_ReturnsError": {
			reason: "delete succeeds but status update fails: returns error",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
				Status: apisv1beta1.ServiceManagerStatus{
					AtProvider: apisv1beta1.ServiceManagerObservation{
						DataSourceLookup: &apisv1beta1.DataSourceLookup{
							ServiceManagerPlanID:       "existing-plan",
							PendingAdminBindingCleanup: true,
						},
					},
				},
			},
			mock:           &PlanIdInitializerMock{deleteBindErr: nil},
			statusUpdateFn: statusUpdateErr,
			wantErr:        true,
			wantPending:    false,
			wantPlanID:     "existing-plan",
		},
		"AlreadyInitialized_PendingFlag_InitializerFails_ReturnsError": {
			reason: "pendingAdminBindingCleanup=true but building the initializer fails: returns error (reconcile fails, CR unhealthy until resolved)",
			cr: &apisv1beta1.ServiceManager{
				Spec: apisv1beta1.ServiceManagerSpec{
					ForProvider: apisv1beta1.ServiceManagerParameters{SubaccountGuid: "sub-1"},
				},
				Status: apisv1beta1.ServiceManagerStatus{
					AtProvider: apisv1beta1.ServiceManagerObservation{
						DataSourceLookup: &apisv1beta1.DataSourceLookup{
							ServiceManagerPlanID:       "existing-plan",
							PendingAdminBindingCleanup: true,
						},
					},
				},
			},
			mock:           &PlanIdInitializerMock{},
			initializerErr: errInitializer,
			statusUpdateFn: statusUpdateOK,
			wantErr:        true,
			wantPending:    true,
			wantPlanID:     "existing-plan",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := &test.MockClient{
				MockStatusUpdate: tc.statusUpdateFn,
			}
			c := &connector{
				kube: kube,
				newPlanIdInitializerFn: func(ctx context.Context, cr *apisv1beta1.ServiceManager) (ServiceManagerPlanIdInitializer, error) {
					if tc.initializerErr != nil {
						return nil, tc.initializerErr
					}
					return tc.mock, nil
				},
			}

			err := c.InitializeServicePlanId(context.Background(), tc.cr)

			if tc.wantErr && err == nil {
				t.Errorf("%s: expected error, got nil", tc.reason)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("%s: unexpected error: %v", tc.reason, err)
			}

			if tc.wantPlanID != "" {
				if tc.cr.Status.AtProvider.DataSourceLookup == nil {
					t.Errorf("%s: expected DataSourceLookup to be set", tc.reason)
				} else {
					if diff := cmp.Diff(tc.wantPlanID, tc.cr.Status.AtProvider.DataSourceLookup.ServiceManagerPlanID); diff != "" {
						t.Errorf("%s: plan ID mismatch (-want +got):\n%s", tc.reason, diff)
					}
					if diff := cmp.Diff(tc.wantPending, tc.cr.Status.AtProvider.DataSourceLookup.PendingAdminBindingCleanup); diff != "" {
						t.Errorf("%s: pending flag mismatch (-want +got):\n%s", tc.reason, diff)
					}
				}
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
				err:                 errors.New("while deleting resources: deleteError"),
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
	// deletionTime is captured once at test-setup so cases that read/write
	// DeletionTimestamp share the exact same value (otherwise two separate
	// metav1.Now() calls in args vs want disagree at microsecond precision).
	deletionTime := metav1.Now()
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
		{
			// status holds a stale plan ID in dataSourceLookup (stalePlan) that no
			// longer matches the live instance. ObserveResources returns the live
			// instance's plan as ObservedPlanID (livePlan); setStatus must overwrite
			// the stale ID with the live one so status shows the instance's real plan.
			name: "SelfHealsStalePlanID",
			args: args{
				cr: NewServiceManager("test",
					WithStatus(apisv1beta1.ServiceManagerObservation{
						DataSourceLookup: &apisv1beta1.DataSourceLookup{ServiceManagerPlanID: "stalePlan"},
					})),
				tfClient: &TfClientFake{
					observeFn: func() (sm.ResourcesStatus, error) {
						return sm.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{
								ResourceExists:   true,
								ResourceUpToDate: true,
							},
							InstanceID:     "someID",
							BindingID:      "anotherID",
							ObservedPlanID: "livePlan",
						}, nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
				cr: NewServiceManager("test",
					WithStatus(apisv1beta1.ServiceManagerObservation{
						Status:            apisv1beta1.ServiceManagerBound,
						ServiceInstanceID: "someID",
						ServiceBindingID:  "anotherID",
						DataSourceLookup:  &apisv1beta1.DataSourceLookup{ServiceManagerPlanID: "livePlan"},
					}),
					WithConditions(xpv1.Available())),
			},
		},
		{
			// must report Deleting()/Unbound, not Available()/Bound. Under the
			// pre-fix code the delete-aware ObserveResources returned
			// ResourceExists:true which then flipped the CR back to Available
			// on every reconcile between Delete() and the final finalize —
			// misleading in kubectl output and dashboards.
			name: "DeletingInstanceStillExists",
			args: args{
				cr: NewServiceManager("test", WithDeletionTimestamp(deletionTime)),
				tfClient: &TfClientFake{
					observeFn: func() (sm.ResourcesStatus, error) {
						return sm.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
							InstanceID:          "someID",
						}, nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
				cr: NewServiceManager("test",
					WithDeletionTimestamp(deletionTime),
					WithStatus(apisv1beta1.ServiceManagerObservation{
						Status:            apisv1beta1.ServiceManagerUnbound,
						ServiceInstanceID: "someID",
					}),
					WithConditions(xpv1.Deleting())),
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
				err: errors.New("while creating resources: createError"),
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
				if err != nil && tc.want.err != nil && strings.Contains(err.Error(), tc.want.err.Error()) {
					return
				}
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
				err: errors.New("while updating resources: updateError"),
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
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				if err != nil && tc.want.err != nil && strings.Contains(err.Error(), tc.want.err.Error()) {
					return
				}
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
				err: errors.New("while deleting resources: deleteError"),
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
				if err != nil && tc.want.err != nil && strings.Contains(err.Error(), tc.want.err.Error()) {
					return
				}
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
	planID        string
	err           error
	cleanupErr    error // returned alongside a valid planID (delete-succeeded-but-failed-to-cleanup scenario)
	deleteBindErr error
}

func (p *PlanIdInitializerMock) ServiceManagerPlanIDByName(ctx context.Context, subaccountId string, servicePlanName string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	id := p.planID
	if id == "" {
		id = "default-plan-id"
	}
	return id, p.cleanupErr
}

func (p *PlanIdInitializerMock) DeleteAdminBinding(ctx context.Context, subaccountId string) error {
	return p.deleteBindErr
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

func WithDeletionTimestamp(t metav1.Time) ServiceManagerModifier {
	return func(r *apisv1beta1.ServiceManager) {
		r.SetDeletionTimestamp(&t)
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

func TestServicePlanName(t *testing.T) {
	c := &connector{}
	cases := map[string]struct {
		planName string
		want     string
	}{
		"EmptyPlanNameDefaultsToSubaccountAdmin": {
			planName: "",
			want:     apisv1beta1.DefaultPlanName,
		},
		"ExplicitPlanNamePassthrough": {
			planName: "service-operator-access",
			want:     "service-operator-access",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := NewServiceManager("test")
			cr.Spec.ForProvider.PlanName = tc.planName
			got := c.ServicePlanName(cr)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ServicePlanName() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
