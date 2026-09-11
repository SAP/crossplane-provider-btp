package serviceinstance

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	smopenapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

var (
	errClient      = errors.New("apiError")
	errInitializer = errors.New("initializerError")
	errTracking    = errors.New("trackingError")
	errClientBuild = errors.New("clientBuildError")
)

const validUUID = "550e8400-e29b-41d4-a716-446655440000"

// ====================================================================================
// Native client mock
// ====================================================================================

var _ siClient.ServiceInstanceClientI = &nativeClientMock{}

type nativeClientMock struct {
	observeRes siClient.ObserveResult
	observeErr error

	createID    string
	createOpID  string
	createErr   error

	updateOpID string
	updateErr  error

	deleteErr error

	getParamsRes map[string]interface{}
	getParamsErr error

	retrievableRes bool
	retrievableErr error
	retrievableOfferingID string

	// captured call args
	createCalled bool
	updateCalled bool
	deleteCalled bool

	updatePayloadCR *v1alpha1.ServiceInstance
	updateObserved  *smopenapi.ServiceInstanceResponseObject
}

func (m *nativeClientMock) Observe(ctx context.Context, externalName string) (siClient.ObserveResult, error) {
	return m.observeRes, m.observeErr
}

func (m *nativeClientMock) Create(ctx context.Context, cr *v1alpha1.ServiceInstance, params map[string]interface{}) (string, string, error) {
	m.createCalled = true
	return m.createID, m.createOpID, m.createErr
}

func (m *nativeClientMock) Update(ctx context.Context, externalName string, cr *v1alpha1.ServiceInstance,
	params map[string]interface{}, observed *smopenapi.ServiceInstanceResponseObject) (string, error) {
	m.updateCalled = true
	m.updatePayloadCR = cr
	m.updateObserved = observed
	return m.updateOpID, m.updateErr
}

func (m *nativeClientMock) Delete(ctx context.Context, externalName string) error {
	m.deleteCalled = true
	return m.deleteErr
}

func (m *nativeClientMock) GetParameters(ctx context.Context, instanceID string) (map[string]interface{}, error) {
	return m.getParamsRes, m.getParamsErr
}

func (m *nativeClientMock) InstancesRetrievable(ctx context.Context, planID string) (bool, string, error) {
	return m.retrievableRes, m.retrievableOfferingID, m.retrievableErr
}

// ====================================================================================
// Connect Tests
// ====================================================================================

func TestConnect(t *testing.T) {
	type fields struct {
		clientErr       error
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
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMockWithError(errTracking),
			},
			args: args{mg: &v1alpha1.ServiceInstance{}},
			want: want{err: errTracking, trackCalled: true},
		},
		"InitializerError": {
			reason: "should return an error when the initializer fails",
			fields: fields{
				initializer:     &InitializerMock{err: errInitializer},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{mg: &v1alpha1.ServiceInstance{}},
			want: want{err: errInitializer, trackCalled: true},
		},
		"ClientBuildError": {
			reason: "should return an error when building the native client fails",
			fields: fields{
				clientErr:       errClientBuild,
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{mg: &v1alpha1.ServiceInstance{}},
			want: want{err: errClientBuild, trackCalled: true},
		},
		"Success": {
			reason: "should return a client when everything succeeds",
			fields: fields{
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{mg: &v1alpha1.ServiceInstance{}},
			want: want{err: nil, trackCalled: true},
		},
		"WrongType": {
			reason: "should return an error when mg is not a ServiceInstance",
			fields: fields{
				initializer:     &InitializerMock{},
				resourcetracker: testutils.NewResourceTrackerMock(),
			},
			args: args{mg: &v1alpha1.Subaccount{}},
			want: want{err: errors.New(errNotServiceInstance), trackCalled: false},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				kube:                        &test.MockClient{},
				newServicePlanInitializerFn: func() Initializer { return tc.fields.initializer },
				resourcetracker:             tc.fields.resourcetracker,
				newServiceInstanceClientFn: func(ctx context.Context, cr *v1alpha1.ServiceInstance) (siClient.ServiceInstanceClientI, error) {
					if tc.fields.clientErr != nil {
						return nil, tc.fields.clientErr
					}
					return &nativeClientMock{}, nil
				},
			}
			_, err := c.Connect(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
			if tc.want.trackCalled != tc.fields.resourcetracker.TrackCalled {
				t.Errorf("expected Track() called=%v, got=%v", tc.want.trackCalled, tc.fields.resourcetracker.TrackCalled)
			}
		})
	}
}

