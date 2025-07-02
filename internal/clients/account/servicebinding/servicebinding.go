package servicebindingclient

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	instanceClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewServiceBindingConnector creates a connector for the service binding client using the generic TfProxyConnector
func NewServiceBindingConnector(saveConditionsCallback tfclient.SaveConditionsFn, kube client.Client) tfclient.TfProxyConnectorI[*v1alpha1.ServiceBinding] {
	con := &ServiceBindingConnector{
		TfProxyConnector: tfclient.NewTfProxyConnector(
			tfclient.NewInternalTfConnector(
				kube,
				"btp_subaccount_service_binding",
				v1alpha1.SubaccountServiceBinding_GroupVersionKind,
				true,
				tfclient.NewAPICallbacks(
					kube,
					saveConditionsCallback,
				),
			),
			&ServiceBindingMapper{},
			kube,
		),
	}
	return con
}

type ServiceBindingConnector struct {
	tfclient.TfProxyConnector[*v1alpha1.ServiceBinding, *v1alpha1.SubaccountServiceBinding]
}

type ServiceBindingMapper struct{}

func (s *ServiceBindingMapper) TfResource(ctx context.Context, sb *v1alpha1.ServiceBinding, kube client.Client) (*v1alpha1.SubaccountServiceBinding, error) {
	sBinding := buildBaseTfResource(sb)

	// combine parameters
	parameterJson, err := instanceClient.BuildComplexParameterJson(ctx, kube, sb.Spec.ForProvider.ParameterSecretRefs, sb.Spec.ForProvider.Parameters.Raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to map tf resource")
	}
	sBinding.Spec.ForProvider.Parameters = internal.Ptr(string(parameterJson))

	// transfer external name
	meta.SetExternalName(sBinding, meta.GetExternalName(sb))

	// in order for the tf reconciler to properly work we need to mimic the ready condition as well
	condition := sb.GetCondition(xpv1.TypeReady)
	sBinding.SetConditions(condition)

	return sBinding, nil
}

func buildBaseTfResource(sb *v1alpha1.ServiceBinding) *v1alpha1.SubaccountServiceBinding {
	sBinding := &v1alpha1.SubaccountServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceBinding_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              sb.Name,
			UID:               sb.UID + "-service-binding",
			DeletionTimestamp: sb.DeletionTimestamp,
		},
		Spec: v1alpha1.SubaccountServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: pcName(sb),
				},
				ManagementPolicies:               sb.GetManagementPolicies(),
				WriteConnectionSecretToReference: sb.GetWriteConnectionSecretToReference(),
			},
			ForProvider: v1alpha1.SubaccountServiceBindingParameters{
				SubaccountID:      sb.Spec.ForProvider.SubaccountID,
				ServiceInstanceID: sb.Spec.ForProvider.ServiceInstanceID,
				Name:              internal.Ptr(sb.Spec.ForProvider.Name),
			},
			InitProvider: v1alpha1.SubaccountServiceBindingInitParameters{},
		},
		Status: v1alpha1.SubaccountServiceBindingStatus{},
	}
	return sBinding
}

func pcName(sb *v1alpha1.ServiceBinding) string {
	pc := sb.GetProviderConfigReference()
	if pc != nil && pc.Name != "" {
		return pc.Name
	}
	return ""
}
