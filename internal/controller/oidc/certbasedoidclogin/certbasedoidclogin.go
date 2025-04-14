package certbasedoidclogin

import (
	"context"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	oidc2 "github.com/int128/kubelogin/pkg/oidc"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/oidc/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/oidc"
)

const (
	errNotCertBasedOIDCLogin = "managed resource is not a CertBasedOIDCLogin custom resource"
	errTrackPCUsage          = "cannot track ProviderConfig usage"
	errNewClient             = "cannot create new LoginPerformer service"
	errResolveUserCert       = "Can't resolve a user certificate secret"
	errResolvePassword       = "Can't resolve a user password secret"
	errCorruptedToken        = "Token loaded from secret does not match expected format"
	errCouldNotJudgeToken    = "Could not introspect idToken"
	errNoToken               = "No token to introspect"
)

type connector struct {
	kube  client.Client
	usage resource.Tracker

	newServiceFn func(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, userCertificate []byte, pw string) (oidc.LoginPerformer, error)
}

var newServiceFn = func(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, userCertificate []byte, pw string) (oidc.LoginPerformer, error) {
	return createCertLoginService(ctx, cr, userCertificate, pw)
}

type external struct {
	service oidc.LoginPerformer
	kube    client.Client
}

type jwtRotation struct {
	duration time.Duration
	judge    oidc.JwtJudge
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.CertBasedOIDCLogin)
	if !ok {
		return nil, errors.New(errNotCertBasedOIDCLogin)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	userCertificate, cerErr := resolveCertificateFromSecret(ctx, cr, c)
	if cerErr != nil {
		return nil, errors.Wrap(cerErr, errResolveUserCert)
	}

	pw, pwErr := resolvePasswordFromSecret(ctx, cr, c)
	if pwErr != nil {
		return nil, errors.Wrap(pwErr, errResolvePassword)
	}

	svc, serviceErr := c.newServiceFn(ctx, cr, userCertificate, string(pw))
	if serviceErr != nil {
		return nil, errors.Wrap(serviceErr, errNewClient)
	}

	return &external{service: svc, kube: c.kube}, nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.CertBasedOIDCLogin)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotCertBasedOIDCLogin)
	}

	if needsCreation(ctx, cr, c) {
		return managed.ExternalObservation{ResourceExists: false, ConnectionDetails: managed.ConnectionDetails{}}, nil
	}

	cr.Status.SetConditions(xpv1.Available())

	tokenSet := resolvePublishedToken(ctx, cr, c.kube)

	tokenRotation := c.updateJwtRotationStatus(tokenSet, cr)

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  !c.needsUpdate(tokenSet, tokenRotation),
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) updateJwtRotationStatus(tokenSet *oidc2.TokenSet, cr *v1alpha1.CertBasedOIDCLogin) *jwtRotation {
	tokenRotation, err := c.calculateRotation(tokenSet)

	if err != nil {
		cr.SetConditions(v1alpha1.IntrospectError(err.Error()))
		cr.Status.AtProvider.JwtStatus = v1alpha1.JwtStatus{}
	} else {
		cr.SetConditions(v1alpha1.IntrospectOk())
		cr.Status.AtProvider.JwtStatus = tokenRotation.judge.Status()
	}
	return tokenRotation
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.CertBasedOIDCLogin)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotCertBasedOIDCLogin)
	}

	cr.Status.SetConditions(xpv1.Creating())

	tokenSet, loginErr := c.service.DoLogin(ctx)
	if loginErr != nil {
		return managed.ExternalCreation{}, loginErr
	}

	return managed.ExternalCreation{
		ConnectionDetails: tokenSetToConnectionDetails(tokenSet),
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.CertBasedOIDCLogin)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotCertBasedOIDCLogin)
	}

	token := resolvePublishedToken(ctx, cr, c.kube)
	if token == nil {
		return managed.ExternalUpdate{}, errors.New("Can not update token, because of missing secret")
	}

	refresh, err := c.service.Refresh(ctx, token.RefreshToken)
	if needsLogin(err) {
		ctrl.Log.Info("Tokens could not be refreshed, attempting to reauthenticate")
		tokenSet, loginErr := c.service.DoLogin(ctx)
		if loginErr != nil {
			return managed.ExternalUpdate{}, loginErr
		}
		ctrl.Log.Info("Reauthentication successful")
		return managed.ExternalUpdate{
			ConnectionDetails: tokenSetToConnectionDetails(tokenSet),
		}, nil
	}

	return managed.ExternalUpdate{
		ConnectionDetails: tokenSetToConnectionDetails(refresh),
	}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	// No-op
	return nil
}
func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.CertBasedOIDCLogin)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotCertBasedOIDCLogin)
	}
	cr.Status.SetConditions(xpv1.Deleting())

	return managed.ExternalDelete{}, cleanupPublishedTokens(ctx, cr, c.kube)
}

