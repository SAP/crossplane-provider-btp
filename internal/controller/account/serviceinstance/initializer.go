package serviceinstance

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errInitialize       = "cannot resolve plan ID"
	errLoadSmBinding    = "cannot load service manager binding secret"
	errInitPlanResolver = "cannot initialize plan ID resolver"
)

type Initializer interface {
	Initialize(kube client.Client, ctx context.Context, mg resource.Managed) error
}

var _ Initializer = &servicePlanInitializer{}

type servicePlanInitializer struct {
	newIdResolverFn func(ctx context.Context, secretData map[string][]byte) (smClient.PlanIdResolver, error)
	loadSecretFn    func(kube client.Client, ctx context.Context, secretName, secretNamespace string) (map[string][]byte, error)
}

// Initialize implements managed.Initializer, initializes an implementation of IdResolver and uses it to resolve the plan ID
func (s *servicePlanInitializer) Initialize(kube client.Client, ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return errors.New(errNotServiceInstance)
	}

	if isInitialized(cr) {
		return nil
	}

	secretData, err := s.loadSecretFn(kube, ctx, cr.Spec.ForProvider.ServiceManagerSecret, cr.Spec.ForProvider.ServiceManagerSecretNamespace)
	if err != nil {
		return errors.Wrap(err, errLoadSmBinding)
	}

	idResolver, err := s.newIdResolverFn(ctx, secretData)
	if err != nil {
		return errors.Wrap(err, errInitPlanResolver)
	}

	planID, err := idResolver.PlanIDByName(ctx, cr.Spec.ForProvider.OfferingName, cr.Spec.ForProvider.PlanName)
	if err != nil {
		return errors.Wrap(err, errInitialize)
	}
	cr.Status.AtProvider.ServiceplanID = planID
	if err := kube.Status().Update(ctx, cr); err != nil {
		return errors.Wrap(err, errSaveData)
	}
	return nil
}

func isInitialized(cr *v1alpha1.ServiceInstance) bool {
	return cr.Status.AtProvider.ServiceplanID != ""
}
