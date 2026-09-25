package serviceinstance

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	tferrors "github.com/crossplane/upjet/v2/pkg/terraform/errors"
	"github.com/sap/crossplane-provider-btp/apis"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

var (
	errClient      = errors.New("apiError")
	errKube        = errors.New("kubeError")
	errCreator     = errors.New("creatorError")
	errInitializer = errors.New("initializerError")
	errTracking    = errors.New("trackingError")
)

// ====================================================================================
// Resource Usage Tests
// ====================================================================================

func TestConnect_ResourceTracking(t *testing.T) {
	type fields struct {
		creator         *TfProxyClientCreatorMock
		initializer     Initializer
		resourcetracker *testutils.ResourceTrackerMock
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
			reason: "should return an error when tracking fails",
			fields: fields{
				creator:         &TfProxyClientCreatorMock{},
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMockWithError(errTracking),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err:         errTracking,
				trackCalled: true,
			},
		},
		"TrackingSuccessBeforeInitialization": {
			reason: "should call Track before initialization",
			fields: fields{
				creator:         &TfProxyClientCreatorMock{},
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err:         nil,
				trackCalled: true,
			},
		},
		"TrackingWithServiceManagerRef": {
			reason: "should track ServiceManagerRef",
			fields: fields{
				creator:         &TfProxyClientCreatorMock{},
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{
					Spec: v1alpha1.ServiceInstanceSpec{
						ForProvider: v1alpha1.ServiceInstanceParameters{
							ServiceManagerRef: &xpv1.Reference{
								Name: "test-servicemanager",
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
		"SkipTrackingWhenDeleted": {
			// Regression: an MR being deleted whose upstream reference has
			// already been removed used to fail Connect() with a "not found"
			// from Track(), which prevented Delete() from ever running and
			// left the BTP-side instance and the finalizer in place forever.
			reason: "should skip Track when the MR is being deleted, even if Track would error",
			fields: fields{
				creator:         &TfProxyClientCreatorMock{},
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMockWithError(errTracking),
			},
			args: args{
				mg: func() *v1alpha1.ServiceInstance {
					now := metav1.Now()
					return &v1alpha1.ServiceInstance{
						ObjectMeta: metav1.ObjectMeta{
							DeletionTimestamp: &now,
						},
					}
				}(),
			},
			want: want{
				err:         nil,
				trackCalled: false,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				clientConnector:             tc.fields.creator,
				newServicePlanInitializerFn: func() Initializer { return tc.fields.initializer },
				resourcetracker:             tc.fields.resourcetracker,
			}
			_, err := c.Connect(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
			// Verify if Track was called
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
		client  *TfProxyMock
		tracker *testutils.ResourceTrackerMock
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
		"BlockedByServiceBinding": {
			reason: "should block deletion when ServiceBindings reference this instance",
			fields: fields{
				client:  &TfProxyMock{},
				tracker: testutils.NewResourceTrackerMockBlocking(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err:                 errors.New(providerv1alpha1.ErrResourceInUse),
				setConditionsCalled: true,
				deleteAttempted:     false,
			},
		},
		"AllowedWhenNoBindings": {
			reason: "should allow deletion when no ServiceBindings reference this instance",
			fields: fields{
				client:  &TfProxyMock{},
				tracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err:                 nil,
				setConditionsCalled: true,
				deleteAttempted:     true,
			},
		},
		"DeleteAPIErrorWhenNotBlocked": {
			reason: "should return API error when deletion proceeds but API fails",
			fields: fields{
				client:  &TfProxyMock{err: errClient},
				tracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err:                 errClient,
				setConditionsCalled: true,
				deleteAttempted:     true,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
				tracker: tc.fields.tracker,
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
			if tc.want.deleteAttempted != tc.fields.client.deleteCalled {
				t.Errorf("expected Delete() called=%v, got=%v",
					tc.want.deleteAttempted, tc.fields.client.deleteCalled)
			}
		})
	}
}
func TestConnect(t *testing.T) {
	type fields struct {
		creator         *TfProxyClientCreatorMock
		initializer     Initializer
		resourcetracker *testutils.ResourceTrackerMock
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err            error
		externalExists bool
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"InitializerError": {
			reason: "should return an error when the initalizer fails",
			fields: fields{
				creator:         &TfProxyClientCreatorMock{},
				initializer:     &InitializerMock{err: errInitializer},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errInitializer,
			},
		},
		"CreatorError": {
			reason: "should return an error when the creator fails",
			fields: fields{
				creator:         &TfProxyClientCreatorMock{err: errCreator},
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errCreator,
			},
		},
		"ConnectSuccess": {
			reason: "should return a client when the creator succeeds",
			fields: fields{
				creator:         &TfProxyClientCreatorMock{},
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				clientConnector:             tc.fields.creator,
				newServicePlanInitializerFn: func() Initializer { return tc.fields.initializer },
				resourcetracker:             tc.fields.resourcetracker,
			}

			got, err := c.Connect(context.Background(), tc.args.mg)
			if tc.want.externalExists && got == nil {
				t.Errorf("expected external client, got nil")
			}
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

func TestObserve(t *testing.T) {
	type fields struct {
		client *TfProxyMock
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		o                managed.ExternalObservation
		err              error
		cr               *v1alpha1.ServiceInstance // Expected complete CR
		wantDriftCond    bool                      // Whether a DriftDetected condition should be set
		wantDiffContains []string                  // Substrings that must appear in the Diff field
		wantReadyReason  string                    // Expected Ready condition reason, asserted when non-empty
		wantReadyStatus  corev1.ConditionStatus    // Expected Ready condition status, asserted with wantReadyReason
		wantReadyMessage string                    // Expected Ready condition message, asserted when non-empty
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		// ADR(external-name):: external-name empty without conflict → no creation attempt, not existing
		"EmptyExternalName_NoConflict": {
			reason: "should return resourceExists:false when external-name is empty and no conflict error present",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: expectedServiceInstance(),
			},
		},
		// ADR(external-name):: external-name empty + conflict in LastAsyncOperation → stay in error loop
		"EmptyExternalName_ConflictError": {
			reason: "should return error and resourceExists:false when external-name is empty but creation failed with Conflict",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: withConflictCondition(&v1alpha1.ServiceInstance{}),
			},
			want: want{
				err: errors.New("creation failed - resource already exists. Please set external-name annotation to adopt the existing resource or change the name to create a new one"),
				o:   managed.ExternalObservation{ResourceExists: false},
				cr:  withConflictCondition(expectedServiceInstance()),
			},
		},
		// ADR(external-name):: external-name empty + conflict condition but spec changed → allow new Create attempt
		"EmptyExternalName_ConflictError_SpecChanged": {
			reason: "should not stay in error loop when spec changed since conflict (Generation > ObservedGeneration in condition)",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				// Generation=2 simulates a spec change after the conflict (condition has ObservedGeneration=1)
				mg: func() *v1alpha1.ServiceInstance {
					cr := &v1alpha1.ServiceInstance{}
					cr.Generation = 1
					withConflictCondition(cr) // stamps ObservedGeneration=1
					cr.Generation = 2         // simulate spec change
					return cr
				}(),
			},
			want: want{
				err: nil,
				o:   managed.ExternalObservation{ResourceExists: false},
				cr: func() *v1alpha1.ServiceInstance {
					cr := expectedServiceInstance()
					cr.Generation = 2
					withConflictCondition(cr)
					cr.Generation = 2
					return cr
				}(),
			},
		},
		// Setting the external-name is the adoption the conflict error asks for,
		// and it does not bump the generation, so the branch must stand down on
		// that signal alone.
		"AdoptedExternalName_ConflictError": {
			reason: "should leave the conflict error loop once the user has set an adoption external-name",
			fields: fields{
				client: &TfProxyMock{
					status: tfclient.UpToDate,
					data: &tfclient.ObservationData{
						ExternalName: "6c5a4e0e-7d1b-4f3a-9a2e-8b0d5f1c3a77",
						ID:           "6c5a4e0e-7d1b-4f3a-9a2e-8b0d5f1c3a77",
					},
					details: map[string][]byte{"some-key": []byte("some-value")},
				},
			},
			args: args{
				mg: withConflictCondition(expectedServiceInstance(
					withExternalName("6c5a4e0e-7d1b-4f3a-9a2e-8b0d5f1c3a77"),
				)),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
					ConnectionDetails: managed.ConnectionDetails{
						"some-key": []byte("some-value"),
					},
				},
				cr: expectedServiceInstance(
					withExternalName("6c5a4e0e-7d1b-4f3a-9a2e-8b0d5f1c3a77"),
					withObservationData("6c5a4e0e-7d1b-4f3a-9a2e-8b0d5f1c3a77", ""),
				),
			},
		},
		// An Observe error returns before the reconciler's deletion block, so the
		// branch must never fire on a terminating CR.
		"DeletingWithConflictCondition": {
			reason: "should not error out of Observe while deleting, which would strand the finalizer",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				mg: withConflictCondition(expectedServiceInstance(withDeletionTimestamp())),
			},
			want: want{
				err: nil,
				o:   managed.ExternalObservation{ResourceExists: false},
				cr:  expectedServiceInstance(withDeletionTimestamp()),
			},
		},
		// ADR(external-name):: external-name set but not a valid UUID → return error
		"InvalidUUIDExternalName": {
			reason: "should return error when external-name is set but not a valid UUID",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: expectedServiceInstance(withExternalName("not-a-uuid")),
			},
			want: want{
				err: errors.New("external-name is not a valid UUID. Please check the value of the external-name annotation and set it to the ServiceInstance ID (UUID format) if you want to adopt an existing resource, or remove the annotation if you want to create a new one"),
				cr:  expectedServiceInstance(withExternalName("not-a-uuid")),
			},
		},
		// ADR(external-name): the same invalid external-name must NOT block a
		// delete — an Observe error returns before Delete runs and would strand
		// the finalizer. Mirrors ServiceBinding's guard (#987).
		"InvalidUUIDExternalName_WhileDeleting": {
			reason: "should skip external-name validation when the CR is being deleted",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				mg: expectedServiceInstance(withExternalName("not-a-uuid"), withDeletionTimestamp()),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: expectedServiceInstance(withExternalName("not-a-uuid"), withDeletionTimestamp()),
			},
		},
		// ADR(external-name):: valid UUID in external-name, resource not found → trigger Create()
		"ValidUUID_NotFound": {
			reason: "should return resourceExists:false when valid UUID is set but resource does not exist (404)",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				mg: expectedServiceInstance(withExternalName("550e8400-e29b-41d4-a716-446655440000")),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: expectedServiceInstance(withExternalName("550e8400-e29b-41d4-a716-446655440000")),
			},
		},
		"LookupError": {
			reason: "error should be returned",
			fields: fields{
				client: &TfProxyMock{err: errClient},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errClient,
				cr:  expectedServiceInstance(), // No annotations, observation data, or conditions
			},
		},
		"NotFound": {
			reason: "should return not existing",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: expectedServiceInstance(), // No annotations, observation data, or conditions
			},
		},
		// ADR(external-name):: drift detected → diff set in observation and condition on CR
		"DriftDetected_DiffReported": {
			reason: "should set drift condition on CR and return non-empty diff in observation when drift is detected",
			fields: fields{
				client: &TfProxyMock{
					status:  tfclient.Drift,
					details: map[string][]byte{},
					tfResource: &v1alpha1.SubaccountServiceInstance{
						Spec: v1alpha1.SubaccountServiceInstanceSpec{
							ForProvider: v1alpha1.SubaccountServiceInstanceParameters{
								Name: internal.Ptr("desired-name"),
							},
						},
						Status: v1alpha1.SubaccountServiceInstanceStatus{
							AtProvider: v1alpha1.SubaccountServiceInstanceObservation{
								Name: internal.Ptr("actual-name"),
							},
						},
					},
				},
			},
			args: args{
				mg: expectedServiceInstance(withExternalName("550e8400-e29b-41d4-a716-446655440000")),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr:               expectedServiceInstance(withExternalName("550e8400-e29b-41d4-a716-446655440000")),
				wantDriftCond:    true,
				wantDiffContains: []string{"desired-name", "actual-name"},
			},
		},
		// ADR(external-name):: external-name set via Observe() after async creation (external-name flows through saveInstanceData)
		"ExternalNameSetFromObservationData": {
			reason: "should set external-name on CR from ObservationData when async creation completes",
			fields: fields{
				client: &TfProxyMock{
					status: tfclient.UpToDate,
					data: &tfclient.ObservationData{
						ExternalName: "550e8400-e29b-41d4-a716-446655440000",
						ID:           "some-id",
					},
					details: map[string][]byte{},
				},
			},
			args: args{
				mg: expectedServiceInstance(withObservationData("", "")),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: expectedServiceInstance(
					withExternalName("550e8400-e29b-41d4-a716-446655440000"),
					withObservationData("some-id", ""),
					withConditions(xpv1.Available()),
				),
			},
		},
		"Requires Update": {
			reason: "should return existing, not up to date",
			fields: fields{
				client: &TfProxyMock{
					status:  tfclient.Drift,
					details: map[string][]byte{},
					tfResource: &v1alpha1.SubaccountServiceInstance{
						Spec: v1alpha1.SubaccountServiceInstanceSpec{
							ForProvider: v1alpha1.SubaccountServiceInstanceParameters{
								Name: internal.Ptr("test-instance"),
							},
						},
						Status: v1alpha1.SubaccountServiceInstanceStatus{
							AtProvider: v1alpha1.SubaccountServiceInstanceObservation{
								Name: internal.Ptr("test-instance-modified"),
							},
						},
					},
				},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
					Diff:              "", // Will be filled with actual diff by calculateDiff
				},
				cr: expectedServiceInstance(), // No annotations, observation data, or conditions
			},
		},
		"Happy, while async in process": {
			reason: "should return existing, but no data",
			fields: fields{
				client: &TfProxyMock{
					status:  tfclient.UpToDate,
					details: map[string][]byte{},
				},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: expectedServiceInstance(), // No annotations, observation data, or conditions
			},
		},
		"Happy, no drift": {
			reason: "should return existing and pull data from embedded tf resource",
			fields: fields{
				client: &TfProxyMock{
					status: tfclient.UpToDate,
					data: &tfclient.ObservationData{
						ExternalName: "some-ext-name",
						ID:           "some-id",
					},
					details: map[string][]byte{
						"some-key": []byte("some-value"),
					},
				},
			},
			args: args{
				mg: expectedServiceInstance(
					withObservationData("", ""),
				),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
					ConnectionDetails: managed.ConnectionDetails{
						"some-key": []byte("some-value"),
					},
				},
				cr: expectedServiceInstance(
					withExternalName("some-ext-name"),
					withObservationData("some-id", ""),
					withConditions(xpv1.Available()),
				),
			},
		},
		"AsyncUpdateFailure_HoldsUnhealthy": {
			reason: "should report the resource as existing but not up to date and keep Ready=False when the last async update was rejected",
			fields: fields{
				client: &TfProxyMock{
					status: tfclient.UpToDate,
					data: &tfclient.ObservationData{
						ExternalName: "some-ext-name",
						ID:           "some-id",
					},
					details: map[string][]byte{
						"some-key": []byte("some-value"),
					},
				},
			},
			args: args{
				mg: expectedServiceInstance(
					withAsyncFailureCondition(ujresource.ReasonAsyncUpdateFailure, "async update failed: Cannot change AppId with update"),
				),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				cr:               expectedServiceInstance(),
				wantReadyReason:  "AsyncOperationFailed",
				wantReadyStatus:  corev1.ConditionFalse,
				wantReadyMessage: "async update failed: Cannot change AppId with update",
			},
		},
		// Must not be re-routed into the create-conflict branch.
		"AsyncUpdateFailure_ConflictWording": {
			reason: "should hold the resource unhealthy when a rejected update's message contains Conflict",
			fields: fields{
				client: &TfProxyMock{
					status: tfclient.UpToDate,
					data: &tfclient.ObservationData{
						ExternalName: "some-ext-name",
						ID:           "some-id",
					},
					details: map[string][]byte{"some-key": []byte("some-value")},
				},
			},
			args: args{
				mg: expectedServiceInstance(
					withAsyncFailureCondition(ujresource.ReasonAsyncUpdateFailure, "async update failed: API Error Updating Resource Service Instance (Subaccount): Conflict"),
				),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				cr:              expectedServiceInstance(),
				wantReadyReason: "AsyncOperationFailed",
				wantReadyStatus: corev1.ConditionFalse,
			},
		},
		// A failure recorded against an older spec no longer describes what the
		// reconciler would apply, and holding on it can never clear when the
		// management policies exclude Update.
		"AsyncUpdateFailure_SpecChanged": {
			reason: "should stop holding the resource unhealthy once the spec changed since the rejected update",
			fields: fields{
				client: &TfProxyMock{
					status: tfclient.UpToDate,
					data: &tfclient.ObservationData{
						ExternalName: "some-ext-name",
						ID:           "some-id",
					},
					details: map[string][]byte{"some-key": []byte("some-value")},
				},
			},
			args: args{
				mg: func() *v1alpha1.ServiceInstance {
					cr := expectedServiceInstance()
					cr.Generation = 1
					withAsyncFailureCondition(ujresource.ReasonAsyncUpdateFailure, "async update failed: Cannot change AppId with update")(cr)
					cr.Generation = 2
					return cr
				}(),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
					ConnectionDetails: managed.ConnectionDetails{
						"some-key": []byte("some-value"),
					},
				},
				cr: func() *v1alpha1.ServiceInstance {
					cr := expectedServiceInstance(
						withExternalName("some-ext-name"),
						withObservationData("some-id", ""),
					)
					cr.Generation = 2
					return cr
				}(),
				wantReadyReason: "Available",
				wantReadyStatus: corev1.ConditionTrue,
			},
		},
		// upjet reports exists+upToDate and withholds the observation data while
		// an operation is running. The retry of a rejected update runs for
		// seconds or minutes, and Ready and Synced must not report healthy for
		// that whole window.
		"AsyncUpdateFailure_InFlightRetry": {
			reason: "should keep holding the resource unhealthy while the retry of a rejected update is still running",
			fields: fields{
				client: &TfProxyMock{status: tfclient.UpToDate},
			},
			args: args{
				mg: expectedServiceInstance(
					withAsyncFailureCondition(ujresource.ReasonAsyncUpdateFailure, "async update failed: Cannot change AppId with update"),
				),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				cr:               expectedServiceInstance(),
				wantReadyReason:  "AsyncOperationFailed",
				wantReadyStatus:  corev1.ConditionFalse,
				wantReadyMessage: "async update failed: Cannot change AppId with update",
			},
		},
		"AsyncUpdateFailure_NotExisting": {
			reason: "should report the resource as not existing when terraform says so, despite a recorded update failure",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				mg: expectedServiceInstance(
					withAsyncFailureCondition(ujresource.ReasonAsyncUpdateFailure, "async update failed: Cannot change AppId with update"),
				),
			},
			want: want{
				err: nil,
				o:   managed.ExternalObservation{ResourceExists: false},
				cr:  expectedServiceInstance(),
			},
		},
		"AsyncCreateFailure_StillCreates": {
			reason: "should still report the resource as not existing when the last async create failed",
			fields: fields{
				client: &TfProxyMock{status: tfclient.NotExisting},
			},
			args: args{
				mg: expectedServiceInstance(
					withAsyncFailureCondition(ujresource.ReasonAsyncCreateFailure, "async create failed: quota exceeded"),
				),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: expectedServiceInstance(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			got, err := e.Observe(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
			// Ignore Diff field in comparison as it contains dynamic content
			if diff := cmp.Diff(tc.want.o, got, cmp.FilterPath(func(p cmp.Path) bool {
				return p.String() == "Diff"
			}, cmp.Ignore())); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			// Verify Diff contains expected substrings
			for _, substr := range tc.want.wantDiffContains {
				if !strings.Contains(got.Diff, substr) {
					t.Errorf("\n%s\nexpected Diff to contain %q, got:\n%s", tc.reason, substr, got.Diff)
				}
			}

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceInstance, got %T", tc.args.mg)
			}
			// Ignore conditions when comparing CR as they may contain timestamps and drift messages
			if diff := cmp.Diff(tc.want.cr, cr, cmpopts.IgnoreFields(xpv1.ConditionedStatus{}, "Conditions")); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}

			// Verify drift condition was set when expected
			if tc.want.wantDriftCond {
				readyCond := cr.GetCondition(xpv1.TypeReady)
				if readyCond.Reason != "DriftDetected" {
					t.Errorf("\n%s\nexpected DriftDetected condition, got reason=%q message=%q", tc.reason, readyCond.Reason, readyCond.Message)
				}
				if readyCond.Message == "" {
					t.Errorf("\n%s\nexpected non-empty drift condition message", tc.reason)
				}
			}

			if tc.want.wantReadyReason != "" {
				readyCond := cr.GetCondition(xpv1.TypeReady)
				if string(readyCond.Reason) != tc.want.wantReadyReason {
					t.Errorf("\n%s\nexpected Ready reason %q, got reason=%q status=%q message=%q",
						tc.reason, tc.want.wantReadyReason, readyCond.Reason, readyCond.Status, readyCond.Message)
				}
				if readyCond.Status != tc.want.wantReadyStatus {
					t.Errorf("\n%s\nexpected Ready status %q, got %q", tc.reason, tc.want.wantReadyStatus, readyCond.Status)
				}
			}

			if tc.want.wantReadyMessage != "" {
				if got := cr.GetCondition(xpv1.TypeReady).Message; got != tc.want.wantReadyMessage {
					t.Errorf("\n%s\nexpected Ready message %q, got %q", tc.reason, tc.want.wantReadyMessage, got)
				}
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type fields struct {
		client *TfProxyMock
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
		cr  *v1alpha1.ServiceInstance // Expected complete CR after creation
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ApiError": {
			reason: "should return an error when the API call fails",
			fields: fields{
				client: &TfProxyMock{err: errClient},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errClient,
				cr: expectedServiceInstance(
					withAsyncOperationMarker(0),
					withConditions(
						xpv1.Creating(),
					),
				),
			},
		},
		// ADR(external-name):: external-name already set means the resource was not found by Observe() (e.g. externally deleted).
		// Create() should proceed normally and recreate it.
		"ExternalNameAlreadySet_ProceedsWithCreate": {
			reason: "should proceed with creation when external-name is already set (resource not found by Observe)",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: expectedServiceInstance(withExternalName("550e8400-e29b-41d4-a716-446655440000")),
			},
			want: want{
				err: nil,
				cr: expectedServiceInstance(
					withExternalName("550e8400-e29b-41d4-a716-446655440000"),
					withAsyncOperationMarker(0),
					withConditions(xpv1.Creating()),
				),
			},
		},
		"HappyPath": {
			reason: "should create the resource successfully and set Creating condition",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				cr: expectedServiceInstance(
					withAsyncOperationMarker(0),
					withConditions(
						xpv1.Creating(),
					),
				),
			},
		},
		// The marker is written before the launch, so it must survive a launch
		// that reports the previous operation's cached failure.
		"CachedAsyncFailure_StillRecordsTheStart": {
			reason: "should record the operation start when upjet returns the previous operation's cached failure",
			fields: fields{
				client: &TfProxyMock{err: tferrors.NewAsyncCreateFailed(errors.New("previous attempt failed"))},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errors.New(errCreateInstance + ": async create failed: previous attempt failed"),
				cr: expectedServiceInstance(
					withAsyncOperationMarker(0),
					withConditions(xpv1.Creating()),
				),
			},
		},
		"WrongType": {
			reason: "should return an error when the managed resource is not a ServiceInstance",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: &v1alpha1.Subaccount{},
			},
			want: want{
				err: errors.New(errNotServiceInstance),
				cr:  nil, // no ServiceInstance to assert on
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			_, err := e.Create(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)

			if tc.want.cr == nil {
				return
			}

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceInstance, got %T", tc.args.mg)
			}
			// Ignore conditions when comparing CR as they may contain timestamps and drift messages
			if diff := cmp.Diff(tc.want.cr, cr, cmpopts.IgnoreFields(xpv1.ConditionedStatus{}, "Conditions")); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type fields struct {
		client *TfProxyMock
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err error
		cr  *v1alpha1.ServiceInstance
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ApiError": {
			reason: "should return an error when the API call fails",
			fields: fields{
				client: &TfProxyMock{err: errClient},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errClient,
				cr:  expectedServiceInstance(withAsyncOperationMarker(0)),
			},
		},
		"HappyPath": {
			reason: "should update the resource successfully",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				cr:  expectedServiceInstance(withAsyncOperationMarker(0)),
			},
		},
		// ADR(external-name):: Update uses external-name to identify the resource; external-name must be preserved
		"HappyPath_WithExternalName": {
			reason: "should update successfully and preserve the external-name on the CR",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: expectedServiceInstance(withExternalName("550e8400-e29b-41d4-a716-446655440000")),
			},
			want: want{
				err: nil,
				cr: expectedServiceInstance(
					withExternalName("550e8400-e29b-41d4-a716-446655440000"),
					withAsyncOperationMarker(0),
				),
			},
		},
		// #968: Synced stays False because upjet hands the cached rejection
		// back from Update() while the terraform resource is still out of date.
		"RejectedUpdate_CachedFailure": {
			reason: "should surface the cached rejection upjet returns from Update",
			fields: fields{
				client: &TfProxyMock{err: tferrors.NewAsyncUpdateFailed(errors.New("Cannot change AppId with update"))},
			},
			args: args{
				mg: expectedServiceInstance(
					withAsyncFailureCondition(ujresource.ReasonAsyncUpdateFailure, "async update failed: Cannot change AppId with update"),
				),
			},
			want: want{
				err: errors.New(errUpdateInstance + ": async update failed: Cannot change AppId with update"),
				cr:  expectedServiceInstance(withAsyncOperationMarker(0)),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			_, err := e.Update(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceInstance, got %T", tc.args.mg)
			}
			// Ignore conditions when comparing CR as they may contain timestamps and drift messages
			if diff := cmp.Diff(tc.want.cr, cr, cmpopts.IgnoreFields(xpv1.ConditionedStatus{}, "Conditions")); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

// TestOperationGenerationSurvivesCreateReconcile asserts that a create rejected
// after the user has corrected the spec stays attributed to the generation it
// applied, and so does not block the corrected one.
//
// The status subresource has to be enabled on the fake client: it is what makes
// crossplane's post-Create main-resource update bring the server's status back
// over anything Create() wrote there.
func TestOperationGenerationSurvivesCreateReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}

	cr := expectedServiceInstance()
	cr.Name = "test-instance"
	cr.Namespace = "default"
	cr.Spec.ForProvider.Name = "original-name"
	cr.Generation = 1

	kube := kubefake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(&v1alpha1.ServiceInstance{}).
		Build()

	key := types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}
	tf := &TfProxyMock{status: tfclient.NotExisting}
	e := &external{tfClient: tf, kube: kube}
	r := managed.NewReconciler(
		&fake.Manager{Client: kube, Scheme: scheme},
		resource.ManagedKind(v1alpha1.ServiceInstanceGroupVersionKind),
		managed.WithExternalConnector(managed.ExternalConnectorFn(
			func(_ context.Context, _ resource.Managed) (managed.ExternalClient, error) { return e, nil },
		)),
		managed.WithInitializers(),
		managed.WithRecorder(event.NewNopRecorder()),
	)

	// Reconcile until Create() has run. One pass can do it, but the reconciler
	// is free to spend passes on its finalizer and create annotations first, so
	// the loop waits for the marker rather than assuming a count.
	var created *v1alpha1.ServiceInstance
	for i := range 5 {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatalf("reconcile %d failed: %v", i, err)
		}
		created = &v1alpha1.ServiceInstance{}
		if err := kube.Get(context.Background(), key, created); err != nil {
			t.Fatalf("cannot read back the instance: %v", err)
		}
		if _, ok := created.GetAnnotations()[AsyncOperationGenerationKey]; ok {
			break
		}
	}

	marked, ok := created.GetAnnotations()[AsyncOperationGenerationKey]
	if !ok {
		t.Fatalf("the operation-start marker did not survive the create reconcile: annotations %v", created.GetAnnotations())
	}
	createGeneration := created.Generation
	if marked != strconv.FormatInt(createGeneration, 10) {
		t.Fatalf("expected the marker to record generation %d, got %q", createGeneration, marked)
	}

	// The user corrects the spec while the create is still running. The fake
	// client does not emulate the API server's generation bump, so drive it.
	created.Spec.ForProvider.Name = "corrected-name"
	created.Generation = createGeneration + 1
	if err := kube.Update(context.Background(), created); err != nil {
		t.Fatalf("cannot edit the spec: %v", err)
	}

	// Only now does the create fail, with the conflict the *original* name
	// caused.
	failure := tferrors.NewAsyncCreateFailed(errors.New("Conflict: resource already exists"))
	if err := saveCallback(context.Background(), kube,
		types.NamespacedName{Namespace: key.Namespace, Name: siClient.TfNamePrefix + key.Name},
		ujresource.LastAsyncOperationCondition(failure), ujresource.AsyncOperationFinishedCondition()); err != nil {
		t.Fatalf("the async callback failed: %v", err)
	}

	settled := &v1alpha1.ServiceInstance{}
	if err := kube.Get(context.Background(), key, settled); err != nil {
		t.Fatalf("cannot read back the settled instance: %v", err)
	}
	recorded := settled.GetCondition(xpv1.ConditionType(ujresource.TypeLastAsyncOperation))
	if recorded.ObservedGeneration != createGeneration {
		t.Errorf("expected the conflict to stay attributed to generation %d, got %d",
			createGeneration, recorded.ObservedGeneration)
	}

	// The corrected spec must reach Create() again instead of the conflict
	// branch of Observe(), which reports the instance as never-created and
	// demands adoption.
	if _, err := e.Observe(context.Background(), settled); err != nil {
		t.Errorf("generation %d is still blocked by the conflict from generation %d: %v",
			settled.Generation, createGeneration, err)
	}
}

