package servicebindingclient

import (
	"context"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

const (
	errConnectTfController = "failed to connect TF controller"
	errCreateTfResource    = "failed to create TF resource"
	errDeleteTfResource    = "failed to delete TF resource"
	errObserveTfResource   = "failed to observe TF resource"
)

// TfConnector provides the interface for connecting to TF controllers
type TfConnector interface {
	Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error)
}

// InstanceManager handles the lifecycle of service binding instances
type InstanceManager struct {
	sbConnector TfConnector
}

func NewInstanceManager(sbConnector TfConnector) *InstanceManager {
	return &InstanceManager{
		sbConnector: sbConnector,
	}
}

// CreateInstance creates a new service binding instance using the btpName from spec
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client create command
// Returns the instance name, UID, and ExternalCreation for tracking
func (m *InstanceManager) CreateInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, btpName string) (string, types.UID, managed.ExternalCreation, error) {
	instanceUID := GenerateInstanceUID(publicCR.UID, btpName)

	subaccountBinding := m.buildSubaccountServiceBinding(publicCR, btpName, instanceUID, "")

	tfController, err := m.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, errConnectTfController)
	}

	creation, err := tfController.Create(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, errCreateTfResource)
	}

	return btpName, instanceUID, creation, nil
}

// DeleteInstance deletes a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client delete command
func (m *InstanceManager) DeleteInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error {
	targetUID := GenerateInstanceUID(publicCR.UID, targetName)
	subaccountBinding := m.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	subaccountBinding.SetDeletionTimestamp(internal.Ptr(metav1.NewTime(time.Now())))
	subaccountBinding.SetConditions(xpv1.Deleting())

	tfController, err := m.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return errors.Wrap(err, errConnectTfController)
	}

	if _, err := tfController.Delete(ctx, subaccountBinding); err != nil {
		return errors.Wrap(err, errDeleteTfResource)
	}

	return nil
}

// UpdateInstance updates a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client update command
func (m *InstanceManager) UpdateInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (managed.ExternalUpdate, error) {
	targetUID := GenerateInstanceUID(publicCR.UID, targetName)
	subaccountBinding := m.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	tfController, err := m.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errConnectTfController)
	}

	return tfController.Update(ctx, subaccountBinding)
}

// ObserveInstance observes a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client observe command
// Returns the observation and the TF resource for data extraction
func (m *InstanceManager) ObserveInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (managed.ExternalObservation, *v1alpha1.SubaccountServiceBinding, error) {
	targetUID := GenerateInstanceUID(publicCR.UID, targetName)
	subaccountBinding := m.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	tfController, err := m.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errConnectTfController)
	}

	observation, err := tfController.Observe(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errObserveTfResource)
	}

	if observation.ResourceExists && observation.ResourceUpToDate {
		publicCR.SetConditions(xpv1.Available())
	}

	return observation, subaccountBinding, nil
}

// buildSubaccountServiceBinding creates a SubaccountServiceBinding resource from a ServiceBinding
func (m *InstanceManager) buildSubaccountServiceBinding(sb *v1alpha1.ServiceBinding, name string, uid types.UID, externalName string) *v1alpha1.SubaccountServiceBinding {
	sBinding := &v1alpha1.SubaccountServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceBinding_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               uid,
			DeletionTimestamp: sb.DeletionTimestamp,
		},
		Spec: v1alpha1.SubaccountServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: sb.GetProviderConfigReference().Name,
				},
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionAll},
			},
			ForProvider: v1alpha1.SubaccountServiceBindingParameters{
				SubaccountID:      sb.Spec.ForProvider.SubaccountID,
				ServiceInstanceID: sb.Spec.ForProvider.ServiceInstanceID,
				Name:              &name,
			},
		},
		Status: v1alpha1.SubaccountServiceBindingStatus{},
	}

	if externalName != "" {
		meta.SetExternalName(sBinding, externalName)
	}
	return sBinding
}