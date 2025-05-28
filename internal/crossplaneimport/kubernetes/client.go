package kubernetes

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sClient is a wrapper around the Kubernetes client
type K8sClient struct {
	Client client.Client
}

// NewK8sClient creates a new Kubernetes client
func NewK8sClient(kubeConfigPath string, scheme *runtime.Scheme) (*K8sClient, error) {
	// Use the kubeconfig to create a REST config
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, err
	}

	// Create a new k8s client with scheme
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &K8sClient{
		Client: k8sClient,
	}, nil
}

// Create creates a resource in the Kubernetes cluster
func (c *K8sClient) Create(ctx context.Context, obj client.Object) error {
	return c.Client.Create(ctx, obj)
}

// Get gets a resource from the Kubernetes cluster
func (c *K8sClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return c.Client.Get(ctx, key, obj)
}

// Update updates a resource in the Kubernetes cluster
func (c *K8sClient) Update(ctx context.Context, obj client.Object) error {
	return c.Client.Update(ctx, obj)
}

// Delete deletes a resource from the Kubernetes cluster
func (c *K8sClient) Delete(ctx context.Context, obj client.Object) error {
	return c.Client.Delete(ctx, obj)
}
