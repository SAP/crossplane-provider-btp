package cloudfoundry

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	env "github.com/sap/crossplane-provider-btp/internal/clients/cfenvironment"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/tracking"

	"github.com/sap/crossplane-provider-btp/btp"
)

const (
	errNotEnvironment          = "managed resource is not a CloudFoundryEnvironment custom resource"
	errExtractSecretKey        = "no Cloud Management Secret Found"
	errGetCredentialsSecret    = "could not get secret of local cloud management"
	errSecretDataInvalid       = "secret spec.Data.__raw is invalid"
	errUpdateNotSupported      = "update not supported"
	errTrackRUsage             = "cannot track ResourceUsage"
	errTrackPCUsage            = "cannot track ProviderConfig usage"
	errCreateConnectionDetails = "Cannot create connection details"
	errDescribeInstance        = "while describing instance"
	errCreate                  = "while creating instance"
	errDelete                  = "while deleting instance"

	errGetPC    = "cannot get ProviderConfig"
	errGetCreds = "cannot get credentials"

	errAdoptLookup  = "cannot look up cloud foundry environment to adopt"
	errAdoptOrgName = "cannot read the org name of the cloud foundry environment to adopt"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           providerconfig.LegacyTracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
	// recorder emits Kubernetes events for adoption. May be nil.
	recorder event.Recorder
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return nil, errors.New(errNotEnvironment)
	}

	lm := mg.(providerconfig.LegacyManaged)

	pc := &providerv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: lm.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	if err := c.usage.Track(ctx, lm); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	if cr.Spec.CloudManagementSecret == "" || cr.Spec.CloudManagementSecretNamespace == "" {
		return nil, errors.New(errExtractSecretKey)
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(
		ctx, types.NamespacedName{
			Namespace: cr.Spec.CloudManagementSecretNamespace,
			Name:      cr.Spec.CloudManagementSecret,
		}, secret,
	); err != nil {
		return nil, errors.Wrap(err, errGetCredentialsSecret)
	}

	cd := pc.Spec.ServiceAccountSecret
	ServiceAccountSecretData, err := resource.CommonCredentialExtractor(
		ctx,
		cd.Source,
		c.kube,
		cd.CommonCredentialSelectors,
	)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	cisBinding := secret.Data[providerv1alpha1.RawBindingKey]
	if cisBinding == nil {
		return nil, errors.New(errGetCredentialsSecret)
	}
	svc, err := c.newServiceFn(cisBinding, ServiceAccountSecretData)

	return &external{client: env.NewCloudFoundryOrganization(*svc), kube: c.kube, recorder: c.recorder}, err
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client env.Client
	kube   client.Client
	// recorder emits Kubernetes events for adoption. May be nil.
	recorder event.Recorder
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

// adopt imports the subaccount's Cloud Foundry environment when the user
// opted in (see package adoption). BTP allows one per subaccount, so it is
// found by type, and every field the spec declares must match it: orgName,
// environmentName and landscape. The provider never updates a Cloud Foundry
// environment, so a declared value that differs could never converge; such a
// match is refused rather than adopted, and nothing is created over it.
// Fields the spec leaves out state no intent and are not compared;
// initialOrgManagers only applies at creation and is never compared. When
// there is no environment, adopt returns nil and Observe goes on to report it
// missing, so it is created.
func (c *external) adopt(ctx context.Context, cr *v1alpha1.CloudFoundryEnvironment) error {
	pending, err := adoption.Pending(cr)
	if err != nil || !pending {
		return err
	}

	instance, found, err := c.client.FindInstance(ctx, *cr)
	if err != nil {
		return errors.Wrap(err, errAdoptLookup)
	}
	if !found {
		return nil
	}

	mismatches, err := identityMismatches(cr.Spec.ForProvider, instance)
	if err != nil {
		return err
	}
	if len(mismatches) > 0 {
		return errIdentityMismatch(instance.GetId(), mismatches)
	}

	return adoption.Commit(ctx, c.kube, c.recorder, cr, instance.GetId(), "the subaccount's cloudfoundry environment")
}

// identityMismatches lists every field fp declares that differs from
// instance. The org name is read from the labels BTP sets on the environment,
// and only when fp declares one.
func identityMismatches(fp v1alpha1.CfEnvironmentParameters, instance provisioningclient.BusinessEnvironmentInstanceResponseObject) ([]string, error) {
	var mismatches []string
	differs := func(field, declared, actual string) {
		if declared != "" && declared != actual {
			mismatches = append(mismatches, fmt.Sprintf("%s: spec declares %q, environment has %q", field, declared, actual))
		}
	}

	if fp.OrgName != "" {
		org, err := btp.NewCloudFoundryOrgByLabel(instance.GetLabels())
		if err != nil {
			return nil, errors.Wrap(err, errAdoptOrgName)
		}
		differs("orgName", fp.OrgName, org.Name)
	}
	differs("environmentName", fp.EnvironmentName, instance.GetName())
	differs("landscape", fp.Landscape, instance.GetLandscapeLabel())
	return mismatches, nil
}

func errIdentityMismatch(id string, mismatches []string) error {
	return errors.Errorf("refusing to adopt cloud foundry environment %s: %s", id, strings.Join(mismatches, "; "))
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotEnvironment)
	}

	if err := c.adopt(ctx, cr); err != nil {
		return managed.ExternalObservation{}, err
	}

	// Check if external-name is empty
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	instance, managers, err := c.client.DescribeInstance(ctx, *cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errDescribeInstance)
	}

	// If instance not found, it's a drift (external resource was deleted)
	if instance == nil {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Set external-name to GUID format if necessary. This means it migrates from old formats
	// Backwards compatibility:  > v1.1.0 (orgName) and v1.0.0 (metadata.name)
	orgName := env.FormOrgName(cr.Spec.ForProvider.OrgName, cr.Spec.SubaccountGuid, cr.Name)
	if externalName == cr.Name || externalName == orgName {
		meta.SetExternalName(cr, *instance.Id)
		if err := c.kube.Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, "failed to update external-name to GUID format")
		}
	}

	cr.Status.AtProvider = env.GenerateObservation(instance, managers)

	if cr.Status.AtProvider.State != nil && *cr.Status.AtProvider.State == v1alpha1.InstanceStateOk {
		cr.Status.SetConditions(xpv1.Available())
	} else {
		cr.Status.SetConditions(xpv1.Unavailable())
	}

	if needsCreation := c.needsCreation(cr); needsCreation {
		return managed.ExternalObservation{
			ResourceExists: !needsCreation,
		}, nil
	}

	details, err := env.GetConnectionDetails(instance)
	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: details,
	}, errors.Wrap(err, errCreateConnectionDetails)
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotEnvironment)
	}

	createdInstanceId, err := c.client.CreateInstance(ctx, *cr)
	if err != nil {
		// Do not set external-name on error (including "already exists" errors)
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	meta.SetExternalName(cr, createdInstanceId)
	if err := c.kube.Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "failed to update external-name with created instance id")
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.x	x
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	_, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotEnvironment)
	}

	// Update is not supported
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotEnvironment)
	}

	cr.SetConditions(xpv1.Deleting())
	// Check if resource is already in deletion state
	if cr.Status.AtProvider.State != nil {
		state := *cr.Status.AtProvider.State
		if state == v1alpha1.InstanceStateDeleting {
			// Already deleting, no need to call delete again
			return managed.ExternalDelete{}, nil
		}
	}

	resp, err := c.client.DeleteInstance(ctx, *cr)
	// Don't treat 404 as error - resource was already deleted externally
	if err != nil && resp != nil && resp.StatusCode == http.StatusNotFound {
		return managed.ExternalDelete{}, nil
	}
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) needsCreation(cr *v1alpha1.CloudFoundryEnvironment) bool {
	return cr.Status.AtProvider.State == nil
}
