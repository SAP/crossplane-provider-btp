package certbasedoidclogin

import (
	"context"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	oidc2 "github.com/int128/kubelogin/pkg/oidc"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oidcv1alpha1 "github.com/sap/crossplane-provider-btp/apis/oidc/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/oidc"
	ctrloidc "github.com/sap/crossplane-provider-btp/internal/controller/oidc"
	tu "github.com/sap/crossplane-provider-btp/internal/testutils"
)

var (
	validToken                = &oidc2.TokenSet{IDToken: tu.JwtToken(tu.Now, tu.ExpiresAt(time.Hour*3), tu.IssuedAt(time.Hour*-1)), RefreshToken: "456"}
	validTokenWithoutIssuedAt = &oidc2.TokenSet{IDToken: tu.JwtToken(tu.Now, tu.ExpiresAt(time.Hour*3)), RefreshToken: "456"}
	expiredToken              = &oidc2.TokenSet{IDToken: tu.JwtToken(tu.Now, tu.ExpiresAt(time.Minute-1), tu.IssuedAt(time.Hour*-1)), RefreshToken: "456"}
	aboutToExpireToken        = &oidc2.TokenSet{IDToken: tu.JwtToken(tu.Now, tu.ExpiresAt(time.Hour), tu.IssuedAt(time.Hour*3*-1)), RefreshToken: "456"}

	fakeCertSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cert-secret"},
		Data: map[string][]byte{
			"cert": []byte("abc"),
		},
	}
	fakePWSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pw-secret"},
		Data: map[string][]byte{
			"password": []byte("def"),
		},
	}
	validTokenSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-secret"},
		Data:       map[string][]byte{oidcv1alpha1.ConDetailsIDToken: []byte(validToken.IDToken), oidcv1alpha1.ConDetailsRefresh: []byte(validToken.RefreshToken)},
	}
	validTokenSecretWoIssuedAt = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-secret"},
		Data:       map[string][]byte{oidcv1alpha1.ConDetailsIDToken: []byte(validTokenWithoutIssuedAt.IDToken), oidcv1alpha1.ConDetailsRefresh: []byte(validTokenWithoutIssuedAt.RefreshToken)},
	}
	expiredTokenSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-secret"},
		Data:       map[string][]byte{oidcv1alpha1.ConDetailsIDToken: []byte(expiredToken.IDToken), oidcv1alpha1.ConDetailsRefresh: []byte(expiredToken.RefreshToken)},
	}
	aboutToExpireTokenSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oidc-secret"},
		Data:       map[string][]byte{oidcv1alpha1.ConDetailsIDToken: []byte(aboutToExpireToken.IDToken), oidcv1alpha1.ConDetailsRefresh: []byte(aboutToExpireToken.RefreshToken)},
	}
)

func defaultResource(name string) *oidcv1alpha1.CertBasedOIDCLogin {
	tokenSecretName := validTokenSecret.Name
	certSecretName := fakeCertSecret.Name
	pwSecretName := fakePWSecret.Name

	return &oidcv1alpha1.CertBasedOIDCLogin{
		TypeMeta: metav1.TypeMeta{},
		// the used meta.GetExternalName reads from the annotations directly
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{meta.AnnotationKeyExternalName: name}},
		Spec: oidcv1alpha1.CertBasedOIDCLoginSpec{
			ForProvider: oidcv1alpha1.CertBasedOIDCLoginParameters{
				Issuer:      "",
				ClientId:    "",
				Certificate: crCertSecret(certSecretName),
				Password:    crPWSecret(pwSecretName),
			},
			ResourceSpec: xpv1.ResourceSpec{
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name: tokenSecretName,
				},
			},
		},
		Status: oidcv1alpha1.CertBasedOIDCLoginStatus{
			AtProvider: oidcv1alpha1.CertBasedOIDCLoginObservation{}},
	}
}

func crCertSecret(name string) oidcv1alpha1.Certificate {
	return oidcv1alpha1.Certificate{
		Type:   "sap-ias",
		Source: xpv1.CredentialsSourceSecret,
		CommonCredentialSelectors: xpv1.CommonCredentialSelectors{SecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{
				Name:      name,
				Namespace: "crossplane-system",
			},
			Key: "cert",
		}},
	}
}

func crPWSecret(name string) oidcv1alpha1.Password {
	return oidcv1alpha1.Password{
		Source: xpv1.CredentialsSourceSecret,
		CommonCredentialSelectors: xpv1.CommonCredentialSelectors{SecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{
				Name:      name,
				Namespace: "crossplane-system",
			},
			Key: "password",
		}},
	}
}

func certError() error {
	return errors.Wrap(errors.Wrap(ctrloidc.ErrNoResource, "cannot get credentials secret"), "Can't resolve a user certificate secret")
}

func tokenSecretError() error {
	return errors.Wrap(errors.New("missing secret"), errUpdateToken)
}

func pwError() error {
	return errors.Wrap(errors.Wrap(ctrloidc.ErrNoResource, "cannot get credentials secret"), "Can't resolve a user password secret")
}

func mockCertLoginService(doLogin bool, expired bool, refresh bool) *CertLoginMock {
	loginMock := &CertLoginMock{Expired: expired}
	if doLogin || refresh {
		loginMock.TokenSet = validToken
	}
	return loginMock
}

func conditions(conds ...xpv1.Condition) func(cr *oidcv1alpha1.CertBasedOIDCLogin) {
	return func(cr *oidcv1alpha1.CertBasedOIDCLogin) {
		for _, cond := range conds {
			cr.Status.SetConditions(cond)
		}
	}
}
func jwtStatus(status oidcv1alpha1.JwtStatus) func(cr *oidcv1alpha1.CertBasedOIDCLogin) {
	return func(cr *oidcv1alpha1.CertBasedOIDCLogin) {
		cr.Status.AtProvider.JwtStatus = status

	}
}

type crModifier func(cr *oidcv1alpha1.CertBasedOIDCLogin)

func cr(cr *oidcv1alpha1.CertBasedOIDCLogin, m ...crModifier) *oidcv1alpha1.CertBasedOIDCLogin {

	for _, f := range m {
		f(cr)

	}
	return cr
}

func newServiceFnWithRecorder(recordedUserCertCall *[]byte, recordedPWCall *string) func(ctx context.Context, cr *oidcv1alpha1.CertBasedOIDCLogin, userCertificate []byte, pw string) (oidc.LoginPerformer, error) {
	return func(ctx context.Context, cr *oidcv1alpha1.CertBasedOIDCLogin, userCertificate []byte, pw string) (oidc.LoginPerformer, error) {
		*recordedUserCertCall = userCertificate
		*recordedPWCall = pw
		return mockCertLoginService(false, false, false), nil
	}
}
