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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	instanceClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
)

const (
	errConnectTfController  = "failed to connect TF controller"
	errCreateTfResource     = "failed to create TF resource"
	errDeleteTfResource     = "failed to delete TF resource"
	errObserveTfResource    = "failed to observe TF resource"
	errBuildTfResource      = "failed to build TF resource"
	errBuildParametersField = "failed to build parameters field"
)

// TfConnector provides the interface for connecting to TF controllers
type TfConnector interface {
	Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error)
}

// InstanceManager handles the lifecycle of service binding instances
type InstanceManager struct {
	sbConnector TfConnector
	kube        client.Client
}

func NewInstanceManager(sbConnector TfConnector, kube client.Client) *InstanceManager {
	return &InstanceManager{
		sbConnector: sbConnector,
		kube:        kube,
	}
}

// CreateInstance creates a new service binding instance using the btpName from spec
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client create command
// Returns the instance name, UID, and ExternalCreation for tracking
func (m *InstanceManager) CreateInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, btpName string) (string, types.UID, managed.ExternalCreation, error) {
	// use a random name once for the creation. Afterwards, the external name sets a
	// reasonable name. This means that when observing the resource for the first time after
	// creating, another store for this resource will be created. This will create a dangling
	// TF workspace, but this way no new name collisions will occur.
	instanceUID := GenerateInstanceUID(publicCR.UID, GenerateRandomName(btpName))

	subaccountBinding, err := m.buildSubaccountServiceBinding(ctx, publicCR, btpName, instanceUID, "")
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, errBuildTfResource)
	}

	tfController, err := m.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, errConnectTfController)
	}

	creation, err := tfController.Create(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, errCreateTfResource)
	}

	return meta.GetExternalName(subaccountBinding), instanceUID, creation, nil
}

// DeleteInstance deletes a the actual service binding instance in the BTP. This is done by deleting the virtual SubaccountServiceBinding CR (the TF CR)
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client delete command
func (m *InstanceManager) DeleteInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (managed.ExternalDelete, error) {

	targetUID := GenerateInstanceUID(publicCR.UID, targetExternalName)
	subaccountBinding, err := m.buildSubaccountServiceBinding(ctx, publicCR, targetName, targetUID, targetExternalName)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errBuildTfResource)
	}

	subaccountBinding.SetDeletionTimestamp(internal.Ptr(metav1.NewTime(time.Now())))
	subaccountBinding.SetConditions(xpv1.Deleting())

	tfController, err := m.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errConnectTfController)
	}

	deletion, err := tfController.Delete(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteTfResource)
	}

	return deletion, nil
}

// UpdateInstance updates a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client update command
func (m *InstanceManager) UpdateInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (managed.ExternalUpdate, error) {
	targetUID := GenerateInstanceUID(publicCR.UID, targetExternalName)
	subaccountBinding, err := m.buildSubaccountServiceBinding(ctx, publicCR, targetName, targetUID, targetExternalName)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errBuildTfResource)
	}

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
	targetUID := GenerateInstanceUID(publicCR.UID, targetExternalName)
	subaccountBinding, err := m.buildSubaccountServiceBinding(ctx, publicCR, targetName, targetUID, targetExternalName)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errBuildTfResource)
	}

	tfController, err := m.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errConnectTfController)
	}

	observation, err := tfController.Observe(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errObserveTfResource)
	}

	return observation, subaccountBinding, nil
}

// buildSubaccountServiceBinding creates a SubaccountServiceBinding resource from a ServiceBinding
func (m *InstanceManager) buildSubaccountServiceBinding(ctx context.Context, sb *v1alpha1.ServiceBinding, name string, uid types.UID, externalName string) (*v1alpha1.SubaccountServiceBinding, error) {

	parameterJson, err := instanceClient.BuildComplexParameterJson(ctx, m.kube, sb.Spec.ForProvider.ParameterSecretRefs, sb.Spec.ForProvider.Parameters.Raw)
	if err != nil {
		return nil, errors.Wrap(err, errBuildParametersField)
	}

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
				Parameters:        internal.Ptr(string(parameterJson)),
			},
		},
		Status: v1alpha1.SubaccountServiceBindingStatus{},
	}

	if externalName != "" {
		meta.SetExternalName(sBinding, externalName)
	}
	return sBinding, nil
}
