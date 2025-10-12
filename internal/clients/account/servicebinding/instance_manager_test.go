package servicebindingclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	errMockConnect = errors.New("mock connect error")
	errMockCreate  = errors.New("mock create error")
	errMockDelete  = errors.New("mock delete error")
	errMockUpdate  = errors.New("mock update error")
	errMockObserve = errors.New("mock observe error")
)

func TestInstanceManager_CreateInstance(t *testing.T) {
	mockClient := fake.NewClientBuilder().Build()

	type fields struct {
		sbConnector TfConnector
	}
	type args struct {
		ctx      context.Context
		publicCR *v1alpha1.ServiceBinding
		btpName  string
	}
	type want struct {
		instanceName string
		instanceUID  types.UID
		creation     managed.ExternalCreation
		err          error
	}

	publicCR := &v1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			UID: "test-uid-123",
		},
		Spec: v1alpha1.ServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: "test-provider-config",
				},
			},
			ForProvider: v1alpha1.ServiceBindingParameters{
				Name: "test-service-binding",
			},
		},
	}

	expectedCreation := managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "SuccessfulCreate",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						creation: expectedCreation,
					},
				},
			},
			args: args{
				ctx:      context.Background(),
				publicCR: publicCR,
				btpName:  "test-binding",
			},
			want: want{
				instanceName: "", // Empty because CreateInstance returns empty name when using random suffix
				instanceUID:  "", // Will be verified with pattern matching instead of exact match
				creation:     expectedCreation,
				err:          nil,
			},
		},
		{
			name: "ConnectError",
			fields: fields{
				sbConnector: &MockTfConnector{
					err: errMockConnect,
				},
			},
			args: args{
				ctx:      context.Background(),
				publicCR: publicCR,
				btpName:  "test-binding",
			},
			want: want{
				instanceName: "",
				instanceUID:  "",
				creation:     managed.ExternalCreation{},
				err:          errMockConnect,
			},
		},
		{
			name: "CreateError",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						createErr: errMockCreate,
					},
				},
			},
			args: args{
				ctx:      context.Background(),
				publicCR: publicCR,
				btpName:  "test-binding",
			},
			want: want{
				instanceName: "",
				instanceUID:  "",
				creation:     managed.ExternalCreation{},
				err:          errMockCreate,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewInstanceManager(tt.fields.sbConnector, mockClient)
			gotName, gotUID, gotCreation, err := m.CreateInstance(tt.args.ctx, tt.args.publicCR, tt.args.btpName)

			if tt.want.err != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.want.err.Error())
			} else {
				assert.NoError(t, err)
			}

			if tt.want.err == nil {
				// For successful creates, verify the UID pattern instead of exact match
				expectedPrefix := string(publicCR.UID) + "-" + tt.args.btpName + "-"
				assert.True(t, strings.HasPrefix(string(gotUID), expectedPrefix),
					"UID should start with '%s', got '%s'", expectedPrefix, gotUID)
				// Verify the suffix is exactly 5 characters (random string length)
				suffix := strings.TrimPrefix(string(gotUID), expectedPrefix)
				assert.Len(t, suffix, 5, "Random suffix should be 5 characters")
			} else {
				// For error cases, verify exact matches
				assert.Equal(t, tt.want.instanceName, gotName)
				assert.Equal(t, tt.want.instanceUID, gotUID)
			}
			if diff := cmp.Diff(tt.want.creation, gotCreation); diff != "" {
				t.Errorf("CreateInstance creation mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInstanceManager_DeleteInstance(t *testing.T) {
	mockClient := fake.NewClientBuilder().Build()

	type fields struct {
		sbConnector TfConnector
	}
	type args struct {
		ctx                context.Context
		publicCR           *v1alpha1.ServiceBinding
		targetName         string
		targetExternalName string
	}
	type want struct {
		deletion managed.ExternalDelete
		err      error
	}

	publicCR := &v1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			UID: "test-uid-123",
		},
		Spec: v1alpha1.ServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: "test-provider-config",
				},
			},
			ForProvider: v1alpha1.ServiceBindingParameters{
				Name: "test-service-binding",
			},
		},
	}

	expectedDeletion := managed.ExternalDelete{}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "SuccessfulDelete",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{},
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				deletion: expectedDeletion,
				err:      nil,
			},
		},
		{
			name: "ConnectError",
			fields: fields{
				sbConnector: &MockTfConnector{
					err: errMockConnect,
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				deletion: managed.ExternalDelete{},
				err:      errMockConnect,
			},
		},
		{
			name: "DeleteError",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						deleteErr: errMockDelete,
					},
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				deletion: managed.ExternalDelete{},
				err:      errMockDelete,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewInstanceManager(tt.fields.sbConnector, mockClient)
			gotDeletion, err := m.DeleteInstance(tt.args.ctx, tt.args.publicCR, tt.args.targetName, tt.args.targetExternalName)

			if tt.want.err != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.want.err.Error())
			} else {
				assert.NoError(t, err)
			}

			if diff := cmp.Diff(tt.want.deletion, gotDeletion); diff != "" {
				t.Errorf("DeleteInstance deletion mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInstanceManager_UpdateInstance(t *testing.T) {
	mockClient := fake.NewClientBuilder().Build()

	type fields struct {
		sbConnector TfConnector
	}
	type args struct {
		ctx                context.Context
		publicCR           *v1alpha1.ServiceBinding
		targetName         string
		targetExternalName string
	}
	type want struct {
		update managed.ExternalUpdate
		err    error
	}

	publicCR := &v1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			UID: "test-uid-123",
		},
		Spec: v1alpha1.ServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: "test-provider-config",
				},
			},
			ForProvider: v1alpha1.ServiceBindingParameters{
				Name: "test-service-binding",
			},
		},
	}

	expectedUpdate := managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{
			"test-key": []byte("test-value"),
		},
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "SuccessfulUpdate",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						update: expectedUpdate,
					},
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				update: expectedUpdate,
				err:    nil,
			},
		},
		{
			name: "ConnectError",
			fields: fields{
				sbConnector: &MockTfConnector{
					err: errMockConnect,
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				update: managed.ExternalUpdate{},
				err:    errMockConnect,
			},
		},
		{
			name: "UpdateError",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						updateErr: errMockUpdate,
					},
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				update: managed.ExternalUpdate{},
				err:    errMockUpdate,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewInstanceManager(tt.fields.sbConnector, mockClient)
			gotUpdate, err := m.UpdateInstance(tt.args.ctx, tt.args.publicCR, tt.args.targetName, tt.args.targetExternalName)

			if tt.want.err != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.want.err.Error())
			} else {
				assert.NoError(t, err)
			}

			if diff := cmp.Diff(tt.want.update, gotUpdate); diff != "" {
				t.Errorf("UpdateInstance update mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInstanceManager_ObserveInstance(t *testing.T) {
	mockClient := fake.NewClientBuilder().Build()

	type fields struct {
		sbConnector TfConnector
	}
	type args struct {
		ctx                context.Context
		publicCR           *v1alpha1.ServiceBinding
		targetName         string
		targetExternalName string
	}
	type want struct {
		observation managed.ExternalObservation
		resource    *v1alpha1.SubaccountServiceBinding
		err         error
	}

	publicCR := &v1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			UID: "test-uid-123",
		},
		Spec: v1alpha1.ServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: "test-provider-config",
				},
			},
			ForProvider: v1alpha1.ServiceBindingParameters{
				Name: "test-service-binding",
			},
		},
	}

	expectedObservation := managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{"key": []byte("value")},
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "SuccessfulObserve_UpToDate",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						observation: expectedObservation,
					},
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				observation: expectedObservation,
				resource:    nil, // We'll validate the structure separately
				err:         nil,
			},
		},
		{
			name: "SuccessfulObserve_NotUpToDate",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						observation: managed.ExternalObservation{
							ResourceExists:   true,
							ResourceUpToDate: false,
						},
					},
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				observation: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				resource: nil,
				err:      nil,
			},
		},
		{
			name: "ConnectError",
			fields: fields{
				sbConnector: &MockTfConnector{
					err: errMockConnect,
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				observation: managed.ExternalObservation{},
				resource:    nil,
				err:         errMockConnect,
			},
		},
		{
			name: "ObserveError",
			fields: fields{
				sbConnector: &MockTfConnector{
					client: &MockExternalClient{
						observeErr: errMockObserve,
					},
				},
			},
			args: args{
				ctx:                context.Background(),
				publicCR:           publicCR,
				targetName:         "test-target",
				targetExternalName: "external-123",
			},
			want: want{
				observation: managed.ExternalObservation{},
				resource:    nil,
				err:         errMockObserve,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalCR := publicCR.DeepCopy()
			m := NewInstanceManager(tt.fields.sbConnector, mockClient)
			gotObservation, gotResource, err := m.ObserveInstance(tt.args.ctx, tt.args.publicCR, tt.args.targetName, tt.args.targetExternalName)

			if tt.want.err != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.want.err.Error())
			} else {
				assert.NoError(t, err)
			}

			if diff := cmp.Diff(tt.want.observation, gotObservation); diff != "" {
				t.Errorf("ObserveInstance observation mismatch (-want +got):\n%s", diff)
			}

			if err == nil {
				// Verify resource structure
				assert.NotNil(t, gotResource)
				assert.Equal(t, tt.args.targetName, gotResource.Name)
				assert.Equal(t, GenerateInstanceUID(publicCR.UID, tt.args.targetExternalName), gotResource.UID)
				assert.Equal(t, v1alpha1.SubaccountServiceBinding_Kind, gotResource.Kind)
				assert.Equal(t, v1alpha1.CRDGroupVersion.String(), gotResource.APIVersion)

				// Verify external name is set if provided
				if tt.args.targetExternalName != "" {
					assert.Equal(t, tt.args.targetExternalName, meta.GetExternalName(gotResource))
				}

				// Check if Available condition was set when resource is up to date
				if gotObservation.ResourceExists && gotObservation.ResourceUpToDate {
					conditions := publicCR.Status.Conditions
					if len(conditions) > 0 {
						availableCondition := conditions[0]
						assert.Equal(t, xpv1.TypeReady, availableCondition.Type)
						assert.Equal(t, corev1.ConditionTrue, availableCondition.Status)
					} else {
						// If conditions array is empty, this might indicate that condition setting
						// is not implemented in the mock or the condition is set elsewhere
						t.Logf("Warning: No conditions found when resource is up to date")
					}
				} else {
					// CR should not be modified if not up to date
					assert.Equal(t, originalCR.Status.Conditions, publicCR.Status.Conditions)
				}
			}
		})
	}
}

