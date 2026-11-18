package globalaccounttrustconfiguration

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const KindName = "globalaccounttrustconfiguration"

var (
	trustCache resources.ResourceCache[*trustConfiguration]
	trustParam = configparam.StringSlice(KindName, "Trust configuration origin key or regex matching the origin key.").
			WithFlagName(KindName).
			WithExample("--globalaccounttrustconfiguration '.*'")
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
		return fmt.Errorf("failed to get global account trust configurations: %w", err)
	}
	slog.DebugContext(ctx, "Trust configurations after selection", "count", cache.Len())
	if cache.Len() == 0 {
		handler.Warn(fmt.Errorf("no global account trust configurations found"))
	}
	for _, trust := range cache.All() {
		handler.Resource(convertTrustConfigurationResource(trust))
	}
	return nil
}

func Get(ctx context.Context, client *btpcli.BtpCli, options resources.Options) (resources.ResourceCache[*trustConfiguration], error) {
	if trustCache != nil {
		return trustCache, nil
	}
	trusts, err := client.ListGlobalaccountTrustConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list global account trust configurations: %w", err)
	}
	cache := resources.NewResourceCache[*trustConfiguration]()
	for _, trust := range trusts {
		cache.Set(&trustConfiguration{
			TrustConfiguration:  &trust,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
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
}

var _ resources.BtpResource = &trustConfiguration{}

func (t *trustConfiguration) GetID() string { return t.OriginKey }

func (t *trustConfiguration) GetDisplayName() string { return t.OriginKey }

func (t *trustConfiguration) GetExternalName() string {
	if t.OriginKey == "" {
		return resources.UndefinedExternalName
	}
	return t.OriginKey
}

func (t *trustConfiguration) GenerateK8sResourceName() string {
	if t.GetExternalName() == resources.UndefinedExternalName {
		return resources.UndefinedName
	}
	name, err := resources.GenerateK8sResourceName("", t.OriginKey)
	if err != nil {
		t.AddComment(fmt.Sprintf("cannot generate trust configuration resource name: %s", err))
	}
	return name
}
