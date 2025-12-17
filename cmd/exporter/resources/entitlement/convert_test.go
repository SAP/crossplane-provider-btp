package entitlement

import (
	"testing"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/yaml"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	openapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

func Test_getEnableOk(t *testing.T) {
	r := require.New(t)

	tests := []struct {
		name                    string
		amount                  *float32
		parentAmount            *float32
		parentRemainingAmount   *float32
		unlimitedAmountAssigned *bool
		wantEnable              bool
		wantOk                  bool
	}{
		{
			name:                    "global unlimited quota",
			amount:                  float32Ptr(amountUnlimited),
			parentAmount:            float32Ptr(amountUnlimited),
			unlimitedAmountAssigned: boolPtr(true),
			wantEnable:              true,
			wantOk:                  true,
		},
		{
			name:                  "assigned, parent not getting less",
			amount:                float32Ptr(1),
			parentAmount:          float32Ptr(1),
			parentRemainingAmount: float32Ptr(1),
			wantEnable:            true,
			wantOk:                true,
		},
		{
			name:                  "assigned, parent getting less",
			amount:                float32Ptr(3),
			parentAmount:          float32Ptr(5),
			parentRemainingAmount: float32Ptr(2),
			wantEnable:            false,
			wantOk:                false,
		},
		{
			name:       "no relevant fields set",
			wantEnable: false,
			wantOk:     false,
		},
		{
			name:                    "unlimitedAmountAssigned false",
			amount:                  float32Ptr(amountUnlimited),
			parentAmount:            float32Ptr(amountUnlimited),
			unlimitedAmountAssigned: boolPtr(false),
			wantEnable:              false,
			wantOk:                  false,
		},
		{
			name:                    "unlimitedAmountAssigned true, but amount not unlimited",
			amount:                  float32Ptr(1),
			parentAmount:            float32Ptr(1),
			unlimitedAmountAssigned: boolPtr(true),
			wantEnable:              false,
			wantOk:                  false,
		},
		{
			name:                  "amount zero",
			amount:                float32Ptr(0),
			parentAmount:          float32Ptr(0),
			parentRemainingAmount: float32Ptr(0),
			wantEnable:            false,
			wantOk:                false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignment := &openapi.AssignedServicePlanSubaccountDTO{
				Amount:                  tt.amount,
				ParentAmount:            tt.parentAmount,
				ParentRemainingAmount:   tt.parentRemainingAmount,
				UnlimitedAmountAssigned: tt.unlimitedAmountAssigned,
			}
			gotEnable, gotOk := getEnableOk(assignment)
			r.Equal(tt.wantEnable, gotEnable)
			r.Equal(tt.wantOk, gotOk)
		})
	}
}

