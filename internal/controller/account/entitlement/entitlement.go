package entitlement

import (
	"context"
	"fmt"
	"reflect"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	entitlementclient "github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotEntitlement = "managed resource is not a Entitlement custom resource"
)

var (
	noOpFilter = func(entitlement apisv1alpha1.Entitlement) bool {
		return true
	}
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker
	newServiceFn    func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return nil, errors.New(errNotEntitlement)
	}

	btpclient, err := providerconfig.CreateClient(ctx, mg, c.kube, c.usage, c.newServiceFn, c.resourcetracker)
	if err != nil {
		return nil, err
	}
	return &external{
		kube:    c.kube,
		client:  entitlementclient.NewEntitlementsClient(*btpclient),
		tracker: c.resourcetracker,
	}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	kube    client.Client
	client  entitlementclient.Client
	tracker tracking.ReferenceResolverTracker
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotEntitlement)
	}

	err := c.updateObservation(ctx, cr)
	cr.SetConditions(c.softValidation(cr))
	c.tracker.SetConditions(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	// Needs create?
	if c.needsCreate(cr) {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Needs Update?
	if c.needsUpdate(cr) {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: false,
		}, nil
	}
	switch cr.Status.AtProvider.Assigned.EntityState { //nolint:exhaustive
	case apisv1alpha1.EntitlementStatusOk:
		cr.Status.SetConditions(xpv1.Available())
	// the state relates to the last operation, not the current state
	// at this point we already verified that the entity does match the desired state for all we know
	// so we will ignore this state to avoid blocking healthy resources
	// example case: attempting to delete an entitlement which is already consumed will fail in that way
	// -> the only resolution is to discard and recreate the cr, in this case we just need to observe the still existing entitlement
	// despite the last operation being a failure
	case apisv1alpha1.EntitlementStatusProcessingFailed:
		cr.Status.SetConditions(xpv1.Available())
	case apisv1alpha1.EntitlementStatusProcessing:
		cr.Status.SetConditions(xpv1.Creating())
	case apisv1alpha1.EntitlementStatusStarted:
		cr.Status.SetConditions(xpv1.Creating())
	default:
		cr.Status.SetConditions(xpv1.Unavailable())
	}

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: true,
	}, nil
}

func (c *external) updateObservation(ctx context.Context, cr *apisv1alpha1.Entitlement) error {
	instance, err := c.client.DescribeInstance(ctx, cr)

	if err != nil {
		return err
	}
	entitlements, err := c.findRelatedEntitlements(ctx, cr, noOpFilter)
	if err != nil {
		return err
	}
	cr.Status.AtProvider, err = entitlementclient.GenerateObservation(instance, entitlements)
	if err != nil {
		return err
	}
	return nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotEntitlement)
	}

	err := c.updateObservation(ctx, cr)

	if err != nil {
		return managed.ExternalCreation{}, err
	}

	if err := c.client.CreateInstance(ctx, cr); err != nil {
		return managed.ExternalCreation{}, err
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotEntitlement)
	}

	if cr.Status.AtProvider == nil {
		return managed.ExternalUpdate{}, nil
	}

	if c.updateInProgress(cr) {
		return managed.ExternalUpdate{}, nil
	}

	if err := c.client.UpdateInstance(ctx, cr); err != nil {
		return managed.ExternalUpdate{}, err
	}
	fmt.Printf("Updating: %+v", cr)

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return errors.New(errNotEntitlement)
	}

	instance, err := c.client.DescribeInstance(ctx, cr)

	if err != nil {
		return err
	}

	if c.updateInProgress(cr) {
		return nil
	}

	c.tracker.SetConditions(ctx, cr)
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return errors.New(providerv1alpha1.ErrResourceInUse)
	}

	entitlements, err := c.findRelatedEntitlements(
		ctx,
		cr,
		func(entitlement apisv1alpha1.Entitlement) bool { return entitlement.UID != cr.UID },
	)
	if err != nil {
		return err
	}
	cr.Status.AtProvider, err = entitlementclient.GenerateObservation(instance, entitlements)
	if err != nil {
		return err
	}

	if err := c.client.DeleteInstance(ctx, cr); err != nil {
		return err
	}

	cr.SetConditions(xpv1.Deleting())
	return nil
}

