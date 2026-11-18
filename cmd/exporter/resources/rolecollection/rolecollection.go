package rolecollection

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	KindName = "role-collection"
)

func init() {
	resources.RegisterKind(exporter{})
}

var (
	roleCollectionCache resources.ResourceCache[*rolecollection]
	roleCollectionParam = configparam.StringSlice(KindName, "Role collection name (or name fragment) to export. If specified, it must be a valid regex expression.").
				WithFlagName(KindName)
)

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Param() configparam.ConfigParam {
	return roleCollectionParam
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, btpClient, options)

	if err != nil {
		return fmt.Errorf("failed to get cache with role collections: %w", err)
	}
	slog.DebugContext(ctx, "Role Collections in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no role collections found"))
	}

	for _, rc := range cache.All() {
		eventHandler.Resource(convertRoleCollectionResource(ctx, btpClient, rc))
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli, options resources.Options) (resources.ResourceCache[*rolecollection], error) {
	if roleCollectionCache != nil {
		return roleCollectionCache, nil
	}

	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}

	cache := resources.NewResourceCache[*rolecollection]()
	for _, saID := range saCache.AllIDs() {
		roleCollections, err := btpClient.ListRoleCollectionsForSubaccounts(ctx, saID)
		if err != nil {
			return nil, fmt.Errorf("failed to get role collections for subaccount %s: %w", saID, err)
		}
		for _, rc := range roleCollections {
			rc.ParentGUID = saID
			cache.Set(&rolecollection{
				roleCollection:      &rc,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			})
		}
	}

	slog.DebugContext(ctx, "Total role collections returned by BTP CLI", "count", cache.Len())

	if err := resources.SelectCache(ctx, cache, roleCollectionParam, options); err != nil {
		return nil, err
	}
	roleCollectionCache = cache
	return roleCollectionCache, nil
}

type rolecollection struct {
	roleCollection *btpcli.RoleCollection
	*yaml.ResourceWithComment
}

var _ resources.BtpResource = &rolecollection{}

// GetID returns the unique identifier of a role collection. A role collection
// has no GUID of its own; its name is only unique within its parent
// (subaccount or directory), so the ID is scoped by the ParentGUID to stay
// unique across subaccounts/directories that may hold a role collection of the
// same name.
func (rc *rolecollection) GetID() string {
	return fmt.Sprintf("%s/%s", rc.roleCollection.ParentGUID, rc.roleCollection.Name)
}

// GetDisplayName returns the human-readable name of the role collection.
func (rc *rolecollection) GetDisplayName() string {
	return rc.roleCollection.Name
}

// GetExternalName returns the external name of a role collection. On the
// managed resource the external name is the role collection's name, used
// verbatim as the XSUAA GetRoleCollectionByName lookup key (see
// internal/controller/security/rolecollection). The name is only unique within
// a single subaccount/directory; uniqueness across parents is handled by the
// generated Kubernetes resource name (see GenerateK8sResourceName), not here.
func (rc *rolecollection) GetExternalName() string {
	if rc.roleCollection.Name == "" {
		return resources.UndefinedExternalName
	}
	return rc.roleCollection.Name
}

// GenerateK8sResourceName builds the Kubernetes resource name from the parent
// GUID (subaccount or directory) and the role collection name. The ParentGUID
// is required because a role collection name is only unique within its parent;
// without it two role collections of the same name in different subaccounts/
// directories would generate the same resource name.
func (rc *rolecollection) GenerateK8sResourceName() string {
	if rc.roleCollection.Name == "" || rc.roleCollection.ParentGUID == "" {
		return resources.UndefinedName
	}

	resourceName, err := resources.GenerateK8sResourceName(rc.roleCollection.ParentGUID, rc.roleCollection.Name)

	if err != nil {
		rc.AddComment(fmt.Sprintf("cannot generate role collections name: %s", err))
	}

	return resourceName
}