// TestUpdateMarkerReachesTheAPIServer asserts the marker is durable on the
// Update path, where the reconciler writes status only afterwards and so
// persists no annotation of its own.
func TestUpdateMarkerReachesTheAPIServer(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}

	cr := expectedServiceInstance()
	cr.Name = "test-instance"
	cr.Namespace = "default"
	cr.Generation = 7

	kube := kubefake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr.DeepCopy()).
		WithStatusSubresource(&v1alpha1.ServiceInstance{}).
		Build()

	// The reconciler always hands Update() the object it just read.
	key := types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}
	if err := kube.Get(context.Background(), key, cr); err != nil {
		t.Fatalf("cannot read the instance: %v", err)
	}
	e := &external{tfClient: &TfProxyMock{}, kube: kube}
	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	stored := &v1alpha1.ServiceInstance{}
	if err := kube.Get(context.Background(), key, stored); err != nil {
		t.Fatalf("cannot read back the instance: %v", err)
	}
	if got := stored.GetAnnotations()[AsyncOperationGenerationKey]; got != "7" {
		t.Errorf("expected the marker %q on the stored instance, got %q (annotations %v)", "7", got, stored.GetAnnotations())
	}
	// The reconciler writes status right after Update(); a stale
	// resourceVersion in memory would make that write conflict.
	if cr.GetResourceVersion() != stored.GetResourceVersion() {
		t.Errorf("expected the in-memory resourceVersion to track the write, got %q want %q",
			cr.GetResourceVersion(), stored.GetResourceVersion())
	}
}

