package servicebinding

import (
	"context"
	"errors"
	"testing"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	controllertest "github.com/sap/crossplane-provider-btp/internal/controller/test"
	"github.com/sap/crossplane-provider-btp/internal/metrics/externaloperation"
)

func TestSetupExternalOperationDelegation(t *testing.T) {
	for _, tc := range []struct {
		outcome string
		err     error
	}{
		{outcome: "success"},
		{outcome: "error", err: errors.New("fake Terraform observe failed")},
	} {
		t.Run(tc.outcome, func(t *testing.T) {
			sb := &v1alpha1.ServiceBinding{}
			sb.SetName("test-binding")
			sb.SetProviderConfigReference(&xpv2.Reference{Name: "fake-provider"})
			sb.SetAnnotations(map[string]string{providerv1alpha1.AnnotationIgnoreReferences: "true"})
			meta.SetExternalName(sb, "12345678-1234-5678-9abc-123456789012")

			scheme := runtime.NewScheme()
			require.NoError(t, v1alpha1.SchemeBuilder.AddToScheme(scheme))
			kube := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(sb).WithStatusSubresource(sb).Build()
			m := &controllertest.Manager{Manager: fake.Manager{Scheme: scheme, Client: kube}}

			original := newTfConnectorFn
			t.Cleanup(func() { newTfConnectorFn = original })
			var connects, observes int
			newTfConnectorFn = func(client.Client) servicebindingclient.TfConnector {
				return managed.ExternalConnectorFn(func(_ context.Context, mg resource.Managed) (managed.ExternalClient, error) {
					connects++
					require.IsType(t, &v1alpha1.SubaccountServiceBinding{}, mg)
					return &managed.ExternalClientFns{
						ObserveFn: func(_ context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
							observes++
							tf, ok := mg.(*v1alpha1.SubaccountServiceBinding)
							require.True(t, ok, "delegate must observe a Terraform resource, got %T", mg)
							tf.Status.AtProvider.State = ptr.To("succeeded")
							return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, tc.err
						},
					}, nil
				})
			}

			reg := prometheus.NewRegistry()
			recorder, err := externaloperation.NewRecorder(reg)
			require.NoError(t, err)
			require.NoError(t, Setup(m, internalopts.CrossplaneOptions{
				Options: xpcontroller.Options{
					Logger:            logging.NewNopLogger(),
					GlobalRateLimiter: ratelimiter.NewGlobal(1),
				},
				ExternalOperations: recorder,
			}))
			require.Len(t, m.Controllers, 1)
			_, err = m.Controllers[0].Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(sb)})
			require.NoError(t, err)
			require.Equal(t, 1, connects)
			require.Equal(t, 1, observes)
			// Both nested calls ran, but only the public external methods are counted.
			controllertest.RequireExternalOperations(t, reg,
				controllertest.ExternalOperation{Kind: "ServiceBinding", Operation: "connect", Outcome: "success"},
				controllertest.ExternalOperation{Kind: "ServiceBinding", Operation: "observe", Outcome: tc.outcome},
			)
		})
	}
}
