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
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

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
		o   managed.ExternalObservation
		cr  resource.Managed
		err error
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return nil, errors.New("Could not call backend")
				}},
				cr: environment(),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New("Could not call backend"),
				cr:  environment(),
			},
		},
		"NeedsCreate": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				err: errors.Wrap(errors.New("invalid character '}' looking for beginning of value"), "can not obtain kubeConfig"),
				cr:  environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"SuccessfulAvailable": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				err: nil,
				cr:  environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"AvailableWithConnectionDetails": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				err: nil,
				cr:  environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"AvailableWithPartialConnectionDetails": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				err: nil,
				cr:  environment(withUID("1234"), withConditions(xpv1.Available())),
			},
		},
		"UpdateInProgress": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				err: nil,
				cr: environment(withConditions(xpv1.Available()),
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`{"foo": "baz"}`)},
					})),
			},
		},
		"Update with YAML Parameters": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				err: nil,
				cr: environment(withConditions(xpv1.Available()),
					withKymaParameters(v1alpha1.KymaEnvironmentParameters{
						Parameters: runtime.RawExtension{Raw: []byte(`foo: baz`)},
					})),
			},
		},
		"Update with invalid json Parameters": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
				client: fake.MockClient{MockDescribeCluster: func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.EnvironmentInstanceResponseObject, error) {
					return &provisioningclient.EnvironmentInstanceResponseObject{
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
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client, httpClient: http.DefaultClient}
			if tc.args.httpClient != nil {
				e.httpClient = tc.args.httpClient
			}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions(), cmpopts.IgnoreTypes(v1alpha1.KymaEnvironmentObservation{})); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
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
