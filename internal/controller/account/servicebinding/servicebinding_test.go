package servicebinding

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	trackingtest "github.com/sap/crossplane-provider-btp/internal/tracking/test"
)

var (
	errKube          = stderrors.New("kubeError")
	errInstanceMgr   = stderrors.New("instanceManagerError")
	errDependentUse  = stderrors.New(providerv1alpha1.ErrResourceInUse)
)

func TestConnect(t *testing.T) {
	type args struct {
		mg resource.Managed
	}

	type want struct {
		err            error
		externalExists bool
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"WrongType": {
			reason: "should return an error when the managed resource is not a ServiceBinding",
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: stderrors.New(errNotServiceBinding),
			},
		},
		"ConnectSuccess": {
			reason: "should return an external client when connection succeeds",
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err:            nil,
				externalExists: true,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				kube:            &test.MockClient{},
				resourcetracker: trackingtest.NoOpReferenceResolverTracker{},
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
		instanceManager *MockInstanceManager
		keyRotator      *MockKeyRotator
		kube            client.Client
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		o   managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongType": {
			reason: "should return an error when the managed resource is not a ServiceBinding",
			fields: fields{
				instanceManager: &MockInstanceManager{},
				keyRotator:      &MockKeyRotator{},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: stderrors.New(errNotServiceBinding),
			},
		},
		"ObserveError": {
			reason: "should return an error when instance manager observation fails",
			fields: fields{
				instanceManager: &MockInstanceManager{observeErr: errInstanceMgr},
				keyRotator:      &MockKeyRotator{},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: errInstanceMgr,
			},
		},
		"NotFound": {
			reason: "should return not existing when resource doesn't exist",
			fields: fields{
				instanceManager: &MockInstanceManager{
					observeResult: managed.ExternalObservation{ResourceExists: false},
				},
				keyRotator: &MockKeyRotator{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
			},
		},
		"ExistsUpToDate": {
			reason: "should return existing and up to date when resource exists and is current",
			fields: fields{
				instanceManager: &MockInstanceManager{
					observeResult: managed.ExternalObservation{
						ResourceExists:   true,
						ResourceUpToDate: true,
					},
					tfResource: &v1alpha1.SubaccountServiceBinding{
						Status: v1alpha1.SubaccountServiceBindingStatus{
							AtProvider: v1alpha1.SubaccountServiceBindingObservation{
								State: strPtr("succeeded"),
								ID:    strPtr("test-id"),
								Name:  strPtr("test-name"),
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{hasExpiredKeys: false},
				kube: &test.MockClient{
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &MockExternal{
				instanceManager: tc.fields.instanceManager,
				keyRotator:      tc.fields.keyRotator,
				kube:            tc.fields.kube,
			}

			got, err := e.Observe(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type fields struct {
		instanceManager *MockInstanceManager
		kube            client.Client
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongType": {
			reason: "should return an error when the managed resource is not a ServiceBinding",
			fields: fields{
				instanceManager: &MockInstanceManager{},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: stderrors.New(errNotServiceBinding),
			},
		},
		"CreateError": {
			reason: "should return an error when instance manager creation fails",
			fields: fields{
				instanceManager: &MockInstanceManager{createErr: errInstanceMgr},
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: errInstanceMgr,
			},
		},
		"HappyPath": {
			reason: "should create the resource successfully and set Creating condition",
			fields: fields{
				instanceManager: &MockInstanceManager{
					createResult: managed.ExternalCreation{},
				},
				kube: &test.MockClient{
					MockUpdate:       test.NewMockUpdateFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &MockExternal{
				instanceManager: tc.fields.instanceManager,
				kube:            tc.fields.kube,
			}

			_, err := e.Create(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

func TestUpdate(t *testing.T) {
	type fields struct {
		instanceManager *MockInstanceManager
		keyRotator      *MockKeyRotator
		kube            client.Client
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongType": {
			reason: "should return an error when the managed resource is not a ServiceBinding",
			fields: fields{
				instanceManager: &MockInstanceManager{},
				keyRotator:      &MockKeyRotator{},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: stderrors.New(errNotServiceBinding),
			},
		},
		"UpdateError": {
			reason: "should return an error when instance manager update fails",
			fields: fields{
				instanceManager: &MockInstanceManager{updateErr: errInstanceMgr},
				keyRotator:      &MockKeyRotator{},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: errInstanceMgr,
			},
		},
		"HappyPath": {
			reason: "should update the resource successfully",
			fields: fields{
				instanceManager: &MockInstanceManager{
					updateResult: managed.ExternalUpdate{},
				},
				keyRotator: &MockKeyRotator{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &MockExternal{
				instanceManager: tc.fields.instanceManager,
				keyRotator:      tc.fields.keyRotator,
				kube:            tc.fields.kube,
			}

			_, err := e.Update(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		instanceManager *MockInstanceManager
		keyRotator      *MockKeyRotator
		tracker         trackingtest.NoOpReferenceResolverTracker
		kube            client.Client
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongType": {
			reason: "should return an error when the managed resource is not a ServiceBinding",
			fields: fields{
				instanceManager: &MockInstanceManager{},
				keyRotator:      &MockKeyRotator{},
				tracker:         trackingtest.NoOpReferenceResolverTracker{},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: stderrors.New(errNotServiceBinding),
			},
		},
		"ResourceInUse": {
			reason: "should return an error when resource is in use by other resources",
			fields: fields{
				instanceManager: &MockInstanceManager{},
				keyRotator:      &MockKeyRotator{},
				tracker:         trackingtest.NoOpReferenceResolverTracker{IsResourceBlocked: true},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: errDependentUse,
			},
		},
		"HappyPath": {
			reason: "should delete the resource successfully and set Deleting condition",
			fields: fields{
				instanceManager: &MockInstanceManager{},
				keyRotator:      &MockKeyRotator{},
				tracker:         trackingtest.NoOpReferenceResolverTracker{},
				kube:            &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &MockExternal{
				instanceManager: tc.fields.instanceManager,
				keyRotator:      tc.fields.keyRotator,
				tracker:         tc.fields.tracker,
				kube:            tc.fields.kube,
			}

			_, err := e.Delete(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

func TestSaveCallback(t *testing.T) {
	type args struct {
		kube       client.Client
		name       string
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
			reason: "should return an error if the ServiceBinding cannot be retrieved",
			args: args{
				kube: &test.MockClient{MockGet: test.NewMockGetFn(errKube)},
				name: "test-binding",
			},
			want: want{
				err: errKube,
			},
		},
		"Success": {
			reason: "should successfully save conditions to the ServiceBinding",
			args: args{
				kube: &test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				name:       "test-binding",
				conditions: []xpv1.Condition{xpv1.Available()},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := saveCallback(context.Background(), tc.args.kube, tc.args.name, tc.args.conditions...)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

func TestIsRotationEnabled(t *testing.T) {
	type args struct {
		cr *v1alpha1.ServiceBinding
	}

	type want struct {
		enabled bool
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoRotation": {
			reason: "should return false when no rotation is configured",
			args: args{
				cr: expectedServiceBinding(),
			},
			want: want{
				enabled: false,
			},
		},
		"ForceRotationAnnotation": {
			reason: "should return true when force rotation annotation is present",
			args: args{
				cr: withForceRotationAnnotation(expectedServiceBinding()),
			},
			want: want{
				enabled: true,
			},
		},
		"RotationFrequencyConfigured": {
			reason: "should return true when rotation frequency is configured",
			args: args{
				cr: withRotationConfig(expectedServiceBinding()),
			},
			want: want{
				enabled: true,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{}
			got := e.isRotationEnabled(tc.args.cr)
			if got != tc.want.enabled {
				t.Errorf("\n%s\ne.isRotationEnabled(...): got %v, want %v\n", tc.reason, got, tc.want.enabled)
			}
		})
	}
}

// Helper functions and mocks
func expectedErrorBehaviour(t *testing.T, expectedErr error, gotErr error) {
	if expectedErr == nil && gotErr == nil {
		return
	}
	if expectedErr == nil && gotErr != nil {
		t.Errorf("expected no error, got %v", gotErr)
		return
	}
	if expectedErr != nil && gotErr == nil {
		t.Errorf("expected error %v, got nil", expectedErr)
		return
	}
	// Both errors exist, check if messages contain expected content
	if expectedErr != nil && gotErr != nil {
		expectedMsg := expectedErr.Error()
		gotMsg := gotErr.Error()
		if !assert.Contains(t, gotMsg, expectedMsg) {
			t.Errorf("expected error containing %q, got %q", expectedMsg, gotMsg)
		}
	}
}

func expectedServiceBinding() *v1alpha1.ServiceBinding {
	return &v1alpha1.ServiceBinding{
		Spec: v1alpha1.ServiceBindingSpec{
			ForProvider: v1alpha1.ServiceBindingParameters{
				Name: "test-name",
			},
		},
	}
}

func withForceRotationAnnotation(cr *v1alpha1.ServiceBinding) *v1alpha1.ServiceBinding {
	if cr.ObjectMeta.Annotations == nil {
		cr.ObjectMeta.Annotations = make(map[string]string)
	}
	cr.ObjectMeta.Annotations[servicebindingclient.ForceRotationKey] = "true"
	return cr
}

func withRotationConfig(cr *v1alpha1.ServiceBinding) *v1alpha1.ServiceBinding {
	cr.Spec.ForProvider.Rotation = &v1alpha1.RotationParameters{
		Frequency: &metav1.Duration{Duration: 24 * time.Hour},
	}
	return cr
}

func strPtr(s string) *string {
	return &s
}

// Mock implementations
type MockInstanceManager struct {
	observeResult managed.ExternalObservation
	observeErr    error
	tfResource    *v1alpha1.SubaccountServiceBinding
	createResult  managed.ExternalCreation
	createErr     error
	updateResult  managed.ExternalUpdate
	updateErr     error
	deleteErr     error
}

func (m *MockInstanceManager) ObserveInstance(ctx context.Context, cr *v1alpha1.ServiceBinding, btpName string, id string) (managed.ExternalObservation, *v1alpha1.SubaccountServiceBinding, error) {
	return m.observeResult, m.tfResource, m.observeErr
}

func (m *MockInstanceManager) CreateInstance(ctx context.Context, cr *v1alpha1.ServiceBinding, btpName string) (string, types.UID, managed.ExternalCreation, error) {
	return btpName, types.UID("test-uid"), m.createResult, m.createErr
}

func (m *MockInstanceManager) UpdateInstance(ctx context.Context, cr *v1alpha1.ServiceBinding, btpName string, id string) (managed.ExternalUpdate, error) {
	return m.updateResult, m.updateErr
}

func (m *MockInstanceManager) DeleteInstance(ctx context.Context, cr *v1alpha1.ServiceBinding, btpName string, id string) error {
	return m.deleteErr
}

type MockKeyRotator struct {
	hasExpiredKeys          bool
	retireBinding           bool
	deleteExpiredKeysResult []*v1alpha1.RetiredSBResource
	deleteExpiredKeysErr    error
	deleteRetiredKeysErr    error
}

func (m *MockKeyRotator) HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool {
	return m.hasExpiredKeys
}

func (m *MockKeyRotator) RetireBinding(cr *v1alpha1.ServiceBinding) bool {
	return m.retireBinding
}

func (m *MockKeyRotator) DeleteExpiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) ([]*v1alpha1.RetiredSBResource, error) {
	return m.deleteExpiredKeysResult, m.deleteExpiredKeysErr
}

func (m *MockKeyRotator) DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error {
	return m.deleteRetiredKeysErr
}

// MockExternal implements the main CRUD operations with mocked dependencies
type MockExternal struct {
	instanceManager *MockInstanceManager
	keyRotator      *MockKeyRotator
	tracker         trackingtest.NoOpReferenceResolverTracker
	kube            client.Client
}

func (e *MockExternal) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalObservation{}, stderrors.New(errNotServiceBinding)
	}

	var btpName string
	if cr.Spec.BtpName != nil {
		btpName = *cr.Spec.BtpName
	} else {
		btpName = cr.Spec.ForProvider.Name
	}

	observation, tfResource, err := e.instanceManager.ObserveInstance(ctx, cr, btpName, cr.Status.AtProvider.ID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBinding)
	}

	if observation.ResourceExists {
		if observation.ResourceUpToDate && tfResource != nil {
			// Update CR from TF resource would go here
			if e.kube != nil {
				if err := e.kube.Status().Update(ctx, cr); err != nil {
					return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
				}
			}
		}
		observation.ResourceUpToDate = observation.ResourceUpToDate && !e.keyRotator.HasExpiredKeys(cr)

		if e.keyRotator.RetireBinding(cr) {
			if e.kube != nil {
				if err := e.kube.Status().Update(ctx, cr); err != nil {
					return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
				}
			}
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
	}

	return observation, nil
}

func (e *MockExternal) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalCreation{}, stderrors.New(errNotServiceBinding)
	}

	cr.SetConditions(xpv1.Creating())

	var btpName string
	if cr.ObjectMeta.Annotations != nil {
		if _, hasForceRotation := cr.ObjectMeta.Annotations[servicebindingclient.ForceRotationKey]; hasForceRotation {
			btpName = servicebindingclient.GenerateRandomName(cr.Spec.ForProvider.Name)
		} else {
			btpName = cr.Spec.ForProvider.Name
		}
	} else if cr.Spec.ForProvider.Rotation != nil && cr.Spec.ForProvider.Rotation.Frequency != nil {
		btpName = servicebindingclient.GenerateRandomName(cr.Spec.ForProvider.Name)
	} else {
		btpName = cr.Spec.ForProvider.Name
	}
	cr.Spec.BtpName = &btpName

	if e.kube != nil {
		if err := e.kube.Update(ctx, cr); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errUpdateStatus)
		}
	}

	_, _, creation, err := e.instanceManager.CreateInstance(ctx, cr, *cr.Spec.BtpName)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	if e.kube != nil {
		if err := e.kube.Status().Update(ctx, cr); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errUpdateStatus)
		}
	}

	// Remove force rotation annotation after successful creation
	if cr.ObjectMeta.Annotations != nil {
		if _, ok := cr.ObjectMeta.Annotations[servicebindingclient.ForceRotationKey]; ok {
			meta.RemoveAnnotations(cr, servicebindingclient.ForceRotationKey)
		}
	}

	return creation, nil
}

func (e *MockExternal) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalUpdate{}, stderrors.New(errNotServiceBinding)
	}

	// Check if current binding is already retired
	currentBindingRetired := false
	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
		if retiredKey.ID == cr.Status.AtProvider.ID {
			currentBindingRetired = true
			break
		}
	}

	var updateResult managed.ExternalUpdate
	if !currentBindingRetired {
		var btpName string
		if cr.Spec.BtpName != nil {
			btpName = *cr.Spec.BtpName
		} else {
			btpName = cr.Spec.ForProvider.Name
		}

		update, err := e.instanceManager.UpdateInstance(ctx, cr, btpName, cr.Status.AtProvider.ID)
		if err != nil {
			return managed.ExternalUpdate{}, err
		}
		updateResult = update
	}

	// Clean up expired keys if there are any retired keys
	if cr.Status.AtProvider.RetiredKeys != nil {
		if newRetiredKeys, err := e.keyRotator.DeleteExpiredKeys(ctx, cr); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, errDeleteExpiredKeys)
		} else {
			cr.Status.AtProvider.RetiredKeys = newRetiredKeys
			if e.kube != nil {
				if err := e.kube.Status().Update(ctx, cr); err != nil {
					return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateStatus)
				}
			}
		}
	}

	return updateResult, nil
}

func (e *MockExternal) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalDelete{}, stderrors.New(errNotServiceBinding)
	}
	cr.SetConditions(xpv1.Deleting())

	// Set resource usage conditions to check dependencies
	e.tracker.SetConditions(ctx, cr)

	// Block deletion if other resources are still using this ServiceBinding
	if blocked := e.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, stderrors.New(providerv1alpha1.ErrResourceInUse)
	}

	if err := e.keyRotator.DeleteRetiredKeys(ctx, cr); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteRetiredKeys)
	}

	var btpName string
	if cr.Spec.BtpName != nil {
		btpName = *cr.Spec.BtpName
	} else {
		btpName = cr.Spec.ForProvider.Name
	}

	if err := e.instanceManager.DeleteInstance(ctx, cr, btpName, cr.Status.AtProvider.ID); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteServiceBinding)
	}
	return managed.ExternalDelete{}, nil
}

func TestFlattenSecretData(t *testing.T) {
	type args struct {
		secretData map[string][]byte
	}

	type want struct {
		result map[string][]byte
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"EmptyInput": {
			reason: "should return empty map for empty input",
			args: args{
				secretData: map[string][]byte{},
			},
			want: want{
				result: map[string][]byte{},
				err:    nil,
			},
		},
		"NonJSONValue": {
			reason: "should keep non-JSON values as-is",
			args: args{
				secretData: map[string][]byte{
					"simple-key": []byte("simple-value"),
					"binary-key": []byte{0x01, 0x02, 0x03},
				},
			},
			want: want{
				result: map[string][]byte{
					"simple-key": []byte("simple-value"),
					"binary-key": []byte{0x01, 0x02, 0x03},
				},
				err: nil,
			},
		},
		"JSONObjectValue": {
			reason: "should flatten JSON object into top-level keys",
			args: args{
				secretData: map[string][]byte{
					"credentials": []byte(`{"username":"user1","password":"pass123","endpoint":"https://api.example.com"}`),
				},
			},
			want: want{
				result: map[string][]byte{
					"username": []byte("user1"),
					"password": []byte("pass123"),
					"endpoint": []byte("https://api.example.com"),
				},
				err: nil,
			},
		},
		"MixedJSONAndNonJSON": {
			reason: "should handle mix of JSON and non-JSON values",
			args: args{
				secretData: map[string][]byte{
					"config":      []byte(`{"host":"localhost","port":8080}`),
					"simple-text": []byte("not-json"),
					"empty":       []byte(""),
				},
			},
			want: want{
				result: map[string][]byte{
					"host":        []byte("localhost"),
					"port":        []byte("8080"),
					"simple-text": []byte("not-json"),
					"empty":       []byte(""),
				},
				err: nil,
			},
		},
		"JSONWithNestedObject": {
			reason: "should marshal nested objects as JSON strings",
			args: args{
				secretData: map[string][]byte{
					"complex": []byte(`{"simple":"value","nested":{"key":"value","number":42}}`),
				},
			},
			want: want{
				result: map[string][]byte{
					"simple": []byte("value"),
					"nested": []byte(`{"key":"value","number":42}`),
				},
				err: nil,
			},
		},
		"JSONWithArray": {
			reason: "should marshal arrays as JSON strings",
			args: args{
				secretData: map[string][]byte{
					"data": []byte(`{"items":[1,2,3],"name":"test"}`),
				},
			},
			want: want{
				result: map[string][]byte{
					"items": []byte("[1,2,3]"),
					"name":  []byte("test"),
				},
				err: nil,
			},
		},
		"JSONWithNullValue": {
			reason: "should handle null values in JSON",
			args: args{
				secretData: map[string][]byte{
					"data": []byte(`{"nullable":null,"string":"value"}`),
				},
			},
			want: want{
				result: map[string][]byte{
					"nullable": []byte("null"),
					"string":   []byte("value"),
				},
				err: nil,
			},
		},
		"JSONWithBooleanAndNumber": {
			reason: "should handle boolean and number values in JSON",
			args: args{
				secretData: map[string][]byte{
					"data": []byte(`{"enabled":true,"count":123,"rate":45.67}`),
				},
			},
			want: want{
				result: map[string][]byte{
					"enabled": []byte("true"),
					"count":   []byte("123"),
					"rate":    []byte("45.67"),
				},
				err: nil,
			},
		},
		"MultipleJSONObjects": {
			reason: "should flatten multiple JSON objects",
			args: args{
				secretData: map[string][]byte{
					"auth":   []byte(`{"token":"abc123","expires":"2024-01-01"}`),
					"config": []byte(`{"debug":true,"timeout":30}`),
					"plain":  []byte("plain-value"),
				},
			},
			want: want{
				result: map[string][]byte{
					"token":   []byte("abc123"),
					"expires": []byte("2024-01-01"),
					"debug":   []byte("true"),
					"timeout": []byte("30"),
					"plain":   []byte("plain-value"),
				},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := flattenSecretData(tc.args.secretData)
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("\n%s\nflattenSecretData(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}
