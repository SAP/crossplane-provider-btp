package serviceinstanceclient

import (
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTfResource(t *testing.T) {

	type args struct {
		si   *v1alpha1.ServiceInstance
		kube client.Client
	}

	type want struct {
		tfResource *v1alpha1.SubaccountServiceInstance
		err        error
	}

	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Corupted json parameters": {
			reason: "Throw error if json parameters are not valid",
			args: args{
				si: expectedServiceInstance(WithParameters(`{no-json}`)),
			},
			want: want{
				err: errors.New("some error"),
			},
		},
		"Simply parameters mapping": {
			reason: "Transfer json parameters from spec to tf resource if valid",
			args: args{
				si: expectedServiceInstance(WithParameters(`{"key": "value"}`)),
			},
			want: want{
				tfResource: expectedTfSerivceInstance(WithTfParameters(`{"key": "value"}`)),
				err:        nil,
			},
		},
		"Secret Lookup failed": {
			reason: "Error should be returned if at least one secret lookup fails",
			args: args{
				si: expectedServiceInstance(WithParameters(`{"key": "value"}`), WithParameterSecrets(map[string]string{"secret1": "secret-key1", "secret2": "secret-key2"})),
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						// only second secret lookup should fail
						if obj.GetName() == "secret1" {
							return nil
						}
						return errors.New("secret not found")
					}),
				},
			},
			want: want{
				err: errors.New("secret not found"),
			},
		},
		"Corrupted Secret Parameters": {
			reason: "Error should be returned if at least one secret is corrupted in its json structure",
			args: args{
				si: expectedServiceInstance(WithParameters(`{"key": "value"}`), WithParameterSecrets(map[string]string{"secret1": "secret-key1", "secret2": "secret-key2"})),
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						s := obj.(*corev1.Secret)
						if obj.GetName() == "secret1" {
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						} else if obj.GetName() == "secret2" {
							s.Data = map[string][]byte{
								"secret-key2": []byte(`{no-json}`),
							}
						}
						return nil
					}),
				},
			},
			want: want{
				err: errors.New("json error"),
			},
		},
		"Successful Combined Parameters mapping": {
			reason: "Parameters from secret and plain spec should be combined in the tf resource",
			args: args{
				si: expectedServiceInstance(WithParameters(`{"key": "value"}`), WithParameterSecrets(map[string]string{"secret1": "secret-key1", "secret2": "secret-key2"})),
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						s := obj.(*corev1.Secret)
						if obj.GetName() == "secret1" {
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						} else if obj.GetName() == "secret2" {
							s.Data = map[string][]byte{
								"secret-key2": []byte(`{"key3": "value3"}`),
							}
						}
						return nil
					}),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			sim := &ServiceInstanceMapper{}

			//TODO: define remaining test cases
			//TODO: add error
			tfResource, err := sim.TfResource(tc.args.si, tc.args.kube)

			if diff := cmp.Diff(tc.want.tfResource, tfResource); diff != "" {
				t.Errorf("TfResource() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.err, err); diff != "" {
				t.Errorf("TfResource() mismatch on error(-want +got):\n%s", diff)
			}

		})
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

// Helper function to build a complete SubaccountServiceInstance CR dynamically
func expectedTfSerivceInstance(opts ...func(*v1alpha1.SubaccountServiceInstance)) *v1alpha1.SubaccountServiceInstance {
	cr := &v1alpha1.SubaccountServiceInstance{}

	// Apply each option to modify the CR
	for _, opt := range opts {
		opt(cr)
	}

	return cr
}

func WithParameters(jsonParams string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ForProvider = v1alpha1.ServiceInstanceParameters{
			SubaccountServiceInstanceParameters: v1alpha1.SubaccountServiceInstanceParameters{
				Parameters: &jsonParams,
			},
		}
	}
}

func WithTfParameters(jsonParams string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ForProvider = v1alpha1.SubaccountServiceInstanceParameters{
			Parameters: &jsonParams,
		}
	}
}