// ====================================================================================
// Observe Tests
// ====================================================================================

func TestObserve(t *testing.T) {
	type fields struct {
		client *nativeClientMock
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		o              managed.ExternalObservation
		err            error
		wantDriftCond  bool
		wantAvailable  bool
		wantCreating   bool
		wantNoReadyChg bool
	}

	ready := true

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"FallbackExternalName_NoCreatePending_NotExisting": {
			reason: "empty/fallback external-name and no create attempt -> not existing, no lookup",
			fields: fields{client: &nativeClientMock{}},
			args:   args{mg: siWithName("inst-1")},
			want:   want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"InvalidUUID": {
			reason: "should error when external-name is set but not a valid UUID",
			fields: fields{client: &nativeClientMock{}},
			args:   args{mg: siWithExternalName("inst-1", "not-a-uuid")},
			want: want{
				err: errors.New("external-name is not a valid UUID. Please check the value of the external-name annotation and set it to the ServiceInstance ID (UUID format) if you want to adopt an existing resource, or remove the annotation if you want to create a new one"),
			},
		},
		"ObserveError": {
			reason: "should wrap client observe error",
			fields: fields{client: &nativeClientMock{observeErr: errClient}},
			args:   args{mg: siWithExternalName("inst-1", validUUID)},
			want:   want{err: errClient},
		},
		"NotFound": {
			reason: "should return not existing when the instance is gone",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{Exists: false}}},
			args:   args{mg: siWithExternalName("inst-1", validUUID)},
			want:   want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"InProgress": {
			reason: "in-progress provisioning (not yet Ready) -> up-to-date to suppress Update, Ready=Creating",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists:   true,
				Instance: instanceWith(validUUID, opStateInProgress, "plan-1", "inst-1", nil),
			}}},
			args: args{mg: withPlan(siWithExternalName("inst-1", validUUID), "plan-1")},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, wantCreating: true},
		},
		"InProgressUpdate": {
			reason: "in-progress update on an already-usable instance -> up-to-date, existing Ready left untouched",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists: true,
				Instance: func() *smopenapi.ServiceInstanceResponseObject {
					in := instanceWith(validUUID, opStateInProgress, "plan-1", "inst-1", nil)
					in.Ready = internal.Ptr(true)
					return in
				}(),
			}}},
			args: args{mg: withPlan(siWithExternalName("inst-1", validUUID), "plan-1")},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, wantNoReadyChg: true},
		},
		"Failed": {
			reason: "failed operation -> exists, not up to date, drift/ready-false condition",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists:   true,
				Instance: instanceWith(validUUID, opStateFailed, "plan-1", "inst-1", nil),
			}}},
			args: args{mg: withPlan(siWithExternalName("inst-1", validUUID), "plan-1")},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, wantDriftCond: true},
		},
		"UpToDate": {
			reason: "succeeded operation, no drift -> up to date + Available",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists: true,
				Instance: func() *smopenapi.ServiceInstanceResponseObject {
					in := instanceWith(validUUID, opStateSucceeded, "plan-1", "inst-1", nil)
					in.Ready = &ready
					return in
				}(),
			}}},
			args: args{mg: withPlan(siWithExternalName("inst-1", validUUID), "plan-1")},
			want: want{
				o:             managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{}},
				wantAvailable: true,
			},
		},
		"DriftName": {
			reason: "name drift -> not up to date, drift condition",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists:   true,
				Instance: instanceWith(validUUID, opStateSucceeded, "plan-1", "actual-name", nil),
			}}},
			args: args{mg: withPlan(siWithExternalName("desired-name", validUUID), "plan-1")},
			want: want{
				o:             managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, ConnectionDetails: managed.ConnectionDetails{}},
				wantDriftCond: true,
			},
		},
		"DriftPlan": {
			reason: "service plan drift -> not up to date",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists:   true,
				Instance: instanceWith(validUUID, opStateSucceeded, "actual-plan", "inst-1", nil),
			}}},
			args: args{mg: withPlan(siWithExternalName("inst-1", validUUID), "desired-plan")},
			want: want{
				o:             managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, ConnectionDetails: managed.ConnectionDetails{}},
				wantDriftCond: true,
			},
		},
		"DriftLabels": {
			reason: "label drift -> not up to date",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists:   true,
				Instance: instanceWith(validUUID, opStateSucceeded, "plan-1", "inst-1", map[string][]string{"team": {"red"}}),
			}}},
			args: args{mg: withLabels(withPlan(siWithExternalName("inst-1", validUUID), "plan-1"), map[string][]*string{"team": {internal.Ptr("blue")}})},
			want: want{
				o:             managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, ConnectionDetails: managed.ConnectionDetails{}},
				wantDriftCond: true,
			},
		},
		"DriftShared": {
			reason: "shared drift when managed -> not up to date",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists: true,
				Instance: func() *smopenapi.ServiceInstanceResponseObject {
					in := instanceWith(validUUID, opStateSucceeded, "plan-1", "inst-1", nil)
					in.Shared = internal.Ptr(false)
					return in
				}(),
			}}},
			args: args{mg: withShared(withPlan(siWithExternalName("inst-1", validUUID), "plan-1"), true)},
			want: want{
				o:             managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, ConnectionDetails: managed.ConnectionDetails{}},
				wantDriftCond: true,
			},
		},
		"ObserveOnly_SuppressesAvailable": {
			reason: "observe-only management policy must not set Available",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists: true,
				Instance: func() *smopenapi.ServiceInstanceResponseObject {
					in := instanceWith(validUUID, opStateSucceeded, "plan-1", "inst-1", nil)
					in.Ready = &ready
					return in
				}(),
			}}},
			args: args{mg: withObserveOnly(withPlan(siWithExternalName("inst-1", validUUID), "plan-1"))},
			want: want{
				o:             managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{}},
				wantAvailable: false,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				client: tc.fields.client,
				kube:   &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil), MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil)},
			}

			got, err := e.Observe(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.o, got, cmp.FilterPath(func(p cmp.Path) bool {
				return p.String() == "Diff"
			}, cmp.Ignore())); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				return
			}
			if tc.want.wantDriftCond {
				readyCond := cr.GetCondition(xpv1.TypeReady)
				if readyCond.Status != "False" {
					t.Errorf("\n%s\nexpected Ready=False condition, got %+v", tc.reason, readyCond)
				}
			}
			if tc.want.wantAvailable {
				readyCond := cr.GetCondition(xpv1.TypeReady)
				if readyCond.Reason != xpv1.ReasonAvailable {
					t.Errorf("\n%s\nexpected Available condition, got %+v", tc.reason, readyCond)
				}
			}
			if tc.want.wantCreating {
				readyCond := cr.GetCondition(xpv1.TypeReady)
				if readyCond.Reason != xpv1.ReasonCreating {
					t.Errorf("\n%s\nexpected Creating condition, got %+v", tc.reason, readyCond)
				}
			}
			if tc.want.wantNoReadyChg {
				readyCond := cr.GetCondition(xpv1.TypeReady)
				if readyCond.Reason == xpv1.ReasonCreating {
					t.Errorf("\n%s\nexpected Ready to be left untouched, but it was downgraded to Creating: %+v", tc.reason, readyCond)
				}
			}
		})
	}
}

