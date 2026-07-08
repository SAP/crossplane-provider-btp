package rolecollection

import (
	"context"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

var apiRoleCollection = xsuaa.RoleCollection{
	Name:        "test-role-collection",
	Description: internal.Ptr("some-description"),
	RoleReferences: []xsuaa.RoleReference{
		{
			Name:              internal.Ptr("viewer"),
			RoleTemplateAppId: internal.Ptr("app-id-viewer"),
			RoleTemplateName:  internal.Ptr("name-viewer"),
		},
	},
}

func TestGenerateObservation(t *testing.T) {
	type want struct {
		obs v1alpha1.RoleCollectionObservation
		err error
	}

	tests := map[string]struct {
		apiFake *roleCollectionApiFake
		want    want
	}{
		"pre call error": {
			apiFake: &roleCollectionApiFake{Scenario: InvalidCreds, RoleCollection: apiRoleCollection},
			want: want{
				err: invalidCredsError,
			},
		},
		"api error": {
			apiFake: &roleCollectionApiFake{Scenario: InternalServerError, RoleCollection: apiRoleCollection},
			want: want{
				err: internalServerError,
			},
		},
		"Not existing rolecollection": {
			apiFake: &roleCollectionApiFake{Scenario: NoRoleCollection, RoleCollection: apiRoleCollection},
			want: want{
				obs: v1alpha1.RoleCollectionObservation{},
				err: nil,
			},
		},
		"existing rolecollection": {
			apiFake: &roleCollectionApiFake{Scenario: RoleCollectionWithRoles, RoleCollection: apiRoleCollection},
			want: want{
				obs: v1alpha1.RoleCollectionObservation{
					Name:        internal.Ptr(apiRoleCollection.Name),
					Description: internal.Ptr("some-description"),
					RoleReferences: &[]v1alpha1.RoleReference{
						{
							Name:              "viewer",
							RoleTemplateAppId: "app-id-viewer",
							RoleTemplateName:  "name-viewer",
						},
					},
				},
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsuaaRoleCollectionMaintainer{
				apiClient: tc.apiFake,
			}
			obs, err := assigner.GenerateObservation(context.Background(), "test")
			if diff := cmp.Diff(tc.want.obs, obs); diff != "" {
				t.Errorf("\n%s\ne.GenerateObservation(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nNeedsCreation(...) error: -want, +got:\n", diff)
			}

		})
	}
}

func TestNeedsCreation(t *testing.T) {
	type want struct {
		o bool
	}

	tests := map[string]struct {
		obs  v1alpha1.RoleCollectionObservation
		want want
	}{
		"empty observation - needs creation": {
			obs: v1alpha1.RoleCollectionObservation{},
			want: want{
				o: true,
			},
		},
		"cached roles - does not need creation": {
			obs: v1alpha1.RoleCollectionObservation{
				Name: internal.Ptr("test-role-collection"),
			},
			want: want{
				o: false,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsuaaRoleCollectionMaintainer{}
			needsCreation := assigner.NeedsCreation(tc.obs)
			if needsCreation != tc.want.o {
				t.Errorf("NeedsCreation() = %v, want %v", needsCreation, tc.want.o)
			}
		})
	}
}

/*
TestNeedsUpdate tests basic behavior for dealing with api and triggering diff check, see specific test cases:
  - TestRoleDiff for more detailed role diff test cases
*/
func TestNeedsUpdate(t *testing.T) {
	tests := map[string]struct {
		params v1alpha1.RoleCollectionParameters
		obs    v1alpha1.RoleCollectionObservation

		needsUpdate bool
	}{
		"no observation, no update needed": {
			params: v1alpha1.RoleCollectionParameters{
				Name: "test-role-collection",
			},
			obs:         v1alpha1.RoleCollectionObservation{},
			needsUpdate: false,
		},
		"roleCollection needs description update": {
			params: v1alpha1.RoleCollectionParameters{
				Name:        "test-role-collection",
				Description: internal.Ptr("changed-description"),
			},
			obs: v1alpha1.RoleCollectionObservation{
				Name:        internal.Ptr("test-role-collection"),
				Description: internal.Ptr("some-description"),
			},
			needsUpdate: true,
		},
		"roleCollection needs role update": {
			// for more detailed role diff test cases see TestRolesDiff()
			params: v1alpha1.RoleCollectionParameters{
				Name:        "test-role-collection",
				Description: internal.Ptr("some-description"),
				RoleReferences: []v1alpha1.RoleReference{
					{
						Name:              "viewer",
						RoleTemplateAppId: "app-id-viewer",
						RoleTemplateName:  "name-viewer",
					},
				},
			},
			obs: v1alpha1.RoleCollectionObservation{
				Name:        internal.Ptr("test-role-collection"),
				Description: internal.Ptr("some-description"),
			},
			needsUpdate: true,
		},
		"rolecollection up to date": {
			params: v1alpha1.RoleCollectionParameters{
				Name:        "test-role-collection",
				Description: internal.Ptr("some-description"),
				RoleReferences: []v1alpha1.RoleReference{
					{
						Name:              "viewer",
						RoleTemplateAppId: "app-id-viewer",
						RoleTemplateName:  "name-viewer",
					},
				},
			},
			obs: v1alpha1.RoleCollectionObservation{
				Name:        internal.Ptr("test-role-collection"),
				Description: internal.Ptr("some-description"),
				RoleReferences: &[]v1alpha1.RoleReference{
					{
						Name:              "viewer",
						RoleTemplateAppId: "app-id-viewer",
						RoleTemplateName:  "name-viewer",
					},
				},
			},
			needsUpdate: false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsuaaRoleCollectionMaintainer{}
			o := assigner.NeedsUpdate(tc.params, tc.obs)
			if o != tc.needsUpdate {
				t.Errorf("NeedsUpdate() = %v, want %v", o, tc.needsUpdate)
			}
		})
	}
}

