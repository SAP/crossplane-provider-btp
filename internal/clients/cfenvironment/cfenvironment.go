package environments

import (
	"context"
	"fmt"
	"strings"

	cfv3 "github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
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

func (c CloudFoundryOrganization) NeedsUpdate(cr v1alpha1.CloudFoundryEnvironment) bool {
	toAdd, toRemove := c.managerDiff(cr)
	return len(toAdd) > 0 || len(toRemove) > 0
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

	return errors.Wrap(err, instanceCreateFailed)
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

// managerDiff returns the users to add and to remove by comparing spec and status and ignoring credentials user email address
func (c CloudFoundryOrganization) managerDiff(cr v1alpha1.CloudFoundryEnvironment) ([]v1alpha1.User, []v1alpha1.User) {
	toAdd := make([]v1alpha1.User, 0)
	toRemove := make([]v1alpha1.User, 0)

	ignoreUser := c.createdByUser()

	for _, managerEmail := range cr.Spec.ForProvider.Managers {
		manager := v1alpha1.User{Username: managerEmail}
		if !strings.EqualFold(ignoreUser.String(), manager.String()) && !containsUser(cr.Status.AtProvider.Managers, manager) {
			toAdd = append(toAdd, manager)
		}
	}

	for _, manager := range cr.Status.AtProvider.Managers {
		if !strings.EqualFold(ignoreUser.String(), manager.String()) && !containsUser(toUsers(cr.Spec.ForProvider.Managers), manager) {
			toRemove = append(toRemove, manager)
		}
	}

	return toAdd, toRemove
}

// returns User from credentials to allow ignoring it in manager Updates (since it's done on the API side)
func (c CloudFoundryOrganization) createdByUser() v1alpha1.User {
	if c.btp.Credential == nil || c.btp.Credential.UserCredential == nil {
		return v1alpha1.User{Username: "", Origin: defaultOrigin}
	}
	return v1alpha1.User{Username: c.btp.Credential.UserCredential.Email, Origin: defaultOrigin}
}

func (o organizationClient) addManager(ctx context.Context, username string, origin string) error {

	_, err := o.c.Roles.CreateOrganizationRoleWithUsername(ctx, o.orgGuid, username, resource.OrganizationRoleManager, origin)

	return err

}

func (o organizationClient) deleteManager(ctx context.Context, username string, origin string) error {
	userGuid, err := o.findUserGuidByName(ctx, username, origin)
	if err != nil {
		return err
	}

	if userGuid == nil {
		return errors.Errorf(errUserNotFound, username)
	}

	// find manager roles for the user and delete them
	listOptions := cfv3.NewRoleListOptions()
	listOptions.OrganizationGUIDs.EqualTo(o.orgGuid)
	listOptions.WithOrganizationRoleType(resource.OrganizationRoleManager)
	listOptions.UserGUIDs.EqualTo(*userGuid)
	roles, err := o.c.Roles.ListAll(ctx, listOptions)
	if err != nil {
		return err
	}

	for _, role := range roles {
		if _, err := o.c.Roles.Delete(ctx, role.GUID); err != nil {
			return err
		}
	}

	return nil
}

func (o organizationClient) findUserGuidByName(ctx context.Context, username string, origin string) (*string, error) {
	ulo := cfv3.NewUserListOptions()

	ulo.UserNames = cfv3.Filter{
		Values: append(make([]string, 0), username),
	}
	ulo.Origins = cfv3.Filter{
		Values: append(make([]string, 0), origin),
	}

	users, err := o.c.Users.ListAll(ctx, ulo)
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, nil
	}

	if len(users) > 1 {
		return nil, errors.Errorf(errUserFoundMultipleTimes, username)
	}

	return &users[0].GUID, nil

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

func (c CloudFoundryOrganization) UpdateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	toAdd, toRemove := c.managerDiff(cr)

	if err := c.updateManagers(ctx, toAdd, toRemove, cr); err != nil {
		return err
	}
	return nil
}

func (c CloudFoundryOrganization) updateManagers(ctx context.Context,
	toAdd []v1alpha1.User,
	toRemove []v1alpha1.User,
	cr v1alpha1.CloudFoundryEnvironment,
) error {
	cloudFoundryClient, err := c.createClientWithType(&cr)
	if err != nil {
		return err
	}

	for _, u := range toAdd {
		if err := cloudFoundryClient.addManager(ctx, u.Username, u.Origin); err != nil {
			return err
		}
	}
	for _, u := range toRemove {
		if err := cloudFoundryClient.deleteManager(ctx, u.Username, u.Origin); err != nil {
			return err
		}
	}
	return nil
}