// ====================================================================================
// Create Tests
// ====================================================================================

func TestCreate(t *testing.T) {
	type fields struct {
		client *nativeClientMock
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err            error
		externalName   string
		wantCreateCall bool
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"HappyPath_SetsExternalName": {
			reason: "should set external-name from the created GUID and set Creating",
			fields: fields{client: &nativeClientMock{createID: validUUID}},
			args:   args{mg: siWithName("inst-1")},
			want:   want{externalName: validUUID, wantCreateCall: true},
		},
		"ApiError_LeavesFallbackExternalName": {
			reason: "on create error external-name must stay fallback (unset)",
			fields: fields{client: &nativeClientMock{createErr: errClient}},
			args:   args{mg: siWithName("inst-1")},
			want:   want{err: errClient, externalName: "", wantCreateCall: true},
		},
		"WrongType": {
			reason: "should error when mg is not a ServiceInstance",
			fields: fields{client: &nativeClientMock{}},
			args:   args{mg: &v1alpha1.Subaccount{}},
			want:   want{err: errors.New(errNotServiceInstance)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				client: tc.fields.client,
				kube:   &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			}
			_, err := e.Create(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)

			if tc.want.wantCreateCall != tc.fields.client.createCalled {
				t.Errorf("expected create called=%v got=%v", tc.want.wantCreateCall, tc.fields.client.createCalled)
			}

			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				return
			}
			if got := meta.GetExternalName(cr); got != tc.want.externalName {
				t.Errorf("\n%s\nexternal-name=%q, want %q", tc.reason, got, tc.want.externalName)
			}
			if creating := cr.GetCondition(xpv1.TypeReady); creating.Reason != xpv1.ReasonCreating {
				t.Errorf("\n%s\nexpected Creating condition, got %+v", tc.reason, creating)
			}
		})
	}
}

