package kymaserviceinstance

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errSecretEmpty         = "KymaEnvironmentBinding secret name or namespace is empty"
	errKubeconfigNotFound  = "kubeconfig not found in KymaEnvironmentBinding secret"
	errFailedToLoadSecret  = "failed to load KymaEnvironmentBinding secret"
	errFailedToParseExpiry = "failed to parse expiration time"
	errExpirationNotFound  = "expiration not found in secret"
	errKubeconfigExpired   = "kubeconfig has expired"

	// Layout for parsing expiration time from secret
	kymaExpirationLayout = "2006-01-02 15:04:05 -0700 MST"
)

// SecretFetcher fetches and validates kubeconfig from KymaEnvironmentBinding secrets
type SecretFetcher struct {
	kube client.Client
}

// NewSecretFetcher creates a new SecretFetcher
func NewSecretFetcher(kube client.Client) *SecretFetcher {
	return &SecretFetcher{
		kube: kube,
	}
}

func (s *SecretFetcher) Fetch(ctx context.Context, cr *v1alpha1.KymaServiceInstance) ([]byte, error) {
	secretName := cr.Spec.KymaEnvironmentBindingSecret
	secretNamespace := cr.Spec.KymaEnvironmentBindingSecretNamespace

	if secretName == "" || secretNamespace == "" {
		return nil, errors.New(errSecretEmpty)
	}

	// Load secret data
	secret, err := internal.LoadSecretData(ctx, s.kube, secretName, secretNamespace)
	if err != nil {
		return nil, errors.Wrap(err, errFailedToLoadSecret)
	}

	// Validate and return kubeconfig
	kubeconfig, err := getValidKubeconfig(secret)
	if err != nil {
		return nil, err
	}

	return kubeconfig, nil
}

func getValidKubeconfig(secret map[string][]byte) ([]byte, error) {
	// Check expiration first
	expirationBytes, ok := secret[v1alpha1.KymaEnvironmentBindingExpirationKey]
	if !ok {
		return nil, errors.New(errExpirationNotFound)
	}

	// Parse expiration time
	expiration, err := time.Parse(kymaExpirationLayout, string(expirationBytes))
	if err != nil {
		return nil, errors.Wrap(err, errFailedToParseExpiry)
	}

	// Check if expired
	if expiration.Before(time.Now()) {
		return nil, errors.Errorf("%s: expired at %s", errKubeconfigExpired, expiration.Format(time.RFC3339))
	}

	// Get kubeconfig
	kubeconfig, ok := secret[v1alpha1.KymaEnvironmentBindingKey]
	if !ok || len(kubeconfig) == 0 {
		return nil, errors.New(errKubeconfigNotFound)
	}

	return kubeconfig, nil
}
