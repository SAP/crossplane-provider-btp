package btp_subaccount_api_credential

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

func ptr(s string) *string { return &s }

func TestNameAsExternalName_Initialize(t *testing.T) {
	type want struct {
		err          error
		externalName string
	}

	cases := map[string]struct {
		cr   *securityv1alpha1.SubaccountApiCredential
		kube client.Client
		want want
	}{
		"name set, annotation empty - sets annotation": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
						Name: ptr("my-credential"),
					},
				},
			},
			kube: &test.MockClient{
				MockUpdate: test.NewMockUpdateFn(nil),
			},
			want: want{err: nil, externalName: "my-credential"},
		},
		"name set, annotation already matches - no-op": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							Name: ptr("my-credential"),
						},
					},
				}
				meta.SetExternalName(cr, "my-credential")
				return cr
			}(),
			kube: &test.MockClient{},
			want: want{err: nil, externalName: "my-credential"},
		},
		"name set, annotation differs - deletes secret and updates annotation": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      "my-secret",
								Namespace: "default",
							},
						},
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							Name: ptr("new-credential"),
						},
					},
				}
				meta.SetExternalName(cr, "old-credential")
				return cr
			}(),
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
				MockDelete: test.NewMockDeleteFn(nil),
				MockUpdate: test.NewMockUpdateFn(nil),
			},
			want: want{err: nil, externalName: "new-credential"},
		},
		"name nil - no-op": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{},
				},
			},
			kube: &test.MockClient{},
			want: want{err: nil},
		},
		"name empty string - no-op": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
						Name: ptr(""),
					},
				},
			},
			kube: &test.MockClient{},
			want: want{err: nil},
		},
		"name set, annotation differs, secret Get fails - propagates error": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      "my-secret",
								Namespace: "default",
							},
						},
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							Name: ptr("new-credential"),
						},
					},
				}
				meta.SetExternalName(cr, "old-credential")
				return cr
			}(),
			kube: &test.MockClient{
				MockGet: test.NewMockGetFn(errors.New("api server unavailable")),
			},
			want: want{err: errors.New("api server unavailable"), externalName: "old-credential"},
		},
		"resource being deleted - no-op": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				now := metav1.Now()
				return &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{
						Annotations:       map[string]string{},
						DeletionTimestamp: &now,
					},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							Name: ptr("my-credential"),
						},
					},
				}
			}(),
			kube: &test.MockClient{},
			want: want{err: nil},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			n := &NameAsExternalName{Kube: tc.kube}
			err := n.Initialize(context.Background(), tc.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Initialize() error mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.externalName, meta.GetExternalName(tc.cr)); diff != "" {
				t.Errorf("external-name mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

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
