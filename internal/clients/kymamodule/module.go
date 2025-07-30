package kymamodule

import (
	"context"
	"slices"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	errKymaModuleCreateFailed = "Could not create KymaModule"
	errKymaModuleDeleteFailed = "Could not delete KymaModule"
	errFailedParse            = "failed to parse kubeconfig"
	errFailedCreateClient     = "failed to create Kubernetes client"
	DefaultKymaName           = "default"
	DefaultKymaNamespace      = "kyma-system"
)

type Client interface {
	Observe(ctx context.Context, moduleName string) (*v1alpha1.ModuleStatus, error)
	Create(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error
	Delete(ctx context.Context, moduleName string) error
}

type KymaModuleClient struct {
	kube client.Client
}

var _ Client = &KymaModuleClient{}

// Takes a Kubeconfig and creates a authenticated Kubernetes client
func NewKymaModuleClient(kymaEnvironmentKubeconfig []byte) (*KymaModuleClient, error) {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kymaEnvironmentKubeconfig)
	if err != nil {
		return nil, errors.Wrap(err, errFailedParse)
	}

	kube, err := client.New(restConfig, client.Options{})
	if err != nil {
		return nil, errors.Wrap(err, errFailedCreateClient)
	}

	return &KymaModuleClient{kube: kube}, nil
}

func (c KymaModuleClient) Observe(ctx context.Context, moduleName string) (*v1alpha1.ModuleStatus, error) {
	kyma, err := getDefaultKyma(ctx, c)
	if err != nil {
		return nil, err
	}

	for _, module := range kyma.Status.Modules {
		if module.Name == moduleName {
			return &module, nil
		}
	}

	return nil, nil
}

// GVKKyma is the GroupVersionKind for Kyma CRs.
var GVKKyma = schema.GroupVersionKind{
	Group:   "operator.kyma-project.io",
	Version: "v1beta2",
	Kind:    "Kyma",
}

// getDefaultKyma gets the default Kyma CR from the kyma-system namespace and cast it to the Kyma structure.
func getDefaultKyma(ctx context.Context, c KymaModuleClient) (*v1alpha1.KymaCr, error) {

	// Note: This is a workaround to get the default Kyma CR.
	// The Kyma CR is not registered in the scheme & no Crossplane CR, so we have to use unstructured.Unstructured to get it.
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(GVKKyma)
	obj.SetName(DefaultKymaName)
	obj.SetNamespace(DefaultKymaNamespace)

	err := c.kube.Get(
		ctx,
		client.ObjectKey{
			Name:      DefaultKymaName,
			Namespace: DefaultKymaNamespace,
		},
		obj,
	)
	if err != nil {
		return nil, err
	}

	mg := &v1alpha1.KymaCr{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, mg); err != nil {
		return nil, err
	}

	return mg, nil
}

// UpdateDefaultKyma updates the default Kyma CR from the kyma-system namespace based on the Kyma CR from arguments
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L169
func (c KymaModuleClient) updateDefaultKyma(ctx context.Context, obj *v1alpha1.KymaCr) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}

	err = c.kube.Update(
		ctx,
		&unstructured.Unstructured{Object: u},
		nil,
	)

	return err
}

// Create adds module to the default Kyma CR in the kyma-system namespace
// if moduleChannel is empty it uses default channel in the Kyma CR
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L212
func (c *KymaModuleClient) Create(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error {
	kymaCR, err := getDefaultKyma(ctx, *c)
	if err != nil {
		return err
	}

	kymaCR = enableModule(kymaCR, moduleName, moduleChannel, customResourcePolicy)

	return c.updateDefaultKyma(ctx, kymaCR)
}

// Delete removes module from the default Kyma CR in the kyma-system namespace
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L226
func (c *KymaModuleClient) Delete(ctx context.Context, moduleName string) error {
	kymaCR, err := getDefaultKyma(ctx, *c)
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
