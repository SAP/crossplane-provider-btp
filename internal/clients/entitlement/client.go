package entitlement

import (
	"context"
	"errors"
	"math"
	"reflect"

	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

const (
	errCollidingEnable = "multiple of kind Entitlement have colliding .Spec.ForProvider.Enable"
	errNegativeInt     = "negative integer not allowed for .Spec.ForProvider.Amount"
)

type Client interface {
	DescribeInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) (*Instance, error)
	CreateInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) error
	DeleteInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) error
	UpdateInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) error
}

type Instance struct {
	EntitledServicePlan *entclient.ServicePlanResponseObject
	Assignment          *entclient.AssignedServicePlanSubaccountDTO
}

func GenerateObservation(
	instance *Instance,
	relatedEntitlements *apisv1alpha1.EntitlementList,
) (*apisv1alpha1.EntitlementObservation, error) {
	observation := &apisv1alpha1.EntitlementObservation{}

	required, err := MergeRelatedEntitlements(relatedEntitlements)

	if err != nil {
		return observation, err
	}
	observation.Required = required

	assignment := newAssigned(*instance)

	observation.Assigned = assignment

	entitled := newEntitled(*instance)
	observation.Entitled = entitled

	return observation, nil
}

// MergeRelatedEntitlements resolves all relevant entitlements which do not match the filter function and other static functions
func MergeRelatedEntitlements(relatedEntitlements *apisv1alpha1.EntitlementList) (
	*apisv1alpha1.EntitlementSummary,
	error,
) {
	summary := &apisv1alpha1.EntitlementSummary{}
	var enable *bool
	var amountSum *int
	var err error
	for _, entitlement := range relatedEntitlements.Items {
		amountSum, err = calcSumOfAmount(entitlement, amountSum)
		if err != nil {
			return summary, err
		}

		enable, err = calculateSumOfEnabled(entitlement, enable, summary)
		if err != nil {
			return summary, err
		}
	}
	summary.Amount = amountSum
	summary.Enable = enable
	count := len(relatedEntitlements.Items)
	summary.EntitlementsCount = &count

	return summary, nil
}

func calculateSumOfEnabled(entitlement apisv1alpha1.Entitlement, enable *bool, summary *apisv1alpha1.EntitlementSummary) (*bool, error) {
	if entitlement.Spec.ForProvider.Enable != nil {
		if enable == nil {
			enable = entitlement.Spec.ForProvider.Enable
		} else if !reflect.DeepEqual(enable, entitlement.Spec.ForProvider.Enable) {
			return nil, errors.New(errCollidingEnable)
		}
	}
	return enable, nil
}

func calcSumOfAmount(entitlement apisv1alpha1.Entitlement, amountSum *int) (*int, error) {
	if entitlement.Spec.ForProvider.Amount != nil {
		amount := *entitlement.Spec.ForProvider.Amount
		if math.Signbit(float64(amount)) {
			return nil, errors.New(errNegativeInt)
		}
		if amountSum == nil {
			amountSum = &amount
		} else {
			sum := amount + *amountSum
			amountSum = &sum
		}
	}
	return amountSum, nil
}

func newAssigned(instance Instance) *apisv1alpha1.Assignable {
	if instance.Assignment == nil {
		return nil
	}

	assignment := apisv1alpha1.Assignable{
		Amount:                  internal.Float32PtrToIntPtr(instance.Assignment.Amount),
		AutoAssign:              internal.Val(instance.Assignment.AutoAssign),
		AutoAssigned:            internal.Val(instance.Assignment.AutoAssigned),
		AutoDistributeAmount:    internal.Val(instance.Assignment.AutoDistributeAmount),
		RequestedAmount:         internal.Val(internal.Float32PtrToIntPtr(instance.Assignment.RequestedAmount)),
		UnlimitedAmountAssigned: internal.Val(instance.Assignment.UnlimitedAmountAssigned),
		Resources:               newResources(instance.Assignment.Resources),
		StateMessage:            internal.Val(instance.Assignment.StateMessage),
		EntityState:             internal.Val(instance.Assignment.EntityState),
		EntityType:              internal.Val(instance.Assignment.EntityType),
		EntityID:                internal.Val(instance.Assignment.EntityId),
	}
	return &assignment
}

func newEntitled(instance Instance) apisv1alpha1.Entitled {
	if instance.EntitledServicePlan == nil {
		return apisv1alpha1.Entitled{}
	}
	entitled := apisv1alpha1.Entitled{
		Amount:                    internal.Val(internal.Float32PtrToIntPtr(instance.EntitledServicePlan.Amount)),
		AutoAssign:                internal.Val(instance.EntitledServicePlan.AutoAssign),
		AutoDistributeAmount:      int(internal.Val(instance.EntitledServicePlan.AutoDistributeAmount)),
		AvailableForInternal:      internal.Val(instance.EntitledServicePlan.AvailableForInternal),
		Beta:                      internal.Val(instance.EntitledServicePlan.Beta),
		Category:                  internal.Val(instance.EntitledServicePlan.Category),
		Description:               internal.Val(instance.EntitledServicePlan.Description),
		DisplayName:               internal.Val(instance.EntitledServicePlan.DisplayName),
		InternalQuotaLimit:        int(internal.Val(instance.EntitledServicePlan.InternalQuotaLimit)),
		MaxAllowedSubaccountQuota: int(internal.Val(instance.EntitledServicePlan.MaxAllowedSubaccountQuota)),
		Name:                      internal.Val(instance.EntitledServicePlan.Name),
		ProvidedBy:                internal.Val(instance.EntitledServicePlan.ProvidedBy),
		ProvisioningMethod:        internal.Val(instance.EntitledServicePlan.ProvisioningMethod),
		RemainingAmount:           int(internal.Val(instance.EntitledServicePlan.RemainingAmount)),
		Resources:                 newResources(instance.EntitledServicePlan.Resources),
		UniqueIdentifier:          internal.Val(instance.EntitledServicePlan.UniqueIdentifier),
		Unlimited:                 internal.Val(instance.EntitledServicePlan.Unlimited),
	}
	return entitled
}

func newResources(res []entclient.ExternalResourceRequestPayload) []*apisv1alpha1.Resource {
	resources := make([]*apisv1alpha1.Resource, len(res))
	for _, re := range res {
		resources = append(
			resources, &apisv1alpha1.Resource{
				ResourceName:          internal.Val(re.ResourceName),
				ResourceProvider:      internal.Val(re.ResourceProvider),
				ResourceTechnicalName: internal.Val(re.ResourceTechnicalName),
				ResourceType:          internal.Val(re.ResourceType),
			},
		)
	}
	return resources
}
