package cfenvironment

import (
	"testing"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func TestConvertCloudFoundryEnvResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Test data
	envID := "1234-5678-9abc-def0-1234-5678-9abc-def0"
	envName := "cf-environment-1"
	subaccountGUID := "sa-12345678-1234-1234-1234-123456789abc"
	landscapeLabel := "cf-eu10"
	orgName := "my-cf-org"
	cmName := "cloud-management-ref"
	resourceName := envName + "-" + subaccountGUID

	tests := []struct {
		name  string
		cfEnv *CloudFoundryEnvironment
		want  *yaml.ResourceWithComment
	}{
		{
			name: "all required fields present",
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           envName,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					Labels: btpcli.Labels{
						OrgName: orgName,
					},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.CloudFoundryEnvironment{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.CfEnvironmentKind,
						APIVersion: v1alpha1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": envID,
						},
					},
					Spec: v1alpha1.CfEnvironmentSpec{
						SubaccountGuid: subaccountGUID,
						ForProvider: v1alpha1.CfEnvironmentParameters{
							Landscape:       landscapeLabel,
							OrgName:         orgName,
							EnvironmentName: envName,
						},
						CloudManagementRef: &v1.Reference{
							Name: cmName,
						},
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							WriteConnectionSecretToReference: &v1.SecretReference{
								Name:      resourceName,
								Namespace: resources.DefaultSecretNamespace,
							},
						},
					},
				}),
		},
		{
			name: "missing subaccount GUID",
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           envName,
					LandscapeLabel: landscapeLabel,
					Labels: btpcli.Labels{
						OrgName: orgName,
					},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							ForProvider: v1alpha1.CfEnvironmentParameters{
								Landscape:       landscapeLabel,
								OrgName:         orgName,
								EnvironmentName: envName,
							},
							CloudManagementRef: &v1.Reference{
								Name: cmName,
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing environment ID (external name)",
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					Name:           envName,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					Labels: btpcli.Labels{
						OrgName: orgName,
					},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.CfEnvironmentParameters{
								Landscape:       landscapeLabel,
								OrgName:         orgName,
								EnvironmentName: envName,
							},
							CloudManagementRef: &v1.Reference{
								Name: cmName,
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedExternalName)
				return rwc
			}(),
		},
		{
			name: "missing landscape label",
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           envName,
					SubaccountGUID: subaccountGUID,
					Labels: btpcli.Labels{
						OrgName: orgName,
					},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.CfEnvironmentParameters{
								OrgName:         orgName,
								EnvironmentName: envName,
							},
							CloudManagementRef: &v1.Reference{
								Name: cmName,
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingLandscapeLabel)
				return rwc
			}(),
		},
		{
			name: "missing org name",
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           envName,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					Labels:         btpcli.Labels{},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.CfEnvironmentParameters{
								Landscape:       landscapeLabel,
								EnvironmentName: envName,
							},
							CloudManagementRef: &v1.Reference{
								Name: cmName,
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingOrgName)
				return rwc
			}(),
		},
		{
			name: "missing environment name",
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					Labels: btpcli.Labels{
						OrgName: orgName,
					},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: cmName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.CfEnvironmentParameters{
								Landscape: landscapeLabel,
								OrgName:   orgName,
							},
							CloudManagementRef: &v1.Reference{
								Name: cmName,
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnMissingEnvironmentName)
				return rwc
			}(),
		},
		{
			name: "missing cloud management name",
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					ID:             envID,
					Name:           envName,
					SubaccountGUID: subaccountGUID,
					LandscapeLabel: landscapeLabel,
					Labels: btpcli.Labels{
						OrgName: orgName,
					},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				CloudManagementName: "",
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.CfEnvironmentParameters{
								Landscape:       landscapeLabel,
								OrgName:         orgName,
								EnvironmentName: envName,
							},
							CloudManagementRef: &v1.Reference{
								Name: "",
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
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
			cfEnv: &CloudFoundryEnvironment{
				EnvironmentInstance: &btpcli.EnvironmentInstance{
					Labels: btpcli.Labels{},
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							ForProvider: v1alpha1.CfEnvironmentParameters{},
							CloudManagementRef: &v1.Reference{
								Name: "",
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingLandscapeLabel)
				rwc.AddComment(resources.WarnMissingOrgName)
				rwc.AddComment(resources.WarnMissingEnvironmentName)
				rwc.AddComment(resources.WarnMissingCloudManagementName)
				return rwc
			}(),
		},
		{
			name: "comments inherited from original resource",
			cfEnv: func() *CloudFoundryEnvironment {
				rwc := yaml.NewResourceWithComment(nil)
				rwc.AddComment("Existing comment from previous processing")
				return &CloudFoundryEnvironment{
					EnvironmentInstance: &btpcli.EnvironmentInstance{
						ID:             envID,
						Name:           envName,
						SubaccountGUID: subaccountGUID,
						LandscapeLabel: landscapeLabel,
						Labels: btpcli.Labels{
							OrgName: orgName,
						},
					},
					ResourceWithComment: rwc,
					CloudManagementName: cmName,
				}
			}(),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.CloudFoundryEnvironment{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.CfEnvironmentKind,
							APIVersion: v1alpha1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": envID,
							},
						},
						Spec: v1alpha1.CfEnvironmentSpec{
							SubaccountGuid: subaccountGUID,
							ForProvider: v1alpha1.CfEnvironmentParameters{
								Landscape:       landscapeLabel,
								OrgName:         orgName,
								EnvironmentName: envName,
							},
							CloudManagementRef: &v1.Reference{
								Name: cmName,
							},
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
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
			result := convertCloudFoundryEnvResource(t.Context(), nil, tt.cfEnv, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotCfEnv, gotOk := result.Resource().(*v1alpha1.CloudFoundryEnvironment)
			wantCfEnv, wantOk := tt.want.Resource().(*v1alpha1.CloudFoundryEnvironment)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.CloudFoundryEnvironment")

			// Final overall comparison.
			r.Equal(wantCfEnv, gotCfEnv)
		})
	}
}
