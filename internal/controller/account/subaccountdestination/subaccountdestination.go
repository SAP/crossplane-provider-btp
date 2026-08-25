package subaccountdestination

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotSubaccountDestination = "managed resource is not a SubaccountDestination"
	errConnect                  = "while connecting to provider"
	errInvalidExternalName      = "invalid external-name: expected <subaccount-id>/<name>"
	errServiceInstanceNotImpl   = "serviceInstanceId is not yet supported for SubaccountDestination"
	errBuildProps               = "cannot build destination property bag"
	errObserve                  = "while observing destination"
	errCreate                   = "while creating destination"
	errUpdate                   = "while updating destination"
	errDelete                   = "while deleting destination"
	errAlreadyExists            = "destination already exists — set crossplane.io/external-name annotation to adopt the existing resource"
)

// connector builds an ExternalClient on every reconcile.
type connector struct {
	kube            client.Client
	usage           providerconfig.LegacyTracker
	newServiceFn    func(cis, sa []byte) (*btp.Client, error)
	resourcetracker tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestination)
	if !ok {
		return nil, errors.New(errNotSubaccountDestination)
	}

	pc, err := providerconfig.ResolveProviderConfig(ctx, cr, c.kube)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	if err = c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	if err = c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errConnect)
	}

	rawCred, err := providerconfig.LoadDestinationCredentials(ctx, c.kube, pc)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	cred, err := destination.ParseCredential(rawCred)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	destClient, err := destination.NewDestinationClient(cred)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	return &external{kube: c.kube, client: destClient}, nil
}

// external implements managed.ExternalClient.
type external struct {
	kube   client.Client
	client destination.DestinationClientI
}

func (e *external) Disconnect(_ context.Context) error { return nil }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestination)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotSubaccountDestination)
	}

	extName := meta.GetExternalName(cr)
	if extName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err := validateExternalName(extName); err != nil {
		return managed.ExternalObservation{}, err
	}

	destName := strings.SplitN(extName, "/", 2)[1]
	props, etag, err := e.client.Get(ctx, destName)
	if err != nil {
		if destination.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	// Sync observed state into status.
	cr.Status.AtProvider.ETag = &etag
	cr.Status.AtProvider.RawProperties = props
	if v, ok := props["Name"]; ok {
		cr.Status.AtProvider.Name = &v
	}
	if v, ok := props["URL"]; ok {
		cr.Status.AtProvider.URL = &v
	}
	if v, ok := props["Authentication"]; ok {
		cr.Status.AtProvider.Authentication = &v
	}
	if v, ok := props["ProxyType"]; ok {
		cr.Status.AtProvider.ProxyType = &v
	}
	if v, ok := props["Description"]; ok {
		cr.Status.AtProvider.Description = &v
	}

	cr.SetConditions(xpv1.Available())

	desired, err := buildPropertyBag(ctx, e.kube, cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  isUpToDate(desired, props),
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestination)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotSubaccountDestination)
	}

	if cr.Spec.ForProvider.ServiceInstanceID != nil {
		return managed.ExternalCreation{}, errors.New(errServiceInstanceNotImpl)
	}

	// Import scenario: external-name already set by the user before creation.
	extName := meta.GetExternalName(cr)
	if extName != "" && extName != cr.Name {
		if err := validateExternalName(extName); err != nil {
			return managed.ExternalCreation{}, err
		}
		destName := strings.SplitN(extName, "/", 2)[1]
		props, _, err := e.client.Get(ctx, destName)
		if err != nil && !destination.IsNotFound(err) {
			return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
		}
		if props != nil {
			// Resource already exists — adoption successful.
			return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}}, nil
		}
	}

	cr.SetConditions(xpv1.Creating())

	props, err := buildPropertyBag(ctx, e.kube, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}
	if err := e.client.Create(ctx, props); err != nil {
		if destination.IsConflict(err) {
			return managed.ExternalCreation{}, errors.New(errAlreadyExists)
		}
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	if cr.Spec.ForProvider.SubaccountID == nil || *cr.Spec.ForProvider.SubaccountID == "" {
		return managed.ExternalCreation{}, errors.New("subaccountId must be resolved before creating a destination")
	}
	meta.SetExternalName(cr, *cr.Spec.ForProvider.SubaccountID+"/"+cr.Spec.ForProvider.Name)

	return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestination)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotSubaccountDestination)
	}

	props, err := buildPropertyBag(ctx, e.kube, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	etag := ""
	if cr.Status.AtProvider.ETag != nil {
		etag = *cr.Status.AtProvider.ETag
	}
	if err := e.client.Update(ctx, props, etag); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}
	return managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestination)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotSubaccountDestination)
	}

	cr.SetConditions(xpv1.Deleting())

	extName := meta.GetExternalName(cr)
	if extName == "" {
		return managed.ExternalDelete{}, nil
	}
	if err := validateExternalName(extName); err != nil {
		return managed.ExternalDelete{}, err
	}
	destName := strings.SplitN(extName, "/", 2)[1]
	if err := e.client.Delete(ctx, destName); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

// validateExternalName checks the 2-part <subaccount-id>/<name> format.
func validateExternalName(extName string) error {
	if extName == "" {
		return errors.New(errInvalidExternalName + ": empty string")
	}
	parts := strings.SplitN(extName, "/", 3)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.Errorf("%s: got %q", errInvalidExternalName, extName)
	}
	return nil
}

// buildPropertyBag builds the map[string]any sent to the Destination Service API.
// Typed fields are set first; additionalProperties overrides them;
// additionalConfigurationSecretRef contents are merged last.
func buildPropertyBag(ctx context.Context, kube client.Client, cr *v1alpha1.SubaccountDestination) (map[string]any, error) {
	p := cr.Spec.ForProvider
	props := map[string]any{
		"Name": p.Name,
		"Type": p.Type,
	}
	if p.URL != nil {
		props["URL"] = *p.URL
	}
	if p.Authentication != nil {
		props["Authentication"] = *p.Authentication
	}
	if p.ProxyType != nil {
		props["ProxyType"] = *p.ProxyType
	}
	if p.Description != nil {
		props["Description"] = *p.Description
	}
	for k, v := range p.AdditionalProperties {
		props[k] = v
	}
	if p.AdditionalConfigurationSecretRef != nil && kube != nil {
		ref := p.AdditionalConfigurationSecretRef
		var secret corev1.Secret
		if err := kube.Get(ctx, types.NamespacedName{
			Namespace: ref.Namespace,
			Name:      ref.Name,
		}, &secret); err != nil {
			return nil, errors.Wrap(err, errBuildProps)
		}
		data, ok := secret.Data[ref.Key]
		if !ok {
			return nil, errors.Errorf("%s: key %q not found in secret %s/%s", errBuildProps, ref.Key, ref.Namespace, ref.Name)
		}
		var extra map[string]any
		if err := json.Unmarshal(data, &extra); err != nil {
			return nil, errors.Wrap(err, errBuildProps+": cannot unmarshal secret value")
		}
		for k, v := range extra {
			props[k] = v
		}
	}
	return props, nil
}

// isUpToDate returns true if every key in desired exists in observed with the same value.
// Intentionally one-sided: keys present in observed but absent from desired are ignored
// because the API injects read-only fields (CreationTime, User, etc.) that we never set.
func isUpToDate(desired map[string]any, observed map[string]string) bool {
	for k, dv := range desired {
		ov, exists := observed[k]
		if !exists {
			return false
		}
		if ov != fmt.Sprintf("%v", dv) {
			return false
		}
	}
	return true
}
