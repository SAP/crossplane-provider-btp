package kubeconfiggenerator

import (
	"context"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/internal/clients/oidc"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/oidc/v1alpha1"
)

const (
	errNotKubeConfigGenerator  = "managed resource is not a KubeConfigGenerator custom resource"
	errTrackPCUsage            = "cannot track ProviderConfig usage"
	errResolveOIDCSecret       = "Can't resolve a OIDC secret"
	errResolveKubeconfigSecret = "Can't resolve a Kubeconfig secret"
	errNoConnectionSecret      = "Need a .Spec.WriteConnectionSecretToReference"

	errNewClient = "cannot create new Service"
)

var (
	newKubeConfigClientFn = func(tokenHash []byte, kubeConfigHash []byte) (oidc.KubeConfigClient, error) {
		return oidc.NewKubeConfigCreator(tokenHash, kubeConfigHash), nil
	}
)

type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func([]byte, []byte) (oidc.KubeConfigClient, error)
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.KubeConfigGenerator)
	if !ok {
		return nil, errors.New(errNotKubeConfigGenerator)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	svc, err := newService(c, cr)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{service: svc, kube: c.kube}, nil
}

type external struct {
	service oidc.KubeConfigClient
	kube    client.Client
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KubeConfigGenerator)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKubeConfigGenerator)
	}

	if meta.WasDeleted(cr) {
		if cr.GetCondition(xpv1.Deleting().Type).Reason == xpv1.Deleting().Reason {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{ResourceExists: true}, nil
	}

	if err := sanityCheck(cr); err != nil {
		return managed.ExternalObservation{}, err
	}

	token, oidcErr := resolveOIDCToken(ctx, cr, c.kube)
	if oidcErr != nil {
		return managed.ExternalObservation{}, errors.Wrap(oidcErr, errResolveOIDCSecret)
	}

	kubeConfig, k8sErr := resolveKubeConfigFromSecret(ctx, cr, c.kube)
	if k8sErr != nil {
		return managed.ExternalObservation{}, errors.Wrap(k8sErr, errResolveKubeconfigSecret)
	}

	if needsCreation(ctx, cr, c) {
		return managed.ExternalObservation{ResourceExists: false, ConnectionDetails: managed.ConnectionDetails{}}, nil
	}
	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  !needsUpdate(cr, c, kubeConfig, token),
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func sanityCheck(cr *v1alpha1.KubeConfigGenerator) error {
	writeTo := cr.Spec.WriteConnectionSecretToReference
	if writeTo == nil {
		return errors.New(errNoConnectionSecret)
	}

	return nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KubeConfigGenerator)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKubeConfigGenerator)
	}

	token, oidcErr := resolveOIDCToken(ctx, cr, c.kube)
	if oidcErr != nil {
		return managed.ExternalCreation{}, errors.Wrap(oidcErr, errResolveOIDCSecret)
	}

	kubeConfigTemplate, k8sErr := resolveKubeConfigFromSecret(ctx, cr, c.kube)
	if k8sErr != nil {
		return managed.ExternalCreation{}, errors.Wrap(k8sErr, errResolveKubeconfigSecret)
	}

	cr.Status.SetConditions(xpv1.Creating())
	result, err := c.service.Generate(kubeConfigTemplate, token, generateConfiguration(cr))
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	updateStatus(cr, result)

	return managed.ExternalCreation{
		ConnectionDetails: kubeConfigToConnectionDetails(result),
	}, nil
}

func generateConfiguration(cr *v1alpha1.KubeConfigGenerator) *oidc.GenerateConfig {
	template := cr.Spec.ForProvider.KubeconfigTemplate
	generate := oidc.ConfigureGenerate().UserIndex(template.UserIndex)
	if template.InjectInline {
		generate.InjectInline()
	}
	return generate
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.KubeConfigGenerator)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotKubeConfigGenerator)
	}

	token, oidcErr := resolveOIDCToken(ctx, cr, c.kube)
	if oidcErr != nil {
		return managed.ExternalUpdate{}, errors.Wrap(oidcErr, errResolveOIDCSecret)
	}

	kubeConfigTemplate, k8sErr := resolveKubeConfigFromSecret(ctx, cr, c.kube)
	if k8sErr != nil {
		return managed.ExternalUpdate{}, errors.Wrap(k8sErr, errResolveKubeconfigSecret)
	}

	result, err := c.service.Generate(kubeConfigTemplate, token, generateConfiguration(cr))
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	updateStatus(cr, result)

	return managed.ExternalUpdate{
		ConnectionDetails: kubeConfigToConnectionDetails(result),
	}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	// No-op
	return nil
}
func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.KubeConfigGenerator)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotKubeConfigGenerator)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	return managed.ExternalDelete{}, cleanupCreatedKubeConfig(ctx, cr, c.kube)
}

