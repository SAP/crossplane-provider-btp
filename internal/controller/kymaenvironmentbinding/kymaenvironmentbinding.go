package kymaenvironmentbinding

import (
	"context"
	"math"
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

	err := c.updateBindingsFromService(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	validBindings, bindings := c.validateBindings(cr)
	cr.Status.AtProvider.Bindings = bindings
	_ = c.kube.Status().Update(ctx, cr)
	if !validBindings {
		return managed.ExternalObservation{ResourceExists: false, ResourceUpToDate: true}, nil
	}
	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (c *external) updateBindingsFromService(ctx context.Context, cr *v1alpha1.KymaEnvironmentBinding) error {
	bindingsAtService, err := c.client.DescribeInstance(ctx, cr.Spec.KymaInstanceId)
	if err != nil {
		return err
	}

	validBindings := []v1alpha1.Binding{}

	for _, b := range cr.Status.AtProvider.Bindings {
		found := false
		for _, bs := range bindingsAtService {
			if b.Id == *bs.BindingId {
				found = true
				break
			}
		}
		if found {
			validBindings = append(validBindings, b)
		}
	}

	// Update the bindings with the valid ones
	cr.Status.AtProvider.Bindings = validBindings
	return nil
}

// validateBindings checks if bindings in status are still active (did not reach rotation deadline) or not yet expired (reached time to live)
func (c *external) validateBindings(cr *v1alpha1.KymaEnvironmentBinding) (bool, []v1alpha1.Binding) {
	bindings := cr.Status.AtProvider.Bindings
	if bindings == nil {
		return false, nil
	}

	hasActiveBinding := false
	validBindings := []v1alpha1.Binding{}
	now := time.Now()

	// First pass: deactivate bindings that need rotation or are expired
	for i := range bindings {
		b := &bindings[i]
		if b.IsActive {
			// Check if binding has expired, might happen if rotation deadline is exceeded and has not been reconciled for a while
			if ttlIsExpired(b, now) {
				b.IsActive = false
				continue
			}

			// Check if rotation interval has been reached
			if reachedRotationDeadline(now, b, cr) {
				b.IsActive = false
			} else {
				hasActiveBinding = true
			}
		}
	}

	// Second pass: keep non-expired bindings (active or inactive)
	for _, b := range bindings {
		if !ttlIsExpired(&b, now) {
			validBindings = append(validBindings, b)
		}
	}

	return hasActiveBinding, validBindings
}

func reachedRotationDeadline(now time.Time, b *v1alpha1.Binding, cr *v1alpha1.KymaEnvironmentBinding) bool {
	deadline := b.CreatedAt.Add(cr.Spec.ForProvider.RotationInterval.Duration)
	return now.After(deadline)
}

func ttlIsExpired(b *v1alpha1.Binding, now time.Time) bool {
	return b.ExpiresAt.Time.Before(now)
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

	// Create new binding only if we don't have a valid one
	ttl := int(math.Round(cr.Spec.ForProvider.BindingTTl.Seconds()))
	clientBinding, err := c.client.CreateInstance(ctx, cr.Spec.KymaInstanceId, ttl)
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
	// Prepare connection details
	connectionDetails := managed.ConnectionDetails{
		"binding_id": []byte(newBinding.Id),
		"expires_at": []byte(newBinding.ExpiresAt.UTC().String()),
		"created_at": []byte(newBinding.CreatedAt.UTC().String()),
		"kubeconfig": []byte(clientBinding.Credentials.Kubeconfig),
	}

	return managed.ExternalCreation{
		ConnectionDetails: connectionDetails,
	}, c.kube.Status().Update(ctx, cr)
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {

	return managed.ExternalUpdate{}, errors.New("Update is not implemented - should not be called, only create")
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.KymaEnvironmentBinding)
	if !ok {
		return errors.New(errNotKymaEnvironmentBinding)
	}

	err := c.client.DeleteInstances(ctx, cr.Status.AtProvider.Bindings, cr.Spec.KymaInstanceId)
	return err
}
