package controller

import (
	"context"
	"errors"
	"testing"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/sap/crossplane-provider-btp/apis"
	account "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	provider "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	controllertest "github.com/sap/crossplane-provider-btp/internal/controller/test"
	"github.com/sap/crossplane-provider-btp/internal/metrics/externaloperation"
)

func TestCustomSetupExternalOperations(t *testing.T) {
	const name = "test-resource"

	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))
	m := &controllertest.Manager{Manager: fake.Manager{
		Scheme: scheme,
		Client: &test.MockClient{
			MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
				// Serve only the managed object; credentials and referenced resources
				// are absent so Connect returns before attempting any remote work.
				mg, ok := obj.(providerconfig.LegacyManaged)
				if !ok || key.Name != name {
					return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
				}
				mg.SetName(name)
				now := metav1.Now()
				mg.SetDeletionTimestamp(&now) // Skip reference resolution so the test reaches Connect.
				mg.SetDeletionPolicy(xpv2.DeletionDelete)
				mg.SetProviderConfigReference(&xpv2.Reference{Name: "missing-provider"})
				mg.SetAnnotations(map[string]string{provider.AnnotationIgnoreReferences: "true"})
				meta.SetExternalName(mg, name)
				if sb, ok := mg.(*account.ServiceBinding); ok {
					sb.Spec.ForProvider.ParameterSecretRefs = []xpv2.SecretKeySelector{{
						SecretReference: xpv2.SecretReference{Name: "missing-parameters", Namespace: "default"},
						Key:             "params",
					}}
				}
				return nil
			},
			MockCreate:       test.NewMockCreateFn(errors.New("fake client rejects ProviderConfigUsage creation")),
			MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
		},
	}}
	reg := prometheus.NewRegistry()
	recorder, err := externaloperation.NewRecorder(reg)
	require.NoError(t, err)
	require.NoError(t, CustomSetup(m, internalopts.CrossplaneOptions{
		Options: xpcontroller.Options{
			Logger:            logging.NewNopLogger(),
			GlobalRateLimiter: ratelimiter.NewGlobal(10), // Burst covers one reconcile per controller.
		},
		ExternalOperations: recorder,
	}))
	kinds := []string{
		"CertBasedOIDCLogin", "CloudFoundryEnvironment", "Directory", "Entitlement",
		"KubeConfigGenerator", "KymaEnvironment", "KymaEnvironmentBinding", "KymaModule",
		"RoleCollection", "RoleCollectionAssignment", "Subaccount", "SubaccountDestination",
		"SubaccountDestinationCertificate", "Subscription",
		"CloudManagement", "ServiceBinding", "ServiceInstance", "ServiceManager",
	}
	// The remaining controller is ResourceUsage; it must not emit a sample.
	require.Len(t, m.Controllers, len(kinds)+1)
	for _, c := range m.Controllers {
		_, err := c.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKey{Name: name}})
		require.NoError(t, err)
	}
	var want []controllertest.ExternalOperation
	for _, kind := range kinds {
		want = append(want, controllertest.ExternalOperation{Kind: kind, Operation: "connect", Outcome: "error"})
	}
	controllertest.RequireExternalOperations(t, reg, want...)
}
