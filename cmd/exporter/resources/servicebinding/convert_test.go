package servicebinding

import (
	"fmt"
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicebindingbase"
)

func TestConvertServiceBindingResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	bindingID := "aabbccdd-1234-5678-abcd-aabbccddeeff"
	bindingName := "my-binding"
	subaccountID := "123e4567-e89b-12d3-a456-426614174000"
	instanceID := "987f6543-b21a-43c9-b321-876543210000"
	instanceName := "my-instance-resource"
	resourceName := fmt.Sprintf("%s-%s", bindingName, instanceID)

	tests := []struct {
		name string
		sb   *servicebindingbase.ServiceBinding
		want *yaml.ResourceWithComment
	}{
		{
			name: "all required fields present",
			sb: &servicebindingbase.ServiceBinding{
				ServiceBinding: &btpcli.ServiceBinding{
					ID:                bindingID,
					Name:              bindingName,
					SubaccountID:      subaccountID,
					ServiceInstanceID: instanceID,
				},
				ServiceInstanceName: instanceName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.ServiceBinding{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.ServiceBindingKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": bindingID,
						},
					},
					Spec: v1alpha1.ServiceBindingSpec{
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{
								v1.ManagementActionObserve,
							},
						},
						ForProvider: v1alpha1.ServiceBindingParameters{
							Name:              bindingName,
							SubaccountID:      &subaccountID,
							ServiceInstanceID: &instanceID,
							ServiceInstanceRef: &v1.Reference{
								Name: instanceName,
							},
						},
					},
				}),
		},
		{
			name: "missing binding name",
			sb: &servicebindingbase.ServiceBinding{
				ServiceBinding: &btpcli.ServiceBinding{
					ID:                bindingID,
					Name:              "",
					SubaccountID:      subaccountID,
					ServiceInstanceID: instanceID,
				},
				ServiceInstanceName: instanceName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceBinding{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceBindingKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": bindingID,
							},
						},
						Spec: v1alpha1.ServiceBindingSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceBindingParameters{
								Name:              "",
								SubaccountID:      &subaccountID,
								ServiceInstanceID: &instanceID,
								ServiceInstanceRef: &v1.Reference{
									Name: instanceName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingBindingName)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing subaccount id",
			sb: &servicebindingbase.ServiceBinding{
				ServiceBinding: &btpcli.ServiceBinding{
					ID:                bindingID,
					Name:              bindingName,
					SubaccountID:      "",
					ServiceInstanceID: instanceID,
				},
				ServiceInstanceName: instanceName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				empty := ""
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceBinding{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceBindingKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": bindingID,
							},
						},
						Spec: v1alpha1.ServiceBindingSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceBindingParameters{
								Name:              bindingName,
								SubaccountID:      &empty,
								ServiceInstanceID: &instanceID,
								ServiceInstanceRef: &v1.Reference{
									Name: instanceName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				return rwc
			}(),
		},
		{
			name: "missing service instance id",
			sb: &servicebindingbase.ServiceBinding{
				ServiceBinding: &btpcli.ServiceBinding{
					ID:                bindingID,
					Name:              bindingName,
					SubaccountID:      subaccountID,
					ServiceInstanceID: "",
				},
				ServiceInstanceName: instanceName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				empty := ""
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceBinding{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceBindingKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": bindingID,
							},
						},
						Spec: v1alpha1.ServiceBindingSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceBindingParameters{
								Name:              bindingName,
								SubaccountID:      &subaccountID,
								ServiceInstanceID: &empty,
								ServiceInstanceRef: &v1.Reference{
									Name: instanceName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingInstanceId)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing service instance name",
			sb: &servicebindingbase.ServiceBinding{
				ServiceBinding: &btpcli.ServiceBinding{
					ID:                bindingID,
					Name:              bindingName,
					SubaccountID:      subaccountID,
					ServiceInstanceID: instanceID,
				},
				ServiceInstanceName: "",
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceBinding{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceBindingKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": bindingID,
							},
						},
						Spec: v1alpha1.ServiceBindingSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceBindingParameters{
								Name:              bindingName,
								SubaccountID:      &subaccountID,
								ServiceInstanceID: &instanceID,
								ServiceInstanceRef: &v1.Reference{
									Name: "",
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingInstanceName)
				return rwc
			}(),
		},
		{
			name: "missing binding id (empty external name)",
			sb: &servicebindingbase.ServiceBinding{
				ServiceBinding: &btpcli.ServiceBinding{
					ID:                "",
					Name:              bindingName,
					SubaccountID:      subaccountID,
					ServiceInstanceID: instanceID,
				},
				ServiceInstanceName: instanceName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceBinding{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceBindingKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": "",
							},
						},
						Spec: v1alpha1.ServiceBindingSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceBindingParameters{
								Name:              bindingName,
								SubaccountID:      &subaccountID,
								ServiceInstanceID: &instanceID,
								ServiceInstanceRef: &v1.Reference{
									Name: instanceName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingExternalName)
				return rwc
			}(),
		},
		{
			name: "multiple missing fields",
			sb: &servicebindingbase.ServiceBinding{
				ServiceBinding: &btpcli.ServiceBinding{
					ID:                "",
					Name:              "",
					SubaccountID:      "",
					ServiceInstanceID: "",
				},
				ServiceInstanceName: "",
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				empty := ""
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceBinding{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceBindingKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": "",
							},
						},
						Spec: v1alpha1.ServiceBindingSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceBindingParameters{
								Name:              "",
								SubaccountID:      &empty,
								ServiceInstanceID: &empty,
								ServiceInstanceRef: &v1.Reference{
									Name: "",
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingBindingName)
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnMissingInstanceId)
				rwc.AddComment(resources.WarnMissingInstanceName)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnMissingExternalName)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertServiceBindingResource(t.Context(), nil, tt.sb, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotRWC := result.(*yaml.ResourceWithComment)
			gotComment, gotHasComment := gotRWC.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotSB, gotOk := gotRWC.Resource().(*v1alpha1.ServiceBinding)
			wantSB, wantOk := tt.want.Resource().(*v1alpha1.ServiceBinding)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.ServiceBinding")

			// Overall comparison.
			r.Equal(wantSB, gotSB)
		})
	}
}
