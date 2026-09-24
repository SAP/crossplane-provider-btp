package subaccountdestination

import (
	"context"
	"fmt"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotSubaccountDestination = "managed resource is not a SubaccountDestination"
	errConnect                  = "while connecting to provider"
	errInvalidExternalName      = "invalid external-name: expected <subaccount-id>/<name>"
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
	resourcetracker tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestination)
	if !ok {
		return nil, errors.New(errNotSubaccountDestination)
	}

	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errConnect)
	}

	if cr.Spec.ForProvider.DestinationServiceBindingSecretRef == nil {
		return nil, errors.Wrap(errors.New("destinationServiceBindingSecretRef must be set"), errConnect)
	}
	rawCred, err := destination.LoadFromSecret(ctx, c.kube, *cr.Spec.ForProvider.DestinationServiceBindingSecretRef)
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
	if v, ok := props["Name"]; ok {
		cr.Status.AtProvider.Name = &v
	}

	cr.SetConditions(xpv1.Available())

	desired, err := buildPropertyBag(ctx, e.kube, cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  isUpToDate(desired, props, cr.Status.AtProvider.ManagedKeys),
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestination)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotSubaccountDestination)
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

	// Validate SubaccountID before calling the API to avoid orphaning a
	// destination that was created in BTP but whose external-name could not
	// be set (which would cause a duplicate-create on the next reconcile).
	if cr.Spec.ForProvider.SubaccountID == nil || *cr.Spec.ForProvider.SubaccountID == "" {
		return managed.ExternalCreation{}, errors.New("subaccountId must be resolved before creating a destination")
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

	meta.SetExternalName(cr, *cr.Spec.ForProvider.SubaccountID+"/"+cr.Spec.ForProvider.Name)
	cr.Status.AtProvider.ManagedKeys = keysOf(props)

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
	cr.Status.AtProvider.ManagedKeys = keysOf(props)
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
	parts := strings.SplitN(extName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.Errorf("%s: got %q", errInvalidExternalName, extName)
	}
	return nil
}

// buildPropertyBag builds the map[string]any sent to the Destination Service API.
// Name and Type are seeded first; additionalProperties is merged next (and can
// override Name/Type if those keys appear); additionalConfigurationSecretRefs
// contents are merged last.
func buildPropertyBag(ctx context.Context, kube client.Client, cr *v1alpha1.SubaccountDestination) (map[string]any, error) {
	p := cr.Spec.ForProvider
	props := map[string]any{
		"Name": p.Name,
		"Type": p.Type,
	}
	for k, v := range p.AdditionalProperties {
		props[k] = v
	}
	if len(p.AdditionalConfigurationSecretRefs) > 0 && kube != nil {
		extra, err := internal.LookupSecrets(ctx, kube, p.AdditionalConfigurationSecretRefs)
		if err != nil {
			return nil, errors.Wrap(err, errBuildProps)
		}
		for k, v := range extra {
			// BTP Destination Service expects all property values as strings
			// (the API spec defines PropertyName as type:string). fmt.Sprintf
			// normalizes any JSON-parsed booleans or numbers to their string form.
			props[k] = fmt.Sprintf("%v", v)
		}
	}
	return props, nil
}

// isUpToDate returns true when desired and observed are in sync for the keys
// this controller manages. managedKeys is the set of property keys last written
// to BTP (stored in status.atProvider.managedKeys after each Create/Update).
//
// Two checks:
//   - every key in desired must exist in observed with the same value
//   - every key in managedKeys must still be in desired (catches removals — a
//     removed key triggers a full-replace PUT that removes it from BTP)
//
// Observed keys that are neither in desired nor in managedKeys are ignored;
// those are server-injected fields that the controller never wrote.
// When managedKeys is empty (first Observe after Create), only the desired→observed
// direction is checked — removal detection begins after the first successful write.
func isUpToDate(desired map[string]any, observed map[string]string, managedKeys []string) bool {
	for k, dv := range desired {
		ov, exists := observed[k]
		if !exists {
			return false
		}
		if ov != fmt.Sprintf("%v", dv) {
			return false
		}
	}
	for _, k := range managedKeys {
		if _, exists := desired[k]; !exists {
			return false
		}
	}
	return true
}

// keysOf returns the keys of a map[string]any as a sorted slice.
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
