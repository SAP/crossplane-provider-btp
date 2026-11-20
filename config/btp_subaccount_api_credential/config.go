package btp_subaccount_api_credential

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/crossplane/upjet/v2/pkg/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	accountsv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errTrackRUsage                     = "cannot track ResourceUsage"
	errTypeAssertion                   = "managed resource is not of type SubaccountApiCredential"
	errMissingClientSecret             = "cannot read client_secret from source, please delete external resource and re-create Crossplane resource"
	errReadConnectionSecret            = "cannot read connection Secret for SubaccountApiCredential"
	errUpdateExternalName              = "cannot update external-name annotation"
	errMissingNameFromState            = "cannot reconstruct external-name: name missing from tfstate"
	errMissingSubaccountIDFromState    = "cannot reconstruct external-name: subaccount_id missing from tfstate"
	errInvalidExternalName             = "invalid external-name annotation for SubaccountApiCredential: expected \"<subaccount-id>/<name>\" or legacy \"<name>\"; segments must be non-empty and must not contain \"/\" or whitespace"
	defaultSubaccountApiCredentialName = "managed-subaccount-api-credential"
	// LegacyManagementApprovalAnnotation is the UID- and identity-bound legacy recovery annotation.
	LegacyManagementApprovalAnnotation = "btp.sap.crossplane.io/legacy-management-approval"
	conditionLegacyManagement          = "LegacyManagementUnresolved"
	conditionFreshManagement           = "FreshManagementEnrollment"
	errLegacyManagementApproval        = "legacy SubaccountApiCredential provenance is unresolved; verify historical provisioning and approve this MR with the UID- and identity-bound legacy-management-approval annotation"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_api_credential", func(r *config.Resource) {
		r.ShortGroup = "security"
		r.Kind = "SubaccountApiCredential"
		r.UseAsync = false

		// Mark all as sensitive to exclude them from the status
		r.TerraformResource.Schema["client_secret"].Sensitive = true
		r.TerraformResource.Schema["client_id"].Sensitive = true
		r.TerraformResource.Schema["token_url"].Sensitive = true
		r.TerraformResource.Schema["api_url"].Sensitive = true
		// key must not be stored in cluster-scoped CR status
		r.TerraformResource.Schema["key"].Sensitive = true

		// ADR(external-name): SubaccountApiCredential is identified by the compound key
		// "<subaccount-id>/<name>". Terraform still expects only the credential name
		// in its "name" argument, so strip the compound-key prefix before rendering
		// Terraform configuration or reconstructed state.
		r.ExternalName.SetIdentifierArgumentFn = setCredentialNameArgument

		r.MetaResource.ArgumentDocs["name"] = "- The name if left unset defaults to managed-subaccount-api-credential"
		if !strings.Contains(r.MetaResource.Description, "Importing or adopting existing API credentials is not supported.") {
			r.MetaResource.Description += ". Importing or adopting existing API credentials is not supported."
		}

		// ADR(external-name): reconstruct the compound key from Terraform state after
		// Create/Observe so Upjet records "<subaccount-id>/<name>" instead of the
		// Terraform resource ID or just the credential name.
		r.ExternalName.GetExternalNameFn = func(tfstate map[string]any) (string, error) {
			return compoundExternalNameFromState(tfstate)
		}

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount", //nolint:staticcheck // hand-written type, not resolvable via TerraformName
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}

		// ADR(external-name): migrate legacy name-only annotations to
		// "<subaccount-id>/<name>" without deleting the existing connection secret.
		// New resources are created without pre-setting external-name; after Create,
		// GetExternalNameFn reconstructs the compound annotation from Terraform state.
		// Enroll before other initializers mutate the MR or reject incomplete Secrets.
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &LegacyManagementEnrollmentInitializer{Kube: kube}
		})
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &CompoundExternalNameInitializer{Kube: kube}
		})
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &LegacyManagementApprovalInitializer{}
		})
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &DeletionProtectionInitializer{Kube: kube}
		})
	})

	p.ConfigureResources()
}

func compoundExternalName(subaccountID, credentialName string) string {
	return fmt.Sprintf("%s/%s", subaccountID, credentialName)
}

func splitCompoundExternalName(externalName string) (subaccountID, credentialName string, ok bool) {
	if strings.Count(externalName, "/") != 1 {
		return "", "", false
	}
	subaccountID, credentialName, _ = strings.Cut(externalName, "/")
	if subaccountID == "" || credentialName == "" {
		return "", "", false
	}
	return subaccountID, credentialName, true
}

func hasWhitespace(value string) bool {
	return strings.IndexFunc(value, unicode.IsSpace) >= 0
}

