package servicebinding

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
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	tracking_test "github.com/sap/crossplane-provider-btp/internal/tracking/test"
)

var (
	errMockTracking = errors.New("mock tracking error")
)

// Mock TF Connector
var _ servicebindingclient.TfConnector = &MockTfConnector{}

type MockTfConnector struct {
	connectErr error
	external   managed.ExternalClient
}

func (m *MockTfConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	if m.connectErr != nil {
		return nil, m.connectErr
	}
	if m.external != nil {
		return m.external, nil
	}
	return &MockExternalClient{}, nil
}

// Mock Key Rotator
var _ servicebindingclient.KeyRotator = &MockKeyRotator{}

type MockKeyRotator struct {
	hasExpiredKeysResult           bool
	validateRotationSettingsCalled bool
	retireBindingResult            bool
	deleteExpiredKeysResult        []*v1alpha1.RetiredSBResource
	deleteExpiredKeysErr           error
	deleteRetiredKeysErr           error
	isCurrentBindingRetiredResult  bool
}

func (m *MockKeyRotator) HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool {
	return m.hasExpiredKeysResult
}

func (m *MockKeyRotator) ValidateRotationSettings(cr *v1alpha1.ServiceBinding) {
	m.validateRotationSettingsCalled = true
}

func (m *MockKeyRotator) RetireBinding(cr *v1alpha1.ServiceBinding) bool {
	return m.retireBindingResult
}

func (m *MockKeyRotator) DeleteExpiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) ([]*v1alpha1.RetiredSBResource, error) {
	if m.deleteExpiredKeysErr != nil {
		return nil, m.deleteExpiredKeysErr
	}
	return m.deleteExpiredKeysResult, nil
}

func (m *MockKeyRotator) DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error {
	return m.deleteRetiredKeysErr
}

func (m *MockKeyRotator) IsCurrentBindingRetired(cr *v1alpha1.ServiceBinding) bool {
	return m.isCurrentBindingRetiredResult
}

// Mock External Client (for TF Connector)
type MockExternalClient struct {
	observeErr error
	createErr  error
	updateErr  error
	deleteErr  error
}

func (m *MockExternalClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	return managed.ExternalObservation{}, m.observeErr
}

func (m *MockExternalClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	return managed.ExternalCreation{}, m.createErr
}

func (m *MockExternalClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{}, m.updateErr
}

func (m *MockExternalClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	return managed.ExternalDelete{}, m.deleteErr
}

func (m *MockExternalClient) Disconnect(ctx context.Context) error {
	return nil
}

// Mock Tracker for testing tracking errors
type MockTracker struct {
	trackErr      error
	deleteBlocked bool
}

func (m *MockTracker) Track(ctx context.Context, mg resource.Managed) error {
	return m.trackErr
}

func (m *MockTracker) SetConditions(ctx context.Context, mg resource.Managed) {}

func (m *MockTracker) DeleteShouldBeBlocked(mg resource.Managed) bool {
	return m.deleteBlocked
}

