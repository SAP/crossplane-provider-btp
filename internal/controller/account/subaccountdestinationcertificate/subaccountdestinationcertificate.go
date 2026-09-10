package subaccountdestinationcertificate

import (
	"context"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"

	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

const (
	errNotSubaccountDestinationCertificate = "managed resource is not a SubaccountDestinationCertificate"
	errConnect                             = "while connecting to provider"
	errInvalidExternalName                 = "invalid external-name: expected <subaccount-id>/<name>"
	errObserve                             = "while observing certificate"
	errCreate                              = "while creating certificate"
	errUpdate                              = "while updating certificate"
	errDelete                              = "while deleting certificate"
	errAlreadyExists                       = "certificate already exists — set crossplane.io/external-name annotation to adopt the existing resource"
)

type connector struct {
	kube            client.Client
	usage           providerconfig.LegacyTracker
	resourcetracker tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationCertificate)
	if !ok {
		return nil, errors.New(errNotSubaccountDestinationCertificate)
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
	certClient, err := destination.NewCertificateClient(cred)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	return &external{client: certClient}, nil
}

type external struct {
	client destination.CertificateClientI
}

func (e *external) Disconnect(_ context.Context) error { return nil }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationCertificate)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotSubaccountDestinationCertificate)
	}

	extName := meta.GetExternalName(cr)
	if extName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err := validateExternalName(extName); err != nil {
		return managed.ExternalObservation{}, err
	}

	certName := strings.SplitN(extName, "/", 2)[1]
	observed, err := e.client.Get(ctx, certName)
	if err != nil {
		if destination.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	name := observed.GetName()
	cr.Status.AtProvider.Name = &name
	cr.Status.AtProvider.Type = observed.Type

	cr.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  isUpToDate(cr, observed),
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationCertificate)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotSubaccountDestinationCertificate)
	}

	if cr.Spec.ForProvider.SubaccountID == nil || *cr.Spec.ForProvider.SubaccountID == "" {
		return managed.ExternalCreation{}, errors.New("subaccountId must be resolved before creating a certificate")
	}

	// certName is the name that will be used both when calling the API and when
	// setting the external-name annotation. If the user pre-set external-name to
	// <subaccount>/<name>, honour that name instead of spec.forProvider.name.
	certName := cr.Spec.ForProvider.Name
	extName := meta.GetExternalName(cr)
	if extName != "" && extName != cr.Name {
		if err := validateExternalName(extName); err != nil {
			return managed.ExternalCreation{}, err
		}
		certName = strings.SplitN(extName, "/", 2)[1]
		existing, err := e.client.Get(ctx, certName)
		if err != nil && !destination.IsNotFound(err) {
			return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
		}
		if existing != nil {
			return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}}, nil
		}
	}

	cr.SetConditions(xpv1.Creating())

	cert := buildCertificateWithName(cr, certName)
	if err := e.client.Create(ctx, cert); err != nil {
		if destination.IsConflict(err) {
			return managed.ExternalCreation{}, errors.New(errAlreadyExists)
		}
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	meta.SetExternalName(cr, *cr.Spec.ForProvider.SubaccountID+"/"+certName)
	return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationCertificate)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotSubaccountDestinationCertificate)
	}

	cert := buildCertificate(cr)
	if err := e.client.Update(ctx, cert); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}
	return managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationCertificate)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotSubaccountDestinationCertificate)
	}

	cr.SetConditions(xpv1.Deleting())

	extName := meta.GetExternalName(cr)
	if extName == "" {
		return managed.ExternalDelete{}, nil
	}
	if err := validateExternalName(extName); err != nil {
		return managed.ExternalDelete{}, err
	}
	certName := strings.SplitN(extName, "/", 2)[1]
	if err := e.client.Delete(ctx, certName); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

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

func buildCertificate(cr *v1alpha1.SubaccountDestinationCertificate) destclient.Certificate {
	return buildCertificateWithName(cr, cr.Spec.ForProvider.Name)
}

func buildCertificateWithName(cr *v1alpha1.SubaccountDestinationCertificate, name string) destclient.Certificate {
	cert := destclient.NewCertificate(name, cr.Spec.ForProvider.Content)
	if cr.Spec.ForProvider.Type != "" {
		cert.Type = &cr.Spec.ForProvider.Type
	}
	return *cert
}

func isUpToDate(cr *v1alpha1.SubaccountDestinationCertificate, observed *destclient.Certificate) bool {
	// BTP's Destination API does not return certificate content in GET responses,
	// so observed.GetContent() is always "". Comparing content would always return
	// false and cause an infinite Update loop — skip it.
	observedType := ""
	if observed.Type != nil {
		observedType = observed.GetType()
	}
	return cr.Spec.ForProvider.Type == observedType
}
