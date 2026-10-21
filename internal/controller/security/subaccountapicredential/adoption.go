package subaccountapicredential

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errUnsupportedAdoption     = "adoption of an existing SubaccountApiCredential is not supported"
	conditionAdoptionRejected  = "AdoptionRejected"
	conditionCredentialCreated = "CredentialCreatedByThisMR"
)

// adoptionRejectingConnector decorates the Terraform connector to prevent a
// newly-created managed resource from treating a successful external Read as
// acquisition of an existing credential.
type adoptionRejectingConnector struct {
	managed.ExternalConnector
	Kube client.Client
}

func (c *adoptionRejectingConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	if cr, ok := mg.(*v1alpha1.SubaccountApiCredential); ok && !usesCertificate(cr) && meta.WasDeleted(cr) && !credentialCreatedByThisMR(cr) {
		// Without trusted provider Create evidence, external deletion is not
		// authorized (this includes failed adoption and legacy resources without
		// provenance). Avoid Connect and finalize non-destructively.
		return managed.ExternalClientFns{
			ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
				return managed.ExternalObservation{ResourceExists: false}, nil
			},
			DisconnectFn: func(context.Context) error { return nil },
		}, nil
	}
	client, err := c.ExternalConnector.Connect(ctx, mg)
	if err != nil {
		return nil, err
	}
	return &adoptionRejectingClient{ExternalClient: client, Kube: c.Kube}, nil
}

type adoptionRejectingClient struct {
	managed.ExternalClient
	Kube client.Client
}

func usesCertificate(cr *v1alpha1.SubaccountApiCredential) bool {
	// forProvider takes precedence because it is the effective managed value;
	// initProvider is consulted when the field has not yet been late-initialized.
	if cr.Spec.ForProvider.CertificatePassed != nil {
		return true
	}
	return cr.Spec.InitProvider.CertificatePassed != nil
}

func lifecycleEvidenceMatches(cr *v1alpha1.SubaccountApiCredential, conditionType string) bool {
	condition := cr.GetCondition(xpv1.ConditionType(conditionType))
	return condition.Status == "True" && condition.Message == lifecycleEvidenceMessage(cr)
}

func lifecycleEvidenceMessage(cr *v1alpha1.SubaccountApiCredential) string {
	identity := meta.GetExternalName(cr)
	if identity == "" && cr.Spec.ForProvider.SubaccountID != nil && *cr.Spec.ForProvider.SubaccountID != "" {
		name := "managed-subaccount-api-credential"
		if cr.Spec.ForProvider.Name != nil && *cr.Spec.ForProvider.Name != "" {
			name = *cr.Spec.ForProvider.Name
		}
		identity = *cr.Spec.ForProvider.SubaccountID + "/" + name
	}
	return string(cr.UID) + ":" + identity
}

func adoptionWasRejected(cr *v1alpha1.SubaccountApiCredential) bool {
	condition := cr.GetCondition(xpv1.ConditionType(conditionAdoptionRejected))
	return condition.Status == "True" && strings.HasPrefix(condition.Message, string(cr.UID)+":") && !credentialCreatedByThisMR(cr)
}

func credentialCreatedByThisMR(cr *v1alpha1.SubaccountApiCredential) bool {
	return lifecycleEvidenceMatches(cr, conditionCredentialCreated)
}

func (c *adoptionRejectingClient) persistEvidence(ctx context.Context, cr *v1alpha1.SubaccountApiCredential, conditions ...xpv1.Condition) error {
	before := cr.DeepCopy()
	updated := cr.DeepCopy()
	for _, condition := range conditions {
		if condition.LastTransitionTime.IsZero() {
			condition.LastTransitionTime = metav1.Now()
		}
		updated.SetConditions(condition)
	}
	if c.Kube == nil {
		for _, condition := range conditions {
			cr.SetConditions(condition)
		}
		return nil
	}
	if err := c.Kube.Status().Patch(ctx, updated, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("persist SubaccountApiCredential lifecycle evidence: %w", err)
	}
	// Copy only the persisted lifecycle conditions and resource version back.
	// Keep pending Upjet mutations (external name, observation, etc.) on cr.
	for _, condition := range conditions {
		cr.SetConditions(condition)
	}
	cr.ResourceVersion = updated.ResourceVersion
	return nil
}

func (c *adoptionRejectingClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	created, err := c.ExternalClient.Create(ctx, mg)
	cr, ok := mg.(*v1alpha1.SubaccountApiCredential)
	if !ok || usesCertificate(cr) || err == nil || !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		if ok && err == nil {
			// Record successful creation and invalidate any earlier rejection in
			// the same status patch. A prior collision must not authorize local
			// finalization after this MR has since created the credential.
			if persistErr := c.persistEvidence(ctx, cr,
				xpv1.Condition{Type: conditionCredentialCreated, Status: "True", Reason: "ExternalCreateSucceeded", Message: lifecycleEvidenceMessage(cr)},
				xpv1.Condition{Type: conditionAdoptionRejected, Status: "False", Reason: "ExternalCreateSucceeded", Message: "A credential was successfully created by this managed resource"},
			); persistErr != nil {
				return created, persistErr
			}
		}
		return created, err
	}
	if persistErr := c.persistEvidence(ctx, cr, xpv1.Condition{Type: conditionAdoptionRejected, Status: "True", Reason: "ExternalCreateCollision", Message: lifecycleEvidenceMessage(cr)}); persistErr != nil {
		return created, persistErr
	}
	cr.SetConditions(xpv1.Unavailable().WithMessage(
		errUnsupportedAdoption + "; use a distinct credential name, or remove the existing BTP credential and verify it is absent before recreating this resource",
	))
	return created, fmt.Errorf("%s: external Create reported an existing credential: %w", errUnsupportedAdoption, err)
}

func (c *adoptionRejectingClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.SubaccountApiCredential)
	if !ok || usesCertificate(cr) {
		return c.ExternalClient.Observe(ctx, mg)
	}

	// Provider-written status evidence is bound to this MR incarnation and its
	// current external identity. Crossplane's user-writable tracking annotations
	// are not authorization to adopt an existing credential.
	hasSuccessfulCreate := credentialCreatedByThisMR(cr)
	observation, err := c.ExternalClient.Observe(ctx, mg)
	if err != nil || !observation.ResourceExists || hasSuccessfulCreate || meta.WasDeleted(cr) {
		return observation, err
	}

	// Keep any public connection fields returned by Observe, but never allow a
	// read of an existing credential to become successful acquisition. Repeated
	// observations remain rejected unless this MR has provider-written creation evidence.
	if persistErr := c.persistEvidence(ctx, cr, xpv1.Condition{Type: conditionAdoptionRejected, Status: "True", Reason: "ExistingCredential", Message: lifecycleEvidenceMessage(cr)}); persistErr != nil {
		return observation, persistErr
	}
	cr.SetConditions(xpv1.Unavailable().WithMessage(fmt.Sprintf(
		"%s; use a distinct credential name, or remove the existing BTP credential and verify it is absent before recreating this resource",
		errUnsupportedAdoption,
	)))
	return observation, fmt.Errorf("%s for external identity %q", errUnsupportedAdoption, meta.GetExternalName(cr))
}
