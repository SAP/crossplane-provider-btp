package kymaserviceinstance

import (
	"context"

	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errFailedParse        = "failed to parse kubeconfig"
	errFailedCreateClient = "failed to create Kubernetes client"
)

// Client is the interface for managing ServiceInstances in Kyma clusters
type Client interface {
	// DescribeInstance gets the current state of a ServiceInstance
	// Returns:
	//   - Observation: current status
	//   - bool: true if external-name was updated/discovered
	//   - error: any error that occurred
	DescribeInstance(ctx context.Context, namespace, name string) (
		*v1alpha1.KymaServiceInstanceObservation,
		bool,
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

func (c *client) DescribeInstance(
	ctx context.Context,
	namespace, name string,
) (*v1alpha1.KymaServiceInstanceObservation, bool, error) {
	// TODO: Implement
	return nil, false, errors.New("not implemented yet")
}

// Create a new client from kubeconfig bytes
func NewKymaServiceInstanceClient(kubeconfigBytes []byte) (Client, error) {
	return nil, errors.New("Not Implemented")
}

// Create a ServiceInstance in Kyma cluster
func (c *client) CreateInstance(ctx context.Context, si *v1alpha1.KymaServiceInstance) error {
	return errors.New("not implemented")
}

// Update a ServiceInstance in Kyma cluster
func (c *client) UpdateInstance(ctx context.Context, si *v1alpha1.KymaServiceInstance) error {
	return errors.New("not implemented")
}

// Delete a ServiceInstance from Kyma cluster
func (c *client) DeleteInstance(ctx context.Context, namespace, name string) error {
	return errors.New("not implemented")
}
