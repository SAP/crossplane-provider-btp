package btp_subaccount_api_credential

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

func TestDeletionProtectionInitializer_Initialize(t *testing.T) {
	type want struct {
		err error
	}

	cases := map[string]struct {
		cr   *securityv1alpha1.SubaccountApiCredential
		kube client.Client
		want want
	}{
		"no secret ref": {
			cr:   &securityv1alpha1.SubaccountApiCredential{},
			kube: &test.MockClient{},
			want: want{err: nil},
		},
		"secret has client_secret": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ResourceSpec: xpv1.ResourceSpec{
						WriteConnectionSecretToReference: &xpv1.SecretReference{
							Name:      "my-secret",
							Namespace: "default",
						},
					},
				},
			},
			kube: &test.MockClient{
				MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					if secret, ok := obj.(*corev1.Secret); ok {
						secret.Data = map[string][]byte{
							"attribute.client_secret": []byte("my-secret-value"),
						}
					}
					return nil
				}),
			},
			want: want{err: nil},
		},
		"secret missing client_secret with other keys present": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ResourceSpec: xpv1.ResourceSpec{
						WriteConnectionSecretToReference: &xpv1.SecretReference{
							Name:      "my-secret",
							Namespace: "default",
						},
					},
				},
			},
			kube: &test.MockClient{
				MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					if secret, ok := obj.(*corev1.Secret); ok {
						secret.Data = map[string][]byte{
							"attribute.client_id": []byte("some-id"),
							"attribute.token_url": []byte("https://token-url"),
							"attribute.api_url":   []byte("https://api-url"),
						}
					}
					return nil
				}),
			},
			want: want{err: errors.New(errMissingClientSecret)},
		},
		"secret incomplete - only client_id present - no error": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ResourceSpec: xpv1.ResourceSpec{
						WriteConnectionSecretToReference: &xpv1.SecretReference{
							Name:      "my-secret",
							Namespace: "default",
						},
					},
				},
			},
			kube: &test.MockClient{
				MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					if secret, ok := obj.(*corev1.Secret); ok {
						secret.Data = map[string][]byte{
							"attribute.client_id": []byte("some-id"),
						}
					}
					return nil
				}),
			},
			want: want{err: nil},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := &DeletionProtectionInitializer{Kube: tc.kube}
			err := d.Initialize(context.Background(), tc.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Initialize() error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
