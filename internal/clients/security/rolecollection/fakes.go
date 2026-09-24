package rolecollection

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

const (
	NoRoleCollection           = ApiScenario("NO_ROLE_COLLECTION")
	InternalServerError        = ApiScenario("INTERNAL_SERVER_ERROR")
	InvalidCreds               = ApiScenario("INVALID_CREDS")
	RoleCollectionWithoutRoles = ApiScenario("ROLE_COLLECTION_WITHOUT_ROLES")
	RoleCollectionWithRoles    = ApiScenario("ROLE_COLLECTION_WITH_ROLES")
)

var (
	notFoundError       = errors.New("not found")
	internalServerError = errors.New("internal server error")
	invalidCredsError   = errors.New("invalid credentials")
)

type ApiScenario string

type roleCollectionApiFake struct {
	Scenario ApiScenario

	// stub data to return whenever a RoleCollection would be returned
	RoleCollection xsuaa.RoleCollection

	// BodyErr, when set, is returned verbatim from GetRoleCollectionByNameExecute
	// (overriding Scenario). Used to exercise the SpecifyAPIError integration.
	BodyErr error
}

var _ xsuaa.RolecollectionsAPI = &roleCollectionApiFake{}

func (rf *roleCollectionApiFake) AddRoleToRoleCollection(ctx context.Context, roleCollectionName string, roleTemplateAppID string, roleName string, roleTemplateName string) xsuaa.RolecollectionsAPIAddRoleToRoleCollectionRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) AddRoleToRoleCollectionExecute(r xsuaa.RolecollectionsAPIAddRoleToRoleCollectionRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) AddRolesToRoleCollection(ctx context.Context, roleCollectionName string) xsuaa.RolecollectionsAPIAddRolesToRoleCollectionRequest {
	return xsuaa.RolecollectionsAPIAddRolesToRoleCollectionRequest{ApiService: rf}
}