func createCertLoginService(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, cert []byte, pw string) (*oidc.CertLogin, error) {
	return oidc.NewCertLogin(oidc.CertConfiguration{
		IssuerURL:       cr.Spec.ForProvider.Issuer,
		ClientID:        cr.Spec.ForProvider.ClientId,
		UserCertificate: cert,
		Password:        pw,
		//TODO: potentially add to CRD
		Scopes: []string{"email"},
	}, ctx)
}

func resolveCertificateFromSecret(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, c *connector) ([]byte, error) {
	certSecret := cr.Spec.ForProvider.Certificate
	userCertificate, err := resource.CommonCredentialExtractor(ctx, certSecret.Source, c.kube, certSecret.CommonCredentialSelectors)
	return userCertificate, err
}

func resolvePasswordFromSecret(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, c *connector) ([]byte, error) {
	pwSecret := cr.Spec.ForProvider.Password
	pw, err := resource.CommonCredentialExtractor(ctx, pwSecret.Source, c.kube, pwSecret.CommonCredentialSelectors)
	return pw, err
}
func resolvePublishedSecret(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, client client.Client) (*corev1.Secret, error) {
	connDetails := cr.Spec.WriteConnectionSecretToReference
	secret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: connDetails.Namespace, Name: connDetails.Name}, secret); err != nil {
		return nil, err
	}
	return secret, nil
}
func resolvePublishedToken(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, client client.Client) *oidc2.TokenSet {
	secret, err := resolvePublishedSecret(ctx, cr, client)
	if err != nil {
		return nil
	}
	return secretToTokenSet(secret)
}

func cleanupPublishedTokens(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, client client.Client) error {
	publishedSecret, loadErr := resolvePublishedSecret(ctx, cr, client)
	if loadErr == nil {
		return client.Delete(ctx, publishedSecret)
	}
	return nil
}

func tokenSetToConnectionDetails(set *oidc2.TokenSet) managed.ConnectionDetails {
	conDetails := managed.ConnectionDetails{}
	conDetails[v1alpha1.ConDetailsIDToken] = []byte(set.IDToken)
	conDetails[v1alpha1.ConDetailsRefresh] = []byte(set.RefreshToken)
	return conDetails
}
func secretToTokenSet(secret *corev1.Secret) *oidc2.TokenSet {
	token := oidc2.TokenSet{}
	token.IDToken = string(secret.Data[v1alpha1.ConDetailsIDToken])
	token.RefreshToken = string(secret.Data[v1alpha1.ConDetailsRefresh])
	if token.IDToken == "" || token.RefreshToken == "" {
		return nil
	}
	return &token
}

func (c *external) needsUpdate(tokenSecret *oidc2.TokenSet, r *jwtRotation) bool {
	if r != nil {
		return r.judge.IsInRenewPeriod(r.duration)
	} else {
		return c.service.IsExpired(tokenSecret.IDToken)
	}
}

func needsLogin(err error) bool {
	return err != nil
}

func needsCreation(ctx context.Context, cr *v1alpha1.CertBasedOIDCLogin, c *external) bool {
	return resolvePublishedToken(ctx, cr, c.kube) == nil
}

func (c *external) calculateRotation(tokenSecret *oidc2.TokenSet) (*jwtRotation, error) {
	if tokenSecret == nil {
		return nil, errors.New(errNoToken)
	}
	judge, err := oidc.NewJwtJudge(tokenSecret.IDToken)
	if err != nil {
		return nil, errors.Wrap(err, errCouldNotJudgeToken)
	}
	duration, err := judge.EstimateRotationDuration()
	if err != nil {
		return nil, errors.Wrap(err, errCouldNotJudgeToken)
	}
	return &jwtRotation{duration: *duration, judge: *judge}, nil
}
