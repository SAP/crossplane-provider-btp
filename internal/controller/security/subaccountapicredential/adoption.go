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
func newCredentialAdoptionConnector(inner managed.ExternalConnector, kube client.Client, mergeInitProvider bool) managed.ExternalConnector {
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
		if !meta.WasDeleted(cr) {
			reconstruct, err := shouldReconstructSecretCredentialState(cr, mergeInitProvider)
			if err != nil {
				return nil, errors.Wrap(err, "cannot determine effective credential mode for Terraform state reconstruction")
			}
			if reconstruct {
				connectResource = &credentialFrameworkState{
					SubaccountApiCredential: cr,
					ctx:                     ctx,
					kube:                    kube,
					mergeInitProvider:       mergeInitProvider,
				}
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

// isObservedCertificateCredential recognizes only the provider's actual
// certificate mode. Unknown/missing observed types are not an exemption from
// the connection-data requirement.
func isObservedCertificateCredential(cr *securityv1alpha1.SubaccountApiCredential) bool {
	return value(cr.Status.AtProvider.CredentialType) == "client certificate"
}

// shouldReconstructSecretCredentialState uses observed mode when available,
// falling back to effective intent only while preparing a resource whose mode
// has not yet been observed. The merge flag must match the Framework connector:
// ignored initProvider values cannot suppress Secret-to-state reconstruction.
func shouldReconstructSecretCredentialState(cr *securityv1alpha1.SubaccountApiCredential, mergeInitProvider bool) (bool, error) {
	if credentialType := value(cr.Status.AtProvider.CredentialType); credentialType != "" {
		return !isObservedCertificateCredential(cr), nil
	}

	parameters, err := cr.GetMergedParameters(mergeInitProvider)
	if err != nil {
		return false, err
	}
	certificatePassed, _ := parameters["certificate_passed"].(string)
	return certificatePassed == "", nil
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
	ctx               context.Context `json:"-"`
	kube              client.Client   `json:"-"`
	mergeInitProvider bool            `json:"-"`
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
	if err := upjetresource.GetSensitiveObservation(r.ctx, &credentialSecretClient{
		kube:                  r.kube,
		connectionDetailsKeys: r.GetConnectionDetailsMapping(),
	}, r.GetWriteConnectionSecretToReference(), observation); err != nil {
		return nil, errors.Wrap(err, "cannot restore sensitive Terraform state from the connection Secret")
	}
	// Restoring sensitive fields makes even a new MR's observation nonempty.
	// Upjet then skips copying spec parameters into reconstructed state, so
	// supply the required Read identity explicitly. Use the generated merge
	// semantics with the same management-policy feature flag as the connector.
	// Established observation wins over all configuration and Secret data.
	parameters, err := r.GetMergedParameters(r.mergeInitProvider)
	if err != nil {
		return nil, errors.Wrap(err, "cannot resolve effective credential parameters for reconstructed Terraform state")
	}
	delete(observation, "subaccount_id")
	delete(observation, "name")
	subaccountID := value(r.Status.AtProvider.SubaccountID)
	if subaccountID == "" {
		subaccountID, _ = parameters["subaccount_id"].(string)
	}
	if subaccountID != "" {
		observation["subaccount_id"] = subaccountID
	}
	name := value(r.Status.AtProvider.Name)
	if name == "" {
		name, _ = parameters["name"].(string)
	}
	if name != "" {
		observation["name"] = name
	}
	return observation, nil
}

type credentialSecretClient struct {
	kube                  client.Client
	connectionDetailsKeys map[string]string
}

func (c *credentialSecretClient) GetSecretData(ctx context.Context, ref *xpv2.SecretReference) (map[string][]byte, error) {
	if ref == nil {
		return nil, nil
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, secret); err != nil {
		return nil, err
	}
	// Reconstruct state only from keys this resource declares as sensitive
	// connection details. Extra Secret data remains untouched in Kubernetes and
	// must not be interpreted as arbitrary Terraform attributes.
	data := make(map[string][]byte, len(c.connectionDetailsKeys))
	for terraformAttribute := range c.connectionDetailsKeys {
		key := "attribute." + terraformAttribute
		if value, ok := secret.Data[key]; ok {
			data[key] = value
		}
	}
	return data, nil
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
	if !ok || meta.WasDeleted(cr) || !obs.ResourceExists || isObservedCertificateCredential(cr) {
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
	if !ok || isObservedCertificateCredential(cr) || hasCompleteCredentialConnectionDetails(created.ConnectionDetails) {
		return created, nil
	}
	cr.SetConditions(xpv2.Unavailable())
	return managed.ExternalCreation{}, errors.Errorf("new secret-based SubaccountApiCredential did not produce a complete connection Secret: %s", errCredentialAdoptionIncomplete)
}

var _ managed.ExternalClient = &credentialAdoptionExternalClient{}
