package di

import (
	"context"
	"fmt"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This file contains creator functions for initializers and clients to decouple that logic from controllers and share it across them

func NewPlanIdResolverFn(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
	binding, err := servicemanager.NewCredsFromOperatorSecret(secretData)
	if err != nil {
		return nil, err
	}
	return servicemanager.NewServiceManagerClient(btp.NewBackgroundContextWithDebugPrintHTTPClient(), &binding)
}

func LoadSecretData(kube client.Client, ctx context.Context, secretName, secretNamespace string) (map[string][]byte, error) {
	if secretName == "" || secretNamespace == "" {
		return nil, fmt.Errorf("secret name and namespace must not be empty")
	}
	secret := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretName,
	}, secret); err != nil {
		return nil, err
	}
	return secret.Data, nil
}
