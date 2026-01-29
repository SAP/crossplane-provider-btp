package kymamodule

import (
	"context"
	"slices"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	errFailedParse                 = "failed to parse kubeconfig"
	errFailedCreateClient          = "failed to create Kubernetes client"
	errFailedGetDefaultKyma        = "failed to get default Kyma CR"
	errFailedConvertToUnstructured = "failed to convert Kyma CR to unstructured"
	DefaultKymaName                = "default"
	DefaultKymaNamespace           = "kyma-system"
)

type Client interface {
	ObserveModule(ctx context.Context, moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error)
	CreateModule(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error
	DeleteModule(ctx context.Context, moduleName string) error
}

type KymaModuleClient struct {
	kube client.Client
}

var _ Client = &KymaModuleClient{}

// GVKKyma is the GroupVersionKind for Kyma CRs.
var GVKKyma = schema.GroupVersionKind{
	Group:   "operator.kyma-project.io",
	Version: "v1beta2",
	Kind:    "Kyma",
}

// Takes a Kubeconfig and creates an authenticated Kubernetes client
func NewKymaModuleClient(kymaEnvironmentKubeconfig []byte) (Client, error) {
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

func (c *KymaModuleClient) ObserveModule(ctx context.Context, moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
	kyma, err := getDefaultKyma(ctx, c)
	if err != nil {
		return nil, err
	}

	// Find the module we use in the list of all activated modules from the kyma resource.
	// Reason: we use one managed resource per module while kyma bundles it all in one cr.
	// To resolve this many to one mapping, for every one of our module managed resource, we query the same kyma cr and find the correct name in it.
	for _, module := range kyma.Status.Modules {
		if module.Name == meta.GetExternalName(moduleCr) {
			moduleCr.Status.AtProvider = module
			return &module, nil
		}
	}

	return nil, nil
}

// CreateModule adds module to the default Kyma CR in the kyma-system namespace
// if moduleChannel is empty it uses default channel in the Kyma CR
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L212
func (c *KymaModuleClient) CreateModule(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error {
	kymaCR, err := getDefaultKyma(ctx, c)
	if err != nil {
		return err
	}

	kymaCR = enableModule(kymaCR, moduleName, moduleChannel, customResourcePolicy)

	return updateDefaultKyma(ctx, c, kymaCR)
}

// DeleteModule removes module from the default Kyma CR in the kyma-system namespace
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L226
func (c *KymaModuleClient) DeleteModule(ctx context.Context, moduleName string) error {
	kymaCR, err := getDefaultKyma(ctx, c)
	if err != nil {
		return err
	}

	kymaCR = disableModule(kymaCR, moduleName)

	return updateDefaultKyma(ctx, c, kymaCR)
}

// getDefaultKyma gets the default Kyma CR from the kyma-system namespace and cast it to the Kyma structure.
func getDefaultKyma(ctx context.Context, c *KymaModuleClient) (*KymaCr, error) {

	// Note: This is a workaround to get the default Kyma CR.
	// The Kyma CR is not registered in the schema & no Crossplane CR, so we have to use unstructured.Unstructured to get it.
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
		return nil, errors.Wrap(err, errFailedGetDefaultKyma)
	}

	mg := &KymaCr{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, mg); err != nil {
		return nil, errors.Wrap(err, errFailedConvertToUnstructured)
	}

	return mg, nil
}

// UpdateDefaultKyma updates the default Kyma CR from the kyma-system namespace based on the Kyma CR from arguments
// adapted from https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L169
func updateDefaultKyma(ctx context.Context, c *KymaModuleClient, obj *KymaCr) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}

	err = c.kube.Update(
		ctx,
		&unstructured.Unstructured{Object: u},
		&client.UpdateOptions{},
	)

	return err
}

// Adds the specified module to the Kyma CR.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L302
func enableModule(kymaCR *KymaCr, moduleName string, moduleChannel string, customResourcePolicy string) *KymaCr {
	for i, m := range kymaCR.Spec.Modules {
		if m.Name == moduleName {
			// module already exists, update
			kymaCR.Spec.Modules[i].Channel = moduleChannel
			kymaCR.Spec.Modules[i].CustomResourcePolicy = customResourcePolicy
			return kymaCR
		}
	}

	kymaCR.Spec.Modules = append(kymaCR.Spec.Modules, Module{
		Name:                 moduleName,
		Channel:              moduleChannel,
		CustomResourcePolicy: customResourcePolicy,
	})

	return kymaCR
}

// Removes the specified module from the Kyma CR.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/kyma.go#L321
func disableModule(kymaCR *KymaCr, moduleName string) *KymaCr {
	kymaCR.Spec.Modules = slices.DeleteFunc(kymaCR.Spec.Modules, func(m Module) bool {
		return m.Name == moduleName
	})

	return kymaCR
}