func TestRoleDiff(t *testing.T) {
	apiRole := func(name string) v1alpha1.RoleReference {
		return v1alpha1.RoleReference{
			Name: name,
		}
	}

	type want struct {
		add    []v1alpha1.RoleReference
		remove []v1alpha1.RoleReference
	}

	tests := map[string]struct {
		specRoles []v1alpha1.RoleReference
		obsRoles  []v1alpha1.RoleReference
		want      want
	}{
		"roles to add to nil": {
			specRoles: []v1alpha1.RoleReference{apiRole("viewer")},
			obsRoles:  nil,
			want: want{
				add:    []v1alpha1.RoleReference{apiRole("viewer")},
				remove: nil,
			},
		},
		"roles to remove from nil": {
			specRoles: nil,
			obsRoles:  []v1alpha1.RoleReference{apiRole("viewer")},
			want: want{
				add:    nil,
				remove: []v1alpha1.RoleReference{apiRole("viewer")},
			},
		},
		"roles to add and remove": {
			specRoles: []v1alpha1.RoleReference{apiRole("viewer")},
			obsRoles:  []v1alpha1.RoleReference{apiRole("admin")},
			want: want{
				add:    []v1alpha1.RoleReference{apiRole("viewer")},
				remove: []v1alpha1.RoleReference{apiRole("admin")},
			},
		},
		"match empty roles": {
			specRoles: nil,
			obsRoles:  nil,
			want: want{
				add:    nil,
				remove: nil,
			},
		},
		"match roles": {
			specRoles: []v1alpha1.RoleReference{apiRole("viewer"), apiRole("admin")},
			obsRoles:  []v1alpha1.RoleReference{apiRole("viewer"), apiRole("admin")},
			want: want{
				add:    nil,
				remove: nil,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			add, remove := roleDiff(tc.specRoles, tc.obsRoles)
			if diff := cmp.Diff(tc.want.add, add); diff != "" {
				t.Errorf("\n%s\nroleDiff(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.remove, remove); diff != "" {
				t.Errorf("\n%s\nroleDiff(...): -want, +got:\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type want struct {
		nameReturned bool
		err          error
	}

	tests := map[string]struct {
		apiFake *roleCollectionApiFake
		params  v1alpha1.RoleCollectionParameters
		want    want
	}{
		"pre call error": {
			params:  crSpec("test-role"),
			apiFake: &roleCollectionApiFake{Scenario: InvalidCreds},
			want: want{
				err: invalidCredsError,
			},
		},
		"api error": {
			params:  crSpec("test-role"),
			apiFake: &roleCollectionApiFake{Scenario: InternalServerError},
			want: want{
				err: internalServerError,
			},
		},
		"created successfully": {
			params: crSpec("test-role"),
			apiFake: &roleCollectionApiFake{
				Scenario: NoRoleCollection,
			},
			want: want{
				nameReturned: true,
				err:          nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsuaaRoleCollectionMaintainer{
				apiClient: tc.apiFake,
			}
			extName, err := assigner.Create(context.Background(), tc.params)
			if tc.want.nameReturned && extName == "" {
				t.Errorf("Create() didn't return a name as expected")
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nCreate(...) error: -want, +got:\n", diff)
			}

		})
	}
}

func TestUpdate(t *testing.T) {
	type want struct {
		err error
	}

	tests := map[string]struct {
		apiFake *roleCollectionApiFake
		params  v1alpha1.RoleCollectionParameters
		obs     v1alpha1.RoleCollectionObservation
		want    want
	}{
		"update description err": {
			params: v1alpha1.RoleCollectionParameters{
				Description: internal.Ptr("changed-description"),
			},
			obs: v1alpha1.RoleCollectionObservation{
				Description: internal.Ptr("some-description"),
			},
			apiFake: &roleCollectionApiFake{
				Scenario: InternalServerError,
			},
			want: want{
				err: internalServerError,
			},
		},
		"update description success": {
			params: v1alpha1.RoleCollectionParameters{
				Description: internal.Ptr("changed-description"),
			},
			obs: v1alpha1.RoleCollectionObservation{
				Description: internal.Ptr("some-description"),
			},
			apiFake: &roleCollectionApiFake{
				Scenario: RoleCollectionWithRoles,
			},
			want: want{
				err: nil,
			},
		},
		"update roles err": {
			params: v1alpha1.RoleCollectionParameters{
				RoleReferences: []v1alpha1.RoleReference{
					{
						Name:              "viewer",
						RoleTemplateAppId: "app-id-viewer",
						RoleTemplateName:  "name-viewer",
					},
				},
			},
			obs: v1alpha1.RoleCollectionObservation{
				RoleReferences: &[]v1alpha1.RoleReference{
					{
						Name:              "admin",
						RoleTemplateAppId: "app-id-admin",
						RoleTemplateName:  "name-admin",
					},
				},
			},
			apiFake: &roleCollectionApiFake{
				Scenario: InternalServerError,
			},
			want: want{
				err: internalServerError,
			},
		},
		"update roles": {
			params: v1alpha1.RoleCollectionParameters{
				RoleReferences: []v1alpha1.RoleReference{
					{
						Name:              "viewer",
						RoleTemplateAppId: "app-id-viewer",
						RoleTemplateName:  "name-viewer",
					},
				},
			},
			obs: v1alpha1.RoleCollectionObservation{
				RoleReferences: &[]v1alpha1.RoleReference{
					{
						Name:              "admin",
						RoleTemplateAppId: "app-id-admin",
						RoleTemplateName:  "name-admin",
					},
				},
			},
			apiFake: &roleCollectionApiFake{
				Scenario: RoleCollectionWithRoles,
			},
			want: want{
				err: nil,
			},
		},
		"nothing to update, avoid api calls": {
			params: v1alpha1.RoleCollectionParameters{
				Name:        "test-role-collection",
				Description: internal.Ptr("some-description"),
				RoleReferences: []v1alpha1.RoleReference{
					{
						Name:              "viewer",
						RoleTemplateAppId: "app-id-viewer",
						RoleTemplateName:  "name-viewer",
					},
				},
			},
			obs: v1alpha1.RoleCollectionObservation{
				Name:        internal.Ptr("test-role-collection"),
				Description: internal.Ptr("some-description"),
				RoleReferences: &[]v1alpha1.RoleReference{
					{
						Name:              "viewer",
						RoleTemplateAppId: "app-id-viewer",
						RoleTemplateName:  "name-viewer",
					},
				},
			},
			apiFake: &roleCollectionApiFake{
				Scenario: RoleCollectionWithRoles,
			},
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsuaaRoleCollectionMaintainer{
				apiClient: tc.apiFake,
			}
			err := assigner.Update(context.Background(), "some-role-collection", tc.params, tc.obs)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nCreate(...) error: -want, +got:\n", diff)
			}

		})
	}
}

func TestMapApiPayload(t *testing.T) {
	params := v1alpha1.RoleCollectionParameters{
		Name:        "test-role",
		Description: internal.Ptr("some-description"),
		RoleReferences: []v1alpha1.RoleReference{
			{
				Name:              "viewer",
				RoleTemplateAppId: "app-id-viewer",
				RoleTemplateName:  "name-viewer",
			},
			{
				Name:              "admin",
				RoleTemplateAppId: "app-id-admin",
				RoleTemplateName:  "name-admin",
			},
		},
	}

	want := xsuaa.RoleCollection{
		Name:        "test-role",
		Description: internal.Ptr("some-description"),
		RoleReferences: []xsuaa.RoleReference{
			{
				Name:              internal.Ptr("viewer"),
				RoleTemplateAppId: internal.Ptr("app-id-viewer"),
				RoleTemplateName:  internal.Ptr("name-viewer"),
			},
			{
				Name:              internal.Ptr("admin"),
				RoleTemplateAppId: internal.Ptr("app-id-admin"),
				RoleTemplateName:  internal.Ptr("name-admin"),
			},
		},
	}

	payload := mapApiPayload(params)

	if diff := cmp.Diff(want, payload); diff != "" {
		t.Errorf("\n%s\nMapping(...): -want, +got:\n", diff)
	}
}

func TestDelete(t *testing.T) {
	type want struct {
		err error
	}

	tests := map[string]struct {
		apiFake            *roleCollectionApiFake
		roleCollectionName string
		want               want
	}{
		"pre call error": {
			roleCollectionName: "test-role",
			apiFake:            &roleCollectionApiFake{Scenario: InvalidCreds},
			want: want{
				err: invalidCredsError,
			},
		},
		"api error": {
			roleCollectionName: "test-role",
			apiFake:            &roleCollectionApiFake{Scenario: InternalServerError},
			want: want{
				err: internalServerError,
			},
		},
		"deleted successfully": {
			roleCollectionName: "test-role",
			apiFake: &roleCollectionApiFake{
				Scenario: RoleCollectionWithRoles,
			},
			want: want{
				err: nil,
			},
		},
		"ignore not found gracefully": {
			roleCollectionName: "test-role",
			apiFake: &roleCollectionApiFake{
				Scenario: NoRoleCollection,
			},
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsuaaRoleCollectionMaintainer{
				apiClient: tc.apiFake,
			}
			err := assigner.Delete(context.Background(), tc.roleCollectionName)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nDelete(...) error: -want, +got:\n", diff)
			}

		})
	}
}

func crSpec(collectionName string) v1alpha1.RoleCollectionParameters {
	return v1alpha1.RoleCollectionParameters{
		Name:           collectionName,
		RoleReferences: []v1alpha1.RoleReference{},
	}
}

// TestGenerateObservation_SurfacesErrorBody verifies that an XSUAA
// GenericOpenAPIError carrying a response body is unwrapped via SpecifyAPIError.
func TestGenerateObservation_SurfacesErrorBody(t *testing.T) {
	assigner := &XsuaaRoleCollectionMaintainer{
		apiClient: &roleCollectionApiFake{BodyErr: testutils.NewXsuaaAPIError([]byte(`{"error":"insufficient_scope"}`))},
	}
	_, err := assigner.GenerateObservation(context.Background(), "test")
	if err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("want error containing body, got %v", err)
	}
}