func validExternalNameSegment(value string) bool {
	return value != "" && !strings.Contains(value, "/") && !hasWhitespace(value)
}

func validateExternalName(externalName string) error {
	if externalName == "" {
		return nil
	}
	if strings.Count(externalName, "/") == 0 {
		if validExternalNameSegment(externalName) {
			return nil
		}
		return errors.New(errInvalidExternalName)
	}
	subaccountID, credentialName, ok := splitCompoundExternalName(externalName)
	if !ok || !validExternalNameSegment(subaccountID) || !validExternalNameSegment(credentialName) {
		return errors.New(errInvalidExternalName)
	}
	return nil
}

func credentialNameFromExternalName(externalName string) string {
	_, credentialName, ok := splitCompoundExternalName(externalName)
	if ok {
		return credentialName
	}
	return externalName
}

func setCredentialNameArgument(base map[string]any, externalName string) {
	if externalName != "" {
		credentialName := credentialNameFromExternalName(externalName)
		if credentialName != "" {
			base["name"] = credentialName
			return
		}
	}
	if name, ok := base["name"].(string); ok && name != "" {
		return
	}
	base["name"] = defaultSubaccountApiCredentialName
}

func stringField(tfstate map[string]any, field string) string {
	value, _ := tfstate[field].(string)
	return value
}

func compoundExternalNameFromState(tfstate map[string]any) (string, error) {
	subaccountID := stringField(tfstate, "subaccount_id")
	if subaccountID == "" {
		return "", errors.New(errMissingSubaccountIDFromState)
	}
	credentialName := stringField(tfstate, "name")
	if credentialName == "" {
		return "", errors.New(errMissingNameFromState)
	}
	return compoundExternalName(subaccountID, credentialName), nil
}

// LegacyManagementEnrollmentInitializer records a durable, UID-bound classification
// before any framework initializer or observation can add historical-looking state.
type LegacyManagementEnrollmentInitializer struct {
	Kube client.Client
}

func (e *LegacyManagementEnrollmentInitializer) Initialize(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok {
		return errors.New(errTypeAssertion)
	}
	if cr.Spec.ForProvider.CertificatePassed != nil || cr.Spec.InitProvider.CertificatePassed != nil {
		return nil
	}
	if lifecycleConditionMatches(cr, conditionFreshManagement) || rejectedAdoptionEvidence(cr) {
		return nil
	}
	if lifecycleConditionMatches(cr, conditionLegacyManagement) {
		return nil
	}

	legacy := !reflect.DeepEqual(cr.Status.AtProvider, securityv1alpha1.SubaccountApiCredentialObservation{}) || len(cr.Status.Conditions) > 0
	if cr.Annotations != nil && (cr.Annotations[meta.AnnotationKeyExternalCreatePending] != "" || cr.Annotations[meta.AnnotationKeyExternalCreateSucceeded] != "" || cr.Annotations[meta.AnnotationKeyExternalCreateFailed] != "") {
		legacy = true
	}
	typeName, reason, message := conditionFreshManagement, "NewManagedResource", "This managed resource was enrolled as fresh for this Kubernetes incarnation"
	if legacy {
		typeName, reason, message = conditionLegacyManagement, "HistoricalProvenanceUnresolved", errLegacyManagementApproval
	}
	condition := xpv1.Condition{Type: xpv1.ConditionType(typeName), Status: "True", LastTransitionTime: metav1.Now(), Reason: xpv1.ConditionReason(reason), Message: string(cr.UID) + ":" + message}
	if e.Kube == nil {
		return errors.New("cannot persist SubaccountApiCredential management classification: Kubernetes client is unavailable")
	}
	before, updated := cr.DeepCopy(), cr.DeepCopy()
	updated.SetConditions(condition)
	if err := e.Kube.Status().Patch(ctx, updated, client.MergeFrom(before)); err != nil {
		return errors.Wrap(err, "cannot persist SubaccountApiCredential management classification")
	}
	cr.SetConditions(condition)
	cr.ResourceVersion = updated.ResourceVersion
	return nil
}

// LegacyManagementApprovalInitializer blocks external reconciliation after
// identity normalization, but before connection-Secret validation or Connect.
type LegacyManagementApprovalInitializer struct{}

func (*LegacyManagementApprovalInitializer) Initialize(_ context.Context, mg resource.Managed) error {
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok {
		return errors.New(errTypeAssertion)
	}
	if !meta.WasDeleted(cr) && lifecycleConditionMatches(cr, conditionLegacyManagement) && !LegacyManagementApproved(cr) {
		return errors.New(errLegacyManagementApproval)
	}
	return nil
}

