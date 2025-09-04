package servicebindingnative

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	servicemanagerclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

const (
	errLoadSmBinding    = "cannot load data from service manager secret"
	errParseCredentials = "cannot parse service manager credentials"
)

func NewServiceManagerClientIFromResource(ctx context.Context, cr *v1alpha1.ServiceBinding, kube client.Client) (*servicemanagerclient.APIClient, error) {

	secretData, err := internal.LoadSecretData(ctx, kube, cr.Spec.ForProvider.ServiceManagerSecret, cr.Spec.ForProvider.ServiceManagerSecretNamespace)
	if err != nil {
		return nil, errors.Wrap(err, errLoadSmBinding)
	}

	binding, err := servicemanager.NewCredsFromOperatorSecret(secretData)
	if err != nil {
		return nil, errors.Wrap(err, errParseCredentials)
	}
	return servicemanager.NewServiceManagerClientI(btp.NewBackgroundContextWithDebugPrintHTTPClient(), &binding)
}
