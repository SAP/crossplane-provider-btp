package kyma

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	kyma "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironment"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/kyma/fake"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

const kubeConfigData = "apiVersion: v1\nkind: Config\ncurrent-context: shoot--kyma-stage--c-5edf6ec\nclusters:\n- name: shoot--kyma-stage--c-5edf6ec\n  cluster:\n    certificate-authority-data: someCaData\n    server: someServerUrl\ncontexts:\n- name: shoot--kyma-stage--c-5edf6ec\n  context:\n    cluster: shoot--kyma-stage--c-5edf6ec\n    user: shoot--kyma-stage--c-5edf6ec\nusers:\n- name: shoot--kyma-stage--c-5edf6ec\n  user:\n    exec:\n      apiVersion: client.authentication.k8s.io/v1beta1\n      args:\n      - get-token\n      - \"--oidc-issuer-url=xxx\"\n      - \"--oidc-client-id=xxx\"\n      - \"--oidc-extra-scope=email\"\n      - \"--oidc-extra-scope=openid\"\n      command: kubectl-oidc_login\n      installHint: |\n        kubelogin plugin is required to proceed with authentication\n        # Homebrew (macOS and Linux)\n        brew install int128/kubelogin/kubelogin\n\n        # Krew (macOS, Linux, Windows and ARM)\n        kubectl krew install oidc-login\n\n        # Chocolatey (Windows)\n        choco install kubelogin\n"

