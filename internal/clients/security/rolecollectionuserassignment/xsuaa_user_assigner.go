package rolecollectionuserassignment

import (
	"context"
	"net/http"
	"net/url"

	"github.com/sap/crossplane-provider-btp/internal/clients/security"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
	"golang.org/x/oauth2/clientcredentials"
)

func NewXsuaaUserRoleAssigner(ctx context.Context, clientId, clientSecret, tokenUrl, apiUrl string) *XsusaaUserRoleAssigner {
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

	userApi := xsuaa.NewAPIClient(apiClientConfig).UsercontrollerAPI

	return &XsusaaUserRoleAssigner{
		userApi: userApi,
	}
}

// XsusaaUserRoleAssigner manages rolecollection assignments for plain users within XSUAA
type XsusaaUserRoleAssigner struct {
	userApi xsuaa.UsercontrollerAPI
}

// HasRole checks if a user has a specific role within XSUAA.
func (x *XsusaaUserRoleAssigner) HasRole(ctx context.Context, origin, username, roleCollection string) (bool, error) {
	// Retrieve user details from XSUAA and check for the specified role
	user, h, err := x.userApi.GetUserByName(ctx, username, origin).Execute()
	if err != nil {
		if h != nil && h.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, security.SpecifyAPIError(err)
	}
	return containsRole(user, roleCollection), nil
}

// AssignRole assigns a specified role to a user within XSUAA.
func (x *XsusaaUserRoleAssigner) AssignRole(ctx context.Context, origin, username, rolecollection string) error {
	// Add the role collection to the user
	_, _, err := x.userApi.AddRoleCollection(ctx, origin, username, rolecollection).CreateUserIfMissing(true).Execute()
	return security.SpecifyAPIError(err)
}

// RevokeRole removes a specified role from a user within XSUAA.
func (x *XsusaaUserRoleAssigner) RevokeRole(ctx context.Context, origin, username, rolecollection string) error {
	_, _, err := x.userApi.RemoveRoleCollection(ctx, origin, username, rolecollection).Execute()
	return security.SpecifyAPIError(err)
}

// containsRole checks if the user's role collections contain the specified role.
func containsRole(user *xsuaa.XSUser, roleCollection string) bool {
	if user.RoleCollections == nil {
		return false
	}
	// Iterate through user's role collections to find a match
	for _, role := range user.RoleCollections {
		if role == roleCollection {
			return true
		}
	}
	return false
}
