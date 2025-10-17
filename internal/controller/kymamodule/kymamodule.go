package kymamodule

import (
	"context"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var (
	errNotKymaModule                  = "managed resource is not a KymaModule custom resource"
	errTrackPCUsage                   = "cannot track ProviderConfig usage"
	errTrackRUsage                    = "cannot track ResourceUsage"
	errSetupClient                    = "cannot setup KymaModule client"
	errObserveResource                = "cannot observe KymaModule"
	errKymaEnvironmentBindingNotFound = "cannot get referenced KymaEnvironmentBinding"
	errFailedToAddFinalizer           = "failed to add finalizer to KymaModule"
	errCannotRemoveFinalizer          = "failed to remove finalizer from KymaModule"
	errCannotCleanupEnvironment       = "cannot get KymaEnvironmentBinding for cleanup"
	errCannotUntrackResourceUsage     = "cannot untrack ResourceUsage"
)

const (
	finalizerName = "finalizer.kymamodule.module.btp.sap.crossplane.io"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(kymaEnvironmentKubeconfig []byte) (kymamodule.Client, error)
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client        kymamodule.Client
	tracker       tracking.ReferenceResolverTracker
	kube          client.Client
	secretfetcher SecretFetcherInterface
}

// This methods connects to the Kyma cluster using the kubeconfig from the KymaEnvironmentBinding
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return nil, errors.New(errNotKymaModule)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	secretfetcher := &SecretFetcher{
		kube: c.kube,
	}
	creds, err := secretfetcher.Fetch(ctx, cr)

	if err != nil {
		return nil, errors.Wrap(err, errSetupClient)
	}

	svc, err := c.newServiceFn(creds)

	return &external{
			client:        svc,
			tracker:       c.resourcetracker,
			kube:          c.kube,
			secretfetcher: secretfetcher,
		},
		err
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {

	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKymaModule)
	}
	// Get the reference for the kyma environment binding
	binding := &v1alpha1.KymaEnvironmentBinding{}
	bindingName := types.NamespacedName{
		Name:      cr.Spec.KymaEnvironmentBindingRef.Name,
		Namespace: cr.GetNamespace(),
	}

	if err := c.kube.Get(ctx, bindingName, binding); err != nil {
		if kerrors.IsNotFound(err) {
			// Reference is gone - can't be processed
			cr.SetConditions(xpv1.Unavailable().WithMessage(fmt.Sprintf("Referenced Kyma Environment Binding %s doesn't exist", binding.Name)))
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errKymaEnvironmentBindingNotFound)

	}
	// Add finalizer
	finalizerName := "finalizer.environment.btp.sap.crossplane.io/kyma-module"
	if !meta.FinalizerExists(binding, finalizerName) {
		meta.AddFinalizer(binding, finalizerName)
		if err := c.kube.Update(ctx, binding); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errFailedToAddFinalizer)
		}
	}
	// Track Resource Usage
	if err := c.tracker.Track(ctx, cr); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errTrackRUsage)
	}

	res, err := c.client.ObserveModule(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveResource)
	}

	if res == nil {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKymaModule)
	}

	cr.Status.SetConditions(xpv1.Creating())

	err := c.client.CreateModule(ctx, cr.Spec.ForProvider.Name, *cr.Spec.ForProvider.Channel, *cr.Spec.ForProvider.CustomResourcePolicy)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	meta.SetExternalName(cr, cr.Spec.ForProvider.Name)

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {

	return managed.ExternalUpdate{}, errors.New("Update is not implemented - should not be called, only create")
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotKymaModule)
	}

	if cr.Status.AtProvider.State == v1alpha1.ModuleStateDeleting {
		return managed.ExternalDelete{}, nil
	}

	err := c.client.DeleteModule(ctx, cr.Spec.ForProvider.Name)
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	// Clean up finalizer

	binding := &v1alpha1.KymaEnvironmentBinding{}
	bindingName := types.NamespacedName{
		Name:      cr.Spec.KymaEnvironmentBindingRef.Name,
		Namespace: cr.GetNamespace(),
	}

	bindingErr := c.kube.Get(ctx, bindingName, binding)
	if bindingErr == nil {
		// Binding exists remove finalizer
		finalizerName := "finalizer.environment.btp.sap.crossplane.io/kyma-module"
		if meta.FinalizerExists(binding, finalizerName) {
			meta.RemoveFinalizer(binding, finalizerName)
			if err := c.kube.Update(ctx, binding); err != nil {
				return managed.ExternalDelete{}, errors.Wrap(err, errCannotRemoveFinalizer)
			}
		}
	} else if !kerrors.IsNotFound(err) {
		// Unexpected error
		return managed.ExternalDelete{}, errors.Wrap(err, errCannotCleanupEnvironment)
	}

	return managed.ExternalDelete{}, nil
}