func TestConvertEntitlementResource(t *testing.T) {
	r := require.New(t)

	svcName := "test-service"
	planName := "standard"
	planId := svcName + "-" + planName
	subAccountUuid := "123e4567-e89b-12d3-a456-426614174000"
	typeSubaccount := "SUBACCOUNT"
	typeDirectory := "DIRECTORY"
	var amount float32 = 1
	var parentAmount float32 = 10
	var parentRemainingAmount float32 = 5
	resourceName := planId + "-" + subAccountUuid

	tests := []struct {
		name    string
		service *openapi.AssignedServiceResponseObject
		want    *yaml.ResourceWithComment
	}{
		{
			name: "standard case for amount",
			service: &openapi.AssignedServiceResponseObject{
				Name: &svcName,
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						Name:             &planName,
						UniqueIdentifier: &planId,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityId:                &subAccountUuid,
								EntityType:              &typeSubaccount,
								Amount:                  &amount,
								ParentAmount:            &parentAmount,
								ParentRemainingAmount:   &parentRemainingAmount,
								UnlimitedAmountAssigned: boolPtr(false),
							},
						},
					},
				},
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.Entitlement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.EntitlementKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": resourceName,
						},
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
			service: &openapi.AssignedServiceResponseObject{
				Name: &svcName,
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						Name:             &planName,
						UniqueIdentifier: &planId,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityId:                &subAccountUuid,
								EntityType:              &typeSubaccount,
								Amount:                  &amount,
								ParentAmount:            &amount,
								ParentRemainingAmount:   &amount,
								UnlimitedAmountAssigned: boolPtr(false),
							},
						},
					},
				},
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.Entitlement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.EntitlementKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": resourceName,
						},
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
			service: &openapi.AssignedServiceResponseObject{
				Name: &svcName,
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						Name:             &planName,
						UniqueIdentifier: &planId,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityId:                &subAccountUuid,
								EntityType:              &typeSubaccount,
								Amount:                  float32Ptr(amountUnlimited),
								ParentAmount:            float32Ptr(amountUnlimited),
								UnlimitedAmountAssigned: boolPtr(true),
							},
						},
					},
				},
			},
			want: yaml.NewResourceWithComment(
				&v1alpha1.Entitlement{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.EntitlementKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": resourceName,
						},
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
			service: &openapi.AssignedServiceResponseObject{
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						Name:             &planName,
						UniqueIdentifier: &planId,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityId:                &subAccountUuid,
								EntityType:              &typeSubaccount,
								Amount:                  &amount,
								ParentAmount:            &parentAmount,
								ParentRemainingAmount:   &parentRemainingAmount,
								UnlimitedAmountAssigned: boolPtr(false),
							},
						},
					},
				},
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
							Annotations: map[string]string{
								"crossplane.io/external-name": resourceName,
							},
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
				rwc.AddComment(warnMissingServiceName)
				return rwc
			}(),
		},
		{
			name: "missing service plan name",
			service: &openapi.AssignedServiceResponseObject{
				Name: &svcName,
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						UniqueIdentifier: &planId,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityId:                &subAccountUuid,
								EntityType:              &typeSubaccount,
								Amount:                  &amount,
								ParentAmount:            &parentAmount,
								ParentRemainingAmount:   &parentRemainingAmount,
								UnlimitedAmountAssigned: boolPtr(false),
							},
						},
					},
				},
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
							Annotations: map[string]string{
								"crossplane.io/external-name": resourceName,
							},
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
				rwc.AddComment(warnMissingServicePlanName)
				return rwc
			}(),
		},
		{
			name: "missing service plan ID",
			service: &openapi.AssignedServiceResponseObject{
				Name: &svcName,
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						Name: &planName,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityId:                &subAccountUuid,
								EntityType:              &typeSubaccount,
								Amount:                  &amount,
								ParentAmount:            &parentAmount,
								ParentRemainingAmount:   &parentRemainingAmount,
								UnlimitedAmountAssigned: boolPtr(false),
							},
						},
					},
				},
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UNDEFINED_NAME,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UNDEFINED_NAME,
							},
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
				rwc.AddComment(warnMissingServicePlanId)
				rwc.AddComment(warnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "missing subaccount ID",
			service: &openapi.AssignedServiceResponseObject{
				Name: &svcName,
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						Name:             &planName,
						UniqueIdentifier: &planId,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityType:              &typeSubaccount,
								Amount:                  &amount,
								ParentAmount:            &parentAmount,
								ParentRemainingAmount:   &parentRemainingAmount,
								UnlimitedAmountAssigned: boolPtr(false),
							},
						},
					},
				},
			},
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Entitlement{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.EntitlementKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UNDEFINED_NAME,
							Annotations: map[string]string{
								"crossplane.io/external-name": resources.UNDEFINED_NAME,
							},
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
				rwc.AddComment(warnMissingSubaccountGuid)
				rwc.AddComment(warnUndefinedResourceName)
				return rwc
			}(),
		},
		{
			name: "unsupported entity type",
			service: &openapi.AssignedServiceResponseObject{
				Name: &svcName,
				ServicePlans: []openapi.AssignedServicePlanResponseObject{
					{
						Name:             &planName,
						UniqueIdentifier: &planId,
						AssignmentInfo: []openapi.AssignedServicePlanSubaccountDTO{
							{
								EntityId:                &subAccountUuid,
								EntityType:              &typeDirectory,
								Amount:                  &amount,
								ParentAmount:            &parentAmount,
								ParentRemainingAmount:   &parentRemainingAmount,
								UnlimitedAmountAssigned: boolPtr(false),
							},
						},
					},
				},
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
							Annotations: map[string]string{
								"crossplane.io/external-name": resourceName,
							},
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
				rwc.AddComment(warnUnsupportedEntityType + ", but got: 'DIRECTORY'")
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &tt.service.ServicePlans[0]
			assignment := &plan.AssignmentInfo[0]
			result := convertEntitlementResource(tt.service, plan, assignment)
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

func float32Ptr(f float32) *float32 {
	return &f
}
