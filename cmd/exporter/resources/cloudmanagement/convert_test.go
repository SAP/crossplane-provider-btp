package cloudmanagement

import (
	"testing"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstancebase"
)

func TestConvertCloudManagementResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Test data
	instanceID := "instance-12345678-1234-1234-1234-123456789abc"
	bindingID := "binding-12345678-1234-1234-1234-123456789abc"
	instanceName := "cis-instance"
	subaccountID := "sa-12345678-1234-1234-1234-123456789abc"
	smName := "service-manager-ref"
	resourceName := instanceName + "-" + subaccountID
	externalName := instanceID + "/" + bindingID

	tests := []struct {
		name string
		si   *serviceinstancebase.ServiceInstance
		want *yaml.ResourceWithComment
	}{
		{
			name: "all required fields present",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subaccountID,
					Usable:       true,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				OfferingName:        serviceinstancebase.CloudManagementOffering,
				PlanName:            serviceinstancebase.CloudManagementPlan,
				BindingID:           bindingID,
				ServiceManagerName:  smName,
			},
			want: yaml.NewResourceWithComment(
				&v1beta1.CloudManagement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1beta1.CloudManagementKind,
						APIVersion: v1beta1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": externalName,
						},
					},
					Spec: v1beta1.CloudManagementSpec{
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							WriteConnectionSecretToReference: &v1.SecretReference{
								Name:      resourceName,
								Namespace: resources.DefaultSecretNamespace,
							},
						},
						ForProvider: v1beta1.CloudManagementParameters{
							SubaccountGuid: subaccountID,
							ServiceManagerRef: &v1.Reference{
								Name: smName,
							},
						},
					},
				}),
		},
		{
			name: "not cloud management instance",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subaccountID,
					Usable:       true,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				OfferingName:        "other-offering",
				PlanName:            "other-plan",
				BindingID:           bindingID,
				ServiceManagerName:  smName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": subaccountID + "," + instanceID, // service instance format
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								SubaccountGuid: subaccountID,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnNotCloudManagement)
				return rwc
			}(),
		},
		{
			name: "missing subaccount ID",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:     instanceID,
					Name:   instanceName,
					Usable: true,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				OfferingName:        serviceinstancebase.CloudManagementOffering,
				PlanName:            serviceinstancebase.CloudManagementPlan,
				BindingID:           bindingID,
				ServiceManagerName:  smName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								ServiceManagerRef: &v1.Reference{
									Name: smName,
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
			name: "missing instance ID",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					Name:         instanceName,
					SubaccountID: subaccountID,
					Usable:       true,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				OfferingName:        serviceinstancebase.CloudManagementOffering,
				PlanName:            serviceinstancebase.CloudManagementPlan,
				BindingID:           bindingID,
				ServiceManagerName:  smName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								SubaccountGuid: subaccountID,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingInstanceId)
				return rwc
			}(),
		},
		{
			name: "missing binding ID",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subaccountID,
					Usable:       true,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				OfferingName:        serviceinstancebase.CloudManagementOffering,
				PlanName:            serviceinstancebase.CloudManagementPlan,
				ServiceManagerName:  smName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								SubaccountGuid: subaccountID,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingBindingId)
				return rwc
			}(),
		},
		{
			name: "missing service manager name",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subaccountID,
					Usable:       true,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				OfferingName:        serviceinstancebase.CloudManagementOffering,
				PlanName:            serviceinstancebase.CloudManagementPlan,
				BindingID:           bindingID,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								SubaccountGuid: subaccountID,
								ServiceManagerRef: &v1.Reference{
									Name: "",
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServiceManagerName)
				return rwc
			}(),
		},
		{
			name: "instance not usable",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subaccountID,
					Usable:       false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				OfferingName:        serviceinstancebase.CloudManagementOffering,
				PlanName:            serviceinstancebase.CloudManagementPlan,
				BindingID:           bindingID,
				ServiceManagerName:  smName,
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								SubaccountGuid: subaccountID,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnServiceInstanceNotUsable)
				return rwc
			}(),
		},
		{
			name: "all fields missing",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance:     &btpcli.ServiceInstance{},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								ServiceManagerRef: &v1.Reference{
									Name: "",
								},
							},
						},
					})
				rwc.AddComment(resources.WarnNotCloudManagement)
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingInstanceId)
				rwc.AddComment(resources.WarnMissingBindingId)
				rwc.AddComment(resources.WarnServiceInstanceNotUsable)
				rwc.AddComment(resources.WarnMissingServiceManagerName)
				return rwc
			}(),
		},
		{
			name: "comments inherited from original resource",
			si: func() *serviceinstancebase.ServiceInstance {
				rwc := yaml.NewResourceWithComment(nil)
				rwc.AddComment("Existing comment from previous processing")
				return &serviceinstancebase.ServiceInstance{
					ServiceInstance: &btpcli.ServiceInstance{
						ID:           instanceID,
						Name:         instanceName,
						SubaccountID: subaccountID,
						Usable:       true,
					},
					ResourceWithComment: rwc,
					OfferingName:        serviceinstancebase.CloudManagementOffering,
					PlanName:            serviceinstancebase.CloudManagementPlan,
					BindingID:           bindingID,
					ServiceManagerName:  smName,
				}
			}(),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								SubaccountGuid: subaccountID,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
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
			result := convertCloudManagementResource(t.Context(), nil, tt.si, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotCM, gotOk := result.Resource().(*v1beta1.CloudManagement)
			wantCM, wantOk := tt.want.Resource().(*v1beta1.CloudManagement)
			r.True(gotOk && wantOk, "expected resource type to be *v1beta1.CloudManagement")

			// Final overall comparison.
			r.Equal(wantCM, gotCM)
		})
	}
}

func TestConvertDefaultCloudManagementResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Test data
	subaccountID := "123e4567-e89b-12d3-a456-426614174000"
	smName := "service-manager-ref"
	resourceName := defaultNamePrefix + "-" + subaccountID

	tests := []struct {
		name string
		si   *serviceinstancebase.ServiceInstance
		want *yaml.ResourceWithComment
	}{
		{
			name: "standard case",
			si: func() *serviceinstancebase.ServiceInstance {
				cm := defaultCloudManagement(subaccountID)
				cm.ServiceManagerName = smName
				return cm
			}(),
			want: yaml.NewResourceWithComment(
				&v1beta1.CloudManagement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1beta1.CloudManagementKind,
						APIVersion: v1beta1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: v1beta1.CloudManagementSpec{
						ResourceSpec: v1.ResourceSpec{
							WriteConnectionSecretToReference: &v1.SecretReference{
								Name:      resourceName,
								Namespace: resources.DefaultSecretNamespace,
							},
						},
						ForProvider: v1beta1.CloudManagementParameters{
							SubaccountGuid: subaccountID,
							ServiceManagerRef: &v1.Reference{
								Name: smName,
							},
						},
					},
				}),
		},
		{
			name: "missing subaccount ID",
			si: func() *serviceinstancebase.ServiceInstance {
				cm := defaultCloudManagement("")
				cm.ServiceManagerName = smName
				return cm
			}(),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								ServiceManagerRef: &v1.Reference{
									Name: smName,
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
			name: "missing service manager name",
			si:   defaultCloudManagement(subaccountID),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.CloudManagement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.CloudManagementKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
						},
						Spec: v1beta1.CloudManagementSpec{
							ResourceSpec: v1.ResourceSpec{
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: resources.DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.CloudManagementParameters{
								SubaccountGuid: subaccountID,
								ServiceManagerRef: &v1.Reference{
									Name: "",
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServiceManagerName)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertDefaultCloudManagementResource(t.Context(), nil, tt.si, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotCM, gotOk := result.Resource().(*v1beta1.CloudManagement)
			wantCM, wantOk := tt.want.Resource().(*v1beta1.CloudManagement)
			r.True(gotOk && wantOk, "expected resource type to be *v1beta1.CloudManagement")

			// Final overall comparison.
			r.Equal(wantCM, gotCM)
		})
	}
}
