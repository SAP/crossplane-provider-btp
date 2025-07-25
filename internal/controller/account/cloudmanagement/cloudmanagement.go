package cloudmanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	cmclient "github.com/sap/crossplane-provider-btp/internal/clients/cis"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotCloudManagement   = "managed resource is not a CloudManagement custom resource"
	errExtractSecretKey     = "No Service Manager Secret Found"
	errGetCredentialsSecret = "Could not Get Secret"
	errTrackRUsage          = "cannot track ResourceUsage"
	errTrackPCUsage         = "cannot track ProviderConfig usage"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube                client.Client
	usage               resource.Tracker
	resourcetracker     tracking.ReferenceResolverTracker
	newPlanIdResolverFn func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error)

	newClientInitalizerFn func() cmclient.ITfClientInitializer
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)

	if !ok {
		return nil, errors.New(errNotCloudManagement)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	if cr.Spec.ForProvider.ServiceManagerSecret == "" || cr.Spec.ForProvider.ServiceManagerSecretNamespace == "" {
		return nil, errors.New(errExtractSecretKey)
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(
		ctx, types.NamespacedName{
			Namespace: cr.Spec.ForProvider.ServiceManagerSecretNamespace,
			Name:      cr.Spec.ForProvider.ServiceManagerSecret,
		}, secret,
	); err != nil {
		return nil, errors.Wrap(err, errGetCredentialsSecret)
	}

	err := c.InitializeServicePlanId(ctx, cr, secret)
	if err != nil {
		return nil, err
	}

	err = c.ensureCompatibility(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "Error While attempting version migration err")
	}

	tfClientInit := c.newClientInitalizerFn()
	tfClient, err := tfClientInit.ConnectResources(ctx, cr)
	if err != nil {
		return nil, err
	}

	return &external{
		kube:     c.kube,
		tracker:  c.resourcetracker,
		tfClient: tfClient,
	}, nil
}

func (c *connector) ensureCompatibility(ctx context.Context, cr *apisv1beta1.CloudManagement) error {
	if c.migrationNeeded(cr) {
		ctrl.Log.Info(fmt.Sprintf("Migrating external-name to new format for cloudmanagement resource %v", cr.Name))
		meta.SetExternalName(cr,
			formExternalName(
				internal.Val(cr.Status.AtProvider.Instance.Id),
				internal.Val(cr.Status.AtProvider.Binding.Id),
			),
		)
		return c.kube.Update(ctx, cr)
	}
	return nil
}

func (c *connector) migrationNeeded(cr *apisv1beta1.CloudManagement) bool {
	extName := meta.GetExternalName(cr)
	instance := cr.Status.AtProvider.Instance
	binding := cr.Status.AtProvider.Binding

	return !strings.Contains(extName, "/") && instance != nil && binding != nil
}

func (c *connector) IsInitialized(cr *apisv1beta1.CloudManagement) bool {
	return cr.Status.AtProvider.DataSourceLookup != nil
}

// InitializeServicePlanId ensures the service plan id for cis local is cached in status
func (c *connector) InitializeServicePlanId(ctx context.Context, cr *apisv1beta1.CloudManagement, secret *corev1.Secret) error {
	if c.IsInitialized(cr) {
		return nil
	}

	sm, err := c.newPlanIdResolverFn(ctx, secret.Data)
	if err != nil {
		return err
	}

	id, err := sm.PlanIDByName(ctx, "cis", "local")
	if err != nil {
		return err
	}

	return c.saveId(ctx, cr, id)
}