// callbackDuringUpdate reports its result from inside Update(), before that
// call returns, as upjet's apply goroutine may.
type callbackDuringUpdate struct {
	*TfProxyMock
	run  func()
	done bool
}

func (t *callbackDuringUpdate) Update(ctx context.Context) error {
	if !t.done {
		t.done = true
		t.run()
	}
	return t.TfProxyMock.Update(ctx)
}

// TestFastCallbackKeepsTheCurrentGeneration asserts that a rejection arriving
// before Update() returns is still attributed to the generation being applied.
// Attributed to the previous one, the generation gate discards it and the hold
// is lost.
func TestFastCallbackKeepsTheCurrentGeneration(t *testing.T) {
	const rejection = "Cannot change AppId with update"

	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}

	// An earlier operation ran against generation 1; the user has since
	// corrected the spec, so the update now being launched applies generation 2.
	cr := expectedServiceInstance(withAsyncOperationMarker(1))
	cr.Name = "test-instance"
	cr.Namespace = "default"
	cr.Generation = 2

	kube := kubefake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr.DeepCopy()).
		WithStatusSubresource(&v1alpha1.ServiceInstance{}).
		Build()

	key := types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}
	tf := &callbackDuringUpdate{
		TfProxyMock: &TfProxyMock{
			status: tfclient.Drift,
			data:   &tfclient.ObservationData{ExternalName: "some-ext-name", ID: "some-id"},
		},
		run: func() {
			failure := tferrors.NewAsyncUpdateFailed(errors.New(rejection))
			if err := saveCallback(context.Background(), kube,
				types.NamespacedName{Namespace: key.Namespace, Name: siClient.TfNamePrefix + key.Name},
				ujresource.LastAsyncOperationCondition(failure), ujresource.AsyncOperationFinishedCondition()); err != nil {
				t.Errorf("the async callback failed: %v", err)
			}
		},
	}

	e := &external{tfClient: tf, kube: kube}
	r := managed.NewReconciler(
		&fake.Manager{Client: kube, Scheme: scheme},
		resource.ManagedKind(v1alpha1.ServiceInstanceGroupVersionKind),
		managed.WithExternalConnector(managed.ExternalConnectorFn(
			func(_ context.Context, _ resource.Managed) (managed.ExternalClient, error) { return e, nil },
		)),
		managed.WithInitializers(),
		managed.WithRecorder(event.NewNopRecorder()),
	)

	// First reconcile launches the update and takes the callback mid-flight;
	// the second observes the instance with that result already recorded.
	for i := range 2 {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatalf("reconcile %d failed: %v", i, err)
		}
	}

	settled := &v1alpha1.ServiceInstance{}
	if err := kube.Get(context.Background(), key, settled); err != nil {
		t.Fatalf("cannot read back the instance: %v", err)
	}

	if recorded := settled.GetCondition(xpv1.ConditionType(ujresource.TypeLastAsyncOperation)); recorded.ObservedGeneration != cr.Generation {
		t.Errorf("expected the rejection to be attributed to generation %d, got %d",
			cr.Generation, recorded.ObservedGeneration)
	}
	if ready := settled.GetCondition(xpv1.TypeReady); ready.Status != corev1.ConditionFalse || string(ready.Reason) != reasonAsyncOperationFailed {
		t.Errorf("expected Ready=False/%s, got %q/%q", reasonAsyncOperationFailed, ready.Status, ready.Reason)
	}
}

