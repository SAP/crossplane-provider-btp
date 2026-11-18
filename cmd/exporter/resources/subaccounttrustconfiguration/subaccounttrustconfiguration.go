package subaccounttrustconfiguration

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

const KindName = "subaccounttrustconfiguration"

var (
	trustCache resources.ResourceCache[*trustConfiguration]
	trustParam = configparam.StringSlice(KindName, "Trust configuration ID (subaccountID/origin) or regex matching the origin key.").
			WithFlagName(KindName).
			WithExample("--subaccounttrustconfiguration '.*'")
)

func init() {
	resources.RegisterKind(exporter{})
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam { return trustParam }

func (e exporter) KindName() string { return KindName }

func (e exporter) Export(ctx context.Context, client *btpcli.BtpCli, handler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, client, options)
	if err != nil {
		return fmt.Errorf("failed to get subaccount trust configurations: %w", err)
	}
	slog.DebugContext(ctx, "Trust configurations after selection", "count", cache.Len())
	if cache.Len() == 0 {
		handler.Warn(fmt.Errorf("no subaccount trust configurations found"))
	}
	for _, trust := range cache.All() {
		handler.Resource(convertTrustConfigurationResource(ctx, client, trust, handler, options.ResolveReferences))
	}
	return nil
}

func Get(ctx context.Context, client *btpcli.BtpCli, options resources.Options) (resources.ResourceCache[*trustConfiguration], error) {
	if trustCache != nil {
		return trustCache, nil
	}
	subaccounts, err := subaccount.Get(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	cache := resources.NewResourceCache[*trustConfiguration]()
	for _, subaccountID := range subaccounts.AllIDs() {
		trusts, err := client.ListSubaccountTrustConfigurations(ctx, subaccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to list trust configurations for subaccount %s: %w", subaccountID, err)
		}
		for _, trust := range trusts {
			cache.Set(&trustConfiguration{
				TrustConfiguration:  &trust,
				SubaccountID:        subaccountID,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			})
		}
	}
	if err := resources.SelectCache(ctx, cache, trustParam, options); err != nil {
		return nil, err
	}
	trustCache = cache
	return trustCache, nil
}

type trustConfiguration struct {
	*btpcli.TrustConfiguration
	*yaml.ResourceWithComment
	SubaccountID string
}

var _ resources.BtpResource = &trustConfiguration{}

func (t *trustConfiguration) GetID() string {
	return fmt.Sprintf("%s/%s", t.SubaccountID, t.OriginKey)
}

func (t *trustConfiguration) GetDisplayName() string { return t.OriginKey }

func (t *trustConfiguration) GetExternalName() string {
	if t.SubaccountID == "" || t.OriginKey == "" {
		return resources.UndefinedExternalName
	}
	return t.GetID()
}

func (t *trustConfiguration) GenerateK8sResourceName() string {
	if t.GetExternalName() == resources.UndefinedExternalName {
		return resources.UndefinedName
	}
	name, err := resources.GenerateK8sResourceName(t.SubaccountID, t.OriginKey)
	if err != nil {
		t.AddComment(fmt.Sprintf("cannot generate trust configuration resource name: %s", err))
	}
	return name
}
