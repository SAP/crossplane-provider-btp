package globalaccount

import (
	"context"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotGlobalAccount = "managed resource is not a GlobalAccount custom resource"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker
	newServiceFn    func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*apisv1alpha1.GlobalAccount)
	if !ok {
		return nil, errors.New(errNotGlobalAccount)
	}

	btpclient, err := providerconfig.CreateClient(ctx, mg, c.kube, c.usage, c.newServiceFn, c.resourcetracker)
	if err != nil {
		return nil, err
	}

	return &external{
		Client:  c.kube,
		btp:     *btpclient,
		tracker: c.resourcetracker,
	}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	client.Client
	btp     btp.Client
	tracker tracking.ReferenceResolverTracker
}

func (c *external) Disconnect(ctx context.Context) error {
	// No-op
	return nil
}
func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1alpha1.GlobalAccount)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotGlobalAccount)
	}

	c.tracker.SetConditions(ctx, cr)
	// We don't actually do anything with the Global Account. The GlobalAccount CR is more or less only here
	// for logical reasons and to transport the GUID.
	if meta.WasDeleted(cr) {
		if cr.GetCondition(providerv1alpha1.UseCondition).Reason == providerv1alpha1.InUseReason {
			return managed.ExternalObservation{ResourceExists: true}, nil
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	response, _, err := c.btp.AccountsServiceClient.GlobalAccountOperationsAPI.GetGlobalAccount(ctx).Execute()

	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "Get global account request failed.")
	}

	globalAccountGuid := response.Guid
	if globalAccountGuid == "" {
		return managed.ExternalObservation{}, errors.New("BTP Global Account GUID is empty")
	}

	cr.Status.AtProvider.Guid = globalAccountGuid

	cr.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1alpha1.GlobalAccount)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotGlobalAccount)
	}

	_ = fmt.Sprintf("Creating: %+v", cr)

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1alpha1.GlobalAccount)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotGlobalAccount)
	}

	_ = fmt.Sprintf("Updating: %+v", cr)

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*apisv1alpha1.GlobalAccount)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotGlobalAccount)
	}

	c.tracker.SetConditions(ctx, cr)
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	cr.Status.SetConditions(xpv1.Deleting())
	return managed.ExternalDelete{}, nil
}