// TestReconcileRejectedUpdate asserts that a rejected update leaves the CR
// Synced=False/ReconcileError and Ready=False, and keeps it there while the
// retry runs. Synced is written by the reconciler, not returned by Observe or
// Update, so the assertions run over a real reconcile.
func TestReconcileRejectedUpdate(t *testing.T) {
	const rejection = "async update failed: Cannot change AppId with update"

	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}

	cases := map[string]struct {
		reason      string
		client      *TfProxyMock
		wantMessage string
	}{
		"Settled": {
			reason: "a rejected update on a settled resource must not report ReconcileSuccess",
			client: &TfProxyMock{
				status:    tfclient.Drift,
				data:      &tfclient.ObservationData{ExternalName: "some-ext-name", ID: "some-id"},
				updateErr: tferrors.NewAsyncUpdateFailed(errors.New("Cannot change AppId with update")),
			},
			wantMessage: "Cannot change AppId with update",
		},
		"RetryInFlight": {
			// upjet answers exists+upToDate and withholds the observation data
			// while the retry is still running, and refuses another update.
			reason: "a rejected update whose retry is still running must not report ReconcileSuccess either",
			client: &TfProxyMock{
				status:    tfclient.UpToDate,
				updateErr: errors.New("update operation that started at X is still running"),
			},
			wantMessage: "still running",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := expectedServiceInstance(
				withExternalName("550e8400-e29b-41d4-a716-446655440000"),
				withAsyncFailureCondition(ujresource.ReasonAsyncUpdateFailure, rejection),
			)
			cr.Name = "test-instance"
			cr.Namespace = "default"

			var saved *v1alpha1.ServiceInstance
			kube := &test.MockClient{
				MockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object) error {
					si, ok := obj.(*v1alpha1.ServiceInstance)
					if !ok {
						return errors.New("unexpected object kind")
					}
					cr.DeepCopyInto(si)
					return nil
				},
				MockUpdate: test.NewMockUpdateFn(nil),
				MockStatusUpdate: func(_ context.Context, obj client.Object, _ ...client.SubResourceUpdateOption) error {
					saved = obj.(*v1alpha1.ServiceInstance).DeepCopy()
					return nil
				},
			}

			e := &external{tfClient: tc.client, kube: kube}
			r := managed.NewReconciler(
				&fake.Manager{Client: kube, Scheme: scheme},
				resource.ManagedKind(v1alpha1.ServiceInstanceGroupVersionKind),
				managed.WithExternalConnector(managed.ExternalConnectorFn(
					func(_ context.Context, _ resource.Managed) (managed.ExternalClient, error) { return e, nil },
				)),
				managed.WithInitializers(),
				managed.WithRecorder(event.NewNopRecorder()),
			)

			if _, err := r.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name},
			}); err != nil {
				t.Fatalf("%s: reconcile returned an error: %v", tc.reason, err)
			}
			if saved == nil {
				t.Fatalf("%s: the reconciler wrote no status", tc.reason)
			}

			synced := saved.GetCondition(xpv1.TypeSynced)
			if synced.Status != corev1.ConditionFalse {
				t.Errorf("%s: expected Synced=False, got %q (reason %q)", tc.reason, synced.Status, synced.Reason)
			}
			if synced.Reason != xpv1.ReasonReconcileError {
				t.Errorf("%s: expected reason %q, got %q", tc.reason, xpv1.ReasonReconcileError, synced.Reason)
			}
			if !strings.Contains(synced.Message, tc.wantMessage) {
				t.Errorf("%s: expected %q in the Synced message, got %q", tc.reason, tc.wantMessage, synced.Message)
			}
			if ready := saved.GetCondition(xpv1.TypeReady); ready.Status != corev1.ConditionFalse || string(ready.Reason) != reasonAsyncOperationFailed {
				t.Errorf("%s: expected Ready=False/%s, got %q/%q", tc.reason, reasonAsyncOperationFailed, ready.Status, ready.Reason)
			}
		})
	}
}

