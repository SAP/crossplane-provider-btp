package environments

import (
	"context"
	"net/http"

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
	errExternalNameNotFound     = "external-name not set"
	errInstanceIdNotFound       = "instance ID not found in status for update operation"
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
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	// If external-name is empty, resource needs to be created. Should be checked by the caller already
	externalName := meta.GetExternalName(&cr)
	if externalName == "" {
		return nil, nil
	}

	environment, err := c.btp.GetKymaEnvironment(ctx, externalName, GetKymaEnvironmentName(cr), btp.KymaEnvironmentType())

	if err != nil {
		return nil, err
	}

	if environment == nil {
		return nil, nil
	}

	return environment, nil
}

func (c KymaEnvironments) CreateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (string, error) {

	parameters, err := internal.UnmarshalRawParameters(cr.Spec.ForProvider.Parameters.Raw)
	parameters = AddKymaDefaultParameters(parameters, GetKymaEnvironmentName(cr), string(cr.UID))
	if err != nil {
		return "", err
	}
	guid, err := c.btp.CreateKymaEnvironment(
		ctx,
		GetKymaEnvironmentName(cr),
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

// DeleteInstance deletes the Kyma environment using the external-name (GUID).
// Returns the HTTP response for status code checking and any error.
func (c KymaEnvironments) DeleteInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (*http.Response, error) {
	externalName := meta.GetExternalName(&cr)

	// Use external-name (GUID) for deletion
	if externalName == "" {
		return nil, errors.New(errExternalNameNotFound)
	}

	return c.btp.DeleteEnvironmentInstanceByID(ctx, externalName)
}

func (c KymaEnvironments) UpdateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error {
	externalName := meta.GetExternalName(&cr)

	if externalName == "" {
		return errors.New(errExternalNameNotFound)
	}

	parameters, err := internal.UnmarshalRawParameters(cr.Spec.ForProvider.Parameters.Raw)
	parameters = AddKymaDefaultParameters(parameters, GetKymaEnvironmentName(cr), string(cr.UID))
	if err != nil {
		return err
	}
	err = c.btp.UpdateKymaEnvironment(
		ctx,
		externalName,
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

// Defaults to the name of the CR if forProvider.name is not set
func GetKymaEnvironmentName(cr v1alpha1.KymaEnvironment) string {
	name := cr.Name
	if cr.Spec.ForProvider.Name != nil {
		name = *cr.Spec.ForProvider.Name
	}
	return name
}