func lifecycleConditionMatches(cr *securityv1alpha1.SubaccountApiCredential, conditionType string) bool {
	condition := cr.GetCondition(xpv1.ConditionType(conditionType))
	return condition.Status == "True" && strings.HasPrefix(condition.Message, string(cr.UID)+":")
}

// LegacyManagementApproved validates the transient annotation against the current
// MR UID and its established, non-conflicting compound identity.
// LegacyManagementApprovalDiagnostic returns the actionable unresolved-provenance message.
func LegacyManagementApprovalDiagnostic() string { return errLegacyManagementApproval }

// IsLegacyManagementUnresolved reports whether this MR has a current-UID legacy classification.
func IsLegacyManagementUnresolved(cr *securityv1alpha1.SubaccountApiCredential) bool {
	return lifecycleConditionMatches(cr, conditionLegacyManagement)
}

func LegacyManagementApproved(cr *securityv1alpha1.SubaccountApiCredential) bool {
	if cr == nil || cr.UID == "" || !lifecycleConditionMatches(cr, conditionLegacyManagement) || adoptionRejectedForCurrentUID(cr) {
		return false
	}
	subaccountID, name, ok := EstablishedCredentialIdentity(cr)
	if !ok || strings.Contains(subaccountID+name, ":") {
		return false
	}
	return cr.Annotations[LegacyManagementApprovalAnnotation] == string(cr.UID)+":"+subaccountID+"/"+name
}

// EstablishedCredentialIdentity returns the normalized external identity only
// when external-name and effective forProvider/initProvider configuration agree.
// EstablishedCredentialIdentity resolves a normalized identity only when all supplied identity sources agree.
func EstablishedCredentialIdentity(cr *securityv1alpha1.SubaccountApiCredential) (subaccountID, name string, ok bool) {
	if cr == nil {
		return "", "", false
	}
	externalName := meta.GetExternalName(cr)
	if externalName != "" {
		if sid, cname, compound := splitCompoundExternalName(externalName); compound {
			subaccountID, name = sid, cname
		} else {
			name = externalName
		}
	}
	fpSID := specSubaccountID(cr)
	initSID := ""
	if cr.Spec.InitProvider.SubaccountID != nil {
		initSID = *cr.Spec.InitProvider.SubaccountID
	}
	if fpSID != "" && initSID != "" && fpSID != initSID {
		return "", "", false
	}
	if fpSID == "" && initSID != "" {
		fpSID = *cr.Spec.InitProvider.SubaccountID
	}
	if fpSID == "" && cr.Status.AtProvider.SubaccountID != nil {
		fpSID = *cr.Status.AtProvider.SubaccountID
	}
	fpName := ""
	if cr.Spec.ForProvider.Name != nil {
		fpName = *cr.Spec.ForProvider.Name
	}
	initName := ""
	if cr.Spec.InitProvider.Name != nil {
		initName = *cr.Spec.InitProvider.Name
	}
	if fpName != "" && initName != "" && fpName != initName {
		return "", "", false
	}
	if fpName == "" {
		fpName = initName
	}
	if subaccountID == "" {
		subaccountID = fpSID
	}
	if name == "" {
		name = fpName
	}
	if name == "" {
		name = defaultSubaccountApiCredentialName
	}
	if subaccountID == "" || !validExternalNameSegment(subaccountID) || !validExternalNameSegment(name) {
		return "", "", false
	}
	if fpSID != "" && fpSID != subaccountID || fpName != "" && fpName != name {
		return "", "", false
	}
	return subaccountID, name, true
}

func adoptionRejectedForCurrentUID(cr *securityv1alpha1.SubaccountApiCredential) bool {
	condition := cr.GetCondition(xpv1.ConditionType("AdoptionRejected"))
	return condition.Status == "True" && strings.HasPrefix(condition.Message, string(cr.UID)+":")
}

func specSubaccountID(cr *securityv1alpha1.SubaccountApiCredential) string {
	if cr.Spec.ForProvider.SubaccountID != nil && *cr.Spec.ForProvider.SubaccountID != "" {
		return *cr.Spec.ForProvider.SubaccountID
	}
	return ""
}

func subaccountIDForLegacyMigration(cr *securityv1alpha1.SubaccountApiCredential) string {
	if subaccountID := specSubaccountID(cr); subaccountID != "" {
		return subaccountID
	}
	if cr.Status.AtProvider.SubaccountID != nil && *cr.Status.AtProvider.SubaccountID != "" {
		return *cr.Status.AtProvider.SubaccountID
	}
	return ""
}

