package rolecollection

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/sap/crossplane-provider-btp/apis/base/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
)

// Client defines the interface for role collection operations.
type Client struct {
	btp btp.Client
}

// Connect creates a Client from a BTP client.
func Connect(btpClient *btp.Client) Client {
	return Client{btp: *btpClient}
}

// MigrateExternalName is a no-op for RoleCollection (no legacy format migration needed).
func MigrateExternalName(_ Client, _ context.Context, _ resource.Managed, _ *base.BaseRoleCollection, _ client.Client) error {
	return nil
}

// Observe checks the external state of a role collection.
func Observe(_ Client, _ context.Context, _ *base.BaseRoleCollection) (managed.ExternalObservation, error) {
	return managed.ExternalObservation{}, nil
}

// Create provisions a new role collection.
func Create(_ Client, _ context.Context, _ *base.BaseRoleCollection) (managed.ExternalCreation, error) {
	return managed.ExternalCreation{}, nil
}

// Update modifies an existing role collection.
func Update(_ Client, _ context.Context, _ *base.BaseRoleCollection) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{}, nil
}

// Delete removes a role collection.
func Delete(_ Client, _ context.Context, _ *base.BaseRoleCollection) (managed.ExternalDelete, error) {
	return managed.ExternalDelete{}, nil
}