func (c *connector) saveId(ctx context.Context, cr *apisv1beta1.CloudManagement, id string) error {
	cr.Status.AtProvider.DataSourceLookup = &apisv1beta1.CloudManagementDataSourceLookup{
		CloudManagementPlanID: id,
	}
	return c.kube.Status().Update(ctx, cr)
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	kube    client.Client
	tracker tracking.ReferenceResolverTracker

	tfClient cmclient.ITfClient
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotCloudManagement)
	}

	resStatus, err := c.tfClient.ObserveResources(ctx, cr)

	statusErr := c.setStatus(ctx, resStatus, cr)
	if statusErr != nil {
		return managed.ExternalObservation{}, statusErr
	}

	return resStatus.ExternalObservation, err
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotCloudManagement)
	}

	cr.SetConditions(xpv1.Creating())

	sID, bID, err := c.tfClient.CreateResources(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	meta.SetExternalName(cr, formExternalName(sID, bID))

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotCloudManagement)
	}

	err := c.tfClient.UpdateResources(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return errors.New(errNotCloudManagement)
	}

	cr.SetConditions(xpv1.Deleting())

	c.tracker.SetConditions(ctx, cr)

	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return errors.New(providerv1alpha1.ErrResourceInUse)
	}

	return c.tfClient.DeleteResources(ctx, cr)
}

func (c *external) setStatus(ctx context.Context, status cmclient.ResourcesStatus, cr *apisv1beta1.CloudManagement) error {
	if status.ResourceExists {
		cr.Status.SetConditions(xpv1.Available())
		cr.Status.AtProvider.Status = apisv1beta1.CisStatusBound
	} else {
		cr.Status.SetConditions(xpv1.Unavailable())
		cr.Status.AtProvider.Status = apisv1beta1.CisStatusUnbound
	}

	if status.Instance.ID != nil {
		cr.Status.AtProvider.Instance = mapToInstance(&status.Instance)
		cr.Status.AtProvider.ServiceInstanceID = *status.Instance.ID
	}

	if status.Binding.ID != nil {
		cr.Status.AtProvider.Binding = mapToBinding(&status.Binding)
		cr.Status.AtProvider.ServiceBindingID = *status.Binding.ID
	}
	// Unfortunately we need to update the CR status manually here, because the reconciler will drop the change otherwise
	// (I guess because we are attempting to save something while ResourceExists remains false for another cycle)
	return c.kube.Status().Update(ctx, cr)
}

// formExternalName forms an externalName from the given serviceInstanceID and serviceBindingID
func formExternalName(serviceInstanceID, serviceBindingID string) string {
	if serviceBindingID == "" {
		return serviceInstanceID
	}
	return serviceInstanceID + "/" + serviceBindingID
}

func mapToInstance(src *apisv1alpha1.SubaccountServiceInstanceObservation) *v1beta1.Instance {
	if src == nil {
		return nil
	}

	return &apisv1beta1.Instance{
		Id:                   src.ID,
		Ready:                src.Ready,
		Name:                 src.Name,
		ServicePlanId:        src.ServiceplanID,
		PlatformId:           src.PlatformID,
		DashboardUrl:         src.DashboardURL,
		ReferencedInstanceId: src.ReferencedInstanceID,
		Shared:               src.Shared,
		Context:              unmarshalContext(src.Context),
		MaintenanceInfo:      nil,
		Usable:               src.Usable,
		CreatedAt:            src.CreatedDate,
		UpdatedAt:            src.LastModified,
		Labels:               nil,
	}
}

func mapToBinding(src *apisv1alpha1.SubaccountServiceBindingObservation) *apisv1beta1.Binding {
	if src == nil {
		return nil
	}

	return &apisv1beta1.Binding{
		Id:                src.ID,
		Ready:             src.Ready,
		Name:              src.Name,
		ServiceInstanceId: src.ServiceInstanceID,
		Context:           unmarshalContext(src.Context),
		BindResource:      nil,
		CreatedAt:         src.CreatedDate,
		UpdatedAt:         src.LastModified,
		Labels:            nil,
	}
}

func unmarshalContext(src *string) *map[string]string {
	if src == nil {
		return nil
	}

	var contextData map[string]string
	if err := json.Unmarshal([]byte(*src), &contextData); err != nil {
		return nil
	}
	return &contextData
}