// ====================================================================================
// Update Tests
// ====================================================================================

func TestUpdate(t *testing.T) {
	type fields struct {
		client *nativeClientMock
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err          error
		updateCalled bool
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"HappyPath": {
			reason: "should update passing observed instance for label diff",
			fields: fields{client: &nativeClientMock{observeRes: siClient.ObserveResult{
				Exists:   true,
				Instance: instanceWith(validUUID, opStateSucceeded, "plan-1", "inst-1", nil),
			}}},
			args: args{mg: withPlan(siWithExternalName("inst-1", validUUID), "plan-1")},
			want: want{updateCalled: true},
		},
		"UpdateError": {
			reason: "should wrap update error",
			fields: fields{client: &nativeClientMock{
				observeRes: siClient.ObserveResult{Exists: true, Instance: instanceWith(validUUID, opStateSucceeded, "plan-1", "inst-1", nil)},
				updateErr:  errClient,
			}},
			args: args{mg: withPlan(siWithExternalName("inst-1", validUUID), "plan-1")},
			want: want{err: errClient, updateCalled: true},
		},
		"WrongType": {
			reason: "should error when mg is not a ServiceInstance",
			fields: fields{client: &nativeClientMock{}},
			args:   args{mg: &v1alpha1.Subaccount{}},
			want:   want{err: errors.New(errNotServiceInstance)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				client: tc.fields.client,
				kube:   &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			}
			_, err := e.Update(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
			if tc.want.updateCalled != tc.fields.client.updateCalled {
				t.Errorf("expected update called=%v got=%v", tc.want.updateCalled, tc.fields.client.updateCalled)
			}
		})
	}
}

// ====================================================================================
// Delete Tests
// ====================================================================================

func TestDelete_DeletionBlocking(t *testing.T) {
	type fields struct {
		client  *nativeClientMock
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
			fields: fields{client: &nativeClientMock{}, tracker: testutils.NewResourceTrackerMockBlocking()},
			args:   args{mg: &v1alpha1.ServiceInstance{}},
			want:   want{err: errors.New(providerv1alpha1.ErrResourceInUse), setConditionsCalled: true, deleteAttempted: false},
		},
		"AllowedWhenNoBindings": {
			reason: "should allow deletion when nothing references this instance",
			fields: fields{client: &nativeClientMock{}, tracker: testutils.NewResourceTrackerMock()},
			args:   args{mg: &v1alpha1.ServiceInstance{}},
			want:   want{err: nil, setConditionsCalled: true, deleteAttempted: true},
		},
		"DeleteAPIErrorWhenNotBlocked": {
			reason: "should return API error when deletion proceeds but API fails",
			fields: fields{client: &nativeClientMock{deleteErr: errClient}, tracker: testutils.NewResourceTrackerMock()},
			args:   args{mg: &v1alpha1.ServiceInstance{}},
			want:   want{err: errClient, setConditionsCalled: true, deleteAttempted: true},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				client:  tc.fields.client,
				kube:    &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
				tracker: tc.fields.tracker,
			}
			_, err := e.Delete(context.Background(), tc.args.mg)

			if tc.want.err != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tc.want.err)
				} else if !errors.Is(err, tc.want.err) && err.Error() != tc.want.err.Error() {
					t.Errorf("expected error %v, got %v", tc.want.err, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tc.want.setConditionsCalled != tc.fields.tracker.SetConditionsCalled {
				t.Errorf("expected SetConditions called=%v got=%v", tc.want.setConditionsCalled, tc.fields.tracker.SetConditionsCalled)
			}
			if tc.want.deleteAttempted != tc.fields.client.deleteCalled {
				t.Errorf("expected Delete called=%v got=%v", tc.want.deleteAttempted, tc.fields.client.deleteCalled)
			}
		})
	}
}

// ====================================================================================
// saveInstanceData Tests
// ====================================================================================

