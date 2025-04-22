package kymaenvironmentbinding

import (
	"context"
	"net/http"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironmentbinding"
)

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.KymaEnvironmentBinding)
	if !ok {
		return nil, errors.New(errNotKymaEnvironmentBinding)
	}

	pc := &providerv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: mg.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	if cr.Spec.CloudManagementSecret == "" || cr.Spec.CloudManagementSecretNamespace == "" {
		return nil, errors.New(errExtractSecretKey)
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(
		ctx, types.NamespacedName{
			Namespace: cr.Spec.CloudManagementSecretNamespace,
			Name:      cr.Spec.CloudManagementSecret,
		}, secret,
	); err != nil {
		return nil, errors.Wrap(err, errGetCredentialsSecret)
	}

	cd := pc.Spec.ServiceAccountSecret
	ServiceAccountSecretData, err := resource.CommonCredentialExtractor(
		ctx,
		cd.Source,
		c.kube,
		cd.CommonCredentialSelectors,
	)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	cisBinding := secret.Data[providerv1alpha1.RawBindingKey]
	if cisBinding == nil {
		return nil, errors.New(errGetCredentialsSecret)
	}
	svc, err := c.newServiceFn(cisBinding, ServiceAccountSecretData)
	return &external{
			client:     kymaenvironmentbinding.NewKymaBindings(*svc),
			tracker:    c.resourcetracker,
			httpClient: btp.DebugPrintHTTPClient(btp.WithHttpClient(&http.Client{Timeout: 10 * time.Second})),
			kube:       c.kube,
		},
		err
}
