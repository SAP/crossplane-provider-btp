package environments

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	cfv3 "github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
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

// NeedsUpdate reports whether configured managers are not yet assigned in the Cloud Foundry org. This is the case when
// a create failed after the instance was created. It relies on the managers observed into the status by Observe.
func (c CloudFoundryOrganization) NeedsUpdate(cr v1alpha1.CloudFoundryEnvironment) bool {
	return len(c.missingManagers(cr)) > 0
}

// UpdateInstance assigns the configured managers that are missing in the Cloud Foundry org. Other drift is not reconciled.
func (c CloudFoundryOrganization) UpdateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	missing := c.missingManagers(cr)
	if len(missing) == 0 {
		return nil
	}

	org, err := c.btp.GetCloudFoundryOrg(ctx, meta.GetExternalName(&cr))
	if err != nil {
		return err
	}
	if org == nil {
		return errors.New("cloud foundry org not found")
	}
	cloudFoundryClient, err := c.createClientWithType(org)
	if err != nil {
		return err
	}
	for _, managerEntry := range missing {
		username, origin := parseManagerString(managerEntry)
		if err := cloudFoundryClient.addManager(ctx, username, origin); err != nil {
			return err
		}
	}
	return nil
}

// missingManagers returns the configured managers that are not among the observed managers. The admin user is skipped,
// as it is assigned by the create itself.
func (c CloudFoundryOrganization) missingManagers(cr v1alpha1.CloudFoundryEnvironment) []string {
	var missing []string
	for _, managerEntry := range filterOutUser(cr.Spec.ForProvider.Managers, c.btp.Credential.UserCredential.Email) {
		username, origin := parseManagerString(managerEntry)
		if !hasManager(cr.Status.AtProvider.Managers, username, origin) {
			missing = append(missing, managerEntry)
		}
	}
	return missing
}

func hasManager(managers []v1alpha1.User, username, origin string) bool {
	for _, m := range managers {
		if strings.EqualFold(m.Username, username) && m.Origin == origin {
			return true
		}
	}
	return false
}

func NewCloudFoundryOrganization(btp btp.Client) *CloudFoundryOrganization {
	return &CloudFoundryOrganization{btp: btp}
}

func (c CloudFoundryOrganization) DescribeInstance(
	ctx context.Context,
	cr v1alpha1.CloudFoundryEnvironment,
) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error) {
	environment, err := c.getEnvironmentByExternalNameWithLegacyHandling(ctx, cr)
	if err != nil {
		return nil, nil, err
	}
	if environment == nil {
		return nil, nil, nil
	}

	managers, err := c.getManagers(ctx, environment)
	if err != nil {
		return nil, nil, err
	}

	return environment, managers, nil
}

