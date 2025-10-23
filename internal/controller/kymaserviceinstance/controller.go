package kymaserviceinstance

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymaserviceinstance"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errNotKymaServiceInstance  = "managed resource is not a KymaServiceInstance custom resource"
	errTrackUsage              = "cannot track ProviderConfig usage"
	errFetchKubeconfig         = "cannot fetch kubeconfig from KymaEnvironmentBinding"
	errNewClient               = "cannot create new Kyma ServiceInstance client"
	errDescribeInstance        = "cannot describe ServiceInstance"
	errCreateInstance          = "cannot create ServiceInstance"
	errUpdateInstance          = "cannot update ServiceInstance"
	errDeleteInstance          = "cannot delete ServiceInstance"
	errTrackResourceUsage      = "cannot track ResourceUsage"
	errCannotGetKymaBinding    = "cannot get KymaEnvironmentBinding"
	errKymaBindingNotSpecified = "KymaEnvironmentBindingRef must be specified"
)

// connector is expected to produce an ExternalClient when its Connect method is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(kymaEnvironmentKubeconfig []byte) (kymaserviceinstance.Client, error)
}

// external observes, then either creates, updates, or deletes an external resource
type external struct {
	client        kymaserviceinstance.Client
	tracker       tracking.ReferenceResolverTracker
	kube          client.Client
	secretfetcher *SecretFetcher
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.KymaServiceInstance)
	if !ok {
		return nil, errors.New(errNotKymaServiceInstance)
	}
	// Track usage
	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	// Track resource references
	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackResourceUsage)
	}
	// Fetch kubeconfig from KymaEnvironmentBinding secret
	secretFetcher := &SecretFetcher{
		kube: c.kube,
	}
	kubeconfigBytes, err := secretFetcher.Fetch(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, errFetchKubeconfig)
	}

	// Create Client
	svc, err := c.newServiceFn(kubeconfigBytes)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{
		client:        svc,
		tracker:       c.resourcetracker,
		kube:          c.kube,
		secretfetcher: secretFetcher,
	}, nil
}

// Observe the external resource
func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KymaServiceInstance)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKymaServiceInstance)
	}

	if cr.Spec.KymaEnvironmentBindingRef == nil {
		cr.SetConditions(xpv1.Unavailable().WithMessage(errKymaBindingNotSpecified))
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, nil
	}
	// Check if referenced binding exists and is not being deleted
	binding := &v1alpha1.KymaEnvironmentBinding{}
	bindingName := types.NamespacedName{
		Name:      cr.Spec.KymaEnvironmentBindingRef.Name,
		Namespace: cr.GetNamespace(),
	}
	if err := e.kube.Get(ctx, bindingName, binding); err != nil {
		if kerrors.IsNotFound(err) {
			// Binding doesn't exist
			cr.SetConditions(xpv1.Unavailable().WithMessage(
				"Referenced KymaEnvironmentBinding not found",
			))
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errCannotGetKymaBinding)
	}
	// Track if binding is pending deletion
	bindingPendingDeletion := binding.GetDeletionTimestamp() != nil

	// Describe the instance in Kyma
	observation, _, err := e.client.DescribeInstance(ctx, cr.Spec.ForProvider.Namespace, cr.Spec.ForProvider.Name)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errDescribeInstance)
	}
	// Resource doesn't exist
	if observation == nil {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}
	// Update status
	cr.Status.AtProvider = *observation

	// Set conditions
	if observation.Ready == corev1.ConditionTrue {
		if bindingPendingDeletion {
			cr.SetConditions(xpv1.Available().WithMessage(
				"ServiceInstance is available. Warning: Referenced KymaEnvironmentBinding is pending deletion. " +
					"Delete this KymaServiceInstance to allow binding cleanup.",
			))
		} else {
			cr.SetConditions(xpv1.Available())
		}
	} else {
		if bindingPendingDeletion {
			cr.SetConditions(xpv1.Creating().WithMessage(
				"ServiceInstance is being created. Warning: Referenced KymaEnvironmentBinding is pending deletion.",
			))
		} else {
			cr.SetConditions(xpv1.Creating())
		}
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true, // TODO: Implement drift detection
	}, nil
}

// Create the external resource
func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KymaServiceInstance)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKymaServiceInstance)
	}
	cr.SetConditions(xpv1.Creating())

	if err := e.client.CreateInstance(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateInstance)
	}
	// Set external name
	meta.SetExternalName(cr, cr.Spec.ForProvider.Name)

	return managed.ExternalCreation{}, nil
}

// Update the external resource
func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.KymaServiceInstance)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotKymaServiceInstance)
	}
	if err := e.client.UpdateInstance(ctx, cr); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateInstance)
	}
	return managed.ExternalUpdate{}, nil
}

// Delete the external resource
func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.KymaServiceInstance)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotKymaServiceInstance)
	}
	cr.SetConditions(xpv1.Deleting())
	if err := e.client.DeleteInstance(ctx, cr.Spec.ForProvider.Namespace, cr.Spec.ForProvider.Name); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteInstance)
	}

	return managed.ExternalDelete{}, nil
}

// Disconnect is a no-op.
func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
