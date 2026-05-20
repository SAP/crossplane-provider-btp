package globalaccount

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/sap/crossplane-provider-btp/apis/base/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/controller/logic"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errGetGlobalAccount = "while getting global account"
	errEmptyGUID        = "BTP Global Account GUID is empty"
)

// Options is the per-Setup configuration for the GlobalAccount controller.
type Options = logic.Options

// Client defines the interface for global account operations.
type Client struct {
	btp     btp.Client
	tracker tracking.ReferenceResolverTracker
}

// Setup wires up the GlobalAccount controller via the shared logic.Setup helper.
func Setup(mgr ctrl.Manager, o Options, obj client.Object, kind string, gvk schema.GroupVersionKind, mk logic.MakeExternal) error {
	return logic.Setup(mgr, o, obj, kind, gvk, mk)
}

// Connect builds a *btp.Client for the supplied managed resource.
func Connect(ctx context.Context, mg resource.Managed, kube client.Client) (Client, error) {
	btpClient, tracker, err := logic.BuildBTPClient(ctx, mg, kube)
	if err != nil {
		return Client{}, err
	}
	return Client{btp: *btpClient, tracker: tracker}, nil
}

// MigrateExternalName is a no-op for GlobalAccount (no legacy format migration needed).
func MigrateExternalName(_ Client, _ context.Context, _ resource.Managed, _ *base.BaseGlobalAccount, _ client.Client) error {
	return nil
}

// Observe checks the external state of a global account.
func Observe(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseGlobalAccount) (managed.ExternalObservation, error) {
	if c.tracker != nil {
		c.tracker.SetConditions(ctx, mg)
	}

	if meta.WasDeleted(cr) {
		if cr.Status.GetCondition(providerv1alpha1.UseCondition).Reason == providerv1alpha1.InUseReason {
			return managed.ExternalObservation{ResourceExists: true}, nil
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	response, _, err := c.btp.AccountsServiceClient.GlobalAccountOperationsAPI.GetGlobalAccount(ctx).Execute()
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetGlobalAccount)
	}

	globalAccountGuid := response.Guid
	if globalAccountGuid == "" {
		return managed.ExternalObservation{}, errors.New(errEmptyGUID)
	}

	cr.Status.AtProvider.Guid = globalAccountGuid
	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

// Create is a no-op for GlobalAccount.
func Create(_ Client, _ context.Context, _ resource.Managed, _ *base.BaseGlobalAccount) (managed.ExternalCreation, error) {
	return managed.ExternalCreation{}, nil
}

// Update is a no-op for GlobalAccount.
func Update(_ Client, _ context.Context, _ resource.Managed, _ *base.BaseGlobalAccount) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{}, nil
}

// Delete handles deletion of a GlobalAccount.
func Delete(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseGlobalAccount) (managed.ExternalDelete, error) {
	if c.tracker != nil {
		c.tracker.SetConditions(ctx, mg)
		if c.tracker.DeleteShouldBeBlocked(mg) {
			return managed.ExternalDelete{}, logic.DeleteBlockedError()
		}
	}

	cr.Status.SetConditions(xpv1.Deleting())
	return managed.ExternalDelete{}, nil
}
