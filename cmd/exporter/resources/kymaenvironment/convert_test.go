package kymaenvironment

import (
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func TestConvertKymaEnvResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Test data
	envID := "1234-5678-9abc-def0-1234-5678-9abc-def0"
	subaccountGUID := "sa-12345678-1234-1234-1234-123456789"
	landscapeLabel := "kyma-eu10"
	cmName := "cloud-management-ref"
	planName := "aws"
	name := "kyma-name"
	resourceName := name + ".x" + envID
	// note: the "x" comes from parsen, as every label has to start with a letter

	// convertKymaEnvResource always marshals the (empty) EnvironmentInstance
	// Parameters into ForProvider.Parameters, which yields "{}".
	wantParameters := runtime.RawExtension{Raw: []byte("{}")}

	tests := []struct {
		name    string
		kymaEnv *KymaEnvironment
		want    *yaml.ResourceWithComment
	}{
		{
			name: "all required fields present",
			kymaEnv: &KymaEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           name,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					PlanName:       planName,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.KymaEnvironment{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.KymaEnvironmentKind,
						APIVersion: v1alpha1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": envID,
						},
					},
					Spec: v1alpha1.KymaEnvironmentSpec{
						SubaccountGuid: subaccountGUID,
						ForProvider: v1alpha1.KymaEnvironmentParameters{
							LandscapeLabel: &landscapeLabel,
							PlanName:       planName,
							Name:           &name,
							Parameters:     wantParameters,
						},
						CloudManagementRef: &xpv2.Reference{
							Name: cmName,
						},
						ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
							ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
							WriteConnectionSecretToReference: &xpv2.SecretReference{
								Name:      resourceName,
								Namespace: resources.DefaultSecretNamespace,
							},
						},
					},
				}),
		},
		{
			name: "missing subaccount GUID",
			kymaEnv: &KymaEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           name,
					LandscapeLabel: landscapeLabel,
					PlanName:       planName,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.KymaEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.KymaEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.KymaEnvironmentSpec{
							ForProvider: v1alpha1.KymaEnvironmentParameters{
								LandscapeLabel: &landscapeLabel,
								PlanName:       planName,
								Name:           &name,
								Parameters:     wantParameters,
							},
							CloudManagementRef: &xpv2.Reference{
								Name: cmName,
							},
							ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
								ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
								WriteConnectionSecretToReference: &xpv2.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				return rwc
			}(),
		},
		{
			name: "missing environment ID (external name)",
			kymaEnv: &KymaEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					Name:           name,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					PlanName:       planName,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.KymaEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.KymaEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1alpha1.KymaEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.KymaEnvironmentParameters{
								LandscapeLabel: &landscapeLabel,
								PlanName:       planName,
								Name:           &name,
								Parameters:     wantParameters,
							},
							CloudManagementRef: &xpv2.Reference{
								Name: cmName,
							},
							ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
								ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
								WriteConnectionSecretToReference: &xpv2.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnUndefinedExternalName)
				return rwc
			}(),
		},
		{
			name: "missing landscape label",
			kymaEnv: &KymaEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           name,
					SubaccountGUID: subaccountGUID,
					PlanName:       planName,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.KymaEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.KymaEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.KymaEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.KymaEnvironmentParameters{
								PlanName:   planName,
								Name:       &name,
								Parameters: wantParameters,
							},
							CloudManagementRef: &xpv2.Reference{
								Name: cmName,
							},
							ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
								ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
								WriteConnectionSecretToReference: &xpv2.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				return rwc
			}(),
		},
		{
			name: "missing name",
			kymaEnv: &KymaEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           "",
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					PlanName:       planName,
					Labels:         btpcli.Labels{},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				emptyName := ""
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.KymaEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.KymaEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.KymaEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.KymaEnvironmentParameters{
								PlanName:       planName,
								Name:           &emptyName,
								LandscapeLabel: &landscapeLabel,
								Parameters:     wantParameters,
							},
							CloudManagementRef: &xpv2.Reference{
								Name: cmName,
							},
							ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
								ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
								WriteConnectionSecretToReference: &xpv2.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnMissingKymaName)
				return rwc
			}(),
		},
		{
			name: "missing cloud management name",
			kymaEnv: &KymaEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           name,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					PlanName:       planName,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: "",
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.KymaEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.KymaEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.KymaEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.KymaEnvironmentParameters{
								LandscapeLabel: &landscapeLabel,
								PlanName:       planName,
								Name:           &name,
								Parameters:     wantParameters,
							},
							CloudManagementRef: &xpv2.Reference{
								Name: "",
							},
							ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
								ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
								WriteConnectionSecretToReference: &xpv2.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingCloudManagementName)
				return rwc
			}(),
		},
		{
			name: "all fields missing",
			kymaEnv: &KymaEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					Labels: btpcli.Labels{},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				emptyName := ""
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.KymaEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.KymaEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1alpha1.KymaEnvironmentSpec{
							ForProvider: v1alpha1.KymaEnvironmentParameters{
								Name:       &emptyName,
								Parameters: wantParameters,
							},
							CloudManagementRef: &xpv2.Reference{
								Name: "",
							},
							ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
								ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
								WriteConnectionSecretToReference: &xpv2.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingKymaName)
				rwc.AddComment(resources.WarnMissingKymaPlanName)
				rwc.AddComment(resources.WarnMissingCloudManagementName)
				return rwc
			}(),
		},
		{
			name: "comments inherited from original resource",
			kymaEnv: func() *KymaEnvironment {
				rwc := yaml.NewResourceWithComment(nil)
				rwc.AddComment("Existing comment from previous processing")
				return &KymaEnvironment{
					EnvironmentInstance: &btpcli.EnvironmentInstance{
						ID:             envID,
						Name:           name,
						SubaccountGUID: subaccountGUID,
						LandscapeLabel: landscapeLabel,
						PlanName:       planName,
					},
					ResourceWithComment: rwc,
					CloudManagementName: cmName,
				}
			}(),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.KymaEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.KymaEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.KymaEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.KymaEnvironmentParameters{
								LandscapeLabel: &landscapeLabel,
								PlanName:       planName,
								Name:           &name,
								Parameters:     wantParameters,
							},
							CloudManagementRef: &xpv2.Reference{
								Name: cmName,
							},
							ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
								ManagementPolicies: []xpv2.ManagementAction{xpv2.ManagementActionObserve},
								WriteConnectionSecretToReference: &xpv2.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment("Existing comment from previous processing")
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertKymaEnvResource(t.Context(), nil, tt.kymaEnv, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotKymaEnv, gotOk := result.Resource().(*v1alpha1.KymaEnvironment)
			wantKymaEnv, wantOk := tt.want.Resource().(*v1alpha1.KymaEnvironment)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.KymaEnvironment")

			// Final overall comparison.
			r.Equal(wantKymaEnv, gotKymaEnv)
		})
	}
}
