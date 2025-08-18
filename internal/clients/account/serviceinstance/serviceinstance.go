package serviceinstanceclient

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewServiceInstanceConnector creates a connector for the service instance client using the generic TfProxyConnector
func NewServiceInstanceConnector(saveConditionsCallback tfclient.SaveConditionsFn, kube client.Client) tfclient.TfProxyConnectorI[*v1alpha1.ServiceInstance] {
	con := &ServiceInstanceConnector{
		TfProxyConnector: tfclient.NewTfProxyConnector(
			tfclient.NewInternalTfConnector(
				kube,
				"btp_subaccount_service_instance",
				v1alpha1.SubaccountServiceInstance_GroupVersionKind,
				true,
				tfclient.NewAPICallbacks(
					kube,
					saveConditionsCallback,
				),
			),
			&ServiceInstanceMapper{},
			kube,
		),
	}
	return con
}

type ServiceInstanceConnector struct {
	tfclient.TfProxyConnector[*v1alpha1.ServiceInstance, *v1alpha1.SubaccountServiceInstance]
}

type ServiceInstanceMapper struct {
}

func (s *ServiceInstanceMapper) TfResource(ctx context.Context, si *v1alpha1.ServiceInstance, kube client.Client) (*v1alpha1.SubaccountServiceInstance, error) {
	sInstance := buildBaseTfResource(si)

	// combine parameters
	parameterJson, err := BuildComplexParameterJson(ctx, kube, si.Spec.ForProvider.ParameterSecretRefs, si.Spec.ForProvider.Parameters.Raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to map tf resource")
	}
	sInstance.Spec.ForProvider.Parameters = internal.Ptr(string(parameterJson))

	// transfer external name
	meta.SetExternalName(sInstance, meta.GetExternalName(si))

	if si.Status.AtProvider.ServiceplanID != "" {
		sInstance.Spec.ForProvider.ServiceplanID = &si.Status.AtProvider.ServiceplanID
	}

	// in order for the tf reconciler to properly work we need to mimic the ready condition as well
	condition := si.GetCondition(xpv1.TypeReady)
	sInstance.SetConditions(condition)

	return sInstance, nil
}

func BuildComplexParameterJson(ctx context.Context, kube client.Client, secretRefs []xpv1.SecretKeySelector, specParams []byte) ([]byte, error) {
	// resolve all parameter secret references and merge them into a single map
	parameterData, err := lookupSecrets(ctx, kube, secretRefs)
	if err != nil {
		return nil, err
	}

	// merge the plain parameters with the secret parameters
	specParamsMap, err := internal.UnmarshalRawParameters(specParams)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec parameters: %w", err)
	}
	addMap(parameterData, specParamsMap)

	parameterJson, err := json.Marshal(parameterData)
	if err != nil {
		return nil, err
	}
	return parameterJson, nil
}

func buildBaseTfResource(si *v1alpha1.ServiceInstance) *v1alpha1.SubaccountServiceInstance {
	sInstance := &v1alpha1.SubaccountServiceInstance{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceInstance_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: si.Name,
			// make sure no naming conflicts are there for upjet tmp folder creation
			UID:               si.UID + "-service-instance",
			DeletionTimestamp: si.DeletionTimestamp,
		},
		Spec: v1alpha1.SubaccountServiceInstanceSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: pcName(si),
				},
				ManagementPolicies:               si.GetManagementPolicies(),
				WriteConnectionSecretToReference: si.GetWriteConnectionSecretToReference(),
			},
			ForProvider: v1alpha1.SubaccountServiceInstanceParameters{
				SubaccountID: si.Spec.ForProvider.SubaccountID,
				Name:         internal.Ptr(si.Spec.ForProvider.Name),
			},
			InitProvider: v1alpha1.SubaccountServiceInstanceInitParameters{},
		},
	}
	return sInstance
}

func pcName(si *v1alpha1.ServiceInstance) string {
	pc := si.GetProviderConfigReference()
	if pc != nil && pc.Name != "" {
		return pc.Name
	}
	return ""
}

// lookupSecrets retrieves the data from secretKeySelectors, converts them from json to a map and merges them into a single map.
func lookupSecrets(ctx context.Context, kube client.Client, secretsSelectors []xpv1.SecretKeySelector) (map[string]interface{}, error) {
	combinedData := make(map[string]interface{})
	for _, secret := range secretsSelectors {
		secretObj := &corev1.Secret{}
		if err := kube.Get(ctx, client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}, secretObj); err != nil {
			return nil, err
		}
		if val, ok := secretObj.Data[secret.Key]; ok {
			if err := mergeJsonData(combinedData, val); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("key %s not found in secret %s", secret.Key, secret.Name)
		}
	}
	return combinedData, nil
}

// mergeJsonData merges the json data into the map
func mergeJsonData(mergedData map[string]interface{}, jsonToMerge []byte) error {
	var toAdd map[string]interface{} = make(map[string]interface{})
	if err := json.Unmarshal(jsonToMerge, &toAdd); err != nil {
		return err
	}
	addMap(mergedData, toAdd)
	return nil
}

// mergeMaps merges a
func addMap(mergedData map[string]interface{}, toAdd map[string]interface{}) {
	for k, v := range toAdd {
		mergedData[k] = v
	}
}
