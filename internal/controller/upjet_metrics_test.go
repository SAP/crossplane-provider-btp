package controller

import (
	"context"
	"errors"
	"testing"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	ujmetrics "github.com/crossplane/upjet/v2/pkg/metrics"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/sap/crossplane-provider-btp/apis"
	provider "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	controllertest "github.com/sap/crossplane-provider-btp/internal/controller/test"
	"github.com/sap/crossplane-provider-btp/internal/metrics/externaloperation"
)

func TestUpjetSetupExternalOperations(t *testing.T) {
	const name = "test-resource"

	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))
	var providerConfigLookedUp bool
	m := &controllertest.Manager{Manager: fake.Manager{
		Scheme: scheme,
		Client: &test.MockClient{
			MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*provider.ProviderConfig); ok {
					providerConfigLookedUp = true
				}
				mg, ok := obj.(ujresource.Terraformed)
				if !ok || key.Name != name {
					return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
				}
				mg.SetName(name)
				gvks, _, err := scheme.ObjectKinds(mg)
				require.NoError(t, err)
				require.Len(t, gvks, 1)
				mg.GetObjectKind().SetGroupVersionKind(gvks[0])
				now := metav1.Now()
				mg.SetCreationTimestamp(now)
				mg.SetDeletionTimestamp(&now) // Skip reference resolution and initializers.
				meta.SetExternalName(mg, name)
				return nil
			},
			MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
		},
	}}
	reg := prometheus.NewRegistry()
	recorder, err := externaloperation.NewRecorder(reg)
	require.NoError(t, err)

	// Keep each existing metric separate from the new histogram's exact-series
	// check. Scrape Upjet's existing collector without changing the global registry.
	mrRegistry := prometheus.NewRegistry()
	mrMetrics := managed.NewMRMetricRecorder()
	require.NoError(t, mrRegistry.Register(mrMetrics))
	upjetRegistry := prometheus.NewRegistry()
	require.NoError(t, upjetRegistry.Register(ujmetrics.ExternalAPITime))
	before := upjetConnectSamples(t, upjetRegistry)

	require.NoError(t, Setup(m, internalopts.UpjetOptions{
		Options: tjcontroller.Options{
			Options: xpcontroller.Options{
				Logger:            logging.NewNopLogger(),
				GlobalRateLimiter: ratelimiter.NewGlobal(10),
				MetricOptions:     &xpcontroller.MetricOptions{MRMetrics: mrMetrics},
			},
			Provider: &ujconfig.Provider{Resources: map[string]*ujconfig.Resource{
				"btp_directory_entitlement":             {},
				"btp_globalaccount_trust_configuration": {},
				"btp_subaccount_api_credential":         {},
				"btp_subaccount_service_broker":         {},
				"btp_subaccount_trust_configuration":    {},
			}},
			SetupFn: func(context.Context, client.Client, xpresource.Managed) (terraform.Setup, error) {
				// Exercise real Upjet Connect without building a remote provider client.
				return terraform.Setup{}, errors.New("fake Terraform setup failed")
			},
		},
		ExternalOperations: recorder,
	}))
	kinds := []string{
		"DirectoryEntitlement", "GlobalaccountTrustConfiguration", "SubaccountApiCredential",
		"SubaccountServiceBroker", "SubaccountTrustConfiguration",
	}
	require.Len(t, m.Controllers, len(kinds)+1) // Includes the bookkeeping controller.
	for _, c := range m.Controllers {
		_, err := c.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKey{Name: name}})
		require.NoError(t, err)
	}
	require.True(t, providerConfigLookedUp)
	var want []controllertest.ExternalOperation
	for _, kind := range kinds {
		want = append(want, controllertest.ExternalOperation{Kind: kind, Operation: "connect", Outcome: "error"})
	}
	controllertest.RequireExternalOperations(t, reg, want...)
	require.Equal(t, before+uint64(len(kinds)), upjetConnectSamples(t, upjetRegistry))

	families, err := mrRegistry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Equal(t, "crossplane_managed_resource_first_time_to_reconcile_seconds", families[0].GetName())
	require.Len(t, families[0].GetMetric(), len(kinds))
	for _, metric := range families[0].GetMetric() {
		require.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
	}
}

func upjetConnectSamples(t *testing.T, reg *prometheus.Registry) uint64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "operation" && label.GetValue() == "connect" {
					return metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}