func TestSaveInstanceData(t *testing.T) {
	ready := true
	usable := true
	shared := true

	in := instanceWith(validUUID, opStateSucceeded, "plan-1", "inst-1", nil)
	in.DashboardUrl = internal.Ptr("https://dash")
	in.PlatformId = internal.Ptr("platform-1")
	in.Ready = &ready
	in.Usable = &usable
	in.Shared = &shared

	cr := &v1alpha1.ServiceInstance{}
	e := external{kube: &test.MockClient{}}
	if err := e.saveInstanceData(context.Background(), cr, in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := v1alpha1.ServiceInstanceObservation{
		ID:           validUUID,
		DashboardURL: "https://dash",
		State:        opStateSucceeded,
		Ready:        &ready,
		Usable:       &usable,
		PlatformID:   "platform-1",
		Shared:       &shared,
	}
	if diff := cmp.Diff(want, cr.Status.AtProvider); diff != "" {
		t.Errorf("status mismatch (-want +got):\n%s", diff)
	}
}

func TestIsValidUUID(t *testing.T) {
	if !isValidUUID(validUUID) {
		t.Errorf("expected %q to be valid", validUUID)
	}
	if isValidUUID("not-a-uuid") {
		t.Errorf("expected invalid")
	}
}

// ====================================================================================
// Helpers
// ====================================================================================

var _ Initializer = &InitializerMock{}

type InitializerMock struct {
	err error
}

func (i *InitializerMock) Initialize(kube client.Client, ctx context.Context, mg resource.Managed) error {
	return i.err
}

func instanceWith(id, state, planID, name string, labels map[string][]string) *smopenapi.ServiceInstanceResponseObject {
	in := &smopenapi.ServiceInstanceResponseObject{
		Id:            internal.Ptr(id),
		Name:          internal.Ptr(name),
		ServicePlanId: internal.Ptr(planID),
	}
	if state != "" {
		in.LastOperation = &smopenapi.OperationResponseObject{State: internal.Ptr(state)}
	}
	if labels != nil {
		in.Labels = &labels
	}
	return in
}

func siWithName(name string) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}
	cr.SetName(name)
	cr.Spec.ForProvider.Name = name
	return cr
}

// expectedServiceInstance builds a ServiceInstance CR by applying option funcs.
// Retained for the initializer tests.
func expectedServiceInstance(opts ...func(*v1alpha1.ServiceInstance)) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}
	for _, opt := range opts {
		opt(cr)
	}
	return cr
}

// withObservationData sets ID and ServiceplanID in the CR status.
func withObservationData(id string, planID string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.AtProvider = v1alpha1.ServiceInstanceObservation{
			ID:            id,
			ServiceplanID: planID,
		}
	}
}

func siWithExternalName(name, externalName string) *v1alpha1.ServiceInstance {
	cr := siWithName(name)
	meta.SetExternalName(cr, externalName)
	return cr
}

func withPlan(cr *v1alpha1.ServiceInstance, planID string) *v1alpha1.ServiceInstance {
	cr.Status.AtProvider.ServiceplanID = planID
	return cr
}

func withLabels(cr *v1alpha1.ServiceInstance, labels map[string][]*string) *v1alpha1.ServiceInstance {
	cr.Spec.ForProvider.Labels = labels
	return cr
}

func withShared(cr *v1alpha1.ServiceInstance, shared bool) *v1alpha1.ServiceInstance {
	cr.Spec.ForProvider.Shared = internal.Ptr(shared)
	return cr
}

func withObserveOnly(cr *v1alpha1.ServiceInstance) *v1alpha1.ServiceInstance {
	cr.Spec.ManagementPolicies = []xpv1.ManagementAction{xpv1.ManagementActionObserve}
	return cr
}

func expectedErrorBehaviour(t *testing.T, expectedErr error, gotErr error) {
	t.Helper()
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

// keep imports used across files
var _ = cmpopts.IgnoreFields
var _ = metav1.Now

// TestCalculateDiff_IgnoresReservedLabels ensures the SM-managed subaccount_id
// label present on the observed instance does not register as drift when the
// spec declares no labels. Otherwise every reconcile would trigger a spurious
// Update that BTP rejects with "Modifying is not allowed for label subaccount_id".
func TestCalculateDiff_IgnoresReservedLabels(t *testing.T) {
	e := &external{}
	cr := &v1alpha1.ServiceInstance{}
	cr.Spec.ForProvider.Name = "my-instance"
	cr.Status.AtProvider.ServiceplanID = "plan-1"

	instance := &smopenapi.ServiceInstanceResponseObject{
		Name:          internal.Ptr("my-instance"),
		ServicePlanId: internal.Ptr("plan-1"),
		Labels: &map[string][]string{
			"subaccount_id": {"550e8400-e29b-41d4-a716-446655440000"},
		},
	}

	diff, err := e.calculateDiff(context.Background(), cr, instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != "" {
		t.Errorf("expected no drift when only reserved labels differ, got: %s", diff)
	}
}
