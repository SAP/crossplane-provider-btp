package kubeconfiggenerator

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/oidc/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/oidc"
	ctrloidc "github.com/sap/crossplane-provider-btp/internal/controller/oidc"
)

var (
	fakeOIDCSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-token"},
		Data: map[string][]byte{
			"IDToken": []byte("abc"), "RefreshToken": []byte("def"),
		},
	}
	fakeKubeConfigSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "kubeconfig-secret"},
		Data: map[string][]byte{
			v1alpha1.KubeConfigSecreKey: []byte("kubeconfig template"),
		},
	}
	fakeKubeConfigTokenSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "kubeconfig-token-secret"},
		Data: map[string][]byte{
			v1alpha1.KubeConfigSecreKey: []byte("kubeconfig template with token"),
		},
	}
)

func crResource(name string, fns ...func(generator *v1alpha1.KubeConfigGenerator)) *v1alpha1.KubeConfigGenerator {
	generatedKubeConfigSecret := fakeKubeConfigTokenSecret.Name
	oidcSecretName := fakeOIDCSecret.Name
	kubeConfigSecretName := fakeKubeConfigSecret.Name

	cr := &v1alpha1.KubeConfigGenerator{
		TypeMeta: metav1.TypeMeta{},
		// the used meta.GetExternalName reads from the annotations directly
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{meta.AnnotationKeyExternalName: name}, Generation: 2},
		Spec: v1alpha1.KubeConfigGeneratorSpec{
			ForProvider: v1alpha1.KubeConfigGeneratorParameters{
				OIDCToken:          crToken(oidcSecretName),
				KubeconfigTemplate: crKubeConfigTemplate(kubeConfigSecretName),
			},
			ResourceSpec: xpv1.ResourceSpec{
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name: generatedKubeConfigSecret,
				},
			},
		},
		Status: v1alpha1.KubeConfigGeneratorStatus{AtProvider: v1alpha1.KubeConfigGeneratorObservation{}},
	}
	for _, fn := range fns {
		fn(cr)
	}
	return cr
}

func crToken(name string) v1alpha1.OIDCToken {
	return v1alpha1.OIDCToken{
		Source: xpv1.CredentialsSourceSecret,
		CommonCredentialSelectors: xpv1.CommonCredentialSelectors{SecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{
				Name:      name,
				Namespace: "crossplane-system",
			},
			Key: "IDToken",
		}},
	}
}

func crKubeConfigTemplate(name string) v1alpha1.KubeconfigTemplate {
	return v1alpha1.KubeconfigTemplate{
		Source: xpv1.CredentialsSourceSecret,
		CommonCredentialSelectors: xpv1.CommonCredentialSelectors{SecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{
				Name:      name,
				Namespace: "crossplane-system",
			},
			Key: "KUBE_CONFIG",
		}},
	}
}

func clientMock(upToDate bool, successfulGeneration bool, serverUrl string) oidc.KubeConfigClient {
	generatedContent := "new_generated_kubeconfig"
	if successfulGeneration {
		return &KubeConfigClientMock{UpToDate: upToDate, GeneratedContent: generatedContent, ServerUrl: serverUrl}
	}
	return &KubeConfigClientMock{UpToDate: upToDate, ServerUrl: serverUrl}
}

func withStatus(cond xpv1.Condition) func(*v1alpha1.KubeConfigGenerator) {
	return func(cr *v1alpha1.KubeConfigGenerator) {
		cr.Status.SetConditions(cond)
	}
}

func withDeletion() func(*v1alpha1.KubeConfigGenerator) {
	return func(cr *v1alpha1.KubeConfigGenerator) {
		cr.DeletionTimestamp = internal.Ptr(metav1.Unix(1, 1))
	}
}

func withStatusData(kubeConfigHash []byte, tokenHash []byte, generation int64, serverUrl string) func(*v1alpha1.KubeConfigGenerator) {
	return func(cr *v1alpha1.KubeConfigGenerator) {
		cr.Status.AtProvider.UpdatedGeneration = generation
		cr.Status.AtProvider.KubeConfigHash = kubeConfigHash
		cr.Status.AtProvider.TokenHash = tokenHash
		cr.Status.AtProvider.ServerUrl = serverUrl
	}
}

func newServiceFnWithRecorder(recordedKubeConfig *[]byte, recordedToken *[]byte) func([]byte, []byte) (oidc.KubeConfigClient, error) {
	return func(tokenHash []byte, kubeConfigHash []byte) (oidc.KubeConfigClient, error) {
		*recordedKubeConfig = kubeConfigHash
		*recordedToken = tokenHash
		return &KubeConfigClientMock{false, "", ""}, nil
	}
}

func oidcError() error {
	return errors.Wrap(errors.Wrap(ctrloidc.ErrNoResource, "cannot get credentials secret"), "Can't resolve a OIDC secret")
}

func kubeConfigError() error {
	return errors.Wrap(errors.Wrap(ctrloidc.ErrNoResource, "cannot get credentials secret"), "Can't resolve a Kubeconfig secret")
}

func kubeConfigGenerationError() error {
	return errors.Wrap(errGenerate, errGenerateKubeconfig)
}
