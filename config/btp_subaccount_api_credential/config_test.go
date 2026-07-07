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

func TestCredentialNameFromExternalName(t *testing.T) {
	cases := map[string]struct {
		externalName string
		want         string
	}{
		"compound":       {externalName: "subaccount-guid/my-credential", want: "my-credential"},
		"legacy name":    {externalName: "my-credential", want: "my-credential"},
		"empty":          {externalName: "", want: ""},
		"invalid prefix": {externalName: "/my-credential", want: "/my-credential"},
		"invalid suffix": {externalName: "subaccount-guid/", want: "subaccount-guid/"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, credentialNameFromExternalName(tc.externalName)); diff != "" {
				t.Errorf("credentialNameFromExternalName() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSetCredentialNameArgument(t *testing.T) {
	cases := map[string]struct {
		base         map[string]any
		externalName string
		want         map[string]any
	}{
		"compound external-name overrides name with credential segment": {
			base:         map[string]any{"name": "spec-name"},
			externalName: "subaccount-guid/annotation-name",
			want:         map[string]any{"name": "annotation-name"},
		},
		"legacy external-name overrides name": {
			base:         map[string]any{"name": "spec-name"},
			externalName: "legacy-name",
			want:         map[string]any{"name": "legacy-name"},
		},
		"empty external-name preserves spec name": {
			base:         map[string]any{"name": "spec-name"},
			externalName: "",
			want:         map[string]any{"name": "spec-name"},
		},
		"empty external-name defaults missing name": {
			base:         map[string]any{},
			externalName: "",
			want:         map[string]any{"name": defaultSubaccountApiCredentialName},
		},
		"empty external-name defaults empty name": {
			base:         map[string]any{"name": ""},
			externalName: "",
			want:         map[string]any{"name": defaultSubaccountApiCredentialName},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			setCredentialNameArgument(tc.base, tc.externalName)
			if diff := cmp.Diff(tc.want, tc.base); diff != "" {
				t.Errorf("setCredentialNameArgument() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCompoundExternalNameFromState(t *testing.T) {
	type want struct {
		externalName string
		err          error
	}

	cases := map[string]struct {
		state map[string]any
		want  want
	}{
		"subaccount id and name present": {
			state: map[string]any{"subaccount_id": "subaccount-guid", "name": "my-credential"},
			want:  want{externalName: "subaccount-guid/my-credential"},
		},
		"missing subaccount id": {
			state: map[string]any{"name": "my-credential"},
			want:  want{err: errors.New(errMissingSubaccountIDFromState)},
		},
		"missing name": {
			state: map[string]any{"subaccount_id": "subaccount-guid"},
			want:  want{err: errors.New(errMissingNameFromState)},
		},
		"wrong field types": {
			state: map[string]any{"subaccount_id": 42, "name": "my-credential"},
			want:  want{err: errors.New(errMissingSubaccountIDFromState)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := compoundExternalNameFromState(tc.state)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("compoundExternalNameFromState() error mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.externalName, got); diff != "" {
				t.Errorf("compoundExternalNameFromState() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCompoundExternalNameInitializer_Initialize(t *testing.T) {
	type want struct {
		err          error
		externalName string
	}

	cases := map[string]struct {
		cr   *securityv1alpha1.SubaccountApiCredential
		kube client.Client
		want want
	}{
		"annotation empty - no-op even when spec has compound key parts": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
						SubaccountID: ptr("subaccount-guid"),
						Name:         ptr("my-credential"),
					},
				},
			},
			kube: &test.MockClient{},
			want: want{},
		},
		"annotation empty - does not reconstruct from status": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				Status: securityv1alpha1.SubaccountApiCredentialStatus{
					AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{
						SubaccountID: ptr("subaccount-guid"),
						Name:         ptr("my-credential"),
					},
				},
			},
			kube: &test.MockClient{},
			want: want{},
		},
		"legacy name-only annotation - migrates using annotation name and spec subaccount": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							SubaccountID: ptr("subaccount-guid"),
							Name:         ptr("renamed-in-spec"),
						},
					},
				}
				meta.SetExternalName(cr, "old-credential")
				return cr
			}(),
			kube: &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			want: want{externalName: "subaccount-guid/old-credential"},
		},
		"legacy name-only annotation - migrates using status subaccount": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{Name: "my-secret", Namespace: "default"},
						},
					},
					Status: securityv1alpha1.SubaccountApiCredentialStatus{
						AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{SubaccountID: ptr("subaccount-guid")},
					},
				}
				meta.SetExternalName(cr, "old-credential")
				return cr
			}(),
			// A failing Delete verifies migration does not delete the existing connection secret.
			kube: &test.MockClient{
				MockDelete: test.NewMockDeleteFn(errors.New("delete must not be called")),
				MockUpdate: test.NewMockUpdateFn(nil),
			},
			want: want{externalName: "subaccount-guid/old-credential"},
		},
		"compound annotation already set - no-op": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							SubaccountID: ptr("subaccount-guid"),
							Name:         ptr("my-credential"),
						},
					},
				}
				meta.SetExternalName(cr, "subaccount-guid/my-credential")
				return cr
			}(),
			kube: &test.MockClient{},
			want: want{externalName: "subaccount-guid/my-credential"},
		},
		"missing subaccount id - no-op": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{Name: ptr("my-credential")},
				},
			},
			kube: &test.MockClient{},
			want: want{},
		},
		"missing name - no-op": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: ptr("subaccount-guid")},
				},
			},
			kube: &test.MockClient{},
			want: want{},
		},
		"legacy annotation without subaccount id - no-op": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
				meta.SetExternalName(cr, "old-credential")
				return cr
			}(),
			kube: &test.MockClient{},
			want: want{externalName: "old-credential"},
		},
		"legacy annotation update fails - propagates error": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				cr := &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							SubaccountID: ptr("subaccount-guid"),
							Name:         ptr("my-credential"),
						},
					},
				}
				meta.SetExternalName(cr, "legacy-name")
				return cr
			}(),
			kube: &test.MockClient{MockUpdate: test.NewMockUpdateFn(errors.New("api server unavailable"))},
			want: want{err: errors.Wrap(errors.New("api server unavailable"), errUpdateExternalName), externalName: "subaccount-guid/legacy-name"},
		},
		"resource being deleted - no-op": {
			cr: func() *securityv1alpha1.SubaccountApiCredential {
				now := metav1.Now()
				return &securityv1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}, DeletionTimestamp: &now},
					Spec: securityv1alpha1.SubaccountApiCredentialSpec{
						ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
							SubaccountID: ptr("subaccount-guid"),
							Name:         ptr("my-credential"),
						},
					},
				}
			}(),
			kube: &test.MockClient{},
			want: want{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			n := &CompoundExternalNameInitializer{Kube: tc.kube}
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