// getEnvironmentByExternalName retrieves CF environment using external-name
// Supports GUID format (new) and backwards compatibility with orgName and metadata.name (legacy)
// returns (environment, needsExternalNameFormatMigration, error)
func (c CloudFoundryOrganization) getEnvironmentByExternalNameWithLegacyHandling(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error) {
	externalName := meta.GetExternalName(&cr)

	// An empty external-name can also mean that a previous create reached CIS but did not record the instance ID.
	// Create stores the resource UID in the instance parameters, so an environment carrying it was created by this
	// resource and gets adopted instead of created a second time.
	if externalName == "" {
		return c.btp.GetCFEnvironmentByResourceUID(ctx, string(cr.UID))
	}

	orgName := FormOrgName(cr.Spec.ForProvider.OrgName, cr.Spec.SubaccountGuid, cr.Name)

	// Try GUID lookup first (new standard format)
	if internal.IsValidUUID(externalName) {
		environment, notFound, err := c.btp.GetEnvironmentInstanceByID(ctx, externalName)
		if notFound {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return environment, nil
	}

	// Backwards compatibility: try legacy lookup by orgName and name. Name was set as external name in v1.1.0
	// This handles migration from v1.1.0 (orgName) and v1.0.0 (metadata.name)
	environment, err := c.btp.GetCFEnvironmentByNameAndOrg(ctx, externalName, orgName)
	if err != nil {
		return nil, err
	}
	return environment, nil
}

func (c CloudFoundryOrganization) getManagers(ctx context.Context, environment *provisioningclient.BusinessEnvironmentInstanceResponseObject) ([]v1alpha1.User, error) {
	cloudFoundryClient, err := c.createClient(environment)
	if err != nil {
		return nil, err
	}

	if cloudFoundryClient == nil {
		return nil, nil
	}

	managers, err := cloudFoundryClient.getManagerUsernames(ctx)
	if err != nil {
		return nil, err
	}
	return managers, nil
}

func (c CloudFoundryOrganization) createClient(environment *provisioningclient.BusinessEnvironmentInstanceResponseObject) (
	*organizationClient,
	error,
) {
	org, err := c.btp.ExtractOrg(environment)
	if err != nil {
		return nil, err
	}

	cloudFoundryClient, err := newOrganizationClient(
		org.Name, org.ApiEndpoint, org.Id, c.btp.Credential.UserCredential.Username,
		c.btp.Credential.UserCredential.Password, c.btp.Credential.UserCredential.Idp,
	)
	return cloudFoundryClient, err
}

func (c CloudFoundryOrganization) createClientWithType(org *btp.CloudFoundryOrg) (
	*organizationClient,
	error,
) {
	cloudFoundryClient, err := newOrganizationClient(
		org.Name, org.ApiEndpoint, org.Id, c.btp.Credential.UserCredential.Username,
		c.btp.Credential.UserCredential.Password, c.btp.Credential.UserCredential.Idp,
	)
	return cloudFoundryClient, err
}

// CreateInstance creates the environment and assigns the managers. If a step after the CIS create fails, the instance
// ID is returned together with the error, so it can still be stored as external-name.
func (c CloudFoundryOrganization) CreateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) (string, error) {
	adminServiceAccountEmail := c.btp.Credential.UserCredential.Email
	orgName := FormOrgName(cr.Spec.ForProvider.OrgName, cr.Spec.SubaccountGuid, cr.Name)
	instanceId, org, err := c.btp.CreateCloudFoundryEnvironmentAndGetOrg(
		ctx, cr.Name, adminServiceAccountEmail, string(cr.UID),
		cr.Spec.ForProvider.Landscape, orgName, cr.Spec.ForProvider.EnvironmentName,
	)
	if err != nil {
		// instanceId is only set once CIS accepted the create. Return it with the error so the caller can keep it.
		return instanceId, errors.Wrap(err, instanceCreateFailed)
	}
	if org == nil {
		return instanceId, errors.New(instanceCreateFailed + ": cloud foundry org not found")
	}

	cloudFoundryClient, err := c.createClientWithType(org)
	if err != nil {
		return instanceId, errors.Wrap(err, instanceCreateFailed)
	}

	for _, managerEntry := range filterOutUser(cr.Spec.ForProvider.Managers, adminServiceAccountEmail) {
		username, origin := parseManagerString(managerEntry)
		if err := cloudFoundryClient.addManager(ctx, username, origin); err != nil {
			return instanceId, errors.Wrap(err, instanceCreateFailed)
		}
	}

	return instanceId, nil
}

func (c CloudFoundryOrganization) DeleteInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) (*http.Response, error) {
	externalName := meta.GetExternalName(&cr)

	// Use external-name for deletion
	// Legacy format does not need to be handled since the ID will be updated in Observe phase already to the GUID
	return c.btp.DeleteEnvironmentInstanceByID(ctx, externalName)
}

func FormOrgName(orgName string, subaccountId string, crName string) string {
	if orgName == "" {
		return subaccountId + "-" + crName
	}
	return orgName
}

func filterOutUser(users []string, exclude string) []string {
	result := make([]string, 0, len(users))
	for _, u := range users {
		if u != exclude {
			result = append(result, u)
		}
	}
	return result
}

// parseManagerString parses "email" or "email|origin" into (username, origin).
// If no origin is provided, defaults to defaultOrigin.
func parseManagerString(s string) (username, origin string) {
	if idx := strings.Index(s, "|"); idx >= 0 {
		return s[:idx], s[idx+1:]
	}
	return s, defaultOrigin
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
		if u != nil && u.Username != nil && u.Origin != nil {
			m := v1alpha1.User{
				Username: *u.Username,
				Origin:   *u.Origin,
			}
			managers = append(managers, m)
		}
	}

	return managers, nil
}

func newOrganizationClient(organizationName string, url string, orgId string, username string, password string, origin string) (
	*organizationClient, error,
) {
	if organizationName == "" {
		return nil, fmt.Errorf("missing or empty organization name")
	}
	if orgId == "" {
		return nil, fmt.Errorf("missing or empty orgGuid")
	}

	// Reuse an authenticated config per credential so we do not log in on every
	// Observe (see cf_auth_cache.go).
	cfv3config, err := cachedCFConfig(url, username, password, origin)
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
