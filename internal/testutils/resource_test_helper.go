package testutils

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	v1alpha12 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewProviderConfig(name string, cisSecret string, saSecret string) *v1alpha12.ProviderConfig {
	return &v1alpha12.ProviderConfig{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha12.ProviderConfigSpec{
			CISSecret: v1alpha12.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{
							Name: cisSecret,
						},
						Key: "data",
					},
				},
			},
			ServiceAccountSecret: v1alpha12.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{
							Name: saSecret,
						},
						Key: "credentials",
					},
				},
			},
		},
	}
}

func NewSecret(name string, data map[string][]byte) *v1.Secret {
	return &v1.Secret{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}

}

func NewDirectory(name string, m ...DirectoryModifier) *v1alpha1.Directory {
	cr := &v1alpha1.Directory{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}

	for _, f := range m {
		f(cr)
	}
	return cr
}

// this pattern can be potentially auto generated, its quite useful to write expressive unittests
type DirectoryModifier func(dirEnvironment *v1alpha1.Directory)

func WithStatus(status v1alpha1.DirectoryObservation) DirectoryModifier {
	return func(r *v1alpha1.Directory) {
		r.Status.AtProvider = status
	}
}

func WithData(data v1alpha1.DirectoryParameters) DirectoryModifier {
	return func(r *v1alpha1.Directory) {
		r.Spec.ForProvider = data
	}
}

func WithConditions(c ...xpv1.Condition) DirectoryModifier {
	return func(r *v1alpha1.Directory) { r.Status.Conditions = c }
}

func WithExternalName(externalName string) DirectoryModifier {
	return func(r *v1alpha1.Directory) {
		meta.SetExternalName(r, externalName)
	}
}
