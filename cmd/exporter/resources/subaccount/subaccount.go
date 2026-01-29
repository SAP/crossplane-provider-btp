package subaccount

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/parsan"
	"github.com/SAP/xp-clifford/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const KIND_NAME = "subaccount"

var (
	subaccountCache resources.ResourceCache[*subaccount]
	subaccountParam = configparam.StringSlice(KIND_NAME, "BTP subaccount ID.").
		WithFlagName(KIND_NAME)
)

func init() {
	resources.RegisterKind(exporter{})
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return subaccountParam
}

func (e exporter) KindName() string {
	return subaccountParam.GetName()
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, _ bool) error {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with subaccounts: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts retrieved", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no subaccounts found"))
	} else {
		for _, sa := range cache.All() {
			eventHandler.Resource(convertSubaccountResource(sa))
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*subaccount], error) {
	if subaccountCache != nil {
		return subaccountCache, nil
	}

	originals, err := btpClient.ListSubaccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subaccounts: %w", err)
	}

	var subaccounts []*subaccount
	for _, sa := range originals {
		subaccounts = append(subaccounts, &subaccount{
			Subaccount:          &sa,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
	}

	cache := resources.NewResourceCache[*subaccount]()
	cache.Store(subaccounts...)
	widgetValues := cache.ValuesForSelection()
	subaccountParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedSubaccounts, err := subaccountParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", subaccountParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected subaccounts", "subaccounts", selectedSubaccounts)

	cache.KeepSelectedOnly(selectedSubaccounts)
	subaccountCache = cache

	return subaccountCache, nil
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
	var resourceName string
	saGuid := s.GetID()
	saDisplayName := s.GetDisplayName()
	hasGuid := saGuid != ""
	hasName := saDisplayName != ""

	switch {
	case !hasName && hasGuid:
		resourceName = KIND_NAME + "-" + saGuid
	case !hasName:
		resourceName = resources.UNDEFINED_NAME
	default:
		names := parsan.ParseAndSanitize(saDisplayName, parsan.RFC1035LowerSubdomain)
		if len(names) == 0 {
			s.AddComment(fmt.Sprintf("error sanitizing subaccount name: %s", saDisplayName))
			resourceName = saDisplayName
		} else {
			resourceName = names[0]
		}
	}

	return resourceName
}
