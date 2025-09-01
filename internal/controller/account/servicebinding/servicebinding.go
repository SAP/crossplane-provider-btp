package servicebinding

import (
	"context"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
)

const (
	errNotServiceBinding = "managed resource is not a ServiceBinding custom resource"
	errTrackPCUsage      = "cannot track ProviderConfig usage"
	errGetPC             = "cannot get ProviderConfig"
	errGetCreds          = "cannot get credentials"

	errObserveBinding    = "cannot observe servicebinding"
	errCreateBinding     = "cannot create servicebinding"
	errSaveData          = "cannot update cr data"
	errGetBinding        = "cannot get servicebinding"
	errDeleteExpiredKeys = "cannot delete expired keys in BTP: %w"
	errDeleteRetiredKeys = "cannot delete retired keys in BTP: %w"

	nameHeader = "test.com/test"
)

// SaveConditionsFn Callback for persisting conditions in the CR
var saveCallback tfClient.SaveConditionsFn = func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error {

	si := &v1alpha1.ServiceBinding{}

	nn := types.NamespacedName{Name: name}
	if kErr := kube.Get(ctx, nn, si); kErr != nil {
		return errors.Wrap(kErr, errGetBinding)
	}

	si.SetConditions(conditions...)

	uErr := kube.Status().Update(ctx, si)

	return errors.Wrap(uErr, errSaveData)
}

type connector struct {
	kube  client.Client
	usage resource.Tracker

	clientConnector tfClient.TfProxyConnectorI[*v1alpha1.ServiceBinding]
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return nil, errors.New(errNotServiceBinding)
	}

	// when working with tf proxy resources we want to keep the Connect() logic as part of the delgating Connect calls of the native resources to
	// deal with errors in the part of process that they belong to
	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceBinding))
	if err != nil {
		return nil, err
	}

	mapper := &servicebindingclient.ServiceBindingMapper{}

	rotator := servicebindingclient.NewSBKeyRotator(client.TfClient(), c.kube, mapper)

	return &external{clientConnector: c.clientConnector, kube: c.kube, rotator: &rotator, mapper: mapper}, nil
}

type external struct {
	clientConnector tfClient.TfProxyConnectorI[*v1alpha1.ServiceBinding]
	kube            client.Client
	rotator         *servicebindingclient.SBKeyRotator
	mapper          *servicebindingclient.ServiceBindingMapper
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceBinding)
	}

	addExistingTfSuffix(cr)

	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceBinding))
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	status, details, err := client.Observe(ctx)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBinding)
	}

	if c.rotator.RetireBinding(cr) {
		if err := c.kube.Status().Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errSaveData)
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	switch status {
	case tfClient.NotExisting:
		removeTfSuffix(cr)
		if err := c.kube.Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, err
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	case tfClient.Drift:
		removeTfSuffix(cr)
		if err := c.kube.Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, err
		}
		return managed.ExternalObservation{
			ResourceExists:    true,
			ResourceUpToDate:  false,
			ConnectionDetails: managed.ConnectionDetails{},
		}, nil
	case tfClient.UpToDate:
		if err := c.saveBindingData(ctx, cr, client); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errSaveData)
		}
		cr.SetConditions(xpv1.Available())

		removeTfSuffix(cr)
		if err := c.kube.Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, err
		}

		return managed.ExternalObservation{
			ResourceExists:    true,
			ResourceUpToDate:  true,
			ConnectionDetails: details,
		}, nil
	}
	return managed.ExternalObservation{}, errors.New(errObserveBinding)
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotServiceBinding)
	}

	cr.SetConditions(xpv1.Creating())

	addNewTfSuffix(cr)

	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceBinding))
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	if err := client.Create(ctx); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	removeTfSuffix(cr)
	if err := c.kube.Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, err
	}

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServiceBinding)
	}
	addExistingTfSuffix(cr)
	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceBinding))
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if err := client.Update(ctx); err != nil {
		return managed.ExternalUpdate{}, err
	}

	if cr.Status.AtProvider.RetiredKeys == nil {
		return managed.ExternalUpdate{}, nil
	}
	removeTfSuffix(cr)
	if err := c.kube.Update(ctx, cr); err != nil {
		return managed.ExternalUpdate{}, err
	}

	if newRetiredKeys, err := c.rotator.DeleteExpiredKeys(ctx, cr); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errDeleteExpiredKeys)
	} else {
		cr.Status.AtProvider.RetiredKeys = newRetiredKeys
		return managed.ExternalUpdate{}, err
	}
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotServiceBinding)
	}
	cr.SetConditions(xpv1.Deleting())

	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceBinding))
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	if err := c.rotator.DeleteRetiredKeys(ctx, cr); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteRetiredKeys)
	}

	if err := client.Delete(ctx); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete servicebinding")
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) saveBindingData(ctx context.Context, cr *v1alpha1.ServiceBinding, client tfclient.TfProxyControllerI) error {

	tfResource := client.TfResource()
	if tfResource.GetCondition(ujresource.TypeAsyncOperation).Reason != ujresource.ReasonFinished {
		return nil
	}

	ttf, ok := tfResource.(*v1alpha1.SubaccountServiceBinding)
	if !ok {
		return errors.New("upjetted resource is not a subaccount service binding")
	}

	if meta.GetExternalName(cr) != meta.GetExternalName(ttf) {
		meta.SetExternalName(cr, meta.GetExternalName(ttf))
		// manually saving external-name, since crossplane reconciler won't update spec and status in one loop
		if err := e.kube.Update(ctx, cr); err != nil {
			return err
		}
	}
	// we rely on status being saved in crossplane reconciler here
	cr.Status.AtProvider.ID = *ttf.Status.AtProvider.ID

	layout := "2006-01-02T15:04:05.000Z"
	str := "2014-11-12T11:45:26.371Z"
	t, err := time.Parse(layout, str)
	if err != nil {
		errors.Wrap(err, "createdDate is not a valid RFC3339 time")
	}
	cr.Status.AtProvider.CreatedAt = internal.Ptr(v1.NewTime(t))
	return nil
}

func addNewTfSuffix(sb *v1alpha1.ServiceBinding) {
	name := randomName(sb.Spec.ForProvider.Name)
	sb.SetAnnotations(map[string]string{
		nameHeader: name,
	})
	addExistingTfSuffix(sb)
}

func addExistingTfSuffix(sb *v1alpha1.ServiceBinding) {
	name := sb.GetAnnotations()[nameHeader]
	if name == "" {
		return
	}
	sb.Spec.ForProvider.Name = name
}

func removeTfSuffix(sb *v1alpha1.ServiceBinding) {
	name := sb.GetAnnotations()[nameHeader]
	if name == "" {
		return
	}
	sb.Spec.ForProvider.Name = sb.Spec.ForProvider.Name[:len(sb.Spec.ForProvider.Name)-6]
}
