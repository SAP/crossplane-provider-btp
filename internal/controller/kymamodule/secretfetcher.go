package kymamodule

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	kymaExpirationLayout    = "2006-01-02 15:04:05.999999999 -0700 MST" // go default time format
	errTimeParser           = "Failed to parse expiration time"
	errCredentialsCorrupted = "secret credentials data not in the expected format"
)

func NewSecretFetcher(kube client.Client) *SecretFetcher {
	return &SecretFetcher{
		kube: kube,
	}
}

type SecretFetcherInterface interface {
	Fetch(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error)
}

type SecretFetcher struct {
	kube client.Client
}

func (c *SecretFetcher) Fetch(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
	secretName := cr.Spec.KymaEnvironmentBindingSecret
	namespace := cr.Spec.KymaEnvironmentBindingSecretNamespace

	secret, errGet := internal.LoadSecretData(ctx, c.kube, secretName, namespace)
	if errGet != nil {
		return nil, errGet
	}

	// Check if the secret contains valid kubeconfig data that is not expired
	kymaCreds, err := getValidKubeconfig(secret)

	if err != nil {
		return nil, err
	}

	return kymaCreds, nil
}

// getValidKubeconfig checks if the secret contains valid kubeconfig data and is not expired
func getValidKubeconfig(secret map[string][]byte) ([]byte, error) {

	expirationBytes := secret[v1alpha1.KymaEnvironmentBindingExpirationKey]
	if len(expirationBytes) == 0 {
		return nil, errors.New(errCredentialsCorrupted)
	}

	expiration, err := time.Parse(kymaExpirationLayout, string(expirationBytes))
	if err != nil {
		return nil, errors.New(errTimeParser)
	}
	if expiration.Before(time.Now()) {
		// Secret has expired
		return nil, nil
	}

	creds := secret[v1alpha1.KymaEnvironmentBindingKey]
	if len(creds) == 0 {
		// No kubeconfig data found
		return nil, errors.New(errCredentialsCorrupted)
	}

	return creds, nil
}
