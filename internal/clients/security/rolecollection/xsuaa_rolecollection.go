package rolecollection

import (
	"context"
	"net/url"
	"reflect"

	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/cluster/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
	"golang.org/x/oauth2/clientcredentials"
)

// NewXsuaaRoleCollectionMaintainer initializes new XsuaaRoleCollectionMaintainer with auth configuration
func NewXsuaaRoleCollectionMaintainer(ctx context.Context, clientId, clientSecret, tokenUrl, apiUrl string) *XsuaaRoleCollectionMaintainer {
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

	roleCollectionApi := xsuaa.NewAPIClient(apiClientConfig).RolecollectionsAPI

	return &XsuaaRoleCollectionMaintainer{
		apiClient: roleCollectionApi,
	}
}

type XsuaaRoleCollectionMaintainer struct {
	apiClient xsuaa.RolecollectionsAPI
}

func (x *XsuaaRoleCollectionMaintainer) GenerateObservation(ctx context.Context, roleCollectionName string) (v1alpha1.RoleCollectionObservation, error) {
	roleCollection, h, err := x.apiClient.GetRoleCollectionByName(ctx, roleCollectionName).Execute()
	if err != nil {
		// error 404 means the role collection does not exist, which is a valid state in this case
		if h != nil && h.StatusCode == 404 {
			return v1alpha1.RoleCollectionObservation{}, nil
		}
		return v1alpha1.RoleCollectionObservation{}, err
	}

	return mapObservation(roleCollection), nil
}

func (x *XsuaaRoleCollectionMaintainer) NeedsCreation(observation v1alpha1.RoleCollectionObservation) bool {
	return observation.Name == nil
}

func (x *XsuaaRoleCollectionMaintainer) NeedsUpdate(params v1alpha1.RoleCollectionParameters, obs v1alpha1.RoleCollectionObservation) bool {
	toAdd, toRemove := roleDiff(params.RoleReferences, internal.Val(obs.RoleReferences))

	return toAdd != nil || toRemove != nil || descriptionChanged(params, obs)
}

func (x *XsuaaRoleCollectionMaintainer) Create(ctx context.Context, params v1alpha1.RoleCollectionParameters) (string, error) {
	execute, _, err := x.apiClient.CreateRoleCollection(ctx).RoleCollection(mapApiPayload(params)).Execute()
	if err != nil {
		return "", err
	}
	return execute.Name, nil
}

func (x *XsuaaRoleCollectionMaintainer) Update(ctx context.Context, roleCollectionName string, params v1alpha1.RoleCollectionParameters, obs v1alpha1.RoleCollectionObservation) error {
	if descriptionChanged(params, obs) {
		if err := x.UpdateDescription(ctx, roleCollectionName, params.Description); err != nil {
			return err
		}
	}

	toAdd, toRemove := roleDiff(params.RoleReferences, internal.Val(obs.RoleReferences))

	if toAdd != nil {
		if err := x.AddRolesToRoleCollection(ctx, roleCollectionName, mapApiRoles(params.RoleReferences)); err != nil {
			return err
		}
	}
	if toRemove != nil {
		if err := x.DeleteRolesFromRoleCollection(ctx, roleCollectionName, mapApiRoles(params.RoleReferences)); err != nil {
			return err
		}
	}
	return nil
}

func (x *XsuaaRoleCollectionMaintainer) Delete(ctx context.Context, roleCollectionName string) error {
	_, h, err := x.apiClient.DeleteRoleCollectionByName(ctx, roleCollectionName).Execute()

	// gracefully ignore errors in case of not found
	if h != nil && h.StatusCode == 404 {
		return nil
	}
	return err
}

// UpdateDescription updates the description of a role collection using the xsuaa api
func (x *XsuaaRoleCollectionMaintainer) UpdateDescription(ctx context.Context, roleCollectionName string, description *string) error {
	_, _, err := x.apiClient.ChangeRoleCollectionDescription(ctx, roleCollectionName).
		RoleCollectionDescription(xsuaa.RoleCollectionDescription{Description: description}).
		Execute()
	return err
}