// CompoundExternalNameInitializer migrates legacy name-only external-name annotations
// to the ADR-compliant compound key "<subaccount-id>/<name>".
type CompoundExternalNameInitializer struct {
	Kube client.Client
}

func (n *CompoundExternalNameInitializer) Initialize(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok {
		return errors.New(errTypeAssertion)
	}

	if meta.WasDeleted(mg) {
		return nil
	}

	currentExternalName := meta.GetExternalName(cr)
	if err := validateExternalName(currentExternalName); err != nil {
		return err
	}
	if _, _, ok := splitCompoundExternalName(currentExternalName); ok {
		return nil
	}

	if currentExternalName == "" {
		return nil
	}

	// Legacy annotations used the credential name only. Treat the existing
	// annotation as authoritative during migration to preserve the currently
	// managed external credential, even if spec.forProvider.name was changed.
	// Status is only a fallback for the subaccount ID during legacy migration,
	// because already-managed resources may have observed state while their
	// resolved spec.forProvider.subaccountId is not populated yet.
	subaccountID := subaccountIDForLegacyMigration(cr)
	if subaccountID == "" {
		return nil
	}
	desiredExternalName := compoundExternalName(subaccountID, currentExternalName)

	meta.SetExternalName(cr, desiredExternalName)
	return errors.Wrap(n.Kube.Update(ctx, cr), errUpdateExternalName)
}

// DeletionProtectionInitializer implements the managed.Initializer interface
type DeletionProtectionInitializer struct {
	Kube client.Client
}

func rejectedAdoptionEvidence(cr *securityv1alpha1.SubaccountApiCredential) bool {
	condition := cr.GetCondition("AdoptionRejected")
	return condition.Status == "True" && strings.HasPrefix(condition.Message, string(cr.UID)+":")
}

// Implement the managed.Initializer interface
func (d *DeletionProtectionInitializer) Initialize(ctx context.Context, mg resource.Managed) error {

	// Default reference tracker for tracking references
	referenceTracker := tracking.NewDefaultReferenceResolverTracker(
		d.Kube,
	)

	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)

	if !ok {
		return errors.New(errTypeAssertion)
	}

	// Manually define reference tracking for relevant fields
	if cr.Spec.ForProvider.SubaccountID != nil && cr.Spec.ForProvider.SubaccountRef != nil {

		// Use a custom reference tracker to track the subaccount reference
		err := referenceTracker.CreateTrackingReference(ctx, cr, *cr.Spec.ForProvider.SubaccountRef, accountsv1alpha1.SubaccountGroupVersionKind)

		if err != nil {
			return errors.Wrap(err, errTrackRUsage)
		}
	}

	if meta.WasDeleted(mg) {

		referenceTracker.SetConditions(ctx, mg)
		if blocked := referenceTracker.DeleteShouldBeBlocked(mg); blocked {
			return errors.New(providerv1alpha1.ErrResourceInUse)
		}
	}

	// Unapproved legacy resources finalize locally; incomplete legacy Secret data
	// must not prevent the connector from choosing its non-destructive path.
	if meta.WasDeleted(mg) && lifecycleConditionMatches(cr, conditionLegacyManagement) && !LegacyManagementApproved(cr) {
		return nil
	}

	// According to the Terraform BTP provider docs, client_secret is only generated
	// "if the certificate is omitted". Certificate-based credentials never have a
	// client_secret, so this check must be skipped for them to avoid false positives.
	// See: https://registry.terraform.io/providers/SAP/btp/latest/docs/resources/subaccount_api_credential
	if cr.Spec.ForProvider.CertificatePassed == nil {
		secretRef := cr.GetWriteConnectionSecretToReference()
		if secretRef != nil && secretRef.Name != "" {
			secret := &corev1.Secret{}
			err := d.Kube.Get(ctx, client.ObjectKey{
				Name:      secretRef.Name,
				Namespace: secretRef.Namespace,
			}, secret)
			if err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrap(err, errReadConnectionSecret)
			}
			if err == nil {
				_, hasClientID := secret.Data["attribute.client_id"]
				_, hasTokenURL := secret.Data["attribute.token_url"]
				_, hasAPIURL := secret.Data["attribute.api_url"]
				_, hasClientSecret := secret.Data["attribute.client_secret"]
				if hasClientID && hasTokenURL && hasAPIURL && !hasClientSecret && (!meta.WasDeleted(mg) || !rejectedAdoptionEvidence(cr)) {
					return errors.New(errMissingClientSecret)
				}
			}
		}
	}
	return nil
}
