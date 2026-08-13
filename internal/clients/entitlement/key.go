package entitlement

import (
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

const (
	externalNameSeparator = "/"
	externalNameMaxLen    = 512
)

var (
	// ErrInvalidExternalName is returned by ParseExternalName when the input
	// does not match the compound-key format.
	ErrInvalidExternalName = errors.New(
		"external-name must be in format 'subaccountGuid/serviceName/servicePlanName[/servicePlanUniqueIdentifier]'",
	)
	// ErrEmptyExternalNameSegment is returned when any segment is empty or has leading/trailing whitespace.
	ErrEmptyExternalNameSegment = errors.New(
		"external-name segment must not be empty or contain leading/trailing whitespace",
	)
	// ErrExternalNameTooLong is returned when the input exceeds externalNameMaxLen.
	ErrExternalNameTooLong = errors.New("external-name exceeds maximum length")
	// ErrExternalNameSpecMismatch is returned by callers wrapping a non-empty
	// Mismatch() result; the immutable identity encoded in the external-name
	// annotation no longer agrees with the CR's spec.
	ErrExternalNameSpecMismatch = errors.New(
		"external-name does not match immutable spec identity",
	)
)

// ExternalNameKey is the parsed identity encoded in an Entitlement's
// crossplane.io/external-name annotation.
type ExternalNameKey struct {
	SubaccountGUID              string
	ServiceName                 string
	ServicePlanName             string
	ServicePlanUniqueIdentifier *string
}

// validateExternalNameSegment rejects empty segments, segments with
// leading/trailing whitespace, and segments that themselves contain the
// separator (which would otherwise be indistinguishable from an additional
// segment once joined).
func validateExternalNameSegment(value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return ErrEmptyExternalNameSegment
	}
	if strings.Contains(value, externalNameSeparator) {
		return ErrInvalidExternalName
	}
	return nil
}

// ParseExternalName splits a compound external-name into its identity
// segments without normalizing the input. It rejects inputs longer than
// externalNameMaxLen, inputs that don't split into 3 or 4 segments, and
// segments that are empty, padded with whitespace, or contain the separator.
func ParseExternalName(value string) (ExternalNameKey, error) {
	if len(value) > externalNameMaxLen {
		return ExternalNameKey{}, ErrExternalNameTooLong
	}
	parts := strings.Split(value, externalNameSeparator)
	if len(parts) != 3 && len(parts) != 4 {
		return ExternalNameKey{}, ErrInvalidExternalName
	}
	for _, part := range parts {
		if err := validateExternalNameSegment(part); err != nil {
			return ExternalNameKey{}, err
		}
	}
	key := ExternalNameKey{
		SubaccountGUID:  parts[0],
		ServiceName:     parts[1],
		ServicePlanName: parts[2],
	}
	if len(parts) == 4 {
		key.ServicePlanUniqueIdentifier = internal.Ptr(parts[3])
	}
	return key, nil
}

// NewExternalNameKey builds the identity key for cr's current spec. Each
// spec field is validated individually so that a slash embedded in a
// three-segment field can never be misread as a legitimate fourth
// segment, then the joined length is checked as ParseExternalName would.
func NewExternalNameKey(cr *v1alpha1.Entitlement) (ExternalNameKey, error) {
	key := ExternalNameKey{
		SubaccountGUID:              cr.Spec.ForProvider.SubaccountGuid,
		ServiceName:                 cr.Spec.ForProvider.ServiceName,
		ServicePlanName:             cr.Spec.ForProvider.ServicePlanName,
		ServicePlanUniqueIdentifier: cr.Spec.ForProvider.ServicePlanUniqueIdentifier,
	}
	segments := []string{key.SubaccountGUID, key.ServiceName, key.ServicePlanName}
	if key.ServicePlanUniqueIdentifier != nil {
		segments = append(segments, *key.ServicePlanUniqueIdentifier)
	}
	for _, segment := range segments {
		if err := validateExternalNameSegment(segment); err != nil {
			return ExternalNameKey{}, err
		}
	}
	if len(key.String()) > externalNameMaxLen {
		return ExternalNameKey{}, ErrExternalNameTooLong
	}
	return key, nil
}

// BuildExternalName returns the compound external-name string for cr's
// current spec.
func BuildExternalName(cr *v1alpha1.Entitlement) (string, error) {
	key, err := NewExternalNameKey(cr)
	if err != nil {
		return "", err
	}
	return key.String(), nil
}

// String renders an already-validated key back into its compound
// external-name form.
func (k ExternalNameKey) String() string {
	parts := []string{k.SubaccountGUID, k.ServiceName, k.ServicePlanName}
	if k.ServicePlanUniqueIdentifier != nil {
		parts = append(parts, *k.ServicePlanUniqueIdentifier)
	}
	return strings.Join(parts, externalNameSeparator)
}

// CacheKey joins the fields shared by every BTP entitlement API request.
// The qualifier is deliberately excluded: it disambiguates plans with the
// same name in the API response, but requests are keyed by subaccount,
// service, and plan name alone.
func (k ExternalNameKey) CacheKey() string {
	return strings.Join([]string{k.SubaccountGUID, k.ServiceName, k.ServicePlanName}, "|")
}

// formatQualifierForMismatch renders a qualifier for a Mismatch() message:
// quoted when present, or the unquoted literal "<unset>" when nil, so a nil
// qualifier is never confused with a present-but-empty one.
func formatQualifierForMismatch(qualifier *string) string {
	if qualifier == nil {
		return "<unset>"
	}
	return fmt.Sprintf("%q", *qualifier)
}

// Mismatch compares k, the identity parsed from the external-name
// annotation, against cr's current (immutable) spec identity. It returns a
// semicolon-delimited, human-readable description of every differing
// component, or "" when they agree.
func (k ExternalNameKey) Mismatch(cr *v1alpha1.Entitlement) string {
	var mismatches []string
	if k.SubaccountGUID != cr.Spec.ForProvider.SubaccountGuid {
		mismatches = append(mismatches, fmt.Sprintf(
			"subaccountGuid mismatch (annotation=%q, spec=%q)",
			k.SubaccountGUID, cr.Spec.ForProvider.SubaccountGuid,
		))
	}
	if k.ServiceName != cr.Spec.ForProvider.ServiceName {
		mismatches = append(mismatches, fmt.Sprintf(
			"serviceName mismatch (annotation=%q, spec=%q)",
			k.ServiceName, cr.Spec.ForProvider.ServiceName,
		))
	}
	if k.ServicePlanName != cr.Spec.ForProvider.ServicePlanName {
		mismatches = append(mismatches, fmt.Sprintf(
			"servicePlanName mismatch (annotation=%q, spec=%q)",
			k.ServicePlanName, cr.Spec.ForProvider.ServicePlanName,
		))
	}
	specQualifier := cr.Spec.ForProvider.ServicePlanUniqueIdentifier
	annotationSet, specSet := k.ServicePlanUniqueIdentifier != nil, specQualifier != nil
	if annotationSet != specSet || (annotationSet && specSet && *k.ServicePlanUniqueIdentifier != *specQualifier) {
		mismatches = append(mismatches, fmt.Sprintf(
			"servicePlanUniqueIdentifier mismatch (annotation=%s, spec=%s)",
			formatQualifierForMismatch(k.ServicePlanUniqueIdentifier), formatQualifierForMismatch(specQualifier),
		))
	}
	return strings.Join(mismatches, "; ")
}
