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

// ServiceBindingClientInterface provides the interface for service binding client operations
type ServiceBindingClientInterface interface {
	// Creates a servicebinding
	Create(ctx context.Context) (string, managed.ExternalCreation, error)
	// Deletes a service binding instance
	Delete(ctx context.Context) (managed.ExternalDelete, error)
	// Updates a service binding instance
	Update(ctx context.Context) (managed.ExternalUpdate, error)
	// Observes a service binding instance
	Observe(ctx context.Context) (managed.ExternalObservation, *v1alpha1.SubaccountServiceBinding, error)
}

// ServiceBindingClient handles the lifecycle of service binding instances
type ServiceBindingClient struct {
	tfClient managed.ExternalClient
	kube     client.Client
	ssb      *v1alpha1.SubaccountServiceBinding
}

func NewServiceBindingClient(ctx context.Context, kube client.Client, tfConnector TfConnector, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (*ServiceBindingClient, error) {
	subaccountServiceBinding, err := buildSubaccountServiceBinding(ctx, kube, cr, targetName, targetExternalName)
	if err != nil {
		return nil, err
	}
	tfClient, err := tfConnector.Connect(ctx, subaccountServiceBinding)
	if err != nil {
		return nil, err
	}

	return &ServiceBindingClient{
		tfClient: tfClient,
		kube:     kube,
		ssb:      subaccountServiceBinding,
	}, nil
}

func (m *ServiceBindingClient) Create(ctx context.Context) (string, managed.ExternalCreation, error) {
	// use a random name once for the creation. Afterwards, the external name sets a
	// reasonable name. This means that when observing the resource for the first time after
	// creating, another store for this resource will be created. This will create a dangling
	// TF workspace, but this way no new name collisions will occur.
	// instanceUID := GenerateInstanceUID(m.ssb.UID, GenerateRandomName(*m.ssb.Spec.ForProvider.Name))
	//
	// m.ssb.SetUID(instanceUID)

	creation, err := m.tfClient.Create(ctx, m.ssb)
	if err != nil {
		return "", managed.ExternalCreation{}, errors.Wrap(err, errCreateTfResource)
	}

	return meta.GetExternalName(m.ssb), creation, nil
}

func (m *ServiceBindingClient) Delete(ctx context.Context) (managed.ExternalDelete, error) {
	m.ssb.SetDeletionTimestamp(internal.Ptr(metav1.NewTime(time.Now())))
	m.ssb.SetConditions(xpv1.Deleting())

	deletion, err := m.tfClient.Delete(ctx, m.ssb)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteTfResource)
	}

	return deletion, nil
}

// Updates a service binding instance
func (m *ServiceBindingClient) Update(ctx context.Context) (managed.ExternalUpdate, error) {
	// Update is ignored. This is because it is assumed that all fields need
	// a recreation of the resource. This logic could be outsourced to the controller,
	// but keeping this in the client keeps the controller straight forward, keeping all
	// special cases in the client.
	return managed.ExternalUpdate{}, nil
}

// Observes a servicebinding
func (m *ServiceBindingClient) Observe(ctx context.Context) (managed.ExternalObservation, *v1alpha1.SubaccountServiceBinding, error) {
	observation, err := m.tfClient.Observe(ctx, m.ssb)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errObserveTfResource)
	}

	// For some unclear reason (underlying code is very abstract,
	// may need some further investigation in the future), the observation
	// always wants an update on the resource because it thinks that the "labels" field
	// in m.ssb has a diff. It has not as you can see in debugging.
	// We can hardcode "ResourceUpToDate = true", because for a servicebinding,
	// all fields are immutable and would require a recreation of the resource.
	// This seems to be abug in the upjet tfpluginfw implementation, because this didn't
	// happen before upgrading to the tfpluginfw implementation.
	// TODO: Check if really all fields are immutable
	observation.ResourceUpToDate = true

	return observation, m.ssb, nil
}

// buildSubaccountServiceBinding creates a SubaccountServiceBinding resource from a ServiceBinding
func buildSubaccountServiceBinding(ctx context.Context, kube client.Client, sb *v1alpha1.ServiceBinding, name string, externalName string) (*v1alpha1.SubaccountServiceBinding, error) {

	parameterJson, err := instanceClient.BuildComplexParameterJson(ctx, kube, sb.Spec.ForProvider.ParameterSecretRefs, sb.Spec.ForProvider.Parameters.Raw)
	if err != nil {
		return nil, errors.Wrap(err, errBuildParametersField)
	}

	targetUID := GenerateInstanceUID(sb.UID, externalName)

	sBinding := &v1alpha1.SubaccountServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceBinding_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               targetUID,
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