func TestObserve(t *testing.T) {
	type args struct {
		cr         resource.Managed
		client     kyma.Client
		httpClient *http.Client
	}

	type want struct {
		o             managed.ExternalObservation
		crCompareOpts []cmp.Option
		cr            resource.Managed
		err           error
	}

	var cases = map[string]struct {
		args args
		want want
	}{
		"NilManaged": {
			args: args{
				client: fake.MockClient{},
				cr:     nil,
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New(errNotKymaEnvironment),
			},
		},
		"ErrorGettingKymaEnvironment": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return nil, errors.New("Could not call backend")
				}},
				cr: environment(),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.Wrap(errors.New("Could not call backend"), "Could not describe kyma instance"),
				cr:  environment(),
			},
		},
		"NeedsCreate": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return nil, nil
				}},
				cr: environment(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: nil,
				cr:  environment(withConditions(xpv1.Unavailable())),
			},
		},
		"ErrorParsingLabels": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:        internal.Ptr("OK"),
						ModifiedDate: internal.Ptr(float32(2000000000000.000000)),
						Labels:       internal.Ptr("}corrupted{"),
						Parameters:   internal.Ptr("{\"name\":\"kyma\"}"),
					}, nil
				}},
				httpClient: mockedHttpClient("someKubeConfigContent"),
				cr:         environment(withUID("1234"), withObservation(v1alpha1.KymaEnvironmentObservation{EnvironmentObservation: v1alpha1.EnvironmentObservation{ModifiedDate: internal.Ptr("1000000000000.000000")}})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				crCompareOpts: []cmp.Option{ignoreCircuitBreakerStatus()},
				err:           errors.Wrap(errors.New("invalid character '}' looking for beginning of value"), "can not obtain kubeConfig"),
				cr:            environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"SuccessfulAvailable": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:      internal.Ptr("OK"),
						Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
					}, nil
				}},
				cr: environment(withUID("1234")),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				crCompareOpts: []cmp.Option{ignoreCircuitBreakerStatus()},
				err:           nil,
				cr:            environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"AvailableWithConnectionDetails": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:        internal.Ptr("OK"),
						ModifiedDate: internal.Ptr(float32(2000000000000.000000)),
						Labels:       internal.Ptr("{\"name\": \"kyma\", \"KubeconfigURL\": \"someUrl\"}"),
						Parameters:   internal.Ptr("{\"name\":\"kyma\"}"),
					}, nil
				}},
				httpClient: mockedHttpClient(kubeConfigData),
				cr:         environment(withUID("1234"), withObservation(v1alpha1.KymaEnvironmentObservation{EnvironmentObservation: v1alpha1.EnvironmentObservation{ModifiedDate: internal.Ptr("1000000000000.000000")}})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
					ConnectionDetails: managed.ConnectionDetails{
						"kubeconfig":                 []byte(kubeConfigData),
						"name":                       []byte("kyma"),
						"KubeconfigURL":              []byte("someUrl"),
						"server":                     []byte("someServerUrl"),
						"certificate-authority-data": []byte("someCaData"),
					},
				},
				crCompareOpts: []cmp.Option{ignoreCircuitBreakerStatus()},
				err:           nil,
				cr:            environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"AvailableWithPartialConnectionDetails": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:        internal.Ptr("OK"),
						ModifiedDate: internal.Ptr(float32(2000000000000.000000)),
						Labels:       internal.Ptr("{\"name\": \"kyma\", \"KubeconfigURL\": \"someUrl\"}"),
						Parameters:   internal.Ptr("{\"name\":\"kyma\"}"),
					}, nil
				}},
				httpClient: mockedHttpClient("someNotMatchingKubeConfigData"),
				cr:         environment(withUID("1234"), withObservation(v1alpha1.KymaEnvironmentObservation{EnvironmentObservation: v1alpha1.EnvironmentObservation{ModifiedDate: internal.Ptr("1000000000000.000000")}})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
					ConnectionDetails: managed.ConnectionDetails{
						"kubeconfig":                 []byte("someNotMatchingKubeConfigData"),
						"name":                       []byte("kyma"),
						"KubeconfigURL":              []byte("someUrl"),
						"server":                     []byte{},
						"certificate-authority-data": []byte{},
					},
				},
				crCompareOpts: []cmp.Option{ignoreCircuitBreakerStatus()},
				err:           nil,
				cr:            environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"UpdateInProgress": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State: internal.Ptr("UPDATING"),
					}, nil
				}},
				cr: environment(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: nil,
				cr:  environment(withConditions(xpv1.Available())),
			},
		},
		"Update with Json Parameters": {
			args: args{
				client: fake.MockClient{
					MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
						return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
							State:      internal.Ptr("OK"),
							Parameters: internal.Ptr(`{"foo": "bar"}`),
						}, nil
					}},
				cr: environment(withKymaParameters(v1alpha1.KymaEnvironmentParameters{
					Parameters: runtime.RawExtension{Raw: []byte(`{"foo": "baz"}`)},
				})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				crCompareOpts: []cmp.Option{ignoreCircuitBreakerStatus()},
				err:           nil,
				cr: environment(withConditions(xpv1.Available()),
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`{"foo": "baz"}`)},
					})),
			},
		},
		"Update with YAML Parameters": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:      internal.Ptr("OK"),
						Parameters: internal.Ptr(`foo: bar`),
					}, nil
				}},
				cr: environment(withKymaParameters(v1alpha1.KymaEnvironmentParameters{
					Parameters: runtime.RawExtension{Raw: []byte(`foo: baz`)},
				})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				crCompareOpts: []cmp.Option{ignoreCircuitBreakerStatus()}, err: nil,
				cr: environment(withConditions(xpv1.Available()),
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`foo: baz`)},
					})),
			},
		},
		"Update with invalid json Parameters": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:      internal.Ptr("OK"),
						Parameters: internal.Ptr(`foo: bar`),
					}, nil
				}},
				cr: environment(withKymaParameters(v1alpha1.KymaEnvironmentParameters{
					Parameters: runtime.RawExtension{Raw: []byte(`{asd:y}`)},
				})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: errors.Wrap(
					errors.Wrap(
						errors.New("ReadString: expects \" or n, but found a, error found in #2 byte of ...|{asd:y}|..., bigger context ...|{asd:y}|..."),
						errParameterParsing),
					errCheckUpdate),
				cr: environment(withConditions(xpv1.Available()),
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`{asd:y}`)},
					})),
			},
		},
		"Update with invalid yaml Parameters": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:      internal.Ptr("OK"),
						Parameters: internal.Ptr(`foo: bar`),
					}, nil
				}},
				cr: environment(withKymaParameters(v1alpha1.KymaEnvironmentParameters{
					Parameters: runtime.RawExtension{Raw: []byte(`asd`)},
				})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: errors.Wrap(
					errors.Wrap(
						errors.New("error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go value of type map[string]interface {}"),
						errParameterParsing),
					errCheckUpdate),
				cr: environment(withConditions(xpv1.Available()),
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`asd`)},
					})),
			},
		},
		"Update with invalid Service response": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:      internal.Ptr("OK"),
						Parameters: internal.Ptr(`asd`),
					}, nil
				}},
				cr: environment(withKymaParameters(v1alpha1.KymaEnvironmentParameters{
					Parameters: runtime.RawExtension{Raw: []byte(`foo: bar`)},
				})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: errors.Wrap(
					errors.Wrap(
						errors.New("error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go value of type map[string]interface {}"),
						errServiceParsing),
					errCheckUpdate),
				cr: environment(withConditions(xpv1.Available()),
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`foo: bar`)},
					})),
			},
		},
		"Deleting": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State: internal.Ptr("DELETING"),
					}, nil
				}},
				cr: environment(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: nil,
				cr:  environment(withConditions(xpv1.Deleting())),
			},
		},
		"Creating": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State: internal.Ptr("CREATING"),
					}, nil
				}},
				cr: environment(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: nil,
				cr:  environment(withConditions(xpv1.Creating())),
			},
		},
		"CircuitBreakerOn": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:      internal.Ptr("OK"),
						Parameters: internal.Ptr(`foo: bar1`),
					}, nil
				}},
				cr: environment(withKymaParameters(v1alpha1.KymaEnvironmentParameters{
					Parameters: runtime.RawExtension{Raw: []byte(`foo: bar2`)},
				}), withRetryStatus(&v1alpha1.RetryStatus{
					DesiredHash: hash(map[string]interface{}{
						"foo":  "bar2",
						"name": "kyma",
					}),
					CurrentHash: hash(map[string]interface{}{
						"foo": "bar1",
					}),
					Count:          2,
					CircuitBreaker: false,
				}),
				),
			},
			want: want{
				crCompareOpts: []cmp.Option{ignoreCircuitBreakerDiff()},
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				err: nil,
				cr: environment(
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`foo: bar2`)},
					}),
					withConditions(xpv1.Available()),
					withRetryStatus(&v1alpha1.RetryStatus{
						CircuitBreaker: true,
						DesiredHash: hash(map[string]interface{}{
							"foo":  "bar2",
							"name": "kyma",
						}),
						CurrentHash: hash(map[string]interface{}{
							"foo": "bar1",
						}),
						Count: 3,
					}),
				),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client, httpClient: http.DefaultClient, kube: test.NewMockClient()}
			if tc.args.httpClient != nil {
				e.httpClient = tc.args.httpClient
			}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			opts := []cmp.Option{
				test.EquateConditions(), cmpopts.IgnoreTypes(v1alpha1.KymaEnvironmentObservation{}),
			}
			opts = append(opts, tc.want.crCompareOpts...)

			if diff := cmp.Diff(tc.want.cr, tc.args.cr, opts...); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got, cmpopts.IgnoreFields(managed.ExternalObservation{}, "Diff")); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func ignoreCircuitBreakerStatus() cmp.Option {
	return cmpopts.IgnoreTypes(&v1alpha1.RetryStatus{})
}
func ignoreCircuitBreakerDiff() cmp.Option {
	return cmpopts.IgnoreFields(v1alpha1.RetryStatus{}, "Diff")
}

