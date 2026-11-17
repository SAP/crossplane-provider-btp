package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/cloudmanagement"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const KindName = "subscription"

var (
	subscriptionCache resources.ResourceCache[*Subscription]
	subscriptionParam = configparam.StringSlice(KindName, "Subscription GUID or regex matching appName/planName.").
				WithFlagName(KindName).
				WithExample("--subscription 'my-app-name'")
)

func init() {
	resources.RegisterKind(exporter{})
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return subscriptionParam
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, btpClient, options)
	if err != nil {
		return fmt.Errorf("failed to get cache with subscriptions: %w", err)
	}
	slog.DebugContext(ctx, "Subscriptions in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no subscriptions found"))
	}
	for _, s := range cache.All() {
		if s.SupportsGetParameters {
			parameters, err := btpClient.GetSubscriptionParameters(ctx, s.SubaccountID, s.AppName, s.PlanName)
			if err != nil {
				eventHandler.Warn(fmt.Errorf("failed to get parameters for subscription %s: %w", s.GetID(), err))
			}
			s.Parameters = parameters
		}
		cmName, err := cloudmanagement.ExportInstanceForSubaccount(ctx, btpClient, s.SubaccountID, eventHandler, options.ResolveReferences)
		if err != nil {
			eventHandler.Warn(fmt.Errorf("failed to export cloud management for subaccount %s: %w", s.SubaccountID, err))
		}
		s.CloudManagementName = cmName
		eventHandler.Resource(convertSubscriptionResource(s))
	}
	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli, options resources.Options) (resources.ResourceCache[*Subscription], error) {
	if subscriptionCache != nil {
		return subscriptionCache, nil
	}

	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}

	cache := resources.NewResourceCache[*Subscription]()
	for _, saID := range saCache.AllIDs() {
		subscriptions, err := btpClient.ListSubscriptions(ctx, saID)
		if err != nil {
			return nil, fmt.Errorf("failed to get subscriptions for subaccount %s: %w", saID, err)
		}
		for _, s := range subscriptions {
			// The CLI also lists entitled applications that have not been subscribed to.
			if s.State == v1alpha1.SubscriptionStateNotSubscribed {
				continue
			}
			cache.Set(&Subscription{
				Subscription:        &s,
				SubaccountID:        saID,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			})
		}
	}
	slog.DebugContext(ctx, "Total subscriptions returned by BTP CLI", "count", cache.Len())

	if err := resources.SelectCache(ctx, cache, subscriptionParam, options); err != nil {
		return nil, err
	}
	subscriptionCache = cache
	return subscriptionCache, nil
}

type Subscription struct {
	*btpcli.Subscription
	*yaml.ResourceWithComment
	SubaccountID        string
	CloudManagementName string
	Parameters          json.RawMessage
}

var _ resources.BtpResource = &Subscription{}

func (s *Subscription) GetID() string {
	if s.SubscriptionGUID != "" {
		return s.SubscriptionGUID
	}
	// Subscriptions without a GUID still need a unique key across subaccounts.
	return fmt.Sprintf("%s/%s/%s", s.SubaccountID, s.AppName, s.PlanName)
}

func (s *Subscription) GetDisplayName() string {
	return fmt.Sprintf("%s/%s", s.AppName, s.PlanName)
}

func (s *Subscription) GetExternalName() string {
	if s.AppName == "" {
		return resources.UndefinedExternalName
	}
	// An empty plan name is valid and must retain the trailing slash.
	return fmt.Sprintf("%s/%s", s.AppName, s.PlanName)
}

func (s *Subscription) GenerateK8sResourceName() string {
	if s.AppName == "" || s.SubaccountID == "" {
		return resources.UndefinedName
	}
	name := s.AppName
	if s.PlanName != "" {
		name += "." + s.PlanName
	}
	resourceName, err := resources.GenerateK8sResourceName(s.SubaccountID, name)
	if err != nil {
		s.AddComment(fmt.Sprintf("cannot generate subscription resource name: %s", err))
	}
	return resourceName
}
