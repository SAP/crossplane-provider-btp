package serviceinstance

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
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

	// PlanID resolves the plan cr declares now, from servicePlanID or
	// offeringName/planName/dataCenter. It neither reads nor writes status.
	PlanID(ctx context.Context, kube client.Client, cr *v1alpha1.ServiceInstance) (string, error)
}

var _ Initializer = &servicePlanInitializer{}

type servicePlanInitializer struct {
	newIdResolverFn func(ctx context.Context, secretData map[string][]byte) (smClient.PlanIdResolver, error)
	loadSecretFn    func(ctx context.Context, kube client.Client, secretName, secretNamespace string) (map[string][]byte, error)
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

	// Direct plan ID provided — skip name-based resolution
	planID, err := s.PlanID(ctx, kube, cr)
	if err != nil {
		return err
	}
	cr.Status.AtProvider.ServiceplanID = planID
	if err := kube.Status().Update(ctx, cr); err != nil {
		return errors.Wrap(err, errSaveData)
	}
	return nil
}

// PlanID implements Initializer.
func (s *servicePlanInitializer) PlanID(ctx context.Context, kube client.Client, cr *v1alpha1.ServiceInstance) (string, error) {
	if id := cr.Spec.ForProvider.ServicePlanID; id != "" {
		return id, nil
	}

	secretData, err := s.loadSecretFn(ctx, kube, cr.Spec.ForProvider.ServiceManagerSecret, cr.Spec.ForProvider.ServiceManagerSecretNamespace)
	if err != nil {
		return "", errors.Wrap(err, errLoadSmBinding)
	}

	idResolver, err := s.newIdResolverFn(ctx, secretData)
	if err != nil {
		return "", errors.Wrap(err, errInitPlanResolver)
	}

	planID, err := idResolver.PlanIDByName(ctx, cr.Spec.ForProvider.OfferingName, cr.Spec.ForProvider.PlanName, cr.Spec.ForProvider.DataCenter)
	if err != nil {
		return "", errors.Wrap(err, errInitialize)
	}
	return planID, nil
}

func isInitialized(cr *v1alpha1.ServiceInstance) bool {
	return cr.Status.AtProvider.ServiceplanID != ""
}