func newService(c *connector, cr *v1alpha1.KubeConfigGenerator) (oidc.KubeConfigClient, error) {
	return c.newServiceFn(cr.Status.AtProvider.TokenHash, cr.Status.AtProvider.KubeConfigHash)
}

func resolveKubeConfigFromSecret(ctx context.Context, cr *v1alpha1.KubeConfigGenerator, c client.Client) ([]byte, error) {
	certSecret := cr.Spec.ForProvider.KubeconfigTemplate
	kubeConfig, err := resource.CommonCredentialExtractor(ctx, certSecret.Source, c, certSecret.CommonCredentialSelectors)
	return kubeConfig, err
}

func resolveOIDCToken(ctx context.Context, cr *v1alpha1.KubeConfigGenerator, c client.Client) ([]byte, error) {
	certSecret := cr.Spec.ForProvider.OIDCToken
	oidcToken, err := resource.CommonCredentialExtractor(ctx, certSecret.Source, c, certSecret.CommonCredentialSelectors)
	return oidcToken, err
}

func resolvePublishedKubeConfig(ctx context.Context, cr *v1alpha1.KubeConfigGenerator, c client.Client) ([]byte, error) {
	writeTo := cr.Spec.WriteConnectionSecretToReference
	kubeConfig, err := resource.CommonCredentialExtractor(ctx, "Secret", c, xpv1.CommonCredentialSelectors{
		SecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{
				Name:      writeTo.Name,
				Namespace: writeTo.Namespace,
			},
			Key: v1alpha1.KubeConfigSecreKey,
		},
	})
	return kubeConfig, err
}

func kubeConfigToConnectionDetails(result oidc.GenerateResult) managed.ConnectionDetails {
	conDetails := managed.ConnectionDetails{}
	conDetails[v1alpha1.KubeConfigSecreKey] = result.GeneratedKubeConfig
	return conDetails
}

func updateStatus(cr *v1alpha1.KubeConfigGenerator, result oidc.GenerateResult) {
	cr.Status.AtProvider.LastUpdatedAt = time.Now().String()
	cr.Status.AtProvider.TokenHash = result.SourceTokenHash
	cr.Status.AtProvider.KubeConfigHash = result.SourceKubeConfigHash
	cr.Status.AtProvider.UpdatedGeneration = cr.ObjectMeta.Generation
	cr.Status.AtProvider.ServerUrl = result.ServerUrl
}

func resolvePublishedSecret(ctx context.Context, cr *v1alpha1.KubeConfigGenerator, client client.Client) (*corev1.Secret, error) {
	connDetails := cr.Spec.WriteConnectionSecretToReference
	secret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: connDetails.Namespace, Name: connDetails.Name}, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func cleanupCreatedKubeConfig(ctx context.Context, cr *v1alpha1.KubeConfigGenerator, client client.Client) error {
	publishedSecret, loadErr := resolvePublishedSecret(ctx, cr, client)
	if loadErr == nil {
		return client.Delete(ctx, publishedSecret)
	}
	return nil
}

func isCrUpToDate(cr *v1alpha1.KubeConfigGenerator) bool {
	return cr.Status.AtProvider.UpdatedGeneration == cr.ObjectMeta.Generation
}

func needsUpdate(cr *v1alpha1.KubeConfigGenerator, c *external, kubeConfig []byte, token []byte) bool {
	return !isCrUpToDate(cr) || !c.service.IsUpToDate(kubeConfig, token)
}

func needsCreation(ctx context.Context, cr *v1alpha1.KubeConfigGenerator, c *external) bool {
	_, cDetailsErr := resolvePublishedKubeConfig(ctx, cr, c.kube)
	return cDetailsErr != nil
}