func TestCalculateDiff(t *testing.T) {
	cases := map[string]struct {
		reason         string
		tfResource     resource.Managed
		wantContains   []string
		wantNotContain string
	}{
		"NilTfResource": {
			reason:       "should return fallback message when GetTfResource returns nil",
			tfResource:   nil,
			wantContains: []string{"unable to retrieve"},
		},
		"WrongResourceType": {
			reason:       "should return fallback message when TfResource is not a SubaccountServiceInstance",
			tfResource:   &v1alpha1.ServiceInstance{},
			wantContains: []string{"unexpected resource type"},
		},
		"NoDiff_NoAsyncMessage": {
			reason: "should return generic fallback when spec and status are identical and no async message",
			tfResource: &v1alpha1.SubaccountServiceInstance{
				Spec: v1alpha1.SubaccountServiceInstanceSpec{
					ForProvider: v1alpha1.SubaccountServiceInstanceParameters{
						Name: internal.Ptr("same-name"),
					},
				},
				Status: v1alpha1.SubaccountServiceInstanceStatus{
					AtProvider: v1alpha1.SubaccountServiceInstanceObservation{
						Name: internal.Ptr("same-name"),
					},
				},
			},
			wantContains: []string{"Drift detected"},
		},
		"FieldDiff": {
			reason: "should contain the differing field values when spec and status diverge",
			tfResource: &v1alpha1.SubaccountServiceInstance{
				Spec: v1alpha1.SubaccountServiceInstanceSpec{
					ForProvider: v1alpha1.SubaccountServiceInstanceParameters{
						Name: internal.Ptr("desired-name"),
					},
				},
				Status: v1alpha1.SubaccountServiceInstanceStatus{
					AtProvider: v1alpha1.SubaccountServiceInstanceObservation{
						Name: internal.Ptr("actual-name"),
					},
				},
			},
			wantContains: []string{"desired-name", "actual-name"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: &TfProxyMock{tfResource: tc.tfResource},
				kube:     &test.MockClient{},
			}

			got := e.calculateDiff(&v1alpha1.ServiceInstance{})

			for _, substr := range tc.wantContains {
				if !strings.Contains(got, substr) {
					t.Errorf("\n%s\nexpected diff to contain %q, got:\n%s", tc.reason, substr, got)
				}
			}
		})
	}
}

