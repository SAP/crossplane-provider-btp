package environments

import (
	"context"
	"fmt"
	cfv3 "github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

const (
	instanceCreateFailed      = "could not create CloudFoundryEnvironment"
	errUserFoundMultipleTimes = "user %s found multiple times"
	errUserNotFound           = "user %s not found"
	errRoleUpdateFailed       = "role update failed with status code %d"
	errLogin                  = "cloud not login to cloud foundry"
	errClient                 = "cloud not create cf client"

	defaultOrigin = "sap.ids"
)

var _ Client = &CloudFoundryOrganization{}

type CloudFoundryOrganization struct {
	btp btp.Client
}

// NeedsUpdate not needed anymore (no reconciliation wanted)
func (c CloudFoundryOrganization) NeedsUpdate(cr v1alpha1.CloudFoundryEnvironment) bool {
	return false
}

// UpdateInstance not needed anymore (no reconciliation wanted)
func (c CloudFoundryOrganization) UpdateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	return nil
}

func NewCloudFoundryOrganization(btp btp.Client) *CloudFoundryOrganization {
	return &CloudFoundryOrganization{btp: btp}
}

func (c CloudFoundryOrganization) DescribeInstance(
	ctx context.Context,
	cr v1alpha1.CloudFoundryEnvironment,
) (*provisioningclient.EnvironmentInstanceResponseObject, []v1alpha1.User, error) {
	name := meta.GetExternalName(&cr)
	environment, err := c.btp.GetEnvironmentByNameAndType(ctx, name, btp.CloudFoundryEnvironmentType())
	if err != nil {
		return nil, nil, err
	}

	if environment == nil {
		return nil, nil, nil
	}

	cloudFoundryClient, err := c.createClient(environment)

	if err != nil {
		return nil, nil, err
	}

	if cloudFoundryClient == nil {
		return environment, nil, nil
	}

	managers, err := cloudFoundryClient.getManagerUsernames(ctx)
	if err != nil {
		return nil, nil, err
	}

	return environment, managers, nil

}

func (c CloudFoundryOrganization) createClient(environment *provisioningclient.EnvironmentInstanceResponseObject) (
	*organizationClient,
	error,
) {
	org, err := c.btp.ExtractOrg(environment)
	if err != nil {
		return nil, err
	}

	cloudFoundryClient, err := newOrganizationClient(
		org.Name, org.ApiEndpoint, org.Id, c.btp.Credential.UserCredential.Username,
		c.btp.Credential.UserCredential.Password,
	)
	return cloudFoundryClient, err
}

func (c CloudFoundryOrganization) createClientWithType(environment *v1alpha1.CloudFoundryEnvironment) (
	*organizationClient,
	error,
) {
	org, err := c.btp.NewCloudFoundryOrgByLabel(*environment.Status.AtProvider.Labels)
	if err != nil {
		return nil, err
	}

	cloudFoundryClient, err := newOrganizationClient(
		org.Name, org.ApiEndpoint, org.Id, c.btp.Credential.UserCredential.Username,
		c.btp.Credential.UserCredential.Password,
	)
	return cloudFoundryClient, err
}

func (c CloudFoundryOrganization) CreateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	cloudFoundryOrgName := cr.Name
	adminServiceAccountEmail := c.btp.Credential.UserCredential.Email

	err := c.btp.CreateCloudFoundryOrgIfNotExists(
		ctx, cloudFoundryOrgName, adminServiceAccountEmail, string(cr.UID),
		cr.Spec.ForProvider.Landscape,
	)
	if err != nil {
		return errors.Wrap(err, instanceCreateFailed)
	}

	cloudFoundryClient, err := c.createClientWithType(&cr)
	if err != nil {
		return errors.Wrap(err, instanceCreateFailed)
	}

	for _, managerEmail := range cr.Spec.ForProvider.Managers {
		if err := cloudFoundryClient.addManager(ctx, managerEmail, defaultOrigin); err != nil {
			return errors.Wrap(err, instanceCreateFailed)
		}
	}

	return errors.New(instanceCreateFailed)
}

func (c CloudFoundryOrganization) DeleteInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	return c.btp.DeleteEnvironment(ctx, cr.Name, btp.CloudFoundryEnvironmentType())
}

type organizationClient struct {
	c                cfv3.Client
	username         string
	organizationName string
	orgGuid          string
}

func (o organizationClient) addManager(ctx context.Context, username string, origin string) error {

	_, err := o.c.Roles.CreateOrganizationRoleWithUsername(ctx, o.orgGuid, username, resource.OrganizationRoleManager, origin)

	return err

}

func (o organizationClient) getManagerUsernames(ctx context.Context) ([]v1alpha1.User, error) {
	listOptions := cfv3.NewRoleListOptions()
	listOptions.OrganizationGUIDs.EqualTo(o.orgGuid)
	listOptions.WithOrganizationRoleType(resource.OrganizationRoleManager)

	_, users, err := o.c.Roles.ListIncludeUsersAll(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	managers := make([]v1alpha1.User, 0)
	for _, u := range users {
		m := v1alpha1.User{
			Username: u.Username,
			Origin:   u.Origin,
		}
		managers = append(managers, m)
	}

	return managers, nil
}

func newOrganizationClient(organizationName string, url string, orgId string, username string, password string) (
	*organizationClient, error,
) {
	cfv3config, err := config.New(url, config.UserPassword(username, password))

	if organizationName == "" {
		return nil, fmt.Errorf("missing or empty organization name")
	}
	if orgId == "" {
		return nil, fmt.Errorf("missing or empty orgGuid")
	}

	if err != nil {
		return nil, errors.Wrap(err, errLogin)
	}

	cfv3client, err := cfv3.New(cfv3config)

	if err != nil {
		return nil, errors.Wrap(err, errClient)
	}
	return &organizationClient{
		c:                *cfv3client,
		username:         username,
		organizationName: organizationName,
		orgGuid:          orgId,
	}, nil
}
