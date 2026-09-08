package serviceinstanceclient

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
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
					withTfTimeouts("60m"),
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
					withTfTimeouts("60m"),
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
					withTfTimeouts("60m"),
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
					withTfTimeouts("60m"),
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
						switch key.Name {
						case "secret1":
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						case "secret2":
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
						switch key.Name {
						case "secret1":
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						case "secret2":
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
					withTfTimeouts("60m"),
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
						switch key.Name {
						case "secret1":
							s.Data = map[string][]byte{
								"secret-key1": []byte(`{"key2": "value2"}`),
							}
						case "secret2":
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
					withTfTimeouts("60m"),
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
					withTfTimeouts("60m"),
				),
			},
		},
		"Labels are propagated to tf resource": {
			reason: "Labels set in forProvider should be passed through to the tf resource",
			args: args{
				si: expectedServiceInstance(
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
					withLabels(map[string][]*string{
						"env":  {internal.Ptr("dev"), internal.Ptr("test")},
						"team": {internal.Ptr("platform")},
					}),
				),
			},
			want: want{
				hasErr: false,
				tfResource: expectedTfSerivceInstance(
					withTfParameters(`{}`),
					withTfExternalName("123"),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
					withTfTimeouts("60m"),
					withTfLabels(map[string][]*string{
						"env":  {internal.Ptr("dev"), internal.Ptr("test")},
						"team": {internal.Ptr("platform")},
					}),
				),
			},
		},
		"TfResource_ManagementPolicies_AlwaysSetToAll": {
			reason: "ADR: ManagementPolicies on the TfResource must always be '*' regardless of the native CR's policies, so that tf does use refresh instead of import",
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
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
					withTfTimeouts("60m"),
				),
			},
		},
		"OperationTimeout wired to Timeouts on TfResource": {
			reason: "operationTimeout field should be wired to Timeouts.Create/Update/Delete on the tf resource so upjet writes a timeouts block into main.tf.json",
			args: args{
				si: expectedServiceInstance(
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
					withOperationTimeout("30m"),
				),
			},
			want: want{
				hasErr: false,
				tfResource: expectedTfSerivceInstance(
					withTfExternalName("123"),
					withTfParameters(`{}`),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
					withTfTimeouts("30m"),
				),
			},
		},
		"DefaultTimeout of 60m applied when operationTimeout not set": {
			reason: "when operationTimeout is not set, Timeouts.Create/Update/Delete should default to 60m",
			args: args{
				si: expectedServiceInstance(
					withExternalName("123"),
					withProviderConfigRef("default"),
					withManagementPolicies(),
				),
			},
			want: want{
				hasErr: false,
				tfResource: expectedTfSerivceInstance(
					withTfExternalName("123"),
					withTfParameters(`{}`),
					withTfProviderConfigRef("default"),
					withTfManagementPolicies(),
					withTfCondition(conditionUnknown),
					withTfTimeouts("60m"),
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

// TestShadowReadyCondition pins which Ready condition the terraform shadow
// carries, which decides whether terraform is allowed to plan at all.
//
// upjet's Observe short-circuits before Workspace.Plan whenever the resource is
// not marked Available: it marks it Available in memory and returns
// ResourceUpToDate=true without planning. The shadow is rebuilt from the native
// resource on every Connect, so mirroring a Ready=False onto it would park
// upjet in that arm forever - no plan, no drift, no Update. Readiness that
// reports external health (upstream issue #280) stays a statement about BTP on
// the native resource only, and must never disable drift detection: an
// unhealthy instance is exactly the one an operator repairs with a spec change.
func TestShadowReadyCondition(t *testing.T) {
	observed := v1alpha1.ServiceInstanceObservation{ID: "some-instance-id"}

	unhealthy := xpv1.Condition{
		Type:    xpv1.TypeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "ExternalResourceFailed",
		Message: "BTP reports the service instance as unhealthy",
	}

	cases := map[string]struct {
		reason string
		si     *v1alpha1.ServiceInstance
		want   xpv1.Condition
	}{
		"UnhealthyButExisting": {
			reason: "an existing instance reported unhealthy must still hand the shadow an Available condition, otherwise terraform never plans again and a repairing spec change is never applied",
			si: expectedServiceInstance(
				withObservation(observed),
				withCondition(unhealthy),
			),
			want: conditionAvailable,
		},
		"HealthyAndExisting": {
			reason: "an existing healthy instance is unchanged",
			si: expectedServiceInstance(
				withObservation(observed),
				withCondition(conditionAvailable),
			),
			want: conditionAvailable,
		},
		"NotYetObserved": {
			reason: "before the external resource exists there is nothing to plan against, so the native condition is mirrored unchanged and upjet's mark-available-first create behaviour is kept",
			si: expectedServiceInstance(
				withCondition(conditionUnknown),
			),
			want: conditionUnknown,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := shadowReadyCondition(tc.si)
			if got.Type != tc.want.Type || got.Status != tc.want.Status || got.Reason != tc.want.Reason {
				t.Errorf("\n%s\nshadowReadyCondition() = %+v, want %+v", tc.reason, got, tc.want)
			}
		})
	}
}

// TestTfResourceMarksExistingUnhealthyShadowAvailable pins the same behaviour
// through the exported mapper, which is what actually feeds upjet.
func TestTfResourceMarksExistingUnhealthyShadowAvailable(t *testing.T) {
	si := expectedServiceInstance(
		withExternalName("123"),
		withProviderConfigRef("default"),
		withObservation(v1alpha1.ServiceInstanceObservation{ID: "some-instance-id"}),
		withCondition(xpv1.Condition{
			Type:    xpv1.TypeReady,
			Status:  corev1.ConditionFalse,
			Reason:  "ExternalResourceFailed",
			Message: "BTP reports the service instance as unhealthy",
		}),
	)

	sim := &ServiceInstanceMapper{}
	tfResource, err := sim.TfResource(context.Background(), si, nil)
	if err != nil {
		t.Fatalf("TfResource() returned unexpected error: %v", err)
	}

	got := tfResource.GetCondition(xpv1.TypeReady)
	if got.Status != corev1.ConditionTrue {
		t.Errorf("terraform shadow must be marked Available so upjet keeps planning, got %+v", got)
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
	cr.Name = "TF-"
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
		cr.Spec.ProviderConfigReference = &xpv1.Reference{
			Name: providerConfigName,
		}
	}
}

func withTfProviderConfigRef(providerConfigName string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ProviderConfigReference = &xpv1.Reference{
			Name: providerConfigName,
		}
	}
}

func withManagementPolicies() func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ManagementPolicies = []xpv1.ManagementAction{
			xpv1.ManagementActionAll,
		}
	}
}

func withTfManagementPolicies() func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ManagementPolicies = []xpv1.ManagementAction{
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

func withLabels(labels map[string][]*string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ForProvider.Labels = labels
	}
}

func withTfLabels(labels map[string][]*string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		cr.Spec.ForProvider.Labels = labels
	}
}

func withOperationTimeout(timeout string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Spec.ForProvider.OperationTimeout = &timeout
	}
}

func withTfTimeouts(timeout string) func(*v1alpha1.SubaccountServiceInstance) {
	return func(cr *v1alpha1.SubaccountServiceInstance) {
		t := timeout
		cr.Spec.ForProvider.Timeouts = &v1alpha1.TimeoutsParameters{
			Create: &t,
			Update: &t,
			Delete: &t,
		}
	}
}