func TestSaveCallback(t *testing.T) {
	type args struct {
		kube       client.Client
		name       types.NamespacedName
		conditions []xpv1.Condition
	}

	type want struct {
		err error
	}

	const crGeneration = 3
	failure := ujresource.LastAsyncOperationCondition(tferrors.NewAsyncUpdateFailed(errors.New("Cannot change AppId with update")))

	// getInto records every lookup key and fills the object the callback will
	// write back, so the assertions can check what it stamped.
	getInto := func(keys *[]client.ObjectKey) test.MockGetFn {
		return func(_ context.Context, key client.ObjectKey, obj client.Object) error {
			*keys = append(*keys, key)
			obj.SetName(key.Name)
			obj.SetGeneration(crGeneration)
			return nil
		}
	}

	// captureWrites records the conditions of every status write.
	captureWrites := func(writes *[][]xpv1.Condition, err func(attempt int) error) test.MockSubResourceUpdateFn {
		return func(_ context.Context, obj client.Object, _ ...client.SubResourceUpdateOption) error {
			si, ok := obj.(*v1alpha1.ServiceInstance)
			if !ok {
				return errors.New("status write did not carry a ServiceInstance")
			}
			*writes = append(*writes, si.Status.Conditions)
			return err(len(*writes))
		}
	}

	// wroteFailure asserts the write carried the failure condition, stamped with
	// the generation the operation applied, plus the finished marker.
	wroteFailure := func(t *testing.T, conditions []xpv1.Condition) {
		t.Helper()
		var got, finished *xpv1.Condition
		for i := range conditions {
			switch conditions[i].Type {
			case xpv1.ConditionType(ujresource.TypeLastAsyncOperation):
				got = &conditions[i]
			case ujresource.TypeAsyncOperation:
				finished = &conditions[i]
			}
		}
		if got == nil {
			t.Fatalf("expected the failure condition to be written, got %v", conditions)
		}
		if got.Reason != failure.Reason || got.Message != failure.Message {
			t.Errorf("expected the failure condition %q/%q, got %q/%q", failure.Reason, failure.Message, got.Reason, got.Message)
		}
		if got.ObservedGeneration != crGeneration {
			t.Errorf("expected ObservedGeneration %d, got %d", crGeneration, got.ObservedGeneration)
		}
		if finished == nil || finished.Status != corev1.ConditionTrue {
			t.Errorf("expected the async operation to be marked finished, got %v", finished)
		}
	}

	cases := map[string]struct {
		reason string
		build  func() (args, func(t *testing.T))
		want   want
	}{
		// The generation a rejection is labelled with decides whether Observe()
		// treats it as a verdict on the current spec. A spec edit during a long
		// apply must not make the result look newer than the operation was.
		"LabelsResultWithTheOperationsGeneration": {
			reason: "should label the result with the generation the operation started against, not the one the CR carries now",
			build: func() (args, func(t *testing.T)) {
				var writes [][]xpv1.Condition
				return args{
						kube: &test.MockClient{
							MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
								si, ok := obj.(*v1alpha1.ServiceInstance)
								if !ok {
									return errors.New("unexpected object kind")
								}
								si.SetName(key.Name)
								// The user edited the spec while the apply ran.
								si.SetGeneration(crGeneration + 1)
								withAsyncOperationMarker(crGeneration)(si)
								return nil
							},
							MockStatusUpdate: captureWrites(&writes, func(int) error { return nil }),
						},
						name:       types.NamespacedName{Name: "TF-test-instance"},
						conditions: []xpv1.Condition{failure, ujresource.AsyncOperationFinishedCondition()},
					}, func(t *testing.T) {
						if len(writes) != 1 {
							t.Fatalf("expected a single status write, got %d", len(writes))
						}
						wroteFailure(t, writes[0])
					}
			},
			want: want{err: nil},
		},
		"GetError": {
			reason: "should return an error if the ServiceInstance cannot be retrieved",
			build: func() (args, func(t *testing.T)) {
				return args{
					kube: &test.MockClient{MockGet: test.NewMockGetFn(errKube)},
					name: types.NamespacedName{Name: "test-instance"},
				}, nil
			},
			want: want{err: errKube},
		},
		"UpdateError": {
			reason: "should return an error if the ServiceInstance status cannot be updated",
			build: func() (args, func(t *testing.T)) {
				return args{
					kube: &test.MockClient{
						MockGet:          test.NewMockGetFn(nil),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(errKube),
					},
					name:       types.NamespacedName{Name: "test-instance"},
					conditions: []xpv1.Condition{ujresource.AsyncOperationFinishedCondition()},
				}, nil
			},
			want: want{err: errKube},
		},
		"Success": {
			reason: "should successfully save conditions to the ServiceInstance",
			build: func() (args, func(t *testing.T)) {
				return args{
					kube: &test.MockClient{
						MockGet:          test.NewMockGetFn(nil),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
					},
					name:       types.NamespacedName{Name: "test-instance"},
					conditions: []xpv1.Condition{ujresource.AsyncOperationFinishedCondition()},
				}, nil
			},
			want: want{err: nil},
		},
		// Regression guard for #968: the prefix is stripped, and the conditions
		// actually land on the object that is written back.
		"TrimsTfPrefixAndStampsConditions": {
			reason: "should look the native ServiceInstance up without the terraform prefix and write the conditions onto it",
			build: func() (args, func(t *testing.T)) {
				var keys []client.ObjectKey
				var writes [][]xpv1.Condition
				return args{
						kube: &test.MockClient{
							MockGet:          getInto(&keys),
							MockStatusUpdate: captureWrites(&writes, func(int) error { return nil }),
						},
						name:       types.NamespacedName{Name: "TF-test-instance"},
						conditions: []xpv1.Condition{failure, ujresource.AsyncOperationFinishedCondition()},
					}, func(t *testing.T) {
						if want := []client.ObjectKey{{Name: "test-instance"}}; !cmp.Equal(keys, want) {
							t.Errorf("expected lookup keys %v, got %v", want, keys)
						}
						if len(writes) != 1 {
							t.Fatalf("expected a single status write, got %d", len(writes))
						}
						wroteFailure(t, writes[0])
					}
			},
			want: want{err: nil},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a, verify := tc.build()
			err := saveCallback(context.Background(), a.kube, a.name, a.conditions...)
			expectedErrorBehaviour(t, tc.want.err, err)
			if verify != nil {
				verify(t)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		client  *TfProxyMock
		tracker *testutils.ResourceTrackerMock
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err error
		cr  *v1alpha1.ServiceInstance
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ApiError": {
			reason: "should return an error when the API call fails",
			fields: fields{
				client:  &TfProxyMock{err: errClient},
				tracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errClient,
				cr: expectedServiceInstance(
					withConditions(xpv1.Deleting()),
				),
			},
		},
		"HappyPath": {
			reason: "should delete the resource successfully and set Deleting condition",
			fields: fields{
				client:  &TfProxyMock{},
				tracker: testutils.NewResourceTrackerMock(),
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				cr: expectedServiceInstance(
					withConditions(xpv1.Deleting()),
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
				tracker: tc.fields.tracker,
			}

			_, err := e.Delete(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceInstance, got %T", tc.args.mg)
			}
			// Ignore conditions when comparing CR as they may contain timestamps and drift messages
			if diff := cmp.Diff(tc.want.cr, cr, cmpopts.IgnoreFields(xpv1.ConditionedStatus{}, "Conditions")); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

var _ tfclient.TfProxyConnectorI[*v1alpha1.ServiceInstance] = &TfProxyClientCreatorMock{}

type TfProxyClientCreatorMock struct {
	err error
}

func (t *TfProxyClientCreatorMock) Connect(ctx context.Context, cr *v1alpha1.ServiceInstance) (tfclient.TfProxyControllerI, error) {
	if t.err != nil {
		return nil, t.err
	}
	return &TfProxyMock{}, nil
}

var _ Initializer = &InitializerMock{}

type InitializerMock struct {
	err error
}

// Initialize implements Initializer.
func (i *InitializerMock) Initialize(kube client.Client, ctx context.Context, mg resource.Managed) error {
	return i.err
}

var _ tfclient.TfProxyControllerI = &TfProxyMock{}

type TfProxyMock struct {
	status       tfclient.Status
	data         *tfclient.ObservationData
	err          error
	updateErr    error
	details      map[string][]byte
	deleteCalled bool
	tfResource   resource.Managed
}

func (t *TfProxyMock) Delete(ctx context.Context) error {
	t.deleteCalled = true
	return t.err
}

func (t *TfProxyMock) QueryAsyncData(ctx context.Context) *tfclient.ObservationData {
	return t.data
}

func (t *TfProxyMock) Create(ctx context.Context) error {
	return t.err
}

func (t *TfProxyMock) Observe(context context.Context) (tfclient.Status, map[string][]byte, error) {
	return t.status, t.details, t.err
}

func (t *TfProxyMock) Update(ctx context.Context) error {
	if t.updateErr != nil {
		return t.updateErr
	}
	return t.err
}

func (t *TfProxyMock) GetTfResource() resource.Managed {
	return t.tfResource
}

func expectedErrorBehaviour(t *testing.T, expectedErr error, gotErr error) {
	if gotErr != nil {
		if expectedErr == nil {
			t.Errorf("expected no error, got %v", gotErr)
			return
		}
		if !errors.Is(gotErr, expectedErr) && gotErr.Error() != expectedErr.Error() {
			t.Errorf("expected error %v, got %v", expectedErr, gotErr)
		}
		return
	}
	if expectedErr != nil {
		t.Errorf("expected error %v, got nil", expectedErr.Error())
	}
}

// Helper function to build a complete ServiceInstance CR dynamically
func expectedServiceInstance(opts ...func(*v1alpha1.ServiceInstance)) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}

	// Apply each option to modify the CR
	for _, opt := range opts {
		opt(cr)
	}

	return cr
}

// Option to set the external name annotation
func withExternalName(externalName string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		if cr.GetAnnotations() == nil {
			cr.SetAnnotations(map[string]string{})
		}
		cr.GetAnnotations()["crossplane.io/external-name"] = externalName
	}
}

// withAsyncOperationMarker records the generation an async operation was
// launched against, as Create()/Update() do.
func withAsyncOperationMarker(generation int64) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		meta.AddAnnotations(cr, map[string]string{AsyncOperationGenerationKey: strconv.FormatInt(generation, 10)})
	}
}