// AddRolesToRoleCollection adds roles to a role collection using the xsuaa api
func (x *XsuaaRoleCollectionMaintainer) AddRolesToRoleCollection(ctx context.Context, roleCollectionName string, roles []xsuaa.RoleReference) error {
	_, _, err := x.apiClient.AddRolesToRoleCollection(ctx, roleCollectionName).RoleReference(roles).Execute()
	return err
}

// DeleteRolesFromRoleCollection deletes roles from a role collection using the xsuaa api
func (x *XsuaaRoleCollectionMaintainer) DeleteRolesFromRoleCollection(ctx context.Context, roleCollectionName string, roles []xsuaa.RoleReference) error {
	_, _, err := x.apiClient.DeleteRolesFromRoleCollection(ctx, roleCollectionName).RoleReference(roles).Execute()
	return err
}

// roleDiff returns the list of roles to be added and removed from the existing role collection
func roleDiff(specRoles, apiRoles []v1alpha1.RoleReference) (toAdd, toRemove []v1alpha1.RoleReference) {
	// calculate roles to be added
	for _, specRole := range specRoles {
		if !containsRole(apiRoles, specRole.Name) {
			toAdd = append(toAdd, specRole)
		}
	}
	// calculate roles to be removed
	for _, apiRole := range apiRoles {
		if !containsRole(specRoles, apiRole.Name) {
			toRemove = append(toRemove, apiRole)
		}
	}
	return toAdd, toRemove
}

// containsRole checks if a role with the given name exists in the list of roles
func containsRole(roles []v1alpha1.RoleReference, roleName string) bool {
	for _, role := range roles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}

// mapApiPayload maps the CRD spec to api payload
func mapApiPayload(params v1alpha1.RoleCollectionParameters) xsuaa.RoleCollection {
	return xsuaa.RoleCollection{
		Name:           params.Name,
		Description:    params.Description,
		RoleReferences: mapApiRoles(params.RoleReferences),
	}
}

// mapObservation maps the API model to the CRD observation, just a simple type mapping
func mapObservation(apiCollection *xsuaa.RoleCollection) v1alpha1.RoleCollectionObservation {
	return v1alpha1.RoleCollectionObservation{
		Name:           internal.Ptr(apiCollection.Name),
		Description:    apiCollection.Description,
		RoleReferences: internal.Ptr(mapObservationRoles(apiCollection.RoleReferences)),
	}
}

// mapObservationRoles maps the role references from the api type to the CRD observation type
func mapObservationRoles(roles []xsuaa.RoleReference) []v1alpha1.RoleReference {
	var result []v1alpha1.RoleReference
	for _, role := range roles {
		result = append(result, v1alpha1.RoleReference{
			RoleTemplateAppId: internal.Val(role.RoleTemplateAppId),
			RoleTemplateName:  internal.Val(role.RoleTemplateName),
			Name:              internal.Val(role.Name),
		})
	}
	return result
}

// mapApiRoles maps the role references from the CRD to the API model
func mapApiRoles(roles []v1alpha1.RoleReference) []xsuaa.RoleReference {
	var result []xsuaa.RoleReference
	for _, role := range roles {
		result = append(result, xsuaa.RoleReference{
			RoleTemplateAppId: internal.Ptr(role.RoleTemplateAppId),
			RoleTemplateName:  internal.Ptr(role.RoleTemplateName),
			Name:              internal.Ptr(role.Name),
		})
	}
	return result
}

// descriptionChanged checks description change between spec and status
func descriptionChanged(params v1alpha1.RoleCollectionParameters, obs v1alpha1.RoleCollectionObservation) bool {
	return !reflect.DeepEqual(params.Description, obs.Description)
}
