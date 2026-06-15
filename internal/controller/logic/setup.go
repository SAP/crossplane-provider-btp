// Package logic exposes shared helpers used by per-resource logic packages
// (e.g. logic/account/subaccount). The baseimpl-generated controllers call into
// per-resource exports — Setup, Connect, Observe, Create, Update, Delete — and the
// helpers in this file let those exports stay thin.
package logic

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Options is the per-Setup configuration the generated controllers receive. Aliasing
// the project's existing options struct lets generated code refer to logic.Options
// without dragging the project's options package into every per-resource package.
type Options = internalopts.CrossplaneOptions

// MakeExternal is the constructor a generated controller supplies to Setup. The shared
// Setup helper adapts it to providerconfig's connector closure.
type MakeExternal func(ctx context.Context, mg resource.Managed, kube client.Client) (managed.ExternalClient, error)

// Setup wires up a controller for one resource using the project's standard
// providerconfig + tracker setup. Per-resource logic.Setup typically delegates here
// directly; resources with custom needs can ignore this and call
// providerconfig.DefaultSetup* themselves.
func Setup(mgr ctrl.Manager, o Options, obj client.Object, kind string, gvk schema.GroupVersionKind, mk MakeExternal) error {
	return providerconfig.DefaultSetupWithoutDefaultInitializer(mgr, o, obj, kind, gvk,
		func(_ client.Client, _ providerconfig.LegacyTracker, _ tracking.ReferenceResolverTracker) managed.ExternalConnector {
			return managed.ExternalConnectorFn(func(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
				return mk(ctx, mg, mgr.GetClient())
			})
		},
	)
}

// BuildBTPClient constructs the project's *btp.Client for the supplied managed
// resource via providerconfig.CreateClient. It also returns a ReferenceResolverTracker
// the per-resource Client can store for later SetConditions / DeleteShouldBeBlocked
// calls inside Observe and Delete.
//
// LegacyManaged note: providerconfig.CreateClient asserts mg.(resource.LegacyManaged),
// which only cluster-scoped CRs satisfy today. Namespaced resources hitting this path
// will receive that error until the providerconfig layer grows a namespaced variant —
// out of scope for this generator change.
func BuildBTPClient(ctx context.Context, mg resource.Managed, kube client.Client) (*btp.Client, tracking.ReferenceResolverTracker, error) {
	rt := tracking.NewDefaultReferenceResolverTracker(kube)
	usage := resource.NewLegacyProviderConfigUsageTracker(kube, &providerv1alpha1.ProviderConfigUsage{})
	cli, err := providerconfig.CreateClient(ctx, mg, kube, usage, btp.NewBTPClient, rt)
	return cli, rt, err
}

// DeleteBlockedError is the sentinel error per-resource logic.Delete should return
// when a tracker reports the resource should not be deleted (e.g. it has open usages).
// Centralised here so resources can return logic.DeleteBlockedError() instead of
// importing the project's providerv1alpha1 package directly.
func DeleteBlockedError() error {
	return errResourceInUse
}

var errResourceInUse = errors.New(providerv1alpha1.ErrResourceInUse)

