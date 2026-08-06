package serviceinstance

import (
	"context"
	"errors"
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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/mrstatus"
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
					withConditions(
						xpv1.Creating(),
					),
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
				cr:  expectedServiceInstance(),
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
				cr:  expectedServiceInstance(),
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
				cr:  expectedServiceInstance(withExternalName("550e8400-e29b-41d4-a716-446655440000")),
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

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GetError": {
			reason: "should return an error if the ServiceInstance cannot be retrieved",
			args: args{
				kube: &test.MockClient{MockGet: test.NewMockGetFn(errKube)},
				name: types.NamespacedName{Name: "test-instance"},
			},
			want: want{
				err: errKube,
			},
		},
		"UpdateError": {
			reason: "should return an error if the ServiceInstance status cannot be updated",
			args: args{
				kube: &test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(errKube),
				},
				name:       types.NamespacedName{Name: "test-instance"},
				conditions: []xpv1.Condition{ujresource.AsyncOperationFinishedCondition()},
			},
			want: want{
				err: errKube,
			},
		},
		"Success": {
			reason: "should successfully save conditions to the ServiceInstance",
			args: args{
				kube: &test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				name:       types.NamespacedName{Name: "test-instance"},
				conditions: []xpv1.Condition{ujresource.AsyncOperationFinishedCondition()},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := newSaveCallback(nil)(context.Background(), tc.args.kube, tc.args.name, tc.args.conditions...)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

// TestSaveCallback_LooksUpTheNativeResource pins the identity the callback
// resolves to. Before the shadow identity was resolved, the lookup key was the
// stringified NamespacedName of the terraform shadow ("/TF-test-instance"),
// which never matched an object and made every async result vanish.
func TestSaveCallback_LooksUpTheNativeResource(t *testing.T) {
	var gotKey types.NamespacedName
	kube := &test.MockClient{
		MockGet: func(_ context.Context, key client.ObjectKey, _ client.Object) error {
			gotKey = key
			return nil
		},
		MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
	}

	want := types.NamespacedName{Name: "test-instance"}
	if err := newSaveCallback(nil)(context.Background(), kube, want, ujresource.AsyncOperationFinishedCondition()); err != nil {
		t.Fatalf("saveCallback returned unexpected error: %v", err)
	}
	if diff := cmp.Diff(want, gotKey); diff != "" {
		t.Errorf("lookup key mismatch (-want, +got):\n%s", diff)
	}
}

// TestSaveCallback_RetriesOnConflict pins that a conflicting status write is
// retried instead of dropping the async result.
//
// Upjet calls an async callback exactly once and only logs whatever error it
// returns, so a single lost write is a permanently lost async result: an
// unrecorded create failure lets the instance report Available, an unrecorded
// destroy failure leaves the deprovision retrying blind with nothing recorded
// anywhere. The callback races the reconciler's own status writes (the
// ServiceInstance status is written on every poll), so a conflict is routine.
func TestSaveCallback_RetriesOnConflict(t *testing.T) {
	gets, updates := 0, 0
	kube := &test.MockClient{
		MockGet: func(_ context.Context, _ client.ObjectKey, _ client.Object) error {
			gets++
			return nil
		},
		MockStatusUpdate: func(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
			updates++
			if updates == 1 {
				return kerrors.NewConflict(
					schema.GroupResource{Group: v1alpha1.CRDGroup, Resource: "serviceinstances"},
					"test-instance", errors.New("object has been modified"))
			}
			return nil
		},
	}

	err := newSaveCallback(nil)(context.Background(), kube,
		types.NamespacedName{Name: "test-instance"},
		ujresource.LastAsyncOperationCondition(errors.New("destroy failed")))
	if err != nil {
		t.Fatalf("expected the conflicting write to be retried, got error: %v", err)
	}
	if updates != 2 {
		t.Errorf("expected a second status update after the conflict, got %d update(s)", updates)
	}
	// The retry has to be rebased on a re-read object, otherwise it just
	// resubmits the same stale resourceVersion and conflicts again.
	if gets != 2 {
		t.Errorf("expected the object to be re-read before the retry, got %d get(s)", gets)
	}
}

// TestSaveCallback_ReadsThroughTheGivenReader pins that the (uncached) reader
// Setup hands in is what the object is read from: rebasing a conflict retry on
// a stale informer cache would only reproduce the conflict.
func TestSaveCallback_ReadsThroughTheGivenReader(t *testing.T) {
	readerUsed := false
	reader := &test.MockClient{
		MockGet: func(_ context.Context, _ client.ObjectKey, _ client.Object) error {
			readerUsed = true
			return nil
		},
	}
	kube := &test.MockClient{
		MockGet:          test.NewMockGetFn(errKube),
		MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
	}

	if err := newSaveCallback(reader)(context.Background(), kube,
		types.NamespacedName{Name: "test-instance"},
		ujresource.AsyncOperationFinishedCondition()); err != nil {
		t.Fatalf("saveCallback returned unexpected error: %v", err)
	}
	if !readerUsed {
		t.Error("expected the supplied reader to be used for the lookup")
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
// ObservedGeneration defaults to 0, matching a CR with Generation=0 (no spec change yet).
func withConflictCondition(cr *v1alpha1.ServiceInstance) *v1alpha1.ServiceInstance {
	cr.SetConditions(xpv1.Condition{
		Type:               ujresource.TypeLastAsyncOperation,
		Status:             "False",
		Reason:             "ReconcileError",
		Message:            "Conflict: resource already exists",
		ObservedGeneration: cr.Generation,
	})
	return cr
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

// ====================================================================================
// External health readiness (upstream issue #280)
// ====================================================================================

// TestObserve_ExternalHealth pins that readiness follows the health BTP
// reports for the instance, not merely the fact that terraform applied the
// spec. Before this, the up-to-date branch set Available unconditionally, so
// instances the platform reported as failed still showed Ready=True.
func TestObserve_ExternalHealth(t *testing.T) {
	// Deliberately not an ApplyFailure and free of the "Conflict" marker: both
	// are short-circuited earlier in Observe by the create-conflict handling.
	tfDetail := "destroy failed: the service instance is in state failed and cannot be deprovisioned"

	cases := map[string]struct {
		reason string
		data   *tfclient.ObservationData
		mutate func(*v1alpha1.ServiceInstance)

		wantReadyStatus  corev1.ConditionStatus
		wantReason       xpv1.ConditionReason
		wantMsgContains  []string
		wantNoReadyCond  bool
		wantMsgMaxLength int
	}{
		"Failed_MaskedCohort": {
			reason: "state=failed must not report Available even when BTP still claims ready and usable",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "failed", Ready: internal.Ptr(true), Usable: internal.Ptr(true),
			},
			wantReadyStatus: corev1.ConditionFalse,
			wantReason:      mrstatus.ReasonExternalResourceFailed,
			wantMsgContains: []string{`state="failed"`, "ready=true", "usable=true"},
		},
		"Failed_NotReadyNotUsable": {
			reason: "state=failed with ready=false/usable=false must report Ready=False",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "failed", Ready: internal.Ptr(false), Usable: internal.Ptr(false),
			},
			wantReadyStatus: corev1.ConditionFalse,
			wantReason:      mrstatus.ReasonExternalResourceFailed,
			wantMsgContains: []string{"ready=false", "usable=false"},
		},
		"Healthy": {
			reason: "a succeeded, ready and usable instance must report Available",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "succeeded", Ready: internal.Ptr(true), Usable: internal.Ptr(true),
			},
			wantReadyStatus: corev1.ConditionTrue,
			wantReason:      xpv1.ReasonAvailable,
		},
		"NotReadyOnNonDeleting": {
			reason: "ready=false on a non-deleting instance is unhealthy even without state=failed",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "in progress", Ready: internal.Ptr(false),
			},
			wantReadyStatus: corev1.ConditionFalse,
			wantReason:      mrstatus.ReasonExternalResourceFailed,
			wantMsgContains: []string{"usable=unknown"},
		},
		"NotReadyWhileDeleting": {
			reason: "ready=false while a deprovision is in flight is expected, not a failure",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "in progress", Ready: internal.Ptr(false), Usable: internal.Ptr(false),
			},
			mutate: func(cr *v1alpha1.ServiceInstance) {
				ts := metav1.Now()
				cr.SetDeletionTimestamp(&ts)
			},
			wantReadyStatus: corev1.ConditionTrue,
			wantReason:      xpv1.ReasonAvailable,
		},
		"FailedWhileDeleting": {
			reason: "state=failed counts unconditionally, including during a deprovision",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "failed", Ready: internal.Ptr(false), Usable: internal.Ptr(false),
			},
			mutate: func(cr *v1alpha1.ServiceInstance) {
				ts := metav1.Now()
				cr.SetDeletionTimestamp(&ts)
			},
			wantReadyStatus: corev1.ConditionFalse,
			wantReason:      mrstatus.ReasonExternalResourceFailed,
		},
		"ObserveOnlyPolicy": {
			reason: "readiness is not ours to assert for Observe-only resources",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "failed", Ready: internal.Ptr(false), Usable: internal.Ptr(false),
			},
			mutate: func(cr *v1alpha1.ServiceInstance) {
				cr.Spec.ManagementPolicies = xpv1.ManagementPolicies{xpv1.ManagementActionObserve}
			},
			wantNoReadyCond: true,
		},
		"MessageCarriesTfDetail": {
			reason: "the terraform failure detail must be readable on the managed resource",
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id",
				State: "failed", Ready: internal.Ptr(false), Usable: internal.Ptr(false),
			},
			mutate: func(cr *v1alpha1.ServiceInstance) {
				cr.SetConditions(xpv1.Condition{
					Type:    xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
					Status:  corev1.ConditionFalse,
					Reason:  ujresource.ReasonDestroyFailure,
					Message: tfDetail,
				})
			},
			wantReadyStatus: corev1.ConditionFalse,
			wantReason:      mrstatus.ReasonExternalResourceFailed,
			wantMsgContains: []string{tfDetail},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &v1alpha1.ServiceInstance{}
			if tc.mutate != nil {
				tc.mutate(cr)
			}

			e := external{
				tfClient: &TfProxyMock{status: tfclient.UpToDate, data: tc.data, details: map[string][]byte{}},
				kube:     &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			}

			got, err := e.Observe(context.Background(), cr)
			if err != nil {
				// Synced must stay truthful: an unhealthy external resource is
				// not a reconcile error, so no retry loop is triggered.
				t.Fatalf("\n%s\nObserve must not return an error for an unhealthy instance: %v", tc.reason, err)
			}
			if !got.ResourceExists || !got.ResourceUpToDate {
				t.Errorf("\n%s\nexpected the instance to stay reported as existing and up to date, got %+v", tc.reason, got)
			}

			ready := cr.GetCondition(xpv1.TypeReady)
			if tc.wantNoReadyCond {
				// GetCondition renders an absent condition as Unknown with no reason.
				if ready.Status != corev1.ConditionUnknown || ready.Reason != "" {
					t.Errorf("\n%s\nexpected no Ready condition to be set, got %+v", tc.reason, ready)
				}
				return
			}
			if ready.Status != tc.wantReadyStatus {
				t.Errorf("\n%s\nexpected Ready=%s, got %s (reason %q, message %q)",
					tc.reason, tc.wantReadyStatus, ready.Status, ready.Reason, ready.Message)
			}
			if ready.Reason != tc.wantReason {
				t.Errorf("\n%s\nexpected reason %q, got %q", tc.reason, tc.wantReason, ready.Reason)
			}
			for _, substr := range tc.wantMsgContains {
				if !strings.Contains(ready.Message, substr) {
					t.Errorf("\n%s\nexpected the condition message to contain %q, got: %s", tc.reason, substr, ready.Message)
				}
			}
		})
	}
}

