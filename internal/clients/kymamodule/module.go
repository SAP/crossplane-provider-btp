package kymamodule

import (
	"context"
	"slices"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	errKymaModuleCreateFailed = "Could not create KymaModule"
	errKymaModuleDeleteFailed = "Could not delete KymaModule"
	DefaultKymaName           = "default"
	DefaultKymaNamespace      = "kyma-system"
)

type KymaModuleClient struct {
	dynamic dynamic.Interface
}

// Resource implements dynamic.Interface.
func (c KymaModuleClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	panic("unimplemented")
}

var _ Client = &KymaModuleClient{}

func NewKymaModuleClient(dynamic dynamic.Interface) *KymaModuleClient {
	return &KymaModuleClient{dynamic: dynamic}
}

// GVRKyma is the GroupVersionResource for Kyma CRs.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/types.go#L154
var GVRKyma = schema.GroupVersionResource{
	Group:    "operator.kyma-project.io",
	Version:  "v1beta2",
	Resource: "kymas",
}

// GetDefaultKyma gets the default Kyma CR from the kyma-system namespace and cast it to the Kyma structure.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L144
func (c KymaModuleClient) GetDefaultKyma(ctx context.Context) (*v1alpha1.KymaCr, error) {
	u, err := c.dynamic.Resource(GVRKyma).
		Namespace(DefaultKymaNamespace).
		Get(ctx, DefaultKymaName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	kyma := &v1alpha1.KymaCr{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, kyma)

	return kyma, err
}

// UpdateDefaultKyma updates the default Kyma CR from the kyma-system namespace based on the Kyma CR from arguments
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L169
func (c KymaModuleClient) updateDefaultKyma(ctx context.Context, obj *v1alpha1.KymaCr) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}

	_, err = c.dynamic.Resource(GVRKyma).
		Namespace(DefaultKymaNamespace).
		Update(ctx, &unstructured.Unstructured{Object: u}, metav1.UpdateOptions{})

	return err
}

// EnableModule adds module to the default Kyma CR in the kyma-system namespace
// if moduleChannel is empty it uses default channel in the Kyma CR
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L212
func (c *KymaModuleClient) EnableModule(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error {
	kymaCR, err := c.GetDefaultKyma(ctx)
	if err != nil {
		return err
	}

	kymaCR = enableModule(kymaCR, moduleName, moduleChannel, customResourcePolicy)

	return c.updateDefaultKyma(ctx, kymaCR)
}

// DisableModule removes module from the default Kyma CR in the kyma-system namespace
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L226
func (c *KymaModuleClient) DisableModule(ctx context.Context, moduleName string) error {
	kymaCR, err := c.GetDefaultKyma(ctx)
	if err != nil {
		return err
	}

	kymaCR = disableModule(kymaCR, moduleName)

	return c.updateDefaultKyma(ctx, kymaCR)
}

// Adds the specified module to the Kyma CR.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L302
func enableModule(kymaCR *v1alpha1.KymaCr, moduleName string, moduleChannel string, customResourcePolicy string) *v1alpha1.KymaCr {
	for i, m := range kymaCR.Spec.Modules {
		if m.Name == moduleName {
			// module already exists, update channel
			kymaCR.Spec.Modules[i].Channel = moduleChannel
			kymaCR.Spec.Modules[i].CustomResourcePolicy = customResourcePolicy
			return kymaCR
		}
	}

	kymaCR.Spec.Modules = append(kymaCR.Spec.Modules, v1alpha1.Module{
		Name:                 moduleName,
		Channel:              moduleChannel,
		CustomResourcePolicy: customResourcePolicy,
	})

	return kymaCR
}

// Removes the specified module from the Kyma CR.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L321
func disableModule(kymaCR *v1alpha1.KymaCr, moduleName string) *v1alpha1.KymaCr {
	kymaCR.Spec.Modules = slices.DeleteFunc(kymaCR.Spec.Modules, func(m v1alpha1.Module) bool {
		return m.Name == moduleName
	})

	return kymaCR
}
