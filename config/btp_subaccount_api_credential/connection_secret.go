package btp_subaccount_api_credential

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const (
	connectionSecretAPIURL       = "attribute.api_url"
	connectionSecretClientID     = "attribute.client_id"
	connectionSecretClientSecret = "attribute.client_secret"
	connectionSecretTokenURL     = "attribute.token_url"
)

// ConnectionSecretErrorKind identifies the class of a connection Secret
// validation failure. It deliberately contains no Secret data.
type ConnectionSecretErrorKind string

const (
	ConnectionSecretNotFound     ConnectionSecretErrorKind = "NotFound"
	ConnectionSecretReadFailure  ConnectionSecretErrorKind = "ReadFailure"
	ConnectionSecretMissingField ConnectionSecretErrorKind = "MissingField"
	ConnectionSecretEmptyField   ConnectionSecretErrorKind = "EmptyField"
)

// ConnectionSecretValidationError reports a structural connection Secret
// problem. Fields contains key names only; Secret values are never included.
type ConnectionSecretValidationError struct {
	Kind      ConnectionSecretErrorKind
	Namespace string
	Name      string
	Fields    []string
	Err       error
}

func (e *ConnectionSecretValidationError) Error() string {
	secretName := fmt.Sprintf("%s/%s", e.Namespace, e.Name)
	switch e.Kind {
	case ConnectionSecretNotFound:
		return fmt.Sprintf("connection Secret %q is missing", secretName)
	case ConnectionSecretReadFailure:
		return fmt.Sprintf("cannot read connection Secret %q: %v", secretName, e.Err)
	case ConnectionSecretMissingField:
		return fmt.Sprintf("connection Secret %q is incomplete: missing required field(s): %s", secretName, strings.Join(e.Fields, ", "))
	case ConnectionSecretEmptyField:
		return fmt.Sprintf("connection Secret %q is incomplete: required field(s) are empty: %s", secretName, strings.Join(e.Fields, ", "))
	default:
		return fmt.Sprintf("connection Secret %q failed validation", secretName)
	}
}

func (e *ConnectionSecretValidationError) Unwrap() error { return e.Err }

// RequiredConnectionSecretFields returns the fields required by the
// credential type represented by cr. Certificate credentials intentionally do
// not require attribute.client_secret.
func RequiredConnectionSecretFields(cr *securityv1alpha1.SubaccountApiCredential) []string {
	fields := []string{
		connectionSecretAPIURL,
		connectionSecretClientID,
		connectionSecretTokenURL,
	}
	if !isCertificateCredential(cr) {
		fields = append(fields, connectionSecretClientSecret)
	}
	sort.Strings(fields)
	return fields
}

func isCertificateCredential(cr *securityv1alpha1.SubaccountApiCredential) bool {
	if cr.Spec.ForProvider.CertificatePassed != nil {
		return true
	}
	// This fallback protects certificate resources reconstructed from state
	// where the creation-only spec field is not available.
	if cr.Status.AtProvider.CredentialType != nil {
		switch strings.ToLower(strings.TrimSpace(*cr.Status.AtProvider.CredentialType)) {
		case "certificate", "certificates":
			return true
		}
	}
	return false
}

// ValidateConnectionSecret verifies only the shape of the configured
// connection Secret. It has no BTP side effects and never modifies the Secret.
// A missing destination reference means that there is no Secret to validate;
// Crossplane will consequently not publish connection details for this
// resource.
func ValidateConnectionSecret(ctx context.Context, kube client.Client, cr *securityv1alpha1.SubaccountApiCredential) error {
	ref := cr.GetWriteConnectionSecretToReference()
	if ref == nil || ref.Name == "" {
		return nil
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}
	if err := kube.Get(ctx, key, secret); err != nil {
		kind := ConnectionSecretReadFailure
		if apierrors.IsNotFound(err) {
			kind = ConnectionSecretNotFound
		}
		return &ConnectionSecretValidationError{
			Kind:      kind,
			Namespace: ref.Namespace,
			Name:      ref.Name,
			Err:       err,
		}
	}

	var missing, empty []string
	for _, field := range RequiredConnectionSecretFields(cr) {
		value, ok := secret.Data[field]
		switch {
		case !ok:
			missing = append(missing, field)
		case len(value) == 0:
			empty = append(empty, field)
		}
	}
	if len(missing) > 0 {
		return &ConnectionSecretValidationError{
			Kind:      ConnectionSecretMissingField,
			Namespace: ref.Namespace,
			Name:      ref.Name,
			Fields:    missing,
		}
	}
	if len(empty) > 0 {
		return &ConnectionSecretValidationError{
			Kind:      ConnectionSecretEmptyField,
			Namespace: ref.Namespace,
			Name:      ref.Name,
			Fields:    empty,
		}
	}
	return nil
}