// Option to mark the CR as being deleted. The timestamp is fixed so that the
// want/got CRs compare equal.
func withDeletionTimestamp() func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		ts := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		cr.SetDeletionTimestamp(&ts)
	}
}

// Option to set observation data (e.g., ID)
func withObservationData(id string, planId string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.AtProvider = v1alpha1.ServiceInstanceObservation{
			ID:            id,
			ServiceplanID: planId,
		}
	}
}

// Option to set conditions
func withConditions(conditions ...xpv1.Condition) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.Conditions = conditions
	}
}

// withConflictCondition sets the LastAsyncOperation condition with a Conflict message,
// simulating a failed Create() that hit an "already exists" error.
// It stamps the CR's current generation, so a case that wants a stale condition
// must bump Generation afterwards.
func withConflictCondition(cr *v1alpha1.ServiceInstance) *v1alpha1.ServiceInstance {
	cr.SetConditions(xpv1.Condition{
		Type:               ujresource.TypeLastAsyncOperation,
		Status:             "False",
		Reason:             ujresource.ReasonAsyncCreateFailure,
		Message:            "Conflict: resource already exists",
		ObservedGeneration: cr.Generation,
	})
	return cr
}

// withAsyncFailureCondition records a broker-rejected apply on the CR, as the
// async callback would.
func withAsyncFailureCondition(reason xpv1.ConditionReason, message string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.SetConditions(xpv1.Condition{
			Type:               ujresource.TypeLastAsyncOperation,
			Status:             corev1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: cr.Generation,
		})
	}
}