func TestCircuitBreaker(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client kyma.Client
	}

	type want struct {
		o   managed.ExternalUpdate
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"CircuitBreakerOn": {
			args: args{
				client: fake.MockClient{},
				cr: environment(func(r *v1alpha1.KymaEnvironment) {
					r.Status.RetryStatus = &v1alpha1.RetryStatus{
						CircuitBreaker: true,
					}
				}),
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: errors.New(errCircutBreak),
			},
		},
		"CircuitBreakerOff": {
			args: args{
				client: fake.MockClient{},
				cr: environment(func(r *v1alpha1.KymaEnvironment) {
					r.Status.RetryStatus = &v1alpha1.RetryStatus{}
				}),
			},
			want: want{
				o:   managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Update(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Update(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\ne.Update(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestUpdateCircuitBreakerStatus(t *testing.T) {
	type args struct {
		cr         *v1alpha1.KymaEnvironment
		desired    any
		current    any
		diff       string
		maxRetries int
	}
	tests := []struct {
		name string
		args args
		want *v1alpha1.RetryStatus
	}{
		{
			name: "Initial Retry Status Creation",
			args: args{
				cr:         &v1alpha1.KymaEnvironment{Status: v1alpha1.KymaEnvironmentStatus{}},
				desired:    "something",
				current:    "something",
				diff:       "",
				maxRetries: 3,
			},
			want: &v1alpha1.RetryStatus{
				DesiredHash:    hash("something"),
				CurrentHash:    hash("something"),
				Diff:           "",
				Count:          1,
				CircuitBreaker: false,
			},
		},
		{
			name: "Count Increment and Circuit Breaker On",
			args: args{
				cr: &v1alpha1.KymaEnvironment{
					Status: v1alpha1.KymaEnvironmentStatus{
						RetryStatus: &v1alpha1.RetryStatus{
							DesiredHash:    hash("something"),
							CurrentHash:    hash("somethingElse"),
							Count:          2,
							CircuitBreaker: false,
						},
					},
				},
				desired:    "something",
				current:    "somethingElse",
				diff:       "some-diff",
				maxRetries: 3,
			},
			want: &v1alpha1.RetryStatus{
				DesiredHash:    hash("something"),
				CurrentHash:    hash("somethingElse"),
				Diff:           "some-diff",
				Count:          3,
				CircuitBreaker: true,
			},
		},
		{
			name: "Reset Retry Status on new diff",
			args: args{
				cr: &v1alpha1.KymaEnvironment{
					Status: v1alpha1.KymaEnvironmentStatus{
						RetryStatus: &v1alpha1.RetryStatus{
							DesiredHash:    hash("something"),
							CurrentHash:    hash("somethingElse"),
							Count:          3,
							CircuitBreaker: true,
						},
					},
				},
				desired:    "changedSomething",
				current:    "somethingElse",
				diff:       "some-diff",
				maxRetries: 3,
			},
			want: &v1alpha1.RetryStatus{
				DesiredHash:    hash("changedSomething"),
				CurrentHash:    hash("somethingElse"),
				Diff:           "some-diff",
				Count:          1,
				CircuitBreaker: false,
			},
		},
		{
			name: "Reset Retry Status on empty diff",
			args: args{
				cr: &v1alpha1.KymaEnvironment{
					Status: v1alpha1.KymaEnvironmentStatus{
						RetryStatus: &v1alpha1.RetryStatus{
							DesiredHash:    hash("something"),
							CurrentHash:    hash("somethingElse"),
							Count:          3,
							CircuitBreaker: true,
						},
					},
				},
				desired:    "somethingElse",
				current:    "somethingElse",
				diff:       "",
				maxRetries: 3,
			},
			want: &v1alpha1.RetryStatus{
				DesiredHash:    hash("somethingElse"),
				CurrentHash:    hash("somethingElse"),
				Diff:           "",
				Count:          1,
				CircuitBreaker: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCircuitBreakerStatus(tt.args.cr, tt.args.desired, tt.args.current, tt.args.diff, tt.args.maxRetries)
			if diff := cmp.Diff(tt.want, tt.args.cr.Status.RetryStatus); diff != "" {
				t.Errorf("updateCircuitBreakerStatus() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMaxRetriesExtraction(t *testing.T) {
	tests := []struct {
		name          string
		annotations   map[string]string
		expectedValue int
		expectError   bool
	}{
		{
			name:          "Valid annotation",
			annotations:   map[string]string{v1alpha1.AnnotationMaxRetries: "5"},
			expectedValue: 5,
			expectError:   false,
		},
		{
			name:          "Invalid annotation value",
			annotations:   map[string]string{v1alpha1.AnnotationMaxRetries: "invalid"},
			expectedValue: 0,
			expectError:   true,
		},
		{
			name:          "Missing annotation",
			annotations:   map[string]string{},
			expectedValue: maxRetriesDefault, // Default value
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &v1alpha1.KymaEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}

			retries, err := lookupMaxRetries(cr, maxRetriesDefault)
			if err != nil {
				return
			}

			if (err != nil) != tt.expectError {
				t.Errorf("expected error: %v, got: %v", tt.expectError, err)
			}
			if retries != tt.expectedValue {
				t.Errorf("expected maxRetries: %d, got: %d", tt.expectedValue, retries)
			}
		})
	}
}

type environmentModifier func(*v1alpha1.KymaEnvironment)

func withConditions(c ...xpv1.Condition) environmentModifier {
	return func(r *v1alpha1.KymaEnvironment) { r.Status.ConditionedStatus.Conditions = c }
}

func withKymaParameters(c v1alpha1.KymaEnvironmentParameters) environmentModifier {
	return func(r *v1alpha1.KymaEnvironment) { r.Spec.ForProvider = c }
}
func withUID(uid types.UID) environmentModifier {
	return func(r *v1alpha1.KymaEnvironment) { r.UID = uid }
}

func withRetryStatus(retryStatus *v1alpha1.RetryStatus) environmentModifier {
	return func(r *v1alpha1.KymaEnvironment) { r.Status.RetryStatus = retryStatus }
}

func withObservation(observation v1alpha1.KymaEnvironmentObservation) environmentModifier {
	return func(r *v1alpha1.KymaEnvironment) {
		r.Status.AtProvider = observation
	}
}

func environment(m ...environmentModifier) *v1alpha1.KymaEnvironment {
	cr := &v1alpha1.KymaEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kyma",
		},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func mockedHttpClient(fileContent string) *http.Client {
	var fn RoundTripFunc = func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(fileContent)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	}
	return &http.Client{
		Transport: fn,
	}
}
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

type RoundTripFunc func(req *http.Request) *http.Response
