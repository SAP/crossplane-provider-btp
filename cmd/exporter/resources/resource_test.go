package resources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateK8sResourceName(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	tests := []struct {
		name        string
		id          string
		displayName string
		kind        string
		wantName    string
		wantErr     bool
	}{
		{
			name:        "resource with valid display name",
			id:          "12345678-1234-1234-1234-123456789abc",
			displayName: "Test resource",
			kind:        "resource",
			wantName:    "test-resource",
			wantErr:     false,
		},
		{
			name:        "resource with special characters in display name",
			id:          "12345678-1234-1234-1234-123456789abc",
			displayName: "Test_Resource With Spaces & Special!@#",
			kind:        "resource",
			wantName:    "test-resource-with-spaces---special--at-x",
			wantErr:     false,
		},
		{
			name:        "resource missing display name uses kind and id",
			id:          "12345678-1234-1234-1234-123456789abc",
			displayName: "",
			kind:        "resource",
			wantName:    "resource-12345678-1234-1234-1234-123456789abc",
			wantErr:     false,
		},
		{
			name:        "resource missing both name and id",
			id:          "",
			displayName: "",
			kind:        "resource",
			wantName:    UndefinedName,
			wantErr:     false,
		},
		{
			name:        "resource with valid display name but empty id and kind",
			id:          "",
			displayName: "Test resource",
			kind:        "",
			wantName:    "test-resource",
			wantErr:     false,
		},

		// Edge cases
		{
			name:        "only id provided with no display name and kind",
			id:          "some-id-123",
			displayName: "",
			kind:        "",
			wantName:    UndefinedName,
			wantErr:     false,
		},
		{
			name:        "only kind provided",
			id:          "",
			displayName: "",
			kind:        "resource",
			wantName:    UndefinedName,
			wantErr:     false,
		},
		{
			name:        "display name with only special characters",
			id:          "id-123",
			displayName: "!@#$%^&*()",
			kind:        "test",
			wantName:    "x--at--------x",
			wantErr:     false,
		},
		{
			name:        "display name with unicode characters",
			id:          "id-123",
			displayName: "Resource 日本語",
			kind:        "test",
			wantName:    "resource---------x",
			wantErr:     false,
		},
		{
			name:        "display name with numbers",
			id:          "id-123",
			displayName: "resource-123-test",
			kind:        "test",
			wantName:    "resource-123-test",
			wantErr:     false,
		},
		{
			name:        "display name with brackets",
			id:          "id-123",
			displayName: "Resource [with brackets]",
			kind:        "test",
			wantName:    "resource--with-bracketsx",
			wantErr:     false,
		},
		{
			name:        "display name with dots",
			id:          "id-123",
			displayName: "service.btp.sap",
			kind:        "test",
			wantName:    "service.btp.sap",
			wantErr:     false,
		},
		{
			name:        "very long display name",
			id:          "id-123",
			displayName: "this-is-a-very-long-resource-name-that-might-need-to-be-truncated-to-fit-kubernetes-naming-constraints",
			kind:        "test",
			wantName:    "this-is-a-very-long-resource-name-that-might-need-to-be-truncat",
			wantErr:     false,
		},
		{
			name:        "display name starting with number",
			id:          "id-123",
			displayName: "123-resource",
			kind:        "test",
			wantName:    "x123-resource",
			wantErr:     false,
		},
		{
			name:        "display name with multiple consecutive special chars",
			id:          "id-123",
			displayName: "resource___---test",
			kind:        "test",
			wantName:    "resource------test",
			wantErr:     false,
		},
		{
			name:        "empty strings for all parameters",
			id:          "",
			displayName: "",
			kind:        "",
			wantName:    UndefinedName,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, err := GenerateK8sResourceName(tt.id, tt.displayName, tt.kind)

			if tt.wantErr {
				r.Error(err, "expected an error")
			} else {
				r.NoError(err, "unexpected error")
			}

			r.Equal(tt.wantName, gotName, "resource name mismatch")
		})
	}
}