// TestObserve_ExternalHealthMessageIsBounded pins that a huge terraform error
// cannot bloat the status of a failing managed resource.
func TestObserve_ExternalHealthMessageIsBounded(t *testing.T) {
	cr := &v1alpha1.ServiceInstance{}
	cr.SetConditions(xpv1.Condition{
		Type:    xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
		Status:  corev1.ConditionFalse,
		Reason:  ujresource.ReasonDestroyFailure,
		Message: strings.Repeat("z", 16*1024),
	})

	e := external{
		tfClient: &TfProxyMock{
			status:  tfclient.UpToDate,
			details: map[string][]byte{},
			data: &tfclient.ObservationData{
				ExternalName: "some-ext-name", ID: "some-id", State: "failed",
			},
		},
		kube: &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
	}

	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatalf("Observe returned unexpected error: %v", err)
	}

	msg := cr.GetCondition(xpv1.TypeReady).Message
	if len(msg) > 2*mrstatus.MaxMessageBytes {
		t.Errorf("expected the condition message to be bounded, got %d bytes", len(msg))
	}
	if !strings.Contains(msg, "truncated") {
		t.Errorf("expected a truncation marker in the bounded message, got: %s", msg)
	}
}

// TestObserve_DeleteFailureVisible pins that a refused external deprovision is
// visible on the managed resource. Before this, a BTP-side refusal produced no
// event, no condition and no error - the only artifact of the whole failure
// class was an info-level line from upjet's terraform workspace, so a blocked
// teardown looked healthy at every observable layer.
func TestObserve_DeleteFailureVisible(t *testing.T) {
	const destroyErr = "cannot destroy: BTP refused the deprovision"

	deleting := func(cr *v1alpha1.ServiceInstance) {
		ts := metav1.NewTime(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))
		cr.SetDeletionTimestamp(&ts)
	}

	cases := map[string]struct {
		reason string
		mutate func(*v1alpha1.ServiceInstance)

		wantCondition bool
		wantMessage   string
	}{
		"DestroyFailure": {
			reason: "a failed async destroy must surface as an ExternalDeletion condition",
			mutate: func(cr *v1alpha1.ServiceInstance) {
				deleting(cr)
				cr.SetConditions(xpv1.Condition{
					Type:    xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
					Status:  corev1.ConditionFalse,
					Reason:  ujresource.ReasonDestroyFailure,
					Message: destroyErr,
				})
			},
			wantCondition: true,
			wantMessage:   destroyErr,
		},
		"AsyncDeleteFailure": {
			reason: "upjet's AsyncDeleteFailure classification must be recognised too",
			mutate: func(cr *v1alpha1.ServiceInstance) {
				deleting(cr)
				cr.SetConditions(xpv1.Condition{
					Type:    xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
					Status:  corev1.ConditionFalse,
					Reason:  ujresource.ReasonAsyncDeleteFailure,
					Message: destroyErr,
				})
			},
			wantCondition: true,
			wantMessage:   destroyErr,
		},
		"UnknownTypeFallback": {
			reason: "upjet emits a condition of type \"Unknown\" for errors it cannot classify; a BTP refusal relayed through terraform frequently lands there",
			mutate: func(cr *v1alpha1.ServiceInstance) {
				deleting(cr)
				cr.SetConditions(xpv1.Condition{
					Type:    xpv1.ConditionType("Unknown"),
					Status:  corev1.ConditionFalse,
					Reason:  "Unknown",
					Message: destroyErr,
				})
			},
			wantCondition: true,
			wantMessage:   destroyErr,
		},
		"SuccessfulDestroy": {
			reason: "a deleting instance whose last async operation succeeded must not be reported as a delete failure",
			mutate: func(cr *v1alpha1.ServiceInstance) {
				deleting(cr)
				cr.SetConditions(xpv1.Condition{
					Type:   xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
					Status: corev1.ConditionTrue,
					Reason: ujresource.ReasonSuccess,
				})
			},
			wantCondition: false,
		},
		"NotDeleting": {
			reason: "a failed apply on a live instance is not a delete failure",
			mutate: func(cr *v1alpha1.ServiceInstance) {
				cr.SetConditions(xpv1.Condition{
					Type:    xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
					Status:  corev1.ConditionFalse,
					Reason:  ujresource.ReasonDestroyFailure,
					Message: destroyErr,
				})
			},
			wantCondition: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &v1alpha1.ServiceInstance{}
			tc.mutate(cr)
			rec := &recorderFake{}

			e := external{
				tfClient: &TfProxyMock{
					status:  tfclient.UpToDate,
					details: map[string][]byte{},
					data: &tfclient.ObservationData{
						// A valid UUID: Observe persists it as the
						// external-name, and the second pass re-validates it.
						ExternalName: "123e4567-e89b-12d3-a456-426614174000", ID: "some-id",
						State: "succeeded", Ready: internal.Ptr(true), Usable: internal.Ptr(true),
					},
				},
				kube:     &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
				recorder: rec,
			}

			got, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("\n%s\nObserve returned unexpected error: %v", tc.reason, err)
			}
			// Observe must keep reporting the resource as existing so the
			// reconciler re-issues Delete - which is what eventually succeeds
			// once the BTP-side blocker clears.
			if !got.ResourceExists {
				t.Errorf("\n%s\nexpected ResourceExists to stay true so Delete is re-issued, got %+v", tc.reason, got)
			}

			cond := cr.GetCondition(mrstatus.TypeExternalDeletion)
			if !tc.wantCondition {
				if cond.Reason == mrstatus.ReasonDeleteFailed {
					t.Errorf("\n%s\nexpected no ExternalDeletion condition, got %+v", tc.reason, cond)
				}
				if len(rec.events) != 0 {
					t.Errorf("\n%s\nexpected no event, got %v", tc.reason, rec.events)
				}
				return
			}

			if cond.Status != corev1.ConditionFalse || cond.Reason != mrstatus.ReasonDeleteFailed {
				t.Errorf("\n%s\nexpected an ExternalDeletion=False/DeleteFailed condition, got %+v", tc.reason, cond)
			}
			if !strings.Contains(cond.Message, tc.wantMessage) {
				t.Errorf("\n%s\nexpected the last destroy error in the message, got %q", tc.reason, cond.Message)
			}
			if !strings.Contains(cond.Message, "2026-08-05T12:00:00Z") {
				t.Errorf("\n%s\nexpected the attempt context (failing since) in the message, got %q", tc.reason, cond.Message)
			}
			if !rec.has(string(mrstatus.EventReasonDeleteFailed)) {
				t.Errorf("\n%s\nexpected a Warning event, got %v", tc.reason, rec.events)
			}
			if len(rec.events) != 1 {
				t.Fatalf("\n%s\nexpected exactly 1 event, got %d", tc.reason, len(rec.events))
			}

			// A second pass over the SAME failure must not emit another event:
			// the deprovision is retried every reconcile, and a per-pass event
			// would be exactly the spam this replaces.
			if _, err := e.Observe(context.Background(), cr); err != nil {
				t.Fatalf("\n%s\nsecond Observe returned unexpected error: %v", tc.reason, err)
			}
			if len(rec.events) != 1 {
				t.Errorf("\n%s\nexpected the event to be edge-triggered, got %d events", tc.reason, len(rec.events))
			}
		})
	}
}
