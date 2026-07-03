package rolecollectionuserassignment

import (
	"context"
	"net/url"

	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/security"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
	"golang.org/x/oauth2/clientcredentials"
)

// Constants for group comparison
const (
	GroupComparisionOperator = "equals"
	GroupAttributeName       = "Groups"
)

func NewXsuaaGroupRoleAssigner(ctx context.Context, clientId, clientSecret, tokenUrl, apiUrl string) *XsusaaGroupRoleAssigner {
	config := clientcredentials.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		TokenURL:     tokenUrl,
	}

	smURL, _ := url.Parse(apiUrl)

	apiClientConfig := xsuaa.NewConfiguration()
	apiClientConfig.Host = smURL.Host
	apiClientConfig.Scheme = smURL.Scheme
	apiClientConfig.HTTPClient = config.Client(ctx)

	groupApi := xsuaa.NewAPIClient(apiClientConfig).IdpRoleCollectionAPI

	return &XsusaaGroupRoleAssigner{
		groupApi: groupApi,
	}
}

// XsusaaGroupRoleAssigner manages role collection assignments for groups within XSUAA.
type XsusaaGroupRoleAssigner struct {
	groupApi xsuaa.IdpRoleCollectionAPI
}

// HasRole checks if a group has a specific role within XSUAA.
func (x *XsusaaGroupRoleAssigner) HasRole(ctx context.Context, origin, groupName, roleCollection string) (bool, error) {
	// Retrieve role collection attributes and check for the specified group
	res, _, err := x.groupApi.GetIdpAttributeValuesFromRoleCollectionByAttribute(ctx, origin, GroupAttributeName, roleCollection).Execute()
	if err != nil {
		return false, security.SpecifyAPIError(err)
	}
	return containsGroup(res, groupName), nil
}

// AssignRole assigns a specified role to a group within XSUAA.
func (x *XsusaaGroupRoleAssigner) AssignRole(ctx context.Context, origin, groupName, rolecollection string) error {
	_, _, err := x.groupApi.AddIdpAttributeToRoleCollection(ctx, origin).IdentityProviderMapping(
		xsuaa.IdentityProviderMapping{
			RoleCollectionName: internal.Ptr(rolecollection),
			AttributeName:      internal.Ptr(GroupAttributeName),
			AttributeValue:     internal.Ptr(groupName),
			Operator:           internal.Ptr(GroupComparisionOperator),
		}).Execute()
	return security.SpecifyAPIError(err)
}

// RevokeRole removes a specified role from a group within XSUAA.
func (x *XsusaaGroupRoleAssigner) RevokeRole(ctx context.Context, origin, groupName, rolecollection string) error {
	_, _, err := x.groupApi.DeleteIdpAttributeToRoleCollection(ctx, origin, GroupAttributeName, GroupComparisionOperator, groupName, rolecollection).Execute()
	return security.SpecifyAPIError(err)
}

// containsGroup checks if the role collection's attributes contain the specified group.
func containsGroup(attrs []xsuaa.RoleCollectionAttribute, group string) bool {
	if attrs == nil {
		return false
	}
	// Iterate through role collection attributes to find a match for the group assignment
	for _, a := range attrs {
		if *a.AttributeName == GroupAttributeName && *a.AttributeValue == group && *a.ComparisonOperator == GroupComparisionOperator {
			return true
		}
	}
	return false
}