// Option to set the full AtProvider observation struct
func withAtProvider(obs v1alpha1.ServiceInstanceObservation) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.AtProvider = obs
	}
}

func TestSaveInstanceData(t *testing.T) {
	ready := true
	usable := true
	createdDate := metav1.NewTime(time.Date(2023, 1, 15, 10, 30, 0, 0, time.UTC))
	lastModified := metav1.NewTime(time.Date(2023, 1, 16, 10, 30, 0, 0, time.UTC))

	cases := map[string]struct {
		reason string
		cr     *v1alpha1.ServiceInstance
		sid    tfclient.ObservationData
		want   *v1alpha1.ServiceInstance
	}{
		"SavesBasicFields": {
			reason: "should save ID and DashboardURL to the CR status",
			cr:     &v1alpha1.ServiceInstance{},
			sid: tfclient.ObservationData{
				ExternalName: "some-ext-name",
				ID:           "some-id",
				DashboardURL: "https://dashboard.example.com",
			},
			want: expectedServiceInstance(
				withExternalName("some-ext-name"),
				withAtProvider(v1alpha1.ServiceInstanceObservation{
					ID:           "some-id",
					DashboardURL: "https://dashboard.example.com",
				}),
			),
		},
		"SavesAllObservationFields": {
			reason: "should save all new observation fields (CreatedDate, LastModified, State, Ready, Usable, PlatformID) to the CR status",
			cr:     &v1alpha1.ServiceInstance{},
			sid: tfclient.ObservationData{
				ExternalName: "some-ext-name",
				ID:           "some-id",
				DashboardURL: "https://dashboard.example.com",
				CreatedDate:  &createdDate,
				LastModified: &lastModified,
				State:        "succeeded",
				Ready:        &ready,
				Usable:       &usable,
				PlatformID:   "test-platform",
			},
			want: expectedServiceInstance(
				withExternalName("some-ext-name"),
				withAtProvider(v1alpha1.ServiceInstanceObservation{
					ID:           "some-id",
					DashboardURL: "https://dashboard.example.com",
					CreatedDate:  &createdDate,
					LastModified: &lastModified,
					State:        "succeeded",
					Ready:        &ready,
					Usable:       &usable,
					PlatformID:   "test-platform",
				}),
			),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			err := e.saveInstanceData(context.Background(), tc.cr, tc.sid)
			if err != nil {
				t.Errorf("saveInstanceData() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, tc.cr); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}