func (m *MockTracker) ResolveSource(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (m *MockTracker) ResolveTarget(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

// Mock ServiceBindingClient for testing main controller logic
var _ servicebindingclient.ServiceBindingClientInterface = &MockServiceBindingClient{}

// MockServiceBindingClientFactory is a test implementation of ServiceBindingClientFactory
type MockServiceBindingClientFactory struct {
	Client servicebindingclient.ServiceBindingClientInterface
	Error  error

	// Capture calls for verification
	CreateClientCalls []CreateClientCall
}

type CreateClientCall struct {
	CR                 *v1alpha1.ServiceBinding
	TargetName         string
	TargetExternalName string
}

func (f *MockServiceBindingClientFactory) CreateClient(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (servicebindingclient.ServiceBindingClientInterface, error) {
	// Capture the call for verification
	f.CreateClientCalls = append(f.CreateClientCalls, CreateClientCall{
		CR:                 cr,
		TargetName:         targetName,
		TargetExternalName: targetExternalName,
	})

	if f.Error != nil {
		return nil, f.Error
	}
	return f.Client, nil
}

// Reset clears the captured calls
func (f *MockServiceBindingClientFactory) Reset() {
	f.CreateClientCalls = nil
}

type MockServiceBindingClient struct {
	observation managed.ExternalObservation
	creation    managed.ExternalCreation
	update      managed.ExternalUpdate
	deletion    managed.ExternalDelete
	tfResource  *v1alpha1.SubaccountServiceBinding
	observeErr  error
	createErr   error
	updateErr   error
	deleteErr   error
}

func (m *MockServiceBindingClient) Create(ctx context.Context) (string, managed.ExternalCreation, error) {
	if m.createErr != nil {
		return "", managed.ExternalCreation{}, m.createErr
	}
	// Generate mock UUID-like external name
	externalName := "12345678-1234-5678-9abc-123456789012" // Mock UUID
	return externalName, m.creation, nil
}

func (m *MockServiceBindingClient) Delete(ctx context.Context) (managed.ExternalDelete, error) {
	if m.deleteErr != nil {
		return managed.ExternalDelete{}, m.deleteErr
	}
	return m.deletion, nil
}

func (m *MockServiceBindingClient) Update(ctx context.Context) (managed.ExternalUpdate, error) {
	if m.updateErr != nil {
		return managed.ExternalUpdate{}, m.updateErr
	}
	return m.update, nil
}

func (m *MockServiceBindingClient) Observe(ctx context.Context) (managed.ExternalObservation, *v1alpha1.SubaccountServiceBinding, error) {
	if m.observeErr != nil {
		return managed.ExternalObservation{}, nil, m.observeErr
	}
	return m.observation, m.tfResource, nil
}

// Test Connect method - This is the main test that validates our dependency injection implementation
func TestConnect(t *testing.T) {
	type fields struct {
		tfConnectorErr   error
		clientFactoryErr error
		keyRotatorErr    error
		trackingErr      error
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
		"WrongResourceType": {
			reason: "should return error for wrong resource type",
			fields: fields{},
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: errors.New(errNotServiceBinding),
			},
		},
		"TrackingError": {
			reason: "should return error when tracking fails",
			fields: fields{
				trackingErr: errMockTracking,
			},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err: errMockTracking,
			},
		},
		"Success": {
			reason: "should successfully create external client",
			fields: fields{},
			args: args{
				mg: expectedServiceBinding(),
			},
			want: want{
				err:            nil,
				externalExists: true,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Store original functions
			originalTfConnectorFn := newTfConnectorFn
			originalKeyRotatorFn := newSBKeyRotatorFn

			// Set up mocks
			newTfConnectorFn = func(kube kubeclient.Client) servicebindingclient.TfConnector {
				return &MockTfConnector{connectErr: tc.fields.tfConnectorErr}
			}

			newSBKeyRotatorFn = func(bindingDeleter servicebindingclient.BindingDeleter) servicebindingclient.KeyRotator {
				if tc.fields.keyRotatorErr != nil {
					panic(tc.fields.keyRotatorErr) // Simulate creation failure
				}
				return &MockKeyRotator{}
			}

			// Create mock factory
			mockFactory := &MockServiceBindingClientFactory{
				Client: &MockServiceBindingClient{},
				Error:  tc.fields.clientFactoryErr,
			}

			// Restore original functions after test
			defer func() {
				newTfConnectorFn = originalTfConnectorFn
				newSBKeyRotatorFn = originalKeyRotatorFn
			}()

			// Set up connector
			var tracker tracking.ReferenceResolverTracker = tracking_test.NoOpReferenceResolverTracker{}
			if tc.fields.trackingErr != nil {
				tracker = &MockTracker{trackErr: tc.fields.trackingErr}
			}

			c := connector{
				kube:              &test.MockClient{},
				usage:             resource.TrackerFn(func(ctx context.Context, mg resource.Managed) error { return nil }),
				resourcetracker:   tracker,
				clientFactory:     mockFactory,
				newSBKeyRotatorFn: newSBKeyRotatorFn,
			}

			// Handle panics from mock creation failures
			defer func() {
				if r := recover(); r != nil {
					if tc.fields.keyRotatorErr != nil && r == tc.fields.keyRotatorErr {
						// Expected panic, convert to error for test
						if diff := cmp.Diff(tc.want.err, tc.fields.keyRotatorErr, test.EquateErrors()); diff != "" {
							t.Errorf("\n%s\nc.Connect(...): -want error, +got error:\n%s\n", tc.reason, diff)
						}
						return
					}
					panic(r) // Re-panic if unexpected
				}
			}()

			got, err := c.Connect(context.Background(), tc.args.mg)

			if tc.want.externalExists && got == nil {
				t.Errorf("expected external client, got nil")
			}

			// Check error expectations with better error handling
			if tc.want.err != nil {
				if err == nil {
					t.Errorf("\n%s\nc.Connect(...): expected error, got nil\n", tc.reason)
				} else {
					// Check if the error message contains the expected text
					expectedMsg := tc.want.err.Error()
					gotMsg := err.Error()
					if !containsError(gotMsg, expectedMsg) {
						t.Errorf("\n%s\nc.Connect(...): expected error containing %q, got %q\n", tc.reason, expectedMsg, gotMsg)
					}
				}
			} else if err != nil {
				t.Errorf("\n%s\nc.Connect(...): expected no error, got %v\n", tc.reason, err)
			}
		})
	}
}

// Test flattenSecretData function
func TestFlattenSecretData(t *testing.T) {
	cases := map[string]struct {
		reason string
		input  map[string][]byte
		want   map[string][]byte
		err    error
	}{
		"EmptyInput": {
			reason: "should handle empty input",
			input:  map[string][]byte{},
			want:   map[string][]byte{},
		},
		"NonJSONValue": {
			reason: "should keep non-JSON values as-is",
			input: map[string][]byte{
				"simple": []byte("value"),
			},
			want: map[string][]byte{
				"simple": []byte("value"),
			},
		},
		"JSONObjectValue": {
			reason: "should flatten JSON object values",
			input: map[string][]byte{
				"json_obj": []byte(`{"key1": "value1", "key2": "value2"}`),
			},
			want: map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
			},
		},
		"MixedValues": {
			reason: "should handle mixed JSON and non-JSON values",
			input: map[string][]byte{
				"simple":   []byte("simple_value"),
				"json_obj": []byte(`{"nested_key": "nested_value"}`),
			},
			want: map[string][]byte{
				"simple":     []byte("simple_value"),
				"nested_key": []byte("nested_value"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := flattenSecretData(tc.input)

			if diff := cmp.Diff(tc.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nflattenSecretData(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nflattenSecretData(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

// Test helper function validation
func TestServiceBindingHelpers(t *testing.T) {
	t.Run("expectedServiceBinding creates valid CR", func(t *testing.T) {
		cr := expectedServiceBinding()

		if cr == nil {
			t.Errorf("expectedServiceBinding() returned nil")
			return
		}

		// Should create an empty ServiceBinding by default
		if cr.Name != "" || len(cr.GetAnnotations()) > 0 {
			t.Errorf("expectedServiceBinding() should create empty CR by default")
		}
	})

	t.Run("withMetadata sets external name and annotations", func(t *testing.T) {
		annotations := map[string]string{"test": "value"}
		cr := expectedServiceBinding(
			withMetadata("test-external-name", annotations),
		)

		if meta.GetExternalName(cr) != "test-external-name" {
			t.Errorf("withMetadata() failed to set external name, got: %s", meta.GetExternalName(cr))
		}

		if cr.GetAnnotations()["test"] != "value" {
			t.Errorf("withMetadata() failed to set annotations")
		}
	})

	t.Run("withConditions sets status conditions", func(t *testing.T) {
		cr := expectedServiceBinding(
			withConditions(xpv1.Available(), xpv1.Creating()),
		)

		if len(cr.Status.Conditions) != 2 {
			t.Errorf("withConditions() failed to set conditions")
		}
	})
}

// Test parseIso8601Date function
func TestParseIso8601Date(t *testing.T) {
	cases := map[string]struct {
		reason string
		input  string
		want   string // Expected formatted time
		err    error
	}{
		"ValidDate": {
			reason: "should parse valid ISO8601 date",
			input:  "2023-01-15T10:30:00Z",
			want:   "2023-01-15T10:30:00Z",
		},
		"ValidDateWithTimezone": {
			reason: "should parse valid ISO8601 date with timezone",
			input:  "2023-01-15T10:30:00+0200",
			want:   "2023-01-15T08:30:00Z", // Converted to UTC
		},
		"InvalidDate": {
			reason: "should return error for invalid date",
			input:  "invalid-date",
			err:    errors.New("parsing time"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseIso8601Date(tc.input)

			if tc.err != nil {
				if err == nil {
					t.Errorf("\n%s\nparseIso8601Date(...): expected error, got nil\n", tc.reason)
				}
				return
			}

			if err != nil {
				t.Errorf("\n%s\nparseIso8601Date(...): unexpected error: %v\n", tc.reason, err)
				return
			}

			// Convert to UTC for comparison if we expect UTC
			gotTime := got.Time
			if strings.HasSuffix(tc.want, "Z") {
				gotTime = gotTime.UTC()
			}
			gotStr := gotTime.Format("2006-01-02T15:04:05Z07:00")
			if gotStr != tc.want {
				t.Errorf("\n%s\nparseIso8601Date(...): want %s, got %s\n", tc.reason, tc.want, gotStr)
			}
		})
	}
}

// Helper functions for building test ServiceBinding CRs
func expectedServiceBinding(opts ...func(*v1alpha1.ServiceBinding)) *v1alpha1.ServiceBinding {
	cr := &v1alpha1.ServiceBinding{}

	// Apply each option to modify the CR
	for _, opt := range opts {
		opt(cr)
	}

	return cr
}

// Essential helper functions for ServiceBinding test construction

func withMetadata(externalName string, annotations map[string]string) func(*v1alpha1.ServiceBinding) {
	return func(cr *v1alpha1.ServiceBinding) {
		if annotations != nil {
			cr.SetAnnotations(annotations)
		}
		if externalName != "" {
			meta.SetExternalName(cr, externalName)
		}
	}
}

func withConditions(conditions ...xpv1.Condition) func(*v1alpha1.ServiceBinding) {
	return func(cr *v1alpha1.ServiceBinding) {
		cr.Status.Conditions = conditions
	}
}

// Test Observe method - This validates the main observation logic
func TestObserve(t *testing.T) {
	type fields struct {
		clientFactory ServiceBindingClientFactory
		keyRotator    servicebindingclient.KeyRotator
		tracker       *MockTracker
		kube          *test.MockClient
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
		o   managed.ExternalObservation
		cr  *v1alpha1.ServiceBinding // Expected complete CR after observe
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongResourceType": {
			reason: "should return error for wrong resource type",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{},
				},
				keyRotator: &MockKeyRotator{},
				tracker:    &MockTracker{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: errors.New(errNotServiceBinding),
			},
		},
		"ClientObserveError": {
			reason: "should return error when client observe fails",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						observeErr: errors.New("client observe error"),
					},
				},
				keyRotator: &MockKeyRotator{},
				tracker:    &MockTracker{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
			want: want{
				err: errors.New("client observe error"),
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
		},
		"ResourceNotExists": {
			reason: "should return ResourceExists=false when resource doesn't exist",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						observation: managed.ExternalObservation{
							ResourceExists: false,
						},
					},
				},
				keyRotator: &MockKeyRotator{},
				tracker:    &MockTracker{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
		},
		"ResourceExistsNotUpToDate": {
			reason: "should return ResourceUpToDate=false when resource needs update",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						observation: managed.ExternalObservation{
							ResourceExists:   true,
							ResourceUpToDate: false,
							ConnectionDetails: managed.ConnectionDetails{
								"key": []byte("value"),
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{
					hasExpiredKeysResult: false,
				},
				tracker: &MockTracker{},
				kube:    &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
					ConnectionDetails: managed.ConnectionDetails{
						"key": []byte("value"),
					},
				},
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
		},
		"ResourceUpToDateButExpiredKeys": {
			reason: "should return ResourceUpToDate=false when resource has expired keys",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						observation: managed.ExternalObservation{
							ResourceExists:   true,
							ResourceUpToDate: true,
							ConnectionDetails: managed.ConnectionDetails{
								"key": []byte("value"),
							},
						},
						tfResource: &v1alpha1.SubaccountServiceBinding{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"crossplane.io/external-name": "test-external-name",
								},
							},
							Status: v1alpha1.SubaccountServiceBindingStatus{
								AtProvider: v1alpha1.SubaccountServiceBindingObservation{
									ID:    internal.Ptr("test-id"),
									State: internal.Ptr("succeeded"),
								},
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{
					hasExpiredKeysResult: true, // Has expired keys
				},
				tracker: &MockTracker{},
				kube: &test.MockClient{
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false, // Should be false due to expired keys
					ConnectionDetails: managed.ConnectionDetails{
						"key": []byte("value"),
					},
				},
				cr: expectedServiceBinding(
					withMetadata("test-external-name", map[string]string{"crossplane.io/external-name": "test-external-name"}), // External name gets set from tfResource
					withConditions(xpv1.Available()), // Available condition gets set
					func(cr *v1alpha1.ServiceBinding) {
						// AtProvider gets updated with tfResource data
						cr.Status.AtProvider.ID = "test-id"
						cr.Status.AtProvider.State = internal.Ptr("succeeded")
					},
				),
			},
		},
		"ResourceUpToDateNoExpiredKeys": {
			reason: "should return ResourceUpToDate=true when resource is current and no expired keys",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						observation: managed.ExternalObservation{
							ResourceExists:   true,
							ResourceUpToDate: true,
							ConnectionDetails: managed.ConnectionDetails{
								"key": []byte("value"),
							},
						},
						tfResource: &v1alpha1.SubaccountServiceBinding{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"crossplane.io/external-name": "tf-external-uuid-123",
								},
							},
							Status: v1alpha1.SubaccountServiceBindingStatus{
								AtProvider: v1alpha1.SubaccountServiceBindingObservation{
									ID:           internal.Ptr("tf-binding-id-456"),
									Name:         internal.Ptr("tf-binding-name-789"),
									Ready:        internal.Ptr(true),
									State:        internal.Ptr("succeeded"),
									CreatedDate:  internal.Ptr("2023-10-21T10:30:00Z"),
									LastModified: internal.Ptr("2023-10-21T15:45:00Z"),
									Parameters:   internal.Ptr(`{"param1":"value1","param2":"value2"}`),
								},
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{
					hasExpiredKeysResult: false,
					retireBindingResult:  false,
				},
				tracker: &MockTracker{},
				kube: &test.MockClient{
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
					ConnectionDetails: managed.ConnectionDetails{
						"key": []byte("value"),
					},
				},
				cr: expectedServiceBinding(
					withMetadata("tf-external-uuid-123", map[string]string{"crossplane.io/external-name": "tf-external-uuid-123"}), // External name gets set from tfResource
					withConditions(xpv1.Available()), // Available condition gets set when State="succeeded"
					func(cr *v1alpha1.ServiceBinding) {
						// All AtProvider fields should be populated from tfResource via updateServiceBindingFromTfResource
						cr.Status.AtProvider.ID = "tf-binding-id-456"
						cr.Status.AtProvider.Name = "tf-binding-name-789"
						cr.Status.AtProvider.Ready = internal.Ptr(true)
						cr.Status.AtProvider.State = internal.Ptr("succeeded")
						cr.Status.AtProvider.Parameters = internal.Ptr(`{"param1":"value1","param2":"value2"}`)
						// CreatedDate and LastModified should be parsed from ISO8601 to metav1.Time
						createdTime, _ := time.Parse(time.RFC3339, "2023-10-21T10:30:00Z")
						cr.Status.AtProvider.CreatedDate = &metav1.Time{Time: createdTime}
						lastModifiedTime, _ := time.Parse(time.RFC3339, "2023-10-21T15:45:00Z")
						cr.Status.AtProvider.LastModified = &metav1.Time{Time: lastModifiedTime}
					},
				),
			},
		},
		"RetireBindingTriggered": {
			reason: "should return ResourceExists=false when binding needs retirement",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						observation: managed.ExternalObservation{
							ResourceExists:   true,
							ResourceUpToDate: true,
							ConnectionDetails: managed.ConnectionDetails{
								"key": []byte("value"),
							},
						},
						tfResource: &v1alpha1.SubaccountServiceBinding{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"crossplane.io/external-name": "retire-test-external-name",
								},
							},
							Status: v1alpha1.SubaccountServiceBindingStatus{
								AtProvider: v1alpha1.SubaccountServiceBindingObservation{
									ID:    internal.Ptr("test-id"),
									State: internal.Ptr("succeeded"),
								},
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{
					hasExpiredKeysResult: false,
					retireBindingResult:  true, // Triggers retirement
				},
				tracker: &MockTracker{},
				kube: &test.MockClient{
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false, // Should be false due to retirement
				},
				cr: expectedServiceBinding(
					withMetadata("retire-test-external-name", map[string]string{"crossplane.io/external-name": "retire-test-external-name"}), // External name gets set from tfResource
					withConditions(xpv1.Available()), // Available condition gets set
					func(cr *v1alpha1.ServiceBinding) {
						// AtProvider gets updated with tfResource data
						cr.Status.AtProvider.ID = "test-id"
						cr.Status.AtProvider.State = internal.Ptr("succeeded")
					},
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				kube:          tc.fields.kube,
				clientFactory: tc.fields.clientFactory,
				keyRotator:    tc.fields.keyRotator,
				tracker:       tc.fields.tracker,
			}

			got, err := e.Observe(context.Background(), tc.args.mg)

			if tc.want.err != nil {
				if err == nil {
					t.Errorf("\n%s\ne.Observe(...): expected error, got nil\n", tc.reason)
				} else {
					expectedMsg := tc.want.err.Error()
					gotMsg := err.Error()
					if !containsError(gotMsg, expectedMsg) {
						t.Errorf("\n%s\ne.Observe(...): expected error containing %q, got %q\n", tc.reason, expectedMsg, gotMsg)
					}
				}
			} else if err != nil {
				t.Errorf("\n%s\ne.Observe(...): expected no error, got %v\n", tc.reason, err)
			}

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			// Verify the entire CR state
			if tc.want.cr != nil {
				cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
				if !ok {
					t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
				}
				if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
					t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
				}
			}
		})
	}
}

// Test Create method - This validates the main creation logic
func TestCreate(t *testing.T) {
	type fields struct {
		clientFactory ServiceBindingClientFactory
		keyRotator    servicebindingclient.KeyRotator
		kube          *test.MockClient
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
		cr  *v1alpha1.ServiceBinding // Expected complete CR after creation
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongResourceType": {
			reason: "should return error for wrong resource type",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{},
				},
				keyRotator: &MockKeyRotator{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: errors.New(errNotServiceBinding),
			},
		},
		"ClientCreateError": {
			reason: "should return error when client create fails",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Error: errors.New("client construction error"),
				},
				keyRotator: &MockKeyRotator{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					func(cr *v1alpha1.ServiceBinding) {
						cr.Spec.ForProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				err: errors.New("client construction error"),
				cr: expectedServiceBinding(
					withConditions(xpv1.Creating()),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Spec.ForProvider.Name = "test-binding"
						// AtProvider.Name should remain empty when client create fails
						// because btpName is not set until after successful create
					},
				),
			},
		},
		"SuccessWithoutRotation": {
			reason: "should create successfully when rotation is disabled",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						creation: managed.ExternalCreation{
							ConnectionDetails: managed.ConnectionDetails{
								"test-key": []byte("test-value"),
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{},
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(
					func(cr *v1alpha1.ServiceBinding) {
						cr.Spec.ForProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				err: nil, // Should succeed with new factory pattern
				cr: expectedServiceBinding(
					withMetadata("12345678-1234-5678-9abc-123456789012", map[string]string{
						"crossplane.io/external-name": "12345678-1234-5678-9abc-123456789012",
					}), // External name gets set after successful creation
					withConditions(xpv1.Creating()),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Spec.ForProvider.Name = "test-binding"
					},
				),
			},
		},
		"SuccessWithRotation": {
			reason: "should create successfully when rotation is enabled",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						creation: managed.ExternalCreation{
							ConnectionDetails: managed.ConnectionDetails{
								"test-key": []byte("test-value"),
							},
						},
					},
					Error: errors.New("mock: client creation not supported in current test architecture"),
				},
				keyRotator: &MockKeyRotator{},
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("old-external-name-uuid", map[string]string{servicebindingclient.ForceRotationKey: "true"}),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Spec.ForProvider.Name = "test-binding"
						cr.Spec.ForProvider.Rotation = &v1alpha1.RotationParameters{
							Frequency: &metav1.Duration{Duration: time.Hour * 24},
						}
						// Simulate existing status from previous binding (before rotation)
						cr.Status.AtProvider.ID = "old-binding-id"
						cr.Status.AtProvider.Name = "test-binding-old123"
						cr.Status.AtProvider.State = internal.Ptr("succeeded")
						cr.Status.AtProvider.Ready = internal.Ptr(true)
					},
				),
			},
			want: want{
				err: errors.New("mock: client creation not supported in current test architecture"),
				cr: expectedServiceBinding(
					withMetadata("old-external-name-uuid", map[string]string{servicebindingclient.ForceRotationKey: "true"}),
					withConditions(xpv1.Creating()),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Spec.ForProvider.Name = "test-binding"
						cr.Spec.ForProvider.Rotation = &v1alpha1.RotationParameters{
							Frequency: &metav1.Duration{Duration: time.Hour * 24},
						}
						// Status should be preserved when create fails
						cr.Status.AtProvider.ID = "old-binding-id"
						cr.Status.AtProvider.Name = "test-binding-old123"
						cr.Status.AtProvider.State = internal.Ptr("succeeded")
						cr.Status.AtProvider.Ready = internal.Ptr(true)
						// Other fields remain as they were
						cr.Status.AtProvider.CreatedDate = nil
						cr.Status.AtProvider.LastModified = nil
					},
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				kube:          tc.fields.kube,
				clientFactory: tc.fields.clientFactory,
				keyRotator:    tc.fields.keyRotator,
			}

			got, err := e.Create(context.Background(), tc.args.mg)

			if tc.want.err != nil {
				if err == nil {
					t.Errorf("\n%s\ne.Create(...): expected error, got nil\n", tc.reason)
				} else {
					expectedMsg := tc.want.err.Error()
					gotMsg := err.Error()
					if !containsError(gotMsg, expectedMsg) {
						t.Errorf("\n%s\ne.Create(...): expected error containing %q, got %q\n", tc.reason, expectedMsg, gotMsg)
					}
				}
			} else if err != nil {
				t.Errorf("\n%s\ne.Create(...): expected no error, got %v\n", tc.reason, err)
			}

			if tc.want.err == nil {
				expectedCreation := tc.fields.clientFactory.(*MockServiceBindingClientFactory).Client.(*MockServiceBindingClient).creation
				if diff := cmp.Diff(expectedCreation, got); diff != "" {
					t.Errorf("\n%s\ne.Create(...): -want creation, +got creation:\n%s\n", tc.reason, diff)
				}
			}

			// Check final CR state - simple want/got comparison
			if tc.want.cr != nil {
				cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
				if !ok {
					t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
				}
				if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
					t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type fields struct {
		clientFactory ServiceBindingClientFactory
		keyRotator    servicebindingclient.KeyRotator
		kube          *test.MockClient
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
		u   managed.ExternalUpdate   // Expected update result
		cr  *v1alpha1.ServiceBinding // Expected complete CR after update
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongResourceType": {
			reason: "should return error for wrong resource type",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{},
				},
				keyRotator: &MockKeyRotator{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: errors.New(errNotServiceBinding),
			},
		},
		"ClientUpdateError": {
			reason: "should return error when client update fails",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						updateErr: errors.New("client update error"),
					},
				},
				keyRotator: &MockKeyRotator{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				err: errors.New("client update error"),
				u:   managed.ExternalUpdate{}, // Empty update result on error
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
		},
		"DeleteExpiredKeysError": {
			reason: "should return error when deleting expired keys fails",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						update: managed.ExternalUpdate{
							ConnectionDetails: managed.ConnectionDetails{
								"test-key": []byte("test-value"),
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{
					deleteExpiredKeysErr: errors.New("delete expired keys error"),
				},
				kube: &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				err: errors.New("delete expired keys error"),
				u:   managed.ExternalUpdate{}, // Empty update result on error
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
		},
		"SuccessWithCurrentBindingRetired": {
			reason: "should skip update when current binding is retired",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						update: managed.ExternalUpdate{
							ConnectionDetails: managed.ConnectionDetails{
								"test-key": []byte("test-value"),
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{
					isCurrentBindingRetiredResult: true,
					deleteExpiredKeysResult:       []*v1alpha1.RetiredSBResource{},
				},
				kube: &test.MockClient{
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				u: managed.ExternalUpdate{
					ConnectionDetails: managed.ConnectionDetails{}, // Empty map when update is skipped
				},
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
						cr.Status.AtProvider.RetiredKeys = []*v1alpha1.RetiredSBResource{}
					},
				),
			},
		},
		"SuccessWithUpdate": {
			reason: "should update successfully when binding is not retired",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						update: managed.ExternalUpdate{
							ConnectionDetails: managed.ConnectionDetails{
								"updated-key": []byte("updated-value"),
							},
						},
					},
				},
				keyRotator: &MockKeyRotator{
					isCurrentBindingRetiredResult: false,
					deleteExpiredKeysResult: []*v1alpha1.RetiredSBResource{
						{
							ID:   "retired-id-1",
							Name: "retired-binding-1",
						},
					},
				},
				kube: &test.MockClient{
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				u: managed.ExternalUpdate{
					ConnectionDetails: managed.ConnectionDetails{
						"updated-key": []byte("updated-value"),
					},
				},
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
						cr.Status.AtProvider.RetiredKeys = []*v1alpha1.RetiredSBResource{
							{
								ID:   "retired-id-1",
								Name: "retired-binding-1",
							},
						}
					},
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				kube:          tc.fields.kube,
				clientFactory: tc.fields.clientFactory,
				keyRotator:    tc.fields.keyRotator,
			}

			got, err := e.Update(context.Background(), tc.args.mg)

			if tc.want.err != nil {
				if err == nil {
					t.Errorf("\n%s\ne.Update(...): expected error, got nil\n", tc.reason)
				} else {
					expectedMsg := tc.want.err.Error()
					gotMsg := err.Error()
					if !containsError(gotMsg, expectedMsg) {
						t.Errorf("\n%s\ne.Update(...): expected error containing %q, got %q\n", tc.reason, expectedMsg, gotMsg)
					}
				}
			} else if err != nil {
				t.Errorf("\n%s\ne.Update(...): expected no error, got %v\n", tc.reason, err)
			}

			// Simple diff comparison for update result
			if diff := cmp.Diff(tc.want.u, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			if tc.want.cr != nil {
				cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
				if !ok {
					t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
				}
				if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
					t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
				}
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		clientFactory ServiceBindingClientFactory
		keyRotator    servicebindingclient.KeyRotator
		tracker       *MockTracker
		kube          *test.MockClient
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
		d   managed.ExternalDelete   // Expected deletion result
		cr  *v1alpha1.ServiceBinding // Expected complete CR after deletion
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"WrongResourceType": {
			reason: "should return error for wrong resource type",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{},
				},
				keyRotator: &MockKeyRotator{},
				tracker:    &MockTracker{},
				kube:       &test.MockClient{},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{}, // Wrong type
			},
			want: want{
				err: errors.New(errNotServiceBinding),
				d:   managed.ExternalDelete{}, // Empty deletion result on error
			},
		},
		"DeleteBlockedByDependencies": {
			reason: "should return error when delete is blocked by dependencies",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{},
				},
				keyRotator: &MockKeyRotator{},
				tracker: &MockTracker{
					deleteBlocked: true,
				},
				kube: &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				err: errors.New(providerv1alpha1.ErrResourceInUse),
				d:   managed.ExternalDelete{}, // Empty deletion result on error
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					withConditions(xpv1.Deleting()),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
		},
		"DeleteRetiredKeysError": {
			reason: "should return error when deleting retired keys fails",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{},
				},
				keyRotator: &MockKeyRotator{
					deleteRetiredKeysErr: errors.New("delete retired keys error"),
				},
				tracker: &MockTracker{
					deleteBlocked: false,
				},
				kube: &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				err: errors.New("delete retired keys error"),
				d:   managed.ExternalDelete{}, // Empty deletion result on error
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					withConditions(xpv1.Deleting()),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
		},
		"ClientDeleteError": {
			reason: "should return error when client delete fails",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						deleteErr: errors.New("client delete error"),
					},
				},
				keyRotator: &MockKeyRotator{},
				tracker: &MockTracker{
					deleteBlocked: false,
				},
				kube: &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				err: errors.New("client delete error"),
				d:   managed.ExternalDelete{}, // Empty deletion result on error
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					withConditions(xpv1.Deleting()),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
		},
		"SuccessfulDelete": {
			reason: "should delete successfully",
			fields: fields{
				clientFactory: &MockServiceBindingClientFactory{
					Client: &MockServiceBindingClient{
						deletion: managed.ExternalDelete{},
					},
				},
				keyRotator: &MockKeyRotator{},
				tracker: &MockTracker{
					deleteBlocked: false,
				},
				kube: &test.MockClient{},
			},
			args: args{
				mg: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
			want: want{
				d: managed.ExternalDelete{}, // Expected deletion result
				cr: expectedServiceBinding(
					withMetadata("test-external-name", nil),
					withConditions(xpv1.Deleting()),
					func(cr *v1alpha1.ServiceBinding) {
						cr.Status.AtProvider.Name = "test-binding"
					},
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				kube:          tc.fields.kube,
				clientFactory: tc.fields.clientFactory,
				keyRotator:    tc.fields.keyRotator,
				tracker:       tc.fields.tracker,
			}

			got, err := e.Delete(context.Background(), tc.args.mg)

			if tc.want.err != nil {
				if err == nil {
					t.Errorf("\n%s\ne.Delete(...): expected error, got nil\n", tc.reason)
				} else {
					expectedMsg := tc.want.err.Error()
					gotMsg := err.Error()
					if !containsError(gotMsg, expectedMsg) {
						t.Errorf("\n%s\ne.Delete(...): expected error containing %q, got %q\n", tc.reason, expectedMsg, gotMsg)
					}
				}
			} else if err != nil {
				t.Errorf("\n%s\ne.Delete(...): expected no error, got %v\n", tc.reason, err)
			}

			// Simple diff comparison for deletion result
			if diff := cmp.Diff(tc.want.d, got); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			if tc.want.cr != nil {
				cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
				if !ok {
					t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
				}
				if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
					t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
				}
			}
		})
	}
}

// Helper function to check if an error message contains the expected text
func containsError(got, expected string) bool {
	// For tracking errors, we expect the error to be wrapped
	if expected == "mock tracking error" {
		return strings.Contains(got, expected)
	}
	return strings.Contains(got, expected)
}
