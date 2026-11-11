package subaccountapicredential

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	upjetresource "github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/pkg/errors"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errCredentialAdoptionIncomplete = "cannot adopt existing SubaccountApiCredential: Terraform state does not provide a complete connection Secret; ensure client_id, client_secret, api_url, and token_url are available, or create a new credential"
	errCredentialIdentityConflict   = "SubaccountApiCredential identity conflicts with the established external identity; restore the established subaccount/name before reconciling"
)

var requiredCredentialConnectionKeys = []string{
	"attribute.client_id",
	"attribute.client_secret",
	"attribute.api_url",
	"attribute.token_url",
}

// newCredentialAdoptionConnector wraps the generated Terraform Framework client
// with the resource-specific complete-connection-data adoption check.
func newCredentialAdoptionConnector(inner managed.ExternalConnector, kube client.Client) managed.ExternalConnector {
	return managed.ExternalConnectorFn(func(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
		cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
		if !ok {
			return inner.Connect(ctx, mg)
		}

		// Pin identity during cleanup too, but do not apply the adoption guard to
		// deleting resources. This prevents cleanup from targeting a different
		// credential if the annotation or spec drifted.
		originalSubaccountID := cr.Spec.ForProvider.SubaccountID
		originalExternalName := meta.GetExternalName(cr)
		if err := pinEstablishedIdentity(cr); err != nil {
			return nil, err
		}
		defer func() {
			cr.Spec.ForProvider.SubaccountID = originalSubaccountID
			meta.SetExternalName(cr, originalExternalName)
		}()

		connectResource := resource.Managed(cr)
		if isSecretCredential(cr) && !meta.WasDeleted(cr) {
			connectResource = &credentialFrameworkState{
				SubaccountApiCredential: cr,
				ctx:                     ctx,
				kube:                    kube,
			}
		}
		connected, err := inner.Connect(ctx, connectResource)
		if err != nil {
			return nil, err
		}
		return &credentialAdoptionExternalClient{ExternalClient: connected}, nil
	})
}

// pinEstablishedIdentity prevents spec or annotation edits from changing the
// identity after Terraform has observed it. Status is the durable framework
// observation; legacy name-only annotations are normalized using that state.
func pinEstablishedIdentity(cr *securityv1alpha1.SubaccountApiCredential) error {
	observedSubaccountID := value(cr.Status.AtProvider.SubaccountID)
	observedName := value(cr.Status.AtProvider.Name)
	if observedSubaccountID == "" || observedName == "" {
		return nil
	}

	annotation := meta.GetExternalName(cr)
	if annotation != "" {
		annotationSubaccountID, annotationName, compound := splitCredentialExternalName(annotation)
		if !compound {
			annotationName = annotation
		}
		if annotationName != observedName || (compound && annotationSubaccountID != observedSubaccountID) {
			return errors.New(errCredentialIdentityConflict)
		}
	}

	// Use the established subaccount while building the Framework client's
	// parameters. Restore the user's spec after Connect has captured them.
	cr.Spec.ForProvider.SubaccountID = &observedSubaccountID
	meta.SetExternalName(cr, observedSubaccountID+"/"+observedName)
	return nil
}

func splitCredentialExternalName(externalName string) (subaccountID, name string, compound bool) {
	for i := 0; i < len(externalName); i++ {
		if externalName[i] == '/' {
			if i == 0 || i == len(externalName)-1 {
				return "", "", false
			}
			return externalName[:i], externalName[i+1:], true
		}
	}
	return "", externalName, false
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func isSecretCredential(cr *securityv1alpha1.SubaccountApiCredential) bool {
	return value(cr.Spec.ForProvider.CertificatePassed) == "" && value(cr.Spec.InitProvider.CertificatePassed) == ""
}

func hasCompleteCredentialConnectionDetails(details managed.ConnectionDetails) bool {
	for _, key := range requiredCredentialConnectionKeys {
		if len(details[key]) == 0 {
			return false
		}
	}
	return true
}

// credentialFrameworkState restores sensitive Terraform observation fields from
// the existing connection Secret when the Framework operation tracker is cold.
// Upjet's CLI connector does this, but its Framework connector currently only
// reconstructs from CR status and spec parameters.
type credentialFrameworkState struct {
	*securityv1alpha1.SubaccountApiCredential
	ctx  context.Context `json:"-"`
	kube client.Client   `json:"-"`
}

// UnwrapManagedResource keeps generic reference tracking on the Kubernetes
// resource, rather than recursively walking this wrapper's runtime helpers.
func (r *credentialFrameworkState) UnwrapManagedResource() resource.Managed {
	return r.SubaccountApiCredential
}

func (r *credentialFrameworkState) GetObservation() (map[string]any, error) {
	observation, err := r.SubaccountApiCredential.GetObservation()
	if err != nil {
		return nil, err
	}
	if err := upjetresource.GetSensitiveObservation(r.ctx, &credentialSecretClient{kube: r.kube}, r.GetWriteConnectionSecretToReference(), observation); err != nil {
		return nil, errors.Wrap(err, "cannot restore sensitive Terraform state from the connection Secret")
	}
	return observation, nil
}

type credentialSecretClient struct {
	kube client.Client
}

func (c *credentialSecretClient) GetSecretData(ctx context.Context, ref *xpv2.SecretReference) (map[string][]byte, error) {
	if ref == nil {
		return nil, nil
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, secret); err != nil {
		return nil, err
	}
	return secret.Data, nil
}

func (c *credentialSecretClient) GetSecretValue(ctx context.Context, selector xpv2.SecretKeySelector) ([]byte, error) {
	data, err := c.GetSecretData(ctx, &selector.SecretReference)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get secret data")
	}
	return data[selector.Key], nil
}

type credentialAdoptionExternalClient struct {
	managed.ExternalClient
}

func (c *credentialAdoptionExternalClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	obs, err := c.ExternalClient.Observe(ctx, mg)
	if err != nil {
		return obs, err
	}
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok || meta.WasDeleted(cr) || !obs.ResourceExists || !isSecretCredential(cr) {
		return obs, nil
	}
	if hasCompleteCredentialConnectionDetails(obs.ConnectionDetails) {
		return obs, nil
	}

	// Upjet eagerly marks any observed Terraform state Available. Undo that
	// status before returning the rejection so an incomplete adoption cannot
	// persist Ready=True.
	cr.SetConditions(xpv2.Unavailable())
	return managed.ExternalObservation{}, errors.New(errCredentialAdoptionIncomplete)
}

func (c *credentialAdoptionExternalClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	created, err := c.ExternalClient.Create(ctx, mg)
	if err != nil {
		return created, err
	}
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok || !isSecretCredential(cr) || hasCompleteCredentialConnectionDetails(created.ConnectionDetails) {
		return created, nil
	}
	cr.SetConditions(xpv2.Unavailable())
	return managed.ExternalCreation{}, errors.Errorf("new secret-based SubaccountApiCredential did not produce a complete connection Secret: %s", errCredentialAdoptionIncomplete)
}

var _ managed.ExternalClient = &credentialAdoptionExternalClient{}
