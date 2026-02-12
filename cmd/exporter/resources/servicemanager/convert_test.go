package servicemanager

import (
	"fmt"
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func TestConvertServiceManagerResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	instanceID := "987f6543-b21a-43c9-b321-876543210000"
	bindingID := "456a7890-c12d-45e6-b987-654321098765"
	instanceName := "service-manager-instance"
	subAccountGuid := "123e4567-e89b-12d3-a456-426614174000"
	smExternalName := fmt.Sprintf("%s/%s", instanceID, bindingID)
	siExternalName := fmt.Sprintf("%s,%s", subAccountGuid, instanceID)
	resourceName := fmt.Sprintf("%s-%s", instanceName, subAccountGuid)

	tests := []struct {
		name string
		si   *ServiceInstance
		want *yaml.ResourceWithComment
	}{
		{
			name: "all required fields present",
			si: &ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        OfferingServiceManager,
				BindingID:           bindingID,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: yaml.NewResourceWithComment(
				&v1beta1.ServiceManager{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1beta1.ServiceManagerKind,
						APIVersion: v1beta1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": smExternalName,
						},
					},
					Spec: v1beta1.ServiceManagerSpec{
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{
								v1.ManagementActionObserve,
							},
							WriteConnectionSecretToReference: &v1.SecretReference{
								Name:      resourceName,
								Namespace: DefaultSecretNamespace,
							},
						},
						ForProvider: v1beta1.ServiceManagerParameters{
							SubaccountGuid: subAccountGuid,
						},
					},
				}),
		},
		{
			name: "not a service manager",
			si: &ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        "not-a-service-manager-offering",
				BindingID:           bindingID,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.ServiceManager{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.ServiceManagerKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": siExternalName,
							},
						},
						Spec: v1beta1.ServiceManagerSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.ServiceManagerParameters{
								SubaccountGuid: subAccountGuid,
							},
						},
					})
				rwc.AddComment(resources.WarnNotServiceManager)
				return rwc
			}(),
		},
		{
			name: "missing instance id",
			si: &ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           "",
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        OfferingServiceManager,
				BindingID:           bindingID,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.ServiceManager{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.ServiceManagerKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1beta1.ServiceManagerSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.ServiceManagerParameters{
								SubaccountGuid: subAccountGuid,
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingInstanceId)
				return rwc
			}(),
		},
		{
			name: "missing binding id",
			si: &ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       true,
				},
				OfferingName:        OfferingServiceManager,
				BindingID:           "",
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.ServiceManager{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.ServiceManagerKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1beta1.ServiceManagerSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.ServiceManagerParameters{
								SubaccountGuid: subAccountGuid,
							},
						},
					})
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingBindingId)
				return rwc
			}(),
		},
		{
			name: "missing subaccount guid",
			si: &ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: "",
					Usable:       true,
				},
				OfferingName:        OfferingServiceManager,
				BindingID:           bindingID,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.ServiceManager{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.ServiceManagerKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": fmt.Sprintf("%s/%s", instanceID, bindingID),
							},
						},
						Spec: v1beta1.ServiceManagerSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.ServiceManagerParameters{
								SubaccountGuid: "",
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "service manager not usable",
			si: &ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           instanceID,
					Name:         instanceName,
					SubaccountID: subAccountGuid,
					Usable:       false,
				},
				OfferingName:        OfferingServiceManager,
				BindingID:           bindingID,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.ServiceManager{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.ServiceManagerKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": smExternalName,
							},
						},
						Spec: v1beta1.ServiceManagerSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resourceName,
									Namespace: DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.ServiceManagerParameters{
								SubaccountGuid: subAccountGuid,
							},
						},
					})
				rwc.AddComment(resources.WarnServiceInstanceNotUsable)
				return rwc
			}(),
		},
		{
			name: "multiple missing fields",
			si: &ServiceInstance{
				ServiceInstance: &btpcli.ServiceInstance{
					ID:           "",
					Name:         "",
					SubaccountID: "",
					Usable:       false,
				},
				OfferingName:        OfferingServiceManager,
				PlanName:            "",
				BindingID:           "",
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.ServiceManager{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.ServiceManagerKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UndefinedExternalName,
							},
						},
						Spec: v1beta1.ServiceManagerSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.ServiceManagerParameters{
								SubaccountGuid: "",
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				rwc.AddComment(resources.WarnUndefinedExternalName)
				rwc.AddComment(resources.WarnMissingInstanceId)
				rwc.AddComment(resources.WarnMissingBindingId)
				rwc.AddComment(resources.WarnServiceInstanceNotUsable)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertServiceManagerResource(t.Context(), nil, tt.si, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotSM, gotOk := result.Resource().(*v1beta1.ServiceManager)
			wantSM, wantOk := tt.want.Resource().(*v1beta1.ServiceManager)
			r.True(gotOk && wantOk, "expected resource type to be *v1beta1.ServiceManager")

			// Overall comparison.
			r.Equal(wantSM, gotSM)
		})
	}
}

func TestDefaultServiceManagerResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	subAccountGuid := "123e4567-e89b-12d3-a456-426614174000"
	resourceName := fmt.Sprintf("%s-%s", DefaultNamePrefix, subAccountGuid)

	tests := []struct {
		name         string
		subaccountID string
		want         *yaml.ResourceWithComment
	}{
		{
			name:         "with subaccount id",
			subaccountID: subAccountGuid,
			want: yaml.NewResourceWithComment(
				&v1beta1.ServiceManager{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1beta1.ServiceManagerKind,
						APIVersion: v1beta1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: v1beta1.ServiceManagerSpec{
						ResourceSpec: v1.ResourceSpec{
							WriteConnectionSecretToReference: &v1.SecretReference{
								Name:      resourceName,
								Namespace: DefaultSecretNamespace,
							},
						},
						ForProvider: v1beta1.ServiceManagerParameters{
							SubaccountGuid: subAccountGuid,
						},
					},
				}),
		},
		{
			name:         "empty subaccount id",
			subaccountID: "",
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1beta1.ServiceManager{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1beta1.ServiceManagerKind,
							APIVersion: v1beta1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1beta1.ServiceManagerSpec{
							ResourceSpec: v1.ResourceSpec{
								WriteConnectionSecretToReference: &v1.SecretReference{
									Name:      resources.UndefinedName,
									Namespace: DefaultSecretNamespace,
								},
							},
							ForProvider: v1beta1.ServiceManagerParameters{
								SubaccountGuid: "",
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaultServiceManagerResource(t.Context(), nil, tt.subaccountID, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotSM, gotOk := result.Resource().(*v1beta1.ServiceManager)
			wantSM, wantOk := tt.want.Resource().(*v1beta1.ServiceManager)
			r.True(gotOk && wantOk, "expected resource type to be *v1beta1.ServiceManager")

			// Overall comparison.
			r.Equal(wantSM, gotSM)
		})
	}
}
