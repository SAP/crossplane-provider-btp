package serviceinstanceclient

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	conditionUnknown = xpv1.Condition{
		Type:   xpv1.TypeReady,
		Status: corev1.ConditionUnknown,
	}
	conditionAvailable = xpv1.Available()
)

func TestTfResource(t *testing.T) {

	type args struct {
		si   *v1alpha1.ServiceInstance
		kube client.Client
	}

	type want struct {
		tfResource *v1alpha1.SubaccountServiceInstance
		hasErr     bool
	}

	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Corrupted parameters": {
			reason: "Throw error if parameters are neither valid as json nor yaml",
			args: args{
				si: expectedServiceInstance(withParameters(`{invalid}`)),
			},
			want: want{
				hasErr: true,
			},
		},
		"Not set parameters": {
			reason: "Gracefully handle unset parameters",
			args: args{
				si: expectedServiceInstance(
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
				),
			},
			want: want{
				tfResource: expectedTfSerivceInstance(
					withTfParameters(`{}`),
					withTfExternalName("123"),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
				),
				hasErr: false,
			},
		},
		"Simply parameters mapping": {
			reason: "Transfer json parameters from spec to tf resource if valid",
			args: args{
				si: expectedServiceInstance(
					withParameters(`{"key": "value"}`),
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
				),
			},
			want: want{
				tfResource: expectedTfSerivceInstance(
					withTfParameters(`{"key":"value"}`),
					withTfExternalName("123"),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
				),
				hasErr: false,
			},
		},
		"Simply yaml parameters mapping": {
			reason: "Transfer yaml parameters from spec to tf resource if valid",
			args: args{
				si: expectedServiceInstance(
					withParameters(`key: value`),
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
				),
			},
			want: want{
				tfResource: expectedTfSerivceInstance(
					withTfParameters(`{"key":"value"}`),
					withTfExternalName("123"),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
				),
				hasErr: false,
			},
		},
		"Resolved ServicePlanID": {
			reason: "If no service plan ID is set, it should be resolved from the status",
			args: args{
				si: expectedServiceInstance(
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
					withObservation(v1alpha1.ServiceInstanceObservation{
						ServiceplanID: "resolved-plan-id",
					}),
				),
			},
			want: want{
				tfResource: expectedTfSerivceInstance(
					withTfParameters(`{}`),
					withTfExternalName("123"),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfServicePlanID("resolved-plan-id"),
					withTfCondition(conditionUnknown),
				),
				hasErr: false,
			},
		},
		"Secret Lookup failed": {
			reason: "Error should be returned if at least one secret lookup fails",
			args: args{
				si: expectedServiceInstance(withParameters(`{"key": "value"}`), withParameterSecrets(map[string]string{"secret1": "secret-key1", "secret2": "secret-key2"})),
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						if key.Name == "secret1" {
							return nil
						}
						return errors.New("secret not found")
					},
				},
			},
			want: want{
				hasErr: true,
			},
		},
		"Corrupted Secret Parameters": {
			reason: "Error should be returned if at least one secret is corrupted in its json structure",
			args: args{
				si: expectedServiceInstance(withParameters(`{"key": "value"}`), withParameterSecrets(map[string]string{"secret1": "secret-key1", "secret2": "secret-key2"})),
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						s := obj.(*corev1.Secret)
						if key.Name == "secret1" {
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						} else if key.Name == "secret2" {
							s.Data = map[string][]byte{
								"secret-key2": []byte(`{no-json}`),
							}
						}
						return nil
					},
				},
			},
			want: want{
				hasErr: true,
			},
		},

		"Successful Combined Parameters mapping": {
			reason: "Parameters from secret and plain spec should be combined in the tf resource",
			args: args{
				si: expectedServiceInstance(
					withParameters(`{"key": "value"}`),
					withParameterSecrets(map[string]string{"secret1": "secret-key1", "secret2": "secret-key2"}),
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
					withCondition(conditionUnknown),
				),
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						s := obj.(*corev1.Secret)
						if key.Name == "secret1" {
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						} else if key.Name == "secret2" {
							s.Data = map[string][]byte{
								"secret-key2": []byte(`{"key3": "value3"}`),
							}
						}
						return nil
					},
				},
			},
			want: want{
				hasErr: false,
				tfResource: expectedTfSerivceInstance(
					withTfParameters(`{"key":"value","key2":"value2","key3":"value3"}`),
					withTfExternalName("123"),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
				),
			},
		},
		"Successful Combined yaml parameters mapping": {
			reason: "Parameters from secret and plain spec as yaml should be combined in the tf resource",
			args: args{
				si: expectedServiceInstance(
					withParameters(`key: value`),
					withParameterSecrets(map[string]string{"secret1": "secret-key1", "secret2": "secret-key2"}),
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
					withCondition(conditionUnknown),
				),
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						s := obj.(*corev1.Secret)
						if key.Name == "secret1" {
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						} else if key.Name == "secret2" {
							s.Data = map[string][]byte{
								"secret-key2": []byte(`{"key3": "value3"}`),
							}
						}
						return nil
					},
				},
			},
			want: want{
				hasErr: false,
				tfResource: expectedTfSerivceInstance(
					withTfParameters(`{"key":"value","key2":"value2","key3":"value3"}`),
					withTfExternalName("123"),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
				),
			},
		},
		"Recurring Successful Reconciliation": {
			reason: "Ready state should be preserved during reconciliation",
			args: args{
				si: expectedServiceInstance(
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
					withCondition(conditionAvailable),
				),
			},
			want: want{
				hasErr: false,
				tfResource: expectedTfSerivceInstance(
					withTfExternalName("123"),
					withTfParameters(`{}`),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionAvailable),
				),
			},
		},
		"Without ManagementPolicies": {
			reason: "Make sure ManagementPolicies transfered to tf resource",
			args: args{
				si: expectedServiceInstance(
					withExternalName("123"),
					withProviderConfigRef("default"),
				),
			},
			want: want{
				hasErr: false,
				tfResource: expectedTfSerivceInstance(
					withTfExternalName("123"),
					withTfParameters(`{}`),
					withTfProviderConfigRef("default"),
					withTfCondition(conditionUnknown),
				),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			sim := &ServiceInstanceMapper{}

			// Call the function under test
			tfResource, err := sim.TfResource(context.Background(), tc.args.si, tc.args.kube)

			if diff := cmp.Diff(tc.want.tfResource, tfResource, cmpopts.IgnoreFields(v1alpha1.SubaccountServiceInstance{}, "TypeMeta", "ObjectMeta.UID")); diff != "" {
				t.Errorf("TfResource() mismatch (-want +got):\n%s", diff)
			}
			// Only check if error presence matches, not the error value itself
			if tc.want.hasErr != (err != nil) {
				t.Errorf("TfResource() error presence mismatch: want error: %v, got error: %v", tc.want.hasErr, err != nil)
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
	cr.Spec.ForProvider.Name = internal.Ptr("")

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

// Option to set the external name annotation
func withTfExternalName(externalName string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		if cr.GetAnnotations() == nil {
			cr.SetAnnotations(map[string]string{})
		}
		cr.GetAnnotations()["crossplane.io/external-name"] = externalName
	}
}

func withProviderConfigRef(providerConfigName string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ResourceSpec.ProviderConfigReference = &xpv1.Reference{
			Name: providerConfigName,
		}
	}
}

func withTfProviderConfigRef(providerConfigName string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ResourceSpec.ProviderConfigReference = &xpv1.Reference{
			Name: providerConfigName,
		}
	}
}

