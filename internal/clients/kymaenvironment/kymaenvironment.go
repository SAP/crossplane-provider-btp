package environments

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/meta"

	"github.com/sap/crossplane-provider-btp/internal"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
)

const (
	errKymaInstanceCreateFailed = "Could not create KymaEnvironment"
	errKymaInstanceUpdateFailed = "Could not update KymaEnvironment"
	errInstanceIdNotFound       = "Could not update kyma instance .status.AtProvider.Id is empty"
)

type KymaEnvironments struct {
	btp btp.Client
}

func NewKymaEnvironments(btp btp.Client) *KymaEnvironments {
	return &KymaEnvironments{btp: btp}
}

func (c KymaEnvironments) DescribeInstance(
	ctx context.Context,
	cr v1alpha1.KymaEnvironment,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
	environment, err := c.btp.GetEnvironment(ctx, meta.GetExternalName(&cr), cr.Name, btp.KymaEnvironmentType())

	if err != nil {
		return nil, false, err
	}

	if environment == nil {
		return nil, false, nil
	}

	// If the external name is not set yet, we set it to the ID of the environment. And force an update.
	if *environment.Id != meta.GetExternalName(&cr) {
		meta.SetExternalName(&cr, *environment.Id)
		return environment, true, nil
	}

	return environment, false, nil
}

func (c KymaEnvironments) CreateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (string, error) {

	parameters, err := internal.UnmarshalRawParameters(cr.Spec.ForProvider.Parameters.Raw)
	parameters = AddKymaDefaultParameters(parameters, cr.Name, string(cr.UID))
	if err != nil {
		return "", err
	}
	guid, err := c.btp.CreateKymaEnvironment(
		ctx,
		cr.Name,
		cr.Spec.ForProvider.PlanName,
		parameters,
		string(cr.UID),
		c.btp.Credential.UserCredential.Email,
	)
	if err != nil {
		return "", errors.Wrap(err, errKymaInstanceCreateFailed)
	}
	return guid, nil
}

func (c KymaEnvironments) DeleteInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error {
	if cr.Status.AtProvider.ID == nil {
		return errors.New(errInstanceIdNotFound)
	}
	return c.btp.DeleteEnvironmentById(ctx, *cr.Status.AtProvider.ID)
}

func (c KymaEnvironments) UpdateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error {

	if cr.Status.AtProvider.ID == nil {
		return errors.New(errInstanceIdNotFound)
	}

	parameters, err := internal.UnmarshalRawParameters(cr.Spec.ForProvider.Parameters.Raw)
	parameters = AddKymaDefaultParameters(parameters, cr.Name, string(cr.UID))
	if err != nil {
		return err
	}
	err = c.btp.UpdateKymaEnvironment(
		ctx,
		*cr.Status.AtProvider.ID,
		cr.Spec.ForProvider.PlanName,
		parameters,
		string(cr.UID),
	)

	return errors.Wrap(err, errKymaInstanceUpdateFailed)
}

func AddKymaDefaultParameters(parameters btp.InstanceParameters, instanceName string, resourceUID string) btp.InstanceParameters {
	parameters[btp.KymaenvironmentParameterInstanceName] = instanceName
	return parameters
}
