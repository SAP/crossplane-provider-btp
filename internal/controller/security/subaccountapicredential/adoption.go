package subaccountapicredential

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const errUnsupportedAdoption = "adoption of an existing SubaccountApiCredential is not supported"

// adoptionRejectingConnector decorates the Terraform connector to prevent a
// newly-created managed resource from treating a successful external Read as
// acquisition of an existing credential.
type adoptionRejectingConnector struct {
	managed.ExternalConnector
}

func (c *adoptionRejectingConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	if cr, ok := mg.(*v1alpha1.SubaccountApiCredential); ok && meta.WasDeleted(cr) && adoptionWasRejected(cr) {
		// Failed adoption never acquired the existing credential. Avoid Connect,
		// which may require the incomplete write-once Secret, and let the normal
		// managed reconciler finalize this MR as externally absent.
		return managed.ExternalClientFns{
			ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
				return managed.ExternalObservation{ResourceExists: false}, nil
			},
			DisconnectFn: func(context.Context) error { return nil },
		}, nil
	}
	client, err := c.ExternalConnector.Connect(ctx, mg)
	if err != nil {
		return nil, err
	}
	return &adoptionRejectingClient{ExternalClient: client}, nil
}

type adoptionRejectingClient struct {
	managed.ExternalClient
}

func adoptionWasRejected(cr *v1alpha1.SubaccountApiCredential) bool {
	return strings.Contains(cr.GetCondition(xpv1.TypeReady).Message, errUnsupportedAdoption)
}

func (c *adoptionRejectingClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	created, err := c.ExternalClient.Create(ctx, mg)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return created, err
	}
	if cr, ok := mg.(*v1alpha1.SubaccountApiCredential); ok {
		cr.SetConditions(xpv1.Unavailable().WithMessage(
			errUnsupportedAdoption + "; use a distinct credential name, or remove the existing BTP credential and verify it is absent before recreating this resource",
		))
	}
	return created, fmt.Errorf("%s: external Create reported an existing credential: %w", errUnsupportedAdoption, err)
}

func hasSuccessfulExternalCreate(cr *v1alpha1.SubaccountApiCredential) bool {
	succeeded := meta.GetExternalCreateSucceeded(cr)
	if succeeded.IsZero() {
		return false
	}

	// A newer pending or failed attempt supersedes an older success. The
	// managed reconciler normally blocks Observe while creation is incomplete;
	// checking timestamps here also fails closed for stale tracking annotations.
	return !meta.GetExternalCreatePending(cr).After(succeeded) && !meta.GetExternalCreateFailed(cr).After(succeeded)
}

func (c *adoptionRejectingClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountApiCredential)
	if !ok {
		return c.ExternalClient.Observe(ctx, mg)
	}

	// Snapshot creation evidence before Upjet Observe mutates status from the
	// remote object. This resource has no Terraform ID attribute, so AtProvider.ID
	// cannot distinguish a resource created by this MR from an existing one.
	hasSuccessfulCreate := hasSuccessfulExternalCreate(cr)
	observation, err := c.ExternalClient.Observe(ctx, mg)
	if err != nil || !observation.ResourceExists || hasSuccessfulCreate || meta.WasDeleted(cr) {
		return observation, err
	}

	// Keep any public connection fields returned by Observe, but never allow a
	// read of an existing credential to become successful acquisition. Repeated
	// observations remain rejected unless the managed reconciler records a
	// successful Create on this resource.
	cr.SetConditions(xpv1.Unavailable().WithMessage(fmt.Sprintf(
		"%s; use a distinct credential name, or remove the existing BTP credential and verify it is absent before recreating this resource",
		errUnsupportedAdoption,
	)))
	return observation, fmt.Errorf("%s for external identity %q", errUnsupportedAdoption, meta.GetExternalName(cr))
}
