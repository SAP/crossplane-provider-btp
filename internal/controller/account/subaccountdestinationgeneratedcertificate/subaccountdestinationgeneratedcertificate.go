package subaccountdestinationgeneratedcertificate

import (
	"context"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotKind             = "managed resource is not a SubaccountDestinationGeneratedCertificate"
	errConnect             = "while connecting to provider"
	errInvalidExternalName = "invalid external-name: expected <subaccount-id>/<name>"
	errObserve             = "while observing generated certificate"
	errCreate              = "while generating certificate"
	errDelete              = "while deleting generated certificate"
	errAlreadyExists       = "certificate already exists — set crossplane.io/external-name annotation to adopt the existing resource"
	errUnsupportedName     = "certificate name %q must end in .pem or .p12"
	errLoadPassword        = "while loading certificate password secret"
	errParseLeaf           = "cannot read leaf certificate from publicKeys"
)

type connector struct {
	kube            client.Client
	usage           providerconfig.LegacyTracker
	resourcetracker tracking.ReferenceResolverTracker
	// recorder emits Kubernetes events for binding-check warnings. May be nil.
	recorder event.Recorder
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationGeneratedCertificate)
	if !ok {
		return nil, errors.New(errNotKind)
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
	sess := destination.NewSession(cred)
	if err := destination.VerifyBinding(sess, cr, cr.Spec.ForProvider.SubaccountID, c.recorder); err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	return &external{kube: c.kube, client: sess.GeneratedCertificateClient(), now: time.Now}, nil
}

type external struct {
	kube   client.Client
	client destination.GeneratedCertificateClientI
	now    func() time.Time
}

func (e *external) Disconnect(_ context.Context) error { return nil }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationGeneratedCertificate)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKind)
	}
	extName := meta.GetExternalName(cr)
	if extName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	certName, err := certNameFrom(extName)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	keys, err := e.client.PublicKeys(ctx, certName)
	if err != nil {
		if destination.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	cr.Status.AtProvider.Name = &certName
	exists := managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{}}

	leaf, err := parseLeaf(keys)
	if err != nil {
		// Report, don't fail: an Observe error would block deletion.
		cr.Status.AtProvider.Certificate = nil
		cr.SetConditions(xpv1.Unavailable().WithMessage(errors.Wrap(err, errParseLeaf).Error()))
		return exists, nil
	}
	cr.Status.AtProvider.Certificate = observe(leaf)
	if leaf.NotAfter.Before(e.now()) {
		cr.SetConditions(xpv1.Unavailable().WithMessage("certificate expired at " + leaf.NotAfter.UTC().Format(time.RFC3339)))
	} else {
		cr.SetConditions(xpv1.Available())
	}
	return exists, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationGeneratedCertificate)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKind)
	}
	p := cr.Spec.ForProvider
	if p.SubaccountID == nil || *p.SubaccountID == "" {
		return managed.ExternalCreation{}, errors.New("subaccountId must be resolved before creating a certificate")
	}

	// Honour a pre-set external-name <subaccount>/<name>. No GET: Observe
	// already established that the certificate doesn't exist, and GET returns
	// the private key material.
	certName := p.Name
	if extName := meta.GetExternalName(cr); extName != "" {
		n, err := certNameFrom(extName)
		if err != nil {
			return managed.ExternalCreation{}, err
		}
		certName = n
	}
	if !strings.HasSuffix(certName, ".pem") && !strings.HasSuffix(certName, ".p12") {
		return managed.ExternalCreation{}, errors.Errorf(errUnsupportedName, certName)
	}

	attrs, err := e.attributes(ctx, p, certName)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	cr.SetConditions(xpv1.Creating())
	if err := e.client.Generate(ctx, attrs); err != nil {
		if destination.IsConflict(err) {
			return managed.ExternalCreation{}, errors.New(errAlreadyExists)
		}
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}
	meta.SetExternalName(cr, *p.SubaccountID+"/"+certName)
	return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

// Update never calls the API: a generated certificate can't be changed
// without turning it into an uploaded one (spec immutability is enforced by CEL).
func (e *external) Update(_ context.Context, _ resource.Managed) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.SubaccountDestinationGeneratedCertificate)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotKind)
	}
	cr.SetConditions(xpv1.Deleting())
	extName := meta.GetExternalName(cr)
	if extName == "" {
		return managed.ExternalDelete{}, nil
	}
	certName, err := certNameFrom(extName)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	if err := e.client.Delete(ctx, certName); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

// attributes builds the POST body. AutomaticRenew is always set: the vendored
// ToMap drops a nil AutomaticRenew.
func (e *external) attributes(ctx context.Context, p v1alpha1.SubaccountDestinationGeneratedCertificateParameters, certName string) (destclient.Attributes, error) {
	a := destclient.NewAttributesAttributes()
	a.AutomaticRenew = new(p.AutomaticRenew != nil && *p.AutomaticRenew)
	a.CN = p.CommonName
	if v := p.Validity; v != nil {
		a.Validity = &destclient.AttributesAttributesValidity{TimeUnit: new(v.TimeUnit), Value: new(v.Value)}
	}
	if ref := p.PasswordSecretRef; ref != nil {
		pw, err := e.password(ctx, *ref)
		if err != nil {
			return destclient.Attributes{}, err
		}
		a.Password = &pw
	}
	attrs := destclient.NewAttributes(certName, *a)
	if strings.HasSuffix(certName, ".p12") {
		attrs.Type = new("P12")
	}
	return *attrs, nil
}

func (e *external) password(ctx context.Context, ref xpv1.SecretKeySelector) (string, error) {
	s := &corev1.Secret{}
	if err := e.kube.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, s); err != nil {
		return "", errors.Wrap(err, errLoadPassword)
	}
	v, ok := s.Data[ref.Key]
	if !ok {
		return "", errors.Wrap(errors.Errorf("key %q not found in secret %s/%s", ref.Key, ref.Namespace, ref.Name), errLoadPassword)
	}
	if len(v) == 0 {
		return "", errors.Wrap(errors.Errorf("key %q in secret %s/%s is empty", ref.Key, ref.Namespace, ref.Name), errLoadPassword)
	}
	return string(v), nil
}

func certNameFrom(extName string) (string, error) {
	parts := strings.SplitN(extName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.Errorf("%s: got %q", errInvalidExternalName, extName)
	}
	return parts[1], nil
}
