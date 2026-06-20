package rolecollectionuserassignment

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/internal"
	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

func TestContainsGroup(t *testing.T) {
	tests := map[string]struct {
		user           []xsuaa.RoleCollectionAttribute
		role           string
		expectContains bool
	}{
		"nil list": {
			user:           nil,
			role:           "group2",
			expectContains: false,
		},
		"empty list": {
			user:           []xsuaa.RoleCollectionAttribute{},
			role:           "group2",
			expectContains: false,
		},
		"wrong roles": {
			user: []xsuaa.RoleCollectionAttribute{{
				AttributeName:      internal.Ptr(GroupAttributeName),
				AttributeValue:     internal.Ptr("group1"),
				ComparisonOperator: internal.Ptr(GroupComparisionOperator),
			}},
			role:           "group2",
			expectContains: false,
		},
		"right groups": {
			user: []xsuaa.RoleCollectionAttribute{{
				AttributeName:      internal.Ptr(GroupAttributeName),
				AttributeValue:     internal.Ptr("group1"),
				ComparisonOperator: internal.Ptr(GroupComparisionOperator),
			}, {
				AttributeName:      internal.Ptr(GroupAttributeName),
				AttributeValue:     internal.Ptr("group2"),
				ComparisonOperator: internal.Ptr(GroupComparisionOperator),
			}},
			role:           "group2",
			expectContains: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			containsRole := containsGroup(tc.user, tc.role)
			if containsRole != tc.expectContains {
				t.Errorf("containsRole() = %v, want %v", containsRole, tc.expectContains)
			}
		})
	}

}

func TestHasRole(t *testing.T) {
	const roleCollection = "Subaccount Administrator"

	type args struct {
		groupName    string
		groupApiFake *groupApiFake
	}
	type want struct {
		o   bool
		err error
	}

	tests := map[string]struct {
		args args
		want want
	}{
		"AuthError": {
			args: args{
				groupName:    "group2",
				groupApiFake: newGroupApiFake(InvalidCreds, roleCollection, nil),
			},
			want: want{
				o:   false,
				err: oauthError,
			},
		},
		"ApiError": {
			args: args{
				groupName:    "group2",
				groupApiFake: newGroupApiFake(InternalServerError, roleCollection, nil),
			},
			want: want{
				o:   false,
				err: internalServerError,
			},
		},
		"NoRoleCollection": {
			args: args{
				groupName:    "group2",
				groupApiFake: newGroupApiFake(NoGroup, roleCollection, nil),
			},
			want: want{
				o:   false,
				err: notFoundError,
			},
		},
		"NoGroupMapping": {
			args: args{
				groupName:    "group2",
				groupApiFake: newGroupApiFake(GroupWithRoles, roleCollection, []string{"group1"}),
			},
			want: want{
				o:   false,
				err: nil,
			},
		},
		"HasRightGroupRole": {
			args: args{
				groupName:    "group2",
				groupApiFake: newGroupApiFake(GroupWithRoles, roleCollection, []string{"group1", "group2"}),
			},
			want: want{
				o:   true,
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsusaaGroupRoleAssigner{
				groupApi: tc.args.groupApiFake,
			}
			hasRole, err := assigner.HasRole(context.Background(), "origin", tc.args.groupName, roleCollection)
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
		groupApiFake *groupApiFake
		want         want
	}{
		"ApiError": {
			groupApiFake: newGroupApiFake(InternalServerError, roleCollection, nil),
			want: want{
				err: internalServerError,
			},
		},
		"Assign successfully": {
			groupApiFake: newGroupApiFake(NoGroup, roleCollection, nil),
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsusaaGroupRoleAssigner{
				groupApi: tc.groupApiFake,
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
		groupApiFake *groupApiFake
		want         want
	}{
		"ApiError": {
			groupApiFake: newGroupApiFake(InternalServerError, roleCollection, nil),
			want: want{
				err: internalServerError,
			},
		},
		"Revoke successfully": {
			groupApiFake: newGroupApiFake(GroupWithRoles, roleCollection, nil),
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assigner := &XsusaaGroupRoleAssigner{
				groupApi: tc.groupApiFake,
			}
			err := assigner.RevokeRole(context.Background(), "origin", "username", roleCollection)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.RevokeRole(...) error: -want, +got:\n", diff)
			}

		})
	}
}

// TestHasRole_SurfacesErrorBody verifies that an XSUAA GenericOpenAPIError
// carrying a response body is unwrapped via SpecifyAPIError.
func TestHasRole_SurfacesErrorBody(t *testing.T) {
	body := []byte(`{"error":"insufficient_scope"}`)
	gErr := &xsuaa.GenericOpenAPIError{}
	bodyField := reflect.ValueOf(gErr).Elem().FieldByName("body")
	reflect.NewAt(bodyField.Type(), unsafe.Pointer(bodyField.UnsafeAddr())).Elem().Set(reflect.ValueOf(body))

	assigner := &XsusaaGroupRoleAssigner{
		groupApi: groupApiFake{BodyErr: gErr},
	}
	_, err := assigner.HasRole(context.Background(), "origin", "g", "rc")
	if err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("want error containing body, got %v", err)
	}
}
