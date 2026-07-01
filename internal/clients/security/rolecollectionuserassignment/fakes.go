package rolecollectionuserassignment

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

const (
	UserWithoutRole     = ApiScenario("USER_WITHOUT_ROLE")
	UserWithRole        = ApiScenario("USER_WITH_ROLE")
	NoUser              = ApiScenario("NO_USER")
	InternalServerError = ApiScenario("INTERNAL_SERVER_ERROR")
	InvalidCreds        = ApiScenario("INVALID_CREDS")
)

var (
	notFoundError       = errors.New("not found")
	internalServerError = errors.New("internal server error")
	oauthError          = errors.New("invalid credentials")
)

type ApiScenario string

type userApiFake struct {
	Scenario       ApiScenario
	RoleCollection string

	// BodyErr, when set, is returned verbatim from AddRoleCollectionExecute
	// (overriding Scenario). Used to exercise the SpecifyAPIError integration.
	BodyErr error
}

func newUserApiFake(scenario ApiScenario, roleCollection string) *userApiFake {
	return &userApiFake{Scenario: scenario, RoleCollection: roleCollection}
}

func (u userApiFake) AddRoleCollection(ctx context.Context, origin string, userName string, roleCollectionName string) xsuaa.UsercontrollerAPIAddRoleCollectionRequest {
	return xsuaa.UsercontrollerAPIAddRoleCollectionRequest{ApiService: u}
}

func (u userApiFake) AddRoleCollectionExecute(r xsuaa.UsercontrollerAPIAddRoleCollectionRequest) (map[string]interface{}, *http.Response, error) {
	if u.BodyErr != nil {
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, u.BodyErr
	}
	switch u.Scenario {
	case UserWithRole, UserWithoutRole, NoUser:
		return map[string]interface{}{}, &http.Response{StatusCode: http.StatusOK}, nil
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (u userApiFake) CreateUser(ctx context.Context) xsuaa.UsercontrollerAPICreateUserRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) CreateUserExecute(r xsuaa.UsercontrollerAPICreateUserRequest) (*xsuaa.XSUser, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) CreateUsers(ctx context.Context) xsuaa.UsercontrollerAPICreateUsersRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) CreateUsersExecute(r xsuaa.UsercontrollerAPICreateUsersRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) DeleteUserById(ctx context.Context, id string) xsuaa.UsercontrollerAPIDeleteUserByIdRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) DeleteUserByIdExecute(r xsuaa.UsercontrollerAPIDeleteUserByIdRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) DeleteUserByName(ctx context.Context, origin string, userName string) xsuaa.UsercontrollerAPIDeleteUserByNameRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) DeleteUserByNameExecute(r xsuaa.UsercontrollerAPIDeleteUserByNameRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) GetOrigins(ctx context.Context) xsuaa.UsercontrollerAPIGetOriginsRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) GetOriginsExecute(r xsuaa.UsercontrollerAPIGetOriginsRequest) ([]string, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) GetUserById(ctx context.Context, id string) xsuaa.UsercontrollerAPIGetUserByIdRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) GetUserByIdExecute(r xsuaa.UsercontrollerAPIGetUserByIdRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) GetUserByName(ctx context.Context, username string, origin string) xsuaa.UsercontrollerAPIGetUserByNameRequest {
	return xsuaa.UsercontrollerAPIGetUserByNameRequest{ApiService: u}
}

func (u userApiFake) GetUserByNameExecute(r xsuaa.UsercontrollerAPIGetUserByNameRequest) (*xsuaa.XSUser, *http.Response, error) {
	switch u.Scenario {
	case UserWithoutRole:
		return &xsuaa.XSUser{}, &http.Response{StatusCode: http.StatusOK}, nil
	case UserWithRole:
		return &xsuaa.XSUser{RoleCollections: []string{u.RoleCollection}}, &http.Response{StatusCode: http.StatusOK}, nil
	case NoUser:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case InvalidCreds:
		return nil, nil, oauthError
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (u userApiFake) GetUserNames(ctx context.Context, origin string) xsuaa.UsercontrollerAPIGetUserNamesRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) GetUserNamesExecute(r xsuaa.UsercontrollerAPIGetUserNamesRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) RemoveRoleCollection(ctx context.Context, origin string, userName string, roleCollectionName string) xsuaa.UsercontrollerAPIRemoveRoleCollectionRequest {
	return xsuaa.UsercontrollerAPIRemoveRoleCollectionRequest{ApiService: u}
}

func (u userApiFake) RemoveRoleCollectionExecute(r xsuaa.UsercontrollerAPIRemoveRoleCollectionRequest) (*xsuaa.XSUser, *http.Response, error) {
	switch u.Scenario {
	case UserWithRole:
		return &xsuaa.XSUser{}, &http.Response{StatusCode: http.StatusOK}, nil
	case UserWithoutRole, NoUser:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (u userApiFake) UpdateUser(ctx context.Context) xsuaa.UsercontrollerAPIUpdateUserRequest {
	//TODO implement me
	panic("implement me")
}

func (u userApiFake) UpdateUserExecute(r xsuaa.UsercontrollerAPIUpdateUserRequest) (*xsuaa.XSUser, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

var _ xsuaa.UsercontrollerAPI = &userApiFake{}
