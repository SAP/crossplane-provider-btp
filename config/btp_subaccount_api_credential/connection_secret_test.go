package btp_subaccount_api_credential

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

func TestValidateConnectionSecret(t *testing.T) {
	secretRef := func() *securityv1alpha1.SubaccountApiCredential {
		return &securityv1alpha1.SubaccountApiCredential{
			ObjectMeta: metav1.ObjectMeta{Name: "credential"},
			Spec: securityv1alpha1.SubaccountApiCredentialSpec{
				ResourceSpec: resourceSpecWithSecret("connection", "workloads"),
			},
		}
	}

	completeSecret := map[string][]byte{
		connectionSecretAPIURL:       []byte("api-url"),
		connectionSecretClientID:     []byte("client-id"),
		connectionSecretClientSecret: []byte("client-secret"),
		connectionSecretTokenURL:     []byte("token-url"),
	}

	cases := map[string]struct {
		cr         *securityv1alpha1.SubaccountApiCredential
		data       map[string][]byte
		getErr     error
		wantKind   ConnectionSecretErrorKind
		wantFields []string
		wantErr    string
	}{
		"destination Secret absent": {
			cr:       secretRef(),
			getErr:   apierrors.NewNotFound(corev1.Resource("secrets"), "connection"),
			wantKind: ConnectionSecretNotFound,
			wantErr:  `connection Secret "workloads/connection" is missing`,
		},
		"data absent": {
			cr:         secretRef(),
			data:       nil,
			wantKind:   ConnectionSecretMissingField,
			wantFields: []string{connectionSecretAPIURL, connectionSecretClientID, connectionSecretClientSecret, connectionSecretTokenURL},
		},
		"data empty": {
			cr:         secretRef(),
			data:       map[string][]byte{},
			wantKind:   ConnectionSecretMissingField,
			wantFields: []string{connectionSecretAPIURL, connectionSecretClientID, connectionSecretClientSecret, connectionSecretTokenURL},
		},
		"client ID only": {
			cr:         secretRef(),
			data:       map[string][]byte{connectionSecretClientID: []byte("client-id")},
			wantKind:   ConnectionSecretMissingField,
			wantFields: []string{connectionSecretAPIURL, connectionSecretClientSecret, connectionSecretTokenURL},
		},
		"client ID and client secret only": {
			cr:         secretRef(),
			data:       map[string][]byte{connectionSecretClientID: []byte("client-id"), connectionSecretClientSecret: []byte("client-secret")},
			wantKind:   ConnectionSecretMissingField,
			wantFields: []string{connectionSecretAPIURL, connectionSecretTokenURL},
		},
		"unrelated data only": {
			cr:         secretRef(),
			data:       map[string][]byte{"unrelated": []byte("value")},
			wantKind:   ConnectionSecretMissingField,
			wantFields: []string{connectionSecretAPIURL, connectionSecretClientID, connectionSecretClientSecret, connectionSecretTokenURL},
		},
		"exact missing client secret": {
			cr: secretRef(),
			data: map[string][]byte{
				connectionSecretAPIURL:   []byte("api-url"),
				connectionSecretClientID: []byte("client-id"),
				connectionSecretTokenURL: []byte("token-url"),
			},
			wantKind:   ConnectionSecretMissingField,
			wantFields: []string{connectionSecretClientSecret},
		},
		"empty required value": {
			cr: secretRef(),
			data: map[string][]byte{
				connectionSecretAPIURL:       []byte("api-url"),
				connectionSecretClientID:     []byte("client-id"),
				connectionSecretClientSecret: []byte("client-secret"),
				connectionSecretTokenURL:     []byte{},
			},
			wantKind:   ConnectionSecretEmptyField,
			wantFields: []string{connectionSecretTokenURL},
		},
		"complete secret-based Secret": {
			cr:   secretRef(),
			data: completeSecret,
		},
		"certificate Secret without client secret": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := secretRef()
				cr.Spec.ForProvider.CertificatePassed = ptr("certificate")
				return cr
			}(),
			data: map[string][]byte{
				connectionSecretAPIURL:   []byte("api-url"),
				connectionSecretClientID: []byte("client-id"),
				connectionSecretTokenURL: []byte("token-url"),
			},
		},
		"certificate type from status": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := secretRef()
				cr.Status.AtProvider.CredentialType = ptr("Certificates")
				return cr
			}(),
			data: map[string][]byte{
				connectionSecretAPIURL:   []byte("api-url"),
				connectionSecretClientID: []byte("client-id"),
				connectionSecretTokenURL: []byte("token-url"),
			},
		},
		"Secret read failure": {
			cr:       secretRef(),
			getErr:   errors.New("api server unavailable"),
			wantKind: ConnectionSecretReadFailure,
			wantErr:  `cannot read connection Secret "workloads/connection": api server unavailable`,
		},
		"no destination reference": {
			cr: &securityv1alpha1.SubaccountApiCredential{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := &test.MockClient{MockGet: test.NewMockGetFn(tc.getErr, func(obj client.Object) error {
				secret, ok := obj.(*corev1.Secret)
				if !ok {
					t.Fatalf("expected Secret, got %T", obj)
				}
				secret.Data = tc.data
				return nil
			})}

			err := ValidateConnectionSecret(context.Background(), kube, tc.cr)
			if tc.wantErr != "" {
				if diff := cmp.Diff(tc.wantErr, errString(err)); diff != "" {
					t.Errorf("error mismatch (-want +got):\n%s", diff)
				}
			} else if tc.wantKind == "" && err != nil {
				t.Fatalf("ValidateConnectionSecret() unexpected error: %v", err)
			}
			if tc.wantKind != "" {
				var validationErr *ConnectionSecretValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("expected ConnectionSecretValidationError, got %T: %v", err, err)
				}
				if diff := cmp.Diff(tc.wantKind, validationErr.Kind); diff != "" {
					t.Errorf("kind mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantFields, validationErr.Fields); diff != "" {
					t.Errorf("fields mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func resourceSpecWithSecret(name, namespace string) xpv1.ResourceSpec {
	return xpv1.ResourceSpec{WriteConnectionSecretToReference: &xpv1.SecretReference{Name: name, Namespace: namespace}}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
