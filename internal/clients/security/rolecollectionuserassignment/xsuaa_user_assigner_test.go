package rolecollectionuserassignment

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

func TestContainsRole(t *testing.T) {
	tests := map[string]struct {
		user           *xsuaa.XSUser
		role           string
		expectContains bool
	}{
		"nil list": {
			user:           &xsuaa.XSUser{RoleCollections: nil},
			role:           "role3",
			expectContains: false,
		},
		"empty list": {
			user:           &xsuaa.XSUser{RoleCollections: []string{}},
			role:           "role3",
			expectContains: false,
		},
		"wrong roles": {
			user:           &xsuaa.XSUser{RoleCollections: []string{"role1", "role2"}},
			role:           "role3",
			expectContains: false,
		},
		"right roles": {
			user:           &xsuaa.XSUser{RoleCollections: []string{"role1", "role2", "role3"}},
			role:           "role3",
			expectContains: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			containsRole := containsRole(tc.user, tc.role)
			if containsRole != tc.expectContains {
				t.Errorf("containsRole() = %v, want %v", containsRole, tc.expectContains)
			}
		})
	}

}

func TestHasRole(t *testing.T) {
	const roleCollection = "Subaccount Administrator"

	type want struct {
		o   bool
		err error
	}

	tests := map[string]struct {
		userApiFake *userApiFake
		want        want
	}{
		"AuthError": {
			userApiFake: newUserApiFake(InvalidCreds, roleCollection),
			want: want{
				o:   false,
				err: oauthError,
			},
		},
		"ApiError": {
			userApiFake: newUserApiFake(InternalServerError, roleCollection),
			want: want{
				o:   false,
				err: internalServerError,
			},
		},
		"Not existing user": {
			userApiFake: newUserApiFake(NoUser, roleCollection),
			want: want{
				o:   false,
				err: nil,
			},
		},
		"User without role": {
			userApiFake: newUserApiFake(UserWithoutRole, roleCollection),
			want: want{
				o:   false,
				err: nil,
			},
		},
		"User has role": {
			userApiFake: newUserApiFake(UserWithRole, roleCollection),
			want: want{
				o:   true,
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsusaaUserRoleAssigner{
				userApi: tc.userApiFake,
			}
			hasRole, err := assigner.HasRole(context.Background(), "origin", "username", roleCollection)
			if hasRole != tc.want.o {
				t.Errorf("HasRole() = %v, want %v", hasRole, tc.want.o)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Has Role(...) error: -want, +got:\n", diff)
			}

		})
	}
}

func TestAssignRole(t *testing.T) {
	const roleCollection = "Subaccount Administrator"

	type want struct {
		err error
	}

	tests := map[string]struct {
		userApiFake *userApiFake
		want        want
	}{
		"ApiError": {
			userApiFake: newUserApiFake(InternalServerError, roleCollection),
			want: want{
				err: internalServerError,
			},
		},
		"Assign successfully": {
			userApiFake: newUserApiFake(NoUser, roleCollection),
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsusaaUserRoleAssigner{
				userApi: tc.userApiFake,
			}
			err := assigner.AssignRole(context.Background(), "origin", "username", roleCollection)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.AssignRole(...) error: -want, +got:\n", diff)
			}

		})
	}
}

func TestRevokeRole(t *testing.T) {
	const roleCollection = "Subaccount Administrator"

	type want struct {
		err error
	}

	tests := map[string]struct {
		userApiFake *userApiFake
		want        want
	}{
		"ApiError": {
			userApiFake: newUserApiFake(InternalServerError, roleCollection),
			want: want{
				err: internalServerError,
			},
		},
		"Revoke successfully": {
			userApiFake: newUserApiFake(UserWithRole, roleCollection),
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsusaaUserRoleAssigner{
				userApi: tc.userApiFake,
			}
			err := assigner.RevokeRole(context.Background(), "origin", "username", roleCollection)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.RevokeRole(...) error: -want, +got:\n", diff)
			}

		})
	}
}