func withManagementPolicies() func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ResourceSpec.ManagementPolicies = []xpv1.ManagementAction{
			xpv1.ManagementActionAll,
		}
	}
}

func withTfManagementPolicies() func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ResourceSpec.ManagementPolicies = []xpv1.ManagementAction{
			xpv1.ManagementActionAll,
		}
	}
}

func withParameters(jsonParams string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ForProvider.Parameters = runtime.RawExtension{Raw: []byte(jsonParams)}
	}
}

func withTfParameters(jsonParams string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ForProvider.Parameters = &jsonParams
	}
}

func withCondition(condition xpv1.Condition) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.SetConditions(condition)
	}
}

func withTfCondition(condition xpv1.Condition) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Status.SetConditions(condition)
	}
}

func withParameterSecrets(parameterSecrets map[string]string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ForProvider.ParameterSecretRefs = make([]xpv1.SecretKeySelector, 0)
		for k, v := range parameterSecrets {
			cr.Spec.ForProvider.ParameterSecretRefs = append(cr.Spec.ForProvider.ParameterSecretRefs, xpv1.SecretKeySelector{
				SecretReference: xpv1.SecretReference{
					Name: k,
				},
				Key: v,
			})
		}
	}
}

func withObservation(obs v1alpha1.ServiceInstanceObservation) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.AtProvider = obs
	}
}

func withTfServicePlanID(servicePlanID string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ForProvider.ServiceplanID = &servicePlanID
	}
}