func (c *external) updateInProgress(cr *apisv1alpha1.Entitlement) bool {
	switch cr.Status.AtProvider.Assigned.EntityState { //nolint:exhaustive
	case apisv1alpha1.EntitlementStatusStarted:
		return true
	case apisv1alpha1.EntitlementStatusProcessing:
		return true
	}
	return false
}

func (c *external) needsUpdate(cr *apisv1alpha1.Entitlement) bool {
	// Just don't touch
	autoAssign := cr.Status.AtProvider.Assigned.AutoAssign
	if autoAssign {
		return false
	}
	unlimitedAmountAssigned := cr.Status.AtProvider.Assigned.UnlimitedAmountAssigned
	if unlimitedAmountAssigned {
		return false
	}

	if cr.Spec.ForProvider.Amount != nil {
		return !reflect.DeepEqual(cr.Status.AtProvider.Required.Amount, cr.Status.AtProvider.Assigned.Amount)
	}

	return false
}

func (c *external) needsCreate(cr *apisv1alpha1.Entitlement) bool {
	return cr.Status.AtProvider.Assigned == nil
}

// findRelatedEntitlements resolves all relevant entitlements which do not match the filter function and other static functions
func (c *external) findRelatedEntitlements(
	ctx context.Context,
	ours *apisv1alpha1.Entitlement,
	isRelevant func(entitlement apisv1alpha1.Entitlement) bool,
) (*apisv1alpha1.EntitlementList, error) {
	allEntitlements := &apisv1alpha1.EntitlementList{}
	// client.MatchingLabels()
	err := c.kube.List(ctx, allEntitlements)

	if err != nil {
		return nil, err
	}
	relatedEntitlements := &apisv1alpha1.EntitlementList{}
	for _, ent := range allEntitlements.Items {
		if !isRelevant(ent) {
			continue
		}
		if ent.Spec.ForProvider.SubaccountGuid != ours.Spec.ForProvider.SubaccountGuid {
			continue
		}
		if ent.Spec.ForProvider.ServiceName != ours.Spec.ForProvider.ServiceName {
			continue
		}
		if ent.Spec.ForProvider.ServicePlanName != ours.Spec.ForProvider.ServicePlanName {
			continue
		}
		if ent.GetCondition(xpv1.Deleting().Type).Reason == xpv1.Deleting().Reason {
			continue
		}
		relatedEntitlements.Items = append(relatedEntitlements.Items, ent)
	}
	return relatedEntitlements, nil
}

// softValidation adds conditions to the CR in order to guide the user with the usage of the Entitlements.
func (c *external) softValidation(cr *apisv1alpha1.Entitlement) xpv1.Condition {
	var errs []string
	if cr.Spec.ForProvider.Amount != nil && cr.Spec.ForProvider.Enable != nil {
		errs = append(errs, ".Spec.ForProvider.Amount & .Spec.ForProvider.Enable set. Only one value is supported. This depends on the type of service")
	}

	// Without further information, we cannot proceed, assuming issue with service calls
	if cr.Status.AtProvider == nil {
		return apisv1alpha1.ValidationCondition(errs)
	}
	if cr.Status.AtProvider.Entitled.Name == "" {
		errs = append(errs, "Could not find service to be entitled. Check if Global Account is entitled for usage (Control Center).")
	}

	if cr.Status.AtProvider.Entitled.Unlimited && cr.Status.AtProvider.Required.Amount != nil {
		errs = append(errs, "This serviceplan is non numeric, please use .Spec.ForProvider.Enable and omit the use of .Spec.ForProvider.Amount to configure the entitlement")
	}

	return apisv1alpha1.ValidationCondition(errs)
}
