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

func (s *ServiceInstanceMapper) TfResource(si *v1alpha1.ServiceInstance, kube client.Client) (*v1alpha1.SubaccountServiceInstance, error) {
	// resolve all parameter secret references and merge them into a single map
	parameterData, err := lookupSecrets(kube, si.Spec.ForProvider.ParameterSecrets)
	if err != nil {
		return nil, errors.Wrap(err, "failed to map tf resource")
	}
	// merge the plain parameters with the secret parameters
	plainParameters := si.Spec.ForProvider.SubaccountServiceInstanceParameters.Parameters
	if err := mergeJsonData(parameterData, []byte(internal.Val(plainParameters))); err != nil {
		return nil, errors.Wrap(err, "failed to map tf resource")
	}
	//TODO: prevent nilpointer
	parameterJson, err := json.Marshal(parameterData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to map tf resource")
	}

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
				ManagementPolicies:               []xpv1.ManagementAction{xpv1.ManagementActionAll},
				WriteConnectionSecretToReference: si.GetWriteConnectionSecretToReference(),
			},
			ForProvider:  si.Spec.ForProvider.SubaccountServiceInstanceParameters,
			InitProvider: v1alpha1.SubaccountServiceInstanceInitParameters{},
		},
		Status: v1alpha1.SubaccountServiceInstanceStatus{},
	}
	sInstance.Spec.ForProvider.Parameters = internal.Ptr(string(parameterJson))
	meta.SetExternalName(sInstance, meta.GetExternalName(si))
	return sInstance, nil
}

func pcName(si *v1alpha1.ServiceInstance) string {
	pc := si.GetProviderConfigReference()
	if pc != nil && pc.Name != "" {
		return pc.Name
	}
	return ""
}

// lookupSecrets retrieves the data from secretKeySelectors, converts them from json to a map and merges them into a single map.
func lookupSecrets(kube client.Client, secretsSelectors []xpv1.SecretKeySelector) (map[string]interface{}, error) {
	combinedData := make(map[string]interface{})
	for _, secret := range secretsSelectors {
		secretObj := &corev1.Secret{}
		//TODO: use context from parent
		if err := kube.Get(context.Background(), client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}, secretObj); err != nil {
			return nil, err
		}
		if val, ok := secretObj.Data[secret.Key]; ok {
			if err := mergeJsonData(combinedData, val); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("key %s not found in secret %s", secret.Key, secret.Name)
		}
		//TODO: add test for not existing key
	}
	return combinedData, nil
}

// mergeJsonData merges the json data into the map
func mergeJsonData(mergedData map[string]interface{}, jsonToMerge []byte) error {
	var toAdd map[string]interface{} = make(map[string]interface{})
	if err := json.Unmarshal(jsonToMerge, &toAdd); err != nil {
		return err
	}
	for k, v := range toAdd {
		mergedData[k] = v
	}
	return nil
}