func (rf *roleCollectionApiFake) AddRolesToRoleCollectionExecute(r xsuaa.RolecollectionsAPIAddRolesToRoleCollectionRequest) (map[string]interface{}, *http.Response, error) {
	switch rf.Scenario {
	case InvalidCreds:
		return nil, nil, invalidCredsError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case NoRoleCollection:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case RoleCollectionWithRoles, RoleCollectionWithoutRoles:
		return map[string]interface{}{}, &http.Response{StatusCode: http.StatusOK}, nil
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (rf *roleCollectionApiFake) ChangeRoleCollectionDescription(ctx context.Context, roleCollectionName string) xsuaa.RolecollectionsAPIChangeRoleCollectionDescriptionRequest {
	return xsuaa.RolecollectionsAPIChangeRoleCollectionDescriptionRequest{ApiService: rf}
}

func (rf *roleCollectionApiFake) ChangeRoleCollectionDescriptionExecute(r xsuaa.RolecollectionsAPIChangeRoleCollectionDescriptionRequest) (map[string]interface{}, *http.Response, error) {
	switch rf.Scenario {
	case InvalidCreds:
		return nil, nil, invalidCredsError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case NoRoleCollection:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case RoleCollectionWithRoles, RoleCollectionWithoutRoles:
		return map[string]interface{}{}, &http.Response{StatusCode: http.StatusOK}, nil
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (rf *roleCollectionApiFake) CreateRoleCollection(ctx context.Context) xsuaa.RolecollectionsAPICreateRoleCollectionRequest {
	return xsuaa.RolecollectionsAPICreateRoleCollectionRequest{ApiService: rf}
}

func (rf *roleCollectionApiFake) CreateRoleCollectionExecute(r xsuaa.RolecollectionsAPICreateRoleCollectionRequest) (*xsuaa.RoleCollection, *http.Response, error) {
	switch rf.Scenario {
	case InvalidCreds:
		return nil, nil, invalidCredsError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case NoRoleCollection:
		// we expect a name is returned, but we don't care about the content
		return &xsuaa.RoleCollection{Name: "api-role"}, &http.Response{StatusCode: http.StatusOK}, nil
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (rf *roleCollectionApiFake) CreateRoleCollections(ctx context.Context) xsuaa.RolecollectionsAPICreateRoleCollectionsRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) CreateRoleCollectionsExecute(r xsuaa.RolecollectionsAPICreateRoleCollectionsRequest) (map[string]map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) CreateRoleCollectionsForUser(ctx context.Context) xsuaa.RolecollectionsAPICreateRoleCollectionsForUserRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) CreateRoleCollectionsForUserExecute(r xsuaa.RolecollectionsAPICreateRoleCollectionsForUserRequest) (*xsuaa.RoleCollectionListDto, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) DeleteRoleCollectionByName(ctx context.Context, roleCollectionName string) xsuaa.RolecollectionsAPIDeleteRoleCollectionByNameRequest {
	return xsuaa.RolecollectionsAPIDeleteRoleCollectionByNameRequest{ApiService: rf}
}

func (rf *roleCollectionApiFake) DeleteRoleCollectionByNameExecute(r xsuaa.RolecollectionsAPIDeleteRoleCollectionByNameRequest) (map[string]interface{}, *http.Response, error) {
	switch rf.Scenario {
	case InvalidCreds:
		return nil, nil, invalidCredsError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case NoRoleCollection:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case RoleCollectionWithRoles, RoleCollectionWithoutRoles:
		return map[string]interface{}{}, &http.Response{StatusCode: http.StatusOK}, nil
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (rf *roleCollectionApiFake) DeleteRoleCollections(ctx context.Context) xsuaa.RolecollectionsAPIDeleteRoleCollectionsRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) DeleteRoleCollectionsExecute(r xsuaa.RolecollectionsAPIDeleteRoleCollectionsRequest) (*http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) DeleteRoleCollectionsForUser(ctx context.Context) xsuaa.RolecollectionsAPIDeleteRoleCollectionsForUserRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) DeleteRoleCollectionsForUserExecute(r xsuaa.RolecollectionsAPIDeleteRoleCollectionsForUserRequest) (*http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) DeleteRoleFromRoleCollection(ctx context.Context, roleCollectionName string, roleTemplateAppID string, roleName string, roleTemplateName string) xsuaa.RolecollectionsAPIDeleteRoleFromRoleCollectionRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) DeleteRoleFromRoleCollectionExecute(r xsuaa.RolecollectionsAPIDeleteRoleFromRoleCollectionRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) DeleteRolesFromRoleCollection(ctx context.Context, roleCollectionName string) xsuaa.RolecollectionsAPIDeleteRolesFromRoleCollectionRequest {
	return xsuaa.RolecollectionsAPIDeleteRolesFromRoleCollectionRequest{ApiService: rf}
}

func (rf *roleCollectionApiFake) DeleteRolesFromRoleCollectionExecute(r xsuaa.RolecollectionsAPIDeleteRolesFromRoleCollectionRequest) (map[string]interface{}, *http.Response, error) {
	switch rf.Scenario {
	case InvalidCreds:
		return nil, nil, invalidCredsError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case NoRoleCollection:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case RoleCollectionWithRoles, RoleCollectionWithoutRoles:
		return map[string]interface{}{}, &http.Response{StatusCode: http.StatusOK}, nil
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (rf *roleCollectionApiFake) GetRoleCollectionByName(ctx context.Context, roleCollectionName string) xsuaa.RolecollectionsAPIGetRoleCollectionByNameRequest {
	return xsuaa.RolecollectionsAPIGetRoleCollectionByNameRequest{ApiService: rf}
}

func (rf *roleCollectionApiFake) GetRoleCollectionByNameExecute(r xsuaa.RolecollectionsAPIGetRoleCollectionByNameRequest) (*xsuaa.RoleCollection, *http.Response, error) {
	if rf.BodyErr != nil {
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, rf.BodyErr
	}
	switch rf.Scenario {
	case InvalidCreds:
		return nil, nil, invalidCredsError
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case NoRoleCollection:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case RoleCollectionWithoutRoles:
		return &xsuaa.RoleCollection{RoleReferences: []xsuaa.RoleReference{}}, &http.Response{StatusCode: http.StatusOK}, nil
	case RoleCollectionWithRoles:
		return &rf.RoleCollection, &http.Response{StatusCode: http.StatusOK}, nil
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (rf *roleCollectionApiFake) GetRoleCollections(ctx context.Context) xsuaa.RolecollectionsAPIGetRoleCollectionsRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsExecute(r xsuaa.RolecollectionsAPIGetRoleCollectionsRequest) ([]xsuaa.RoleCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByAppIdTemplateNameAndRoleName(ctx context.Context, appId string, roleTemplateName string, roleName string) xsuaa.RolecollectionsAPIGetRoleCollectionsByAppIdTemplateNameAndRoleNameRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByAppIdTemplateNameAndRoleNameExecute(r xsuaa.RolecollectionsAPIGetRoleCollectionsByAppIdTemplateNameAndRoleNameRequest) ([]xsuaa.RoleCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByPaging(ctx context.Context, pageId int32) xsuaa.RolecollectionsAPIGetRoleCollectionsByPagingRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByPagingExecute(r xsuaa.RolecollectionsAPIGetRoleCollectionsByPagingRequest) ([]xsuaa.RoleCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByPaging1(ctx context.Context, pageId int32) xsuaa.RolecollectionsAPIGetRoleCollectionsByPaging1Request {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByPaging1Execute(r xsuaa.RolecollectionsAPIGetRoleCollectionsByPaging1Request) ([]xsuaa.RoleCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByPaging2(ctx context.Context, pageId int32) xsuaa.RolecollectionsAPIGetRoleCollectionsByPaging2Request {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsByPaging2Execute(r xsuaa.RolecollectionsAPIGetRoleCollectionsByPaging2Request) ([]xsuaa.RoleCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsForUser(ctx context.Context, userId string) xsuaa.RolecollectionsAPIGetRoleCollectionsForUserRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsForUserExecute(r xsuaa.RolecollectionsAPIGetRoleCollectionsForUserRequest) (*xsuaa.RoleCollectionListDto, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsForUser1(ctx context.Context, userId string) xsuaa.RolecollectionsAPIGetRoleCollectionsForUser1Request {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRoleCollectionsForUser1Execute(r xsuaa.RolecollectionsAPIGetRoleCollectionsForUser1Request) (*xsuaa.RoleCollectionListDto, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRolesByRoleCollectionName(ctx context.Context, roleCollectionName string) xsuaa.RolecollectionsAPIGetRolesByRoleCollectionNameRequest {
	//TODO implement me
	panic("implement me")
}

func (rf *roleCollectionApiFake) GetRolesByRoleCollectionNameExecute(r xsuaa.RolecollectionsAPIGetRolesByRoleCollectionNameRequest) ([]xsuaa.Role, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}
