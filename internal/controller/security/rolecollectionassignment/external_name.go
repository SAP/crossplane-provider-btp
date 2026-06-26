package rolecollectionassignment

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const (
	externalNameSeparator = "/"
	externalNameMaxLen    = 512
)

var (
	// ErrInvalidExternalName is returned by ParseExternalName when the input
	// does not match the compound-key format.
	ErrInvalidExternalName = errors.New("external-name must be in format 'origin/userOrGroup/roleCollection'")
	// ErrEmptyExternalNameSegment is returned when any segment is empty or has leading/trailing whitespace.
	ErrEmptyExternalNameSegment = errors.New("external-name segment must not be empty or contain leading/trailing whitespace")
	// ErrExternalNameTooLong is returned when the input exceeds externalNameMaxLen.
	ErrExternalNameTooLong = errors.New("external-name exceeds maximum length")
)

// BuildExternalName returns the compound external-name for the given CR in the
// format 'origin/userOrGroupName/roleCollectionName'.
func BuildExternalName(cr *v1alpha1.RoleCollectionAssignment) string {
	return fmt.Sprintf(
		"%s%s%s%s%s",
		cr.Spec.ForProvider.Origin,
		externalNameSeparator,
		IdentifierName(cr),
		externalNameSeparator,
		cr.Spec.ForProvider.RoleCollectionName,
	)
}

// ParseExternalName splits a compound external-name into its three segments.
// It rejects empty segments, segments with leading/trailing whitespace, and
// inputs longer than externalNameMaxLen.
func ParseExternalName(s string) (origin, name, roleCollection string, err error) {
	if len(s) > externalNameMaxLen {
		return "", "", "", ErrExternalNameTooLong
	}
	parts := strings.Split(s, externalNameSeparator)
	if len(parts) != 3 {
		return "", "", "", ErrInvalidExternalName
	}
	for _, p := range parts {
		if p == "" || p != strings.TrimSpace(p) {
			return "", "", "", ErrEmptyExternalNameSegment
		}
	}
	return parts[0], parts[1], parts[2], nil
}

// externalNameSpecMismatch returns a non-empty description when the parsed
// external-name segments don't match the corresponding spec values. Empty
// string means they match. Spec fields are immutable, so a mismatch means an
// import manifest with inconsistent values that the controller cannot
// reconcile.
func externalNameSpecMismatch(cr *v1alpha1.RoleCollectionAssignment, parsedOrigin, parsedName, parsedRoleCollection string) string {
	var mismatches []string
	if parsedOrigin != cr.Spec.ForProvider.Origin {
		mismatches = append(mismatches, fmt.Sprintf("origin mismatch (annotation=%q, spec=%q)", parsedOrigin, cr.Spec.ForProvider.Origin))
	}
	if parsedName != IdentifierName(cr) {
		mismatches = append(mismatches, fmt.Sprintf("userOrGroup mismatch (annotation=%q, spec=%q)", parsedName, IdentifierName(cr)))
	}
	if parsedRoleCollection != cr.Spec.ForProvider.RoleCollectionName {
		mismatches = append(mismatches, fmt.Sprintf("roleCollectionName mismatch (annotation=%q, spec=%q)", parsedRoleCollection, cr.Spec.ForProvider.RoleCollectionName))
	}
	return strings.Join(mismatches, "; ")
}
