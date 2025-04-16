package kymaenvironmentbinding

import (
	"context"
	"net/http"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	kymabinding "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironmentbinding"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotKymaEnvironmentBinding = "managed resource is not a KymaEnvironmentBinding custom resource"
	errTrackPCUsage              = "cannot track ProviderConfig usage"
	errGetPC                     = "cannot get ProviderConfig"
	errGetCreds                  = "cannot get credentials"
	errExtractSecretKey          = "No Cloud Management Secret Found"
	errGetCredentialsSecret      = "Could not get secret of local cloud management"
	errTrackRUsage               = "cannot track ResourceUsage"
	errNoSecretsToPublish        = "no secrets to publish, please set the write connection secret reference or publish connection details to reference"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client  kymabinding.Client
	tracker tracking.ReferenceResolverTracker

	httpClient *http.Client
	kube       client.Client
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KymaEnvironmentBinding)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKymaEnvironmentBinding)
	}

	if cr.GetWriteConnectionSecretToReference() == nil && cr.GetPublishConnectionDetailsTo() == nil {
		return managed.ExternalObservation{}, errors.New(errNoSecretsToPublish)
	}

	validBindings, bindings := c.validateBindings(cr)
	cr.Status.AtProvider.Bindings = bindings
	err := c.kube.Status().Update(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if !validBindings {
		return managed.ExternalObservation{ResourceExists: false, ResourceUpToDate: true}, nil
	}
	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: true,
	}, nil
}

func (c *external) validateBindings(cr *v1alpha1.KymaEnvironmentBinding) (bool, []v1alpha1.Binding) {
	bindings := cr.Status.AtProvider.Bindings
	if bindings == nil {
		return false, nil
	}

	hasValidBinding := false
	validBindings := []v1alpha1.Binding{}
	now := time.Now()

	// First pass: deactivate bindings that need rotation or are expired
	for i := range bindings {
		b := &bindings[i]
		if b.IsActive {
			// Check if binding has expired
			if b.ExpiresAt.Time.Before(now) {
				b.IsActive = false
				continue
			}

			// Check if rotation interval has been reached
			deadline := b.CreatedAt.Add(cr.Spec.ForProvider.RotationInterval.Duration)
			if now.After(deadline) {
				b.IsActive = false
			} else {
				hasValidBinding = true
			}
		}
	}

	// Second pass: keep non-expired bindings (active or inactive)
	for _, b := range bindings {
		if !b.ExpiresAt.Time.Before(now) {
			validBindings = append(validBindings, b)
		}
	}

	return hasValidBinding, validBindings
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KymaEnvironmentBinding)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKymaEnvironmentBinding)
	}

	// Initialize status if needed
	if cr.Status.AtProvider.Bindings == nil {
		cr.Status.AtProvider.Bindings = []v1alpha1.Binding{}
	}

	// Check if we have any valid active bindings
	hasValidBinding, validBindings := c.validateBindings(cr)
	cr.Status.AtProvider.Bindings = validBindings

	// If we already have a valid binding, return its details
	if hasValidBinding {
		err := c.kube.Status().Update(ctx, cr)
		if err != nil {
			return managed.ExternalCreation{}, err
		}
	}

	// Create new binding only if we don't have a valid one
	clientBinding, err := c.client.CreateInstance(ctx, *cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	// Create new binding from client binding
	newBinding := v1alpha1.Binding{
		Id:        clientBinding.Metadata.Id,
		IsActive:  true,
		CreatedAt: metav1.NewTime(time.Now().UTC()),
		ExpiresAt: metav1.NewTime(clientBinding.Metadata.ExpiresAt.UTC()),
	}

	// Add new binding to status
	cr.Status.AtProvider.Bindings = append(cr.Status.AtProvider.Bindings, newBinding)
	err = c.kube.Status().Update(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	// Prepare connection details
	connectionDetails := managed.ConnectionDetails{
		"binding_id": []byte(newBinding.Id),
		"expires_at": []byte(newBinding.ExpiresAt.UTC().String()),
		"created_at": []byte(newBinding.CreatedAt.UTC().String()),
		"kubeconfig": []byte(clientBinding.Credentials.Kubeconfig),
	}

	return managed.ExternalCreation{
		ConnectionDetails: connectionDetails,
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {

	return managed.ExternalUpdate{}, errors.New("Update is not implemented - should not be called, only create")
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.KymaEnvironmentBinding)
	if !ok {
		return errors.New(errNotKymaEnvironmentBinding)
	}

	err := c.client.DeleteInstance(ctx, cr)
	err2 := c.kube.Status().Update(ctx, cr)
	if err2 != nil {
		return err2
	}
	return err
}
