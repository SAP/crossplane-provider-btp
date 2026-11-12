package subaccount

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/scope"
)

const KindName = "subaccount"

var (
	subaccountCache resources.ResourceCache[*subaccount]
)

func init() {
	resources.RegisterKind(exporter{})
}

// Reset clears the memoized subaccount cache. Intended for tests.
func Reset() {
	subaccountCache = nil
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	// The scope package owns the combined subaccount/directory selector; the
	// subaccount kind does not register its own selector flag.
	return nil
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with subaccounts: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no subaccounts found"))
	} else {
		for _, sa := range cache.All() {
			eventHandler.Resource(convertSubaccountResource(ctx, btpClient, sa, eventHandler, options.ResolveReferences))
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*subaccount], error) {
	if subaccountCache != nil {
		return subaccountCache, nil
	}

	// Resolve the combined subaccount/directory scope selection (prompts once).
	selection, err := scope.Select(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve scope selection: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts selected via scope", "count", len(selection.Subaccounts))

	// The hierarchy response lacks Labels, so enrich from the flat subaccount
	// list with a single CLI call, indexed by GUID.
	labelsByGUID, err := subaccountLabels(ctx, btpClient)
	if err != nil {
		return nil, err
	}

	// Build the cache from the selected hierarchy subaccounts.
	cache := resources.NewResourceCache[*subaccount]()
	for _, hsa := range selection.Subaccounts {
		cache.Set(&subaccount{
			Subaccount:          toSubaccount(hsa, labelsByGUID[hsa.Guid]),
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
	}
	subaccountCache = cache

	return subaccountCache, nil
}

// subaccountLabels fetches the flat subaccount list once and returns Labels
// keyed by subaccount GUID. The hierarchy response does not carry Labels.
func subaccountLabels(ctx context.Context, btpClient *btpcli.BtpCli) (map[string]map[string][]string, error) {
	flat, err := btpClient.ListSubaccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list subaccounts for label enrichment: %w", err)
	}
	labels := make(map[string]map[string][]string, len(flat))
	for _, sa := range flat {
		labels[sa.GUID] = sa.Labels
	}
	return labels, nil
}

// toSubaccount maps a hierarchy subaccount node to the btpcli.Subaccount shape
// used by the converter, merging in Labels from the flat list.
func toSubaccount(hsa btpcli.HierarchySubaccount, labels map[string][]string) *btpcli.Subaccount {
	return &btpcli.Subaccount{
		GUID:              hsa.Guid,
		TechnicalName:     hsa.TechnicalName,
		DisplayName:       hsa.DisplayName,
		GlobalAccountGUID: hsa.GlobalAccountGUID,
		ParentGUID:        hsa.ParentGUID,
		ParentType:        hsa.ParentType,
		Region:            hsa.Region,
		Subdomain:         hsa.Subdomain,
		BetaEnabled:       hsa.BetaEnabled,
		UsedForProduction: hsa.UsedForProduction,
		Description:       hsa.Description,
		State:             hsa.State,
		StateMessage:      hsa.StateMessage,
		Labels:            labels,
		CreatedDate:       hsa.CreatedDate,
		CreatedBy:         hsa.CreatedBy,
		ModifiedDate:      hsa.ModifiedDate,
	}
}

func GetK8sResourceNameByID(ctx context.Context, btpClient *btpcli.BtpCli, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("subaccount ID is not set")
	}

	saCache, err := Get(ctx, btpClient)
	if err != nil {
		return "", fmt.Errorf("failed to get cache with subaccounts: %w", err)
	}

	sa := saCache.Get(id)
	if sa == nil {
		return "", fmt.Errorf("subaccount not found by ID: %s", id)
	}

	return sa.GenerateK8sResourceName(), nil
}

type subaccount struct {
	*btpcli.Subaccount
	*yaml.ResourceWithComment
}

var _ resources.BtpResource = &subaccount{}

func (s *subaccount) GetID() string {
	return s.GUID
}

func (s *subaccount) GetDisplayName() string {
	return s.DisplayName
}

func (s *subaccount) GetExternalName() string {
	return s.GUID
}

func (s *subaccount) GenerateK8sResourceName() string {
	if s.GetDisplayName() == "" || s.Region == "" {
		return resources.UndefinedName
	}

	// Format: <display-name>.<region>
	rn, err := resources.GenerateK8sResourceName("", fmt.Sprintf("%s.%s", s.GetDisplayName(), s.Region))
	if err != nil {
		s.AddComment(fmt.Sprintf("cannot generate subaccount resource name: %s", err))
	}

	return rn
}