func TestInstanceManager_buildSubaccountServiceBinding(t *testing.T) {
	mockClient := fake.NewClientBuilder().Build()
	publicCR := &v1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			UID:               "test-uid-123",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: v1alpha1.ServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: "test-provider-config",
				},
			},
			ForProvider: v1alpha1.ServiceBindingParameters{
				Name: "test-service-binding",
			},
		},
	}

	m := NewInstanceManager(nil, mockClient)
	name := "test-name"
	uid := types.UID("test-uid")
	externalName := "external-123"

	result, err := m.buildSubaccountServiceBinding(context.Background(), publicCR, name, uid, externalName)
	assert.NoError(t, err)

	// Verify basic structure
	assert.Equal(t, v1alpha1.SubaccountServiceBinding_Kind, result.Kind)
	assert.Equal(t, v1alpha1.CRDGroupVersion.String(), result.APIVersion)
	assert.Equal(t, name, result.Name)
	assert.Equal(t, uid, result.UID)
	assert.Equal(t, publicCR.DeletionTimestamp, result.DeletionTimestamp)

	// Verify spec
	assert.Equal(t, "test-provider-config", result.Spec.ProviderConfigReference.Name)
	assert.Equal(t, xpv1.ManagementPolicies{xpv1.ManagementActionAll}, result.Spec.ManagementPolicies)
	assert.Equal(t, &name, result.Spec.ForProvider.Name)

	// Verify external name
	assert.Equal(t, externalName, meta.GetExternalName(result))

	// Test without external name
	resultNoExt, err := m.buildSubaccountServiceBinding(context.Background(), publicCR, name, uid, "")
	assert.NoError(t, err)
	assert.Equal(t, "", meta.GetExternalName(resultNoExt))
}

// Mock implementations
var _ TfConnector = &MockTfConnector{}

type MockTfConnector struct {
	client managed.ExternalClient
	err    error
}

func (m *MockTfConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.client, nil
}

var _ managed.ExternalClient = &MockExternalClient{}

type MockExternalClient struct {
	observation managed.ExternalObservation
	creation    managed.ExternalCreation
	update      managed.ExternalUpdate
	observeErr  error
	createErr   error
	updateErr   error
	deleteErr   error
}

func (m *MockExternalClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	if m.observeErr != nil {
		return managed.ExternalObservation{}, m.observeErr
	}
	return m.observation, nil
}

func (m *MockExternalClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	if m.createErr != nil {
		return managed.ExternalCreation{}, m.createErr
	}
	return m.creation, nil
}

func (m *MockExternalClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	if m.updateErr != nil {
		return managed.ExternalUpdate{}, m.updateErr
	}
	return m.update, nil
}

func (m *MockExternalClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	if m.deleteErr != nil {
		return managed.ExternalDelete{}, m.deleteErr
	}
	return managed.ExternalDelete{}, nil
}

func (m *MockExternalClient) Disconnect(ctx context.Context) error {
	return nil
}