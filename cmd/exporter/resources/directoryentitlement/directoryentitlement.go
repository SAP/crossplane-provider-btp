package directoryentitlement

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const KindName = "directoryentitlement"

var (
	entitlementCache resources.ResourceCache[*directoryEntitlement]
	entitlementParam = configparam.StringSlice(KindName, "Directory entitlement ID (directoryID/serviceName/planName) or regex matching serviceName/planName.").
				WithFlagName(KindName).
				WithExample("--directoryentitlement '.*'")
	directoryParam = configparam.StringSlice("directory", "Source directory GUIDs for directory entitlement export (comma-separated).").
			WithFlagName("directory")
)

func init() {
	resources.RegisterKind(exporter{})
	export.AddConfigParams(directoryParam)
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) KindName() string { return KindName }

func (e exporter) Param() configparam.ConfigParam { return entitlementParam }

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, handler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, btpClient, options)
	if err != nil {
		return fmt.Errorf("failed to get directory entitlements: %w", err)
	}
	slog.DebugContext(ctx, "Directory entitlements after selection", "count", cache.Len())
	if cache.Len() == 0 {
		handler.Warn(fmt.Errorf("no directory entitlements found"))
	} else if options.ResolveReferences {
		handler.Warn(fmt.Errorf("directory reference resolution is not yet supported; exporting directoryId instead of directoryRef"))
	}
	for _, entitlement := range cache.All() {
		handler.Resource(convertDirectoryEntitlementResource(entitlement))
	}
	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli, options resources.Options) (resources.ResourceCache[*directoryEntitlement], error) {
	if entitlementCache != nil {
		return entitlementCache, nil
	}
	directoryIDs := directoryParam.Value()
	if len(directoryIDs) == 0 {
		return nil, fmt.Errorf("--directory is required for directoryentitlement export")
	}
	cache := resources.NewResourceCache[*directoryEntitlement]()
	seen := make(map[string]bool)
	for _, directoryID := range directoryIDs {
		directoryID = strings.TrimSpace(directoryID)
		if directoryID == "" {
			return nil, fmt.Errorf("--directory must contain non-empty directory GUIDs")
		}
		if seen[directoryID] {
			continue
		}
		seen[directoryID] = true
		services, err := btpClient.ListDirectoryEntitlements(ctx, directoryID)
		if err != nil {
			return nil, fmt.Errorf("failed to list entitlements for directory %s: %w", directoryID, err)
		}
		for _, service := range services {
			for _, plan := range service.ServicePlans {
				cache.Set(&directoryEntitlement{
					directoryID:         directoryID,
					serviceName:         service.Name,
					plan:                plan,
					ResourceWithComment: yaml.NewResourceWithComment(nil),
				})
			}
		}
	}
	if err := resources.SelectCache(ctx, cache, entitlementParam, options); err != nil {
		return nil, err
	}
	entitlementCache = cache
	return entitlementCache, nil
}

type directoryEntitlement struct {
	directoryID string
	serviceName string
	plan        btpcli.DirectoryEntitledServicePlan
	*yaml.ResourceWithComment
}

var _ resources.BtpResource = &directoryEntitlement{}

func (e *directoryEntitlement) GetID() string {
	return fmt.Sprintf("%s/%s/%s", e.directoryID, e.serviceName, e.plan.Name)
}

func (e *directoryEntitlement) GetDisplayName() string {
	return fmt.Sprintf("%s/%s", e.serviceName, e.plan.Name)
}

func (e *directoryEntitlement) GetExternalName() string {
	if e.directoryID == "" || e.serviceName == "" || e.plan.Name == "" {
		return resources.UndefinedExternalName
	}
	return e.GetID()
}

func (e *directoryEntitlement) GenerateK8sResourceName() string {
	if e.GetExternalName() == resources.UndefinedExternalName {
		return resources.UndefinedName
	}
	name, err := resources.GenerateK8sResourceName(e.directoryID, fmt.Sprintf("%s.%s", e.serviceName, e.plan.Name))
	if err != nil {
		e.AddComment(fmt.Sprintf("cannot generate directory entitlement resource name: %s", err))
	}
	return name
}
