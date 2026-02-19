package kymaserviceinstance

import (
	"context"

	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errFailedParse                        = "failed to parse kubeconfig"
	errFailedCreateClient                 = "failed to create Kubernetes client"
	errFailedToGetServiceInstance         = "failed to get service instance"
	errFailedToConvertToServiceInstance   = "failed to convert ServiceInstance from unstructured"
	errFailedToConvertFromServiceInstance = "failed to convert ServiceInstance to unstructured"
	errFailedToCreateServiceInstance      = "failed to create ServiceInstance %s in %s"
	errFailedToDeleteServiceInstance      = "failed to delete ServiceInstance %s in %s"
	errFailedToGetExistingServiceInstance = "failed to get existing ServiceInstance for update"
	errFailedToUpdateServiceInstance      = "failed to update ServiceInstance %s in %s"
)

// Client is the interface for managing ServiceInstances in Kyma clusters
type Client interface {
	// DescribeInstance gets the current state of a ServiceInstance
	// Returns:
	//   - Observation: current status
	//   - error: any error that occurred
	DescribeInstance(ctx context.Context, namespace, name string) (
		*v1alpha1.KymaServiceInstanceObservation,
		error,
	)

	// CreateInstance creates a new ServiceInstance in Kyma
	CreateInstance(ctx context.Context, cr *v1alpha1.KymaServiceInstance) error

	// UpdateInstance updates an existing ServiceInstance in Kyma
	UpdateInstance(ctx context.Context, cr *v1alpha1.KymaServiceInstance) error

	// DeleteInstance deletes a ServiceInstance from Kyma
	DeleteInstance(ctx context.Context, namespace, name string) error
}

type client struct {
	kube crclient.Client
}

var _ Client = &client{}

// GVK for BTP Service Operator ServiceInstance
var GVKServiceInstance = schema.GroupVersionKind{
	Group:   "services.cloud.sap.com",
	Version: "v1",
	Kind:    "ServiceInstance",
}

// Create a new client from kubeconfig bytes
func NewKymaServiceInstanceClient(kubeconfigBytes []byte) (Client, error) {
	restClient, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		return nil, errors.Wrap(err, errFailedParse)
	}
	kubeClient, err := crclient.New(restClient, crclient.Options{})
	if err != nil {
		return nil, errors.Wrap(err, errFailedCreateClient)
	}
	return &client{kube: kubeClient}, nil
}

func (c *client) DescribeInstance(
	ctx context.Context,
	namespace,
	name string,
) (*v1alpha1.KymaServiceInstanceObservation, error) {
	// Get the ServiceInstance from Kyma
	si, err := c.getServiceInstance(ctx, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, errFailedToGetServiceInstance)
	}

	obs := &v1alpha1.KymaServiceInstanceObservation{
		Ready:      si.Status.Ready,
		InstanceID: si.Status.InstanceID,
		Conditions: extractConditions(si),
	}

	return obs, nil

}

func extractConditions(si *ServiceInstance) []v1alpha1.ServiceInstanceCondition {
	if si.Status.Conditions == nil {
		return nil
	}
	conditions := make([]v1alpha1.ServiceInstanceCondition, 0, len(si.Status.Conditions))
	for _, cond := range si.Status.Conditions {
		conditions = append(conditions, v1alpha1.ServiceInstanceCondition(cond))
	}
	return conditions
}

// Create a ServiceInstance in Kyma cluster
func (c *client) CreateInstance(ctx context.Context, si *v1alpha1.KymaServiceInstance) error {
	// Build the ServiceInstance object
	newSI := &ServiceInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GVKServiceInstance.GroupVersion().String(),
			Kind:       GVKServiceInstance.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      si.Spec.ForProvider.Name,
			Namespace: si.Spec.ForProvider.Namespace,
		},
		Spec: ServiceInstanceSpec{
			ServiceOfferingName: si.Spec.ForProvider.ServiceOfferingName,
			ServicePlanName:     si.Spec.ForProvider.ServicePlanName,
			ExternalName:        si.Spec.ForProvider.ExternalName,
			Parameters:          si.Spec.ForProvider.Parameters,
		},
	}
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newSI)
	if err != nil {
		return errors.Wrap(err, errFailedToConvertFromServiceInstance)
	}
	obj := &unstructured.Unstructured{Object: unstructuredObj}
	// Create in Kyma cluster
	if err := c.kube.Create(ctx, obj); err != nil {
		return errors.Wrapf(err, errFailedToCreateServiceInstance,
			si.Spec.ForProvider.Name, si.Spec.ForProvider.Namespace)
	}
	return nil

}

// Update a ServiceInstance in Kyma cluster
func (c *client) UpdateInstance(ctx context.Context, si *v1alpha1.KymaServiceInstance) error {
	name := si.Spec.ForProvider.Name
	namespace := si.Spec.ForProvider.Namespace
	existing, err := c.getServiceInstance(ctx, namespace, name)
	if err != nil {
		return errors.Wrap(err, errFailedToGetExistingServiceInstance)
	}
	//Update the spec fields
	existing.Spec.ServiceOfferingName = si.Spec.ForProvider.ServiceOfferingName
	existing.Spec.ServicePlanName = si.Spec.ForProvider.ServicePlanName
	existing.Spec.ExternalName = si.Spec.ForProvider.ExternalName
	existing.Spec.Parameters = si.Spec.ForProvider.Parameters

	// Convert to unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(existing)
	if err != nil {
		return errors.Wrap(err, errFailedToConvertFromServiceInstance)
	}
	obj := &unstructured.Unstructured{Object: unstructuredObj}

	// Update in Kyma cluster
	if err := c.kube.Update(ctx, obj); err != nil {
		return errors.Wrapf(err, errFailedToUpdateServiceInstance,
			si.Spec.ForProvider.Name, si.Spec.ForProvider.Namespace)
	}
	return nil
}

// Delete a ServiceInstance from Kyma cluster
func (c *client) DeleteInstance(ctx context.Context, namespace, name string) error {
	// Delete from Kyma cluster
	si := &unstructured.Unstructured{}
	si.SetGroupVersionKind(GVKServiceInstance)
	si.SetName(name)
	si.SetNamespace(namespace)

	if err := c.kube.Delete(ctx, si); err != nil {
		if apierrors.IsNotFound(err) {
			// Already Deleted
			return nil
		}
		return errors.Wrapf(err, errFailedToDeleteServiceInstance, name, namespace)
	}
	return nil
}

func (c *client) getServiceInstance(ctx context.Context, namespace, name string) (*ServiceInstance, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(GVKServiceInstance)

	// Get key from Kyma Cluster
	key := crclient.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}

	if err := c.kube.Get(ctx, key, obj); err != nil {
		return nil, err
	}
	// Convert to typed struct
	si := &ServiceInstance{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, si); err != nil {
		return nil, errors.Wrap(err, errFailedToConvertToServiceInstance)
	}
	return si, nil
}
