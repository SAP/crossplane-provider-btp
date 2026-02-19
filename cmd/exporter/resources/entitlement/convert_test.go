package entitlement

import (
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func Test_getEnableOk(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	tests := []struct {
		name                    string
		amount                  float64
		parentAmount            float64
		parentRemainingAmount   *float64
		unlimitedAmountAssigned bool
		wantEnable              bool
	}{
		{
			name:                    "global unlimited quota",
			amount:                  amountUnlimited,
			parentAmount:            amountUnlimited,
			unlimitedAmountAssigned: true,
			wantEnable:              true,
		},
		{
			name:                  "assigned, parent not getting less",
			amount:                1,
			parentAmount:          1,
			parentRemainingAmount: float64Ptr(1),
			wantEnable:            true,
		},
		{
			name:                  "assigned, parent getting less",
			amount:                3,
			parentAmount:          5,
			parentRemainingAmount: float64Ptr(2),
			wantEnable:            false,
		},
		{
			name:       "no relevant fields set",
			wantEnable: false,
		},
		{
			name:                    "unlimitedAmountAssigned false",
			amount:                  amountUnlimited,
			parentAmount:            amountUnlimited,
			unlimitedAmountAssigned: false,
			wantEnable:              false,
		},
		{
			name:                    "unlimitedAmountAssigned true, but amount not unlimited",
			amount:                  1,
			parentAmount:            1,
			unlimitedAmountAssigned: true,
			wantEnable:              false,
		},
		{
			name:                  "amount zero",
			amount:                0,
			parentAmount:          0,
			parentRemainingAmount: float64Ptr(0),
			wantEnable:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &entitlement{
				assignment: &btpcli.AssignmentInfo{
					Amount:                  tt.amount,
					ParentAmount:            tt.parentAmount,
					ParentRemainingAmount:   tt.parentRemainingAmount,
					UnlimitedAmountAssigned: tt.unlimitedAmountAssigned,
				},
			}
			r.Equal(tt.wantEnable, e.isEnable())
		})
	}
}

func TestConvertEntitlementResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	svcName := "test-service"
	planName := "standard"
	planId := svcName + "-" + planName
	subAccountUuid := "123e4567-e89b-12d3-a456-426614174000"
	typeSubaccount := "SUBACCOUNT"
	typeDirectory := "DIRECTORY"
	var amount float64 = 1
	var parentAmount float64 = 10
	var parentRemainingAmount float64 = 5
	resourceName := planId + "-" + subAccountUuid

	tests := []struct {
		name string
		ent  *entitlement
		want *yaml.ResourceWithComment
	}{
		{
			name: "standard case for amount",
			ent: &entitlement{
				serviceName: svcName,
				planName:    planName,
				assignment: &btpcli.AssignmentInfo{
					EntityID:                subAccountUuid,
					EntityType:              typeSubaccount,
					Amount:                  amount,
					ParentAmount:            parentAmount,
					ParentRemainingAmount:   float64Ptr(parentRemainingAmount),
					UnlimitedAmountAssigned: false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.Entitlement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.EntitlementKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: v1alpha1.EntitlementSpec{
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{
								v1.ManagementActionObserve,
							},
						},
						ForProvider: v1alpha1.EntitlementParameters{
							ServicePlanName: planName,
							ServiceName:     svcName,
							SubaccountGuid:  subAccountUuid,
							Amount:          intPtr(int(amount)),
						},
					},
				}),
		},
		{
			name: "standard case for enable",
			ent: &entitlement{
				serviceName: svcName,
				planName:    planName,
				assignment: &btpcli.AssignmentInfo{
					EntityID:                subAccountUuid,
					EntityType:              typeSubaccount,
					Amount:                  amount,
					ParentAmount:            amount,
					ParentRemainingAmount:   float64Ptr(amount),
					UnlimitedAmountAssigned: false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.Entitlement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.EntitlementKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: v1alpha1.EntitlementSpec{
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{
								v1.ManagementActionObserve,
							},
						},
						ForProvider: v1alpha1.EntitlementParameters{
							ServicePlanName: planName,
							ServiceName:     svcName,
							SubaccountGuid:  subAccountUuid,
							Enable:          boolPtr(true),
						},
					},
				}),
		},
		{
			name: "enable if unlimited",
			ent: &entitlement{
				serviceName: svcName,
				planName:    planName,
				assignment: &btpcli.AssignmentInfo{
					EntityID:                subAccountUuid,
					EntityType:              typeSubaccount,
					Amount:                  amountUnlimited,
					ParentAmount:            amountUnlimited,
					UnlimitedAmountAssigned: true,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.Entitlement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.EntitlementKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: v1alpha1.EntitlementSpec{
						ResourceSpec: v1.ResourceSpec{
							ManagementPolicies: []v1.ManagementAction{
								v1.ManagementActionObserve,
							},
						},
						ForProvider: v1alpha1.EntitlementParameters{
							ServicePlanName: planName,
							ServiceName:     svcName,
							SubaccountGuid:  subAccountUuid,
							Enable:          boolPtr(true),
						},
					},
				}),
		},
		{
			name: "missing service name",
			ent: &entitlement{
				planName: planName,
				assignment: &btpcli.AssignmentInfo{
					EntityID:                subAccountUuid,
					EntityType:              typeSubaccount,
					Amount:                  amount,
					ParentAmount:            parentAmount,
					ParentRemainingAmount:   float64Ptr(parentRemainingAmount),
					UnlimitedAmountAssigned: false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.EntitlementParameters{
								ServicePlanName: planName,
								SubaccountGuid:  subAccountUuid,
								Amount:          intPtr(int(amount)),
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServiceName)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing service plan name",
			ent: &entitlement{
				serviceName: svcName,
				assignment: &btpcli.AssignmentInfo{
					EntityID:                subAccountUuid,
					EntityType:              typeSubaccount,
					Amount:                  amount,
					ParentAmount:            parentAmount,
					ParentRemainingAmount:   float64Ptr(parentRemainingAmount),
					UnlimitedAmountAssigned: false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.EntitlementParameters{
								ServiceName:    svcName,
								SubaccountGuid: subAccountUuid,
								Amount:         intPtr(int(amount)),
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServicePlanName)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing subaccount ID",
			ent: &entitlement{
				serviceName: svcName,
				planName:    planName,
				assignment: &btpcli.AssignmentInfo{
					EntityType:              typeSubaccount,
					Amount:                  amount,
					ParentAmount:            parentAmount,
					ParentRemainingAmount:   float64Ptr(parentRemainingAmount),
					UnlimitedAmountAssigned: false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.EntitlementParameters{
								ServiceName:     svcName,
								ServicePlanName: planName,
								Amount:          intPtr(int(amount)),
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "unsupported entity type",
			ent: &entitlement{
				serviceName: svcName,
				planName:    planName,
				assignment: &btpcli.AssignmentInfo{
					EntityID:                subAccountUuid,
					EntityType:              typeDirectory,
					Amount:                  amount,
					ParentAmount:            parentAmount,
					ParentRemainingAmount:   float64Ptr(parentRemainingAmount),
					UnlimitedAmountAssigned: false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ResourceSpec: v1.ResourceSpec{
								ManagementPolicies: []v1.ManagementAction{
									v1.ManagementActionObserve,
								},
							},
							ForProvider: v1alpha1.EntitlementParameters{
								ServiceName:     svcName,
								ServicePlanName: planName,
								SubaccountGuid:  subAccountUuid,
								Amount:          intPtr(int(amount)),
							},
						},
					})
				rwc.AddComment(resources.WarnUnsupportedEntityType + ", but got: 'DIRECTORY'")
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertEntitlementResource(t.Context(), nil, tt.ent, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotEntitlement, gotOk := result.Resource().(*v1alpha1.Entitlement)
			wantEntitlement, wantOk := tt.want.Resource().(*v1alpha1.Entitlement)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.Entitlement")

			// Overall comparison.
			r.Equal(wantEntitlement, gotEntitlement)
		})
	}
}

func TestConvertDefaultEntitlementResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	svcName := "test-service"
	planName := "standard"
	planId := svcName + "-" + planName
	subAccountUuid := "123e4567-e89b-12d3-a456-426614174000"

	var amount float64 = 1
	var parentAmount float64 = 10
	var parentRemainingAmount float64 = 5
	resourceName := planId + "-" + subAccountUuid

	tests := []struct {
		name string
		ent  *entitlement
		want *yaml.ResourceWithComment
	}{
		{
			name: "standard case",
			ent:  defaultEntitlement(subAccountUuid, svcName, planName),
			want: yaml.NewResourceWithComment(
				&v1alpha1.Entitlement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.EntitlementKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							ServicePlanName: planName,
							ServiceName:     svcName,
							SubaccountGuid:  subAccountUuid,
							Enable:          boolPtr(true),
						},
					},
				}),
		},
		{
			name: "amount not supported",
			ent: &entitlement{
				serviceName: svcName,
				planName:    planName,
				assignment: &btpcli.AssignmentInfo{
					EntityID:                subAccountUuid,
					Amount:                  amount,
					ParentAmount:            parentAmount,
					ParentRemainingAmount:   float64Ptr(parentRemainingAmount),
					UnlimitedAmountAssigned: false,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resourceName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ForProvider: v1alpha1.EntitlementParameters{
								ServicePlanName: planName,
								ServiceName:     svcName,
								SubaccountGuid:  subAccountUuid,
							},
						},
					})
				rwc.AddComment(resources.WarnDefaultEntitlementEnableFalse)
				return rwc
			}(),
		},
		{
			name: "missing service name",
			ent:  defaultEntitlement(subAccountUuid, "", planName),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ForProvider: v1alpha1.EntitlementParameters{
								ServicePlanName: planName,
								SubaccountGuid:  subAccountUuid,
								Enable:          boolPtr(true),
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServiceName)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing service plan name",
			ent:  defaultEntitlement(subAccountUuid, svcName, ""),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ForProvider: v1alpha1.EntitlementParameters{
								ServiceName:    svcName,
								SubaccountGuid: subAccountUuid,
								Enable:         boolPtr(true),
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServicePlanName)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing subaccount ID",
			ent:  defaultEntitlement("", svcName, planName),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ForProvider: v1alpha1.EntitlementParameters{
								ServiceName:     svcName,
								ServicePlanName: planName,
								Enable:          boolPtr(true),
							},
						},
					})
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "all required fields missing",
			ent:  defaultEntitlement("", "", ""),
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
						},
						Spec: v1alpha1.EntitlementSpec{
							ForProvider: v1alpha1.EntitlementParameters{
								Enable: boolPtr(true),
							},
						},
					})
				rwc.AddComment(resources.WarnMissingServiceName)
				rwc.AddComment(resources.WarnMissingServicePlanName)
				rwc.AddComment(resources.WarnMissingSubaccountGuid)
				rwc.AddComment(resources.WarnUndefinedResourceName)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertDefaultEntitlementResource(t.Context(), nil, tt.ent, nil, false)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotEntitlement, gotOk := result.Resource().(*v1alpha1.Entitlement)
			wantEntitlement, wantOk := tt.want.Resource().(*v1alpha1.Entitlement)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.Entitlement")

			// Overall comparison.
			r.Equal(wantEntitlement, gotEntitlement)
		})
	}
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}
