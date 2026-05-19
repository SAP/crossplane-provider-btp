package globalaccount

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/sap/crossplane-provider-btp/apis/base/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
)

const (
	errGetGlobalAccount = "while getting global account"
	errEmptyGUID        = "BTP Global Account GUID is empty"
)

// Client defines the interface for global account operations.
type Client struct {
	btp btp.Client
}

// Connect creates a Client from a BTP client.
func Connect(btpClient *btp.Client, _ client.Client) Client {
	return Client{btp: *btpClient}
}

// MigrateExternalName is a no-op for GlobalAccount (no legacy format migration needed).
func MigrateExternalName(_ Client, _ context.Context, _ resource.Managed, _ *base.BaseGlobalAccount, _ client.Client) error {
	return nil
}

// Observe checks the external state of a global account.
func Observe(c Client, ctx context.Context, cr *base.BaseGlobalAccount) (managed.ExternalObservation, error) {
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
func Create(_ Client, _ context.Context, _ *base.BaseGlobalAccount) (managed.ExternalCreation, error) {
	return managed.ExternalCreation{}, nil
}

// Update is a no-op for GlobalAccount.
func Update(_ Client, _ context.Context, _ *base.BaseGlobalAccount) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{}, nil
}

// Delete handles deletion of a GlobalAccount.
func Delete(_ Client, _ context.Context, cr *base.BaseGlobalAccount) (managed.ExternalDelete, error) {
	cr.Status.SetConditions(xpv1.Deleting())
	return managed.ExternalDelete{}, nil
}
