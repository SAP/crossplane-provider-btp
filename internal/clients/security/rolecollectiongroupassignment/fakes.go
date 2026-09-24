package rolecollectionuserassignment

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/internal"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

const (
	GroupWithRoles      = ApiScenario("GROUP_WITH_ROLE")
	NoGroup             = ApiScenario("NO_GROUP")
	InternalServerError = ApiScenario("INTERNAL_SERVER_ERROR")
	InvalidCreds        = ApiScenario("INVALID_CREDS")
)

var (
	notFoundError       = errors.New("not found")
	internalServerError = errors.New("internal server error")
	oauthError          = errors.New("invalid credentials")
)

type ApiScenario string

type groupApiFake struct {
	RoleCollection string
	Scenario       ApiScenario
	Groups         []string

	// BodyErr, when set, is returned verbatim from GetIdpAttributeValuesFromRoleCollectionByAttributeExecute
	// (overriding Scenario). Used to exercise the SpecifyAPIError integration.
	BodyErr error
}

var _ xsuaa.IdpRoleCollectionAPI = &groupApiFake{}

func newGroupApiFake(scenario ApiScenario, roleCollection string, group []string) *groupApiFake {
	return &groupApiFake{Scenario: scenario, RoleCollection: roleCollection, Groups: group}
}

func (g groupApiFake) AddIdpAttributeToRoleCollection(ctx context.Context, origin string) xsuaa.IdpRoleCollectionAPIAddIdpAttributeToRoleCollectionRequest {
	return xsuaa.IdpRoleCollectionAPIAddIdpAttributeToRoleCollectionRequest{ApiService: g}
}

func (g groupApiFake) AddIdpAttributeToRoleCollectionExecute(r xsuaa.IdpRoleCollectionAPIAddIdpAttributeToRoleCollectionRequest) (map[string]interface{}, *http.Response, error) {
	switch g.Scenario {
	case NoGroup, GroupWithRoles:
		return map[string]interface{}{}, &http.Response{StatusCode: http.StatusOK}, nil
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError

	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (g groupApiFake) DeleteIdpAttributeToRoleCollection(ctx context.Context, origin string, attributeName string, operator string, attributeValue string, roleCollectionName string) xsuaa.IdpRoleCollectionAPIDeleteIdpAttributeToRoleCollectionRequest {
	return xsuaa.IdpRoleCollectionAPIDeleteIdpAttributeToRoleCollectionRequest{ApiService: g}
}

func (g groupApiFake) DeleteIdpAttributeToRoleCollectionExecute(r xsuaa.IdpRoleCollectionAPIDeleteIdpAttributeToRoleCollectionRequest) (map[string]interface{}, *http.Response, error) {
	switch g.Scenario {
	case NoGroup, GroupWithRoles:
		return map[string]interface{}{}, &http.Response{StatusCode: http.StatusOK}, nil
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (g groupApiFake) GetIdpAttributeValues(ctx context.Context, origin string) xsuaa.IdpRoleCollectionAPIGetIdpAttributeValuesRequest {
	//TODO implement me
	panic("implement me")
}

func (g groupApiFake) GetIdpAttributeValuesExecute(r xsuaa.IdpRoleCollectionAPIGetIdpAttributeValuesRequest) ([]xsuaa.RoleCollectionAttribute, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (g groupApiFake) GetIdpAttributeValuesFromRoleCollection(ctx context.Context, origin string, roleCollectionName string) xsuaa.IdpRoleCollectionAPIGetIdpAttributeValuesFromRoleCollectionRequest {
	//TODO implement me
	panic("implement me")
}

func (g groupApiFake) GetIdpAttributeValuesFromRoleCollectionExecute(r xsuaa.IdpRoleCollectionAPIGetIdpAttributeValuesFromRoleCollectionRequest) ([]xsuaa.RoleCollectionAttribute, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (g groupApiFake) GetIdpAttributeValuesFromRoleCollectionByAttribute(ctx context.Context, origin string, attributeName string, roleCollectionName string) xsuaa.IdpRoleCollectionAPIGetIdpAttributeValuesFromRoleCollectionByAttributeRequest {
	return xsuaa.IdpRoleCollectionAPIGetIdpAttributeValuesFromRoleCollectionByAttributeRequest{ApiService: g}
}

func (g groupApiFake) GetIdpAttributeValuesFromRoleCollectionByAttributeExecute(r xsuaa.IdpRoleCollectionAPIGetIdpAttributeValuesFromRoleCollectionByAttributeRequest) ([]xsuaa.RoleCollectionAttribute, *http.Response, error) {
	if g.BodyErr != nil {
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, g.BodyErr
	}
	switch g.Scenario {
	case NoGroup:
		return nil, &http.Response{StatusCode: http.StatusNotFound}, notFoundError
	case GroupWithRoles:
		return mapRoleCollectionAttributes(g.RoleCollection, g.Groups), &http.Response{StatusCode: http.StatusOK}, nil
	case InternalServerError:
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
	case InvalidCreds:
		return nil, nil, oauthError
	}
	return nil, &http.Response{StatusCode: http.StatusInternalServerError}, internalServerError
}

func (g groupApiFake) GetRoleCollectionByAttributeValue(ctx context.Context, origin string, attributeName string, attributeValue string) xsuaa.IdpRoleCollectionAPIGetRoleCollectionByAttributeValueRequest {
	//TODO implement me
	panic("implement me")
}

func (g groupApiFake) GetRoleCollectionByAttributeValueExecute(r xsuaa.IdpRoleCollectionAPIGetRoleCollectionByAttributeValueRequest) ([]xsuaa.RoleCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

// mapRoleCollectionAttributes maps the given groups to a slice of RoleCollectionAttribute
func mapRoleCollectionAttributes(roleCollection string, groups []string) []xsuaa.RoleCollectionAttribute {
	if groups == nil {
		return nil
	}
	var roleCollectionAttributes []xsuaa.RoleCollectionAttribute
	for _, group := range groups {
		roleCollectionAttributes = append(roleCollectionAttributes, xsuaa.RoleCollectionAttribute{
			RoleCollectionName: internal.Ptr(roleCollection),
			AttributeName:      internal.Ptr(GroupAttributeName),
			ComparisonOperator: internal.Ptr(GroupComparisionOperator),
			AttributeValue:     internal.Ptr(group),
		})
	}
	return roleCollectionAttributes
}
