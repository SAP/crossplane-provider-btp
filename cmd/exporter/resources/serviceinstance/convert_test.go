package serviceinstance

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
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstancebase"
)

func TestConvertServiceInstanceResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	instanceID := "987f6543-b21a-43c9-b321-876543210000"
	instanceName := "my-instance"
	serviceName := "test-service"
	planName := "standard"
	subAccountGuid := "123e4567-e89b-12d3-a456-426614174000"
	externalName := fmt.Sprintf("%s,%s", subAccountGuid, instanceID)
	smName := "service-manager-resource"
	resourceName := fmt.Sprintf("%s-%s", instanceName, subAccountGuid)

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
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        serviceName,
				PlanName:            planName,
				ServiceManagerName:  smName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.ServiceInstance{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.ServiceInstanceKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": externalName,
						},
					},
					Spec: v1alpha1.ServiceInstanceSpec{
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{
								v1.ManagementActionObserve,
							},
						},
						ForProvider: v1alpha1.ServiceInstanceParameters{
							Name:         instanceName,
							OfferingName: serviceName,
							PlanName:     planName,
							SubaccountID: &subAccountGuid,
							ServiceManagerRef: &v1.Reference{
								Name: smName,
							},
						},
					},
				}),
		},
		{
			name: "missing service name",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        "",
				PlanName:            planName,
				ServiceManagerName:  smName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceInstance{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceInstanceKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1alpha1.ServiceInstanceSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceInstanceParameters{
								Name:         instanceName,
								PlanName:     planName,
								SubaccountID: &subAccountGuid,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServiceName)
				return rwc
			}(),
		},
		{
			name: "missing service plan name",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        serviceName,
				PlanName:            "",
				ServiceManagerName:  smName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceInstance{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceInstanceKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1alpha1.ServiceInstanceSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceInstanceParameters{
								Name:         instanceName,
								OfferingName: serviceName,
								SubaccountID: &subAccountGuid,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServicePlanName)
				return rwc
			}(),
		},
		{
			name: "missing subaccount guid",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: "",
					Usable:       true,
				},
				OfferingName:        serviceName,
				PlanName:            planName,
				ServiceManagerName:  smName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				empty := ""
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceInstance{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceInstanceKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1alpha1.ServiceInstanceSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceInstanceParameters{
								Name:         instanceName,
								OfferingName: serviceName,
								PlanName:     planName,
								SubaccountID: &empty,
								ServiceManagerRef: &v1.Reference{
									Name: smName,
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnUndefinedExternalName)
				return rwc
			}(),
		},
		{
			name: "missing instance id",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           "",
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        serviceName,
				PlanName:            planName,
				ServiceManagerName:  smName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceInstance{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceInstanceKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1alpha1.ServiceInstanceSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceInstanceParameters{
								Name:         instanceName,
								OfferingName: serviceName,
								PlanName:     planName,
								SubaccountID: &subAccountGuid,
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
			name: "service instance not usable",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       false,
				},
				OfferingName:        serviceName,
				PlanName:            planName,
				ServiceManagerName:  smName,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceInstance{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceInstanceKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1alpha1.ServiceInstanceSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceInstanceParameters{
								Name:         instanceName,
								OfferingName: serviceName,
								PlanName:     planName,
								SubaccountID: &subAccountGuid,
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
			name: "missing service manager name",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        serviceName,
				PlanName:            planName,
				ServiceManagerName:  "",
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceInstance{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceInstanceKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": externalName,
							},
						},
						Spec: v1alpha1.ServiceInstanceSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceInstanceParameters{
								Name:         instanceName,
								OfferingName: serviceName,
								PlanName:     planName,
								SubaccountID: &subAccountGuid,
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
			name: "multiple missing fields",
			si: &serviceinstancebase.ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           "",
					Name:         "",
					SubaccountID: "",
					Usable:       false,
				},
				OfferingName:        "",
				PlanName:            "",
				ServiceManagerName:  "",
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				empty := ""
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.ServiceInstance{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.ServiceInstanceKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1alpha1.ServiceInstanceSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.ServiceInstanceParameters{
								Name:         "",
								SubaccountID: &empty,
								ServiceManagerRef: &v1.Reference{
									Name: "",
								},
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServiceName)
				rwc.AddComment(resources.WarnMissingServicePlanName)
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingInstanceId)
				rwc.AddComment(resources.WarnServiceInstanceNotUsable)
				rwc.AddComment(resources.WarnMissingServiceManagerName)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertServiceInstanceResource(t.Context(), nil, tt.si, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotSI, gotOk := result.Resource().(*v1alpha1.ServiceInstance)
			wantSI, wantOk := tt.want.Resource().(*v1alpha1.ServiceInstance)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.ServiceInstance")

			// Overall comparison.
			r.Equal(wantSI, gotSI)
		})
	}
}
