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
		wantName    string
		wantErr     bool
	}{
		{
			name:        "resource with valid display name and id",
			id:          "12345678-1234-1234-1234-123456789abc",
			displayName: "Test resource",
			wantName:    "test-resource.x12345678-1234-1234-1234-123456789abc",
			wantErr:     false,
		},
		{
			name:        "resource with special characters in display name",
			id:          "12345678-1234-1234-1234-123456789abc",
			displayName: "Test_Resource With Spaces & Special!@#",
			wantName:    "test-resource-with-spaces---special--at-x.x12345678-1234-1234-1234-123456789abc",
			wantErr:     false,
		},
		{
			name:        "resource missing display name",
			id:          "12345678-1234-1234-1234-123456789abc",
			displayName: "",
			wantName:    UndefinedName,
			wantErr:     false,
		},
		{
			name:        "resource missing both name and id",
			id:          "",
			displayName: "",
			wantName:    UndefinedName,
			wantErr:     false,
		},
		{
			name:        "resource with valid display name but empty id",
			id:          "",
			displayName: "Test resource",
			wantName:    "test-resource",
			wantErr:     false,
		},

		// Edge cases
		{
			name:        "display name with only special characters",
			id:          "id-123",
			displayName: "!@#$%^&*()",
			wantName:    "x--at--------x.id-123",
			wantErr:     false,
		},
		{
			name:        "display name with unicode characters",
			id:          "id-123",
			displayName: "Resource 日本語",
			wantName:    "resource---------x.id-123",
			wantErr:     false,
		},
		{
			name:        "display name with numbers",
			id:          "id-123",
			displayName: "resource-123-test",
			wantName:    "resource-123-test.id-123",
			wantErr:     false,
		},
		{
			name:        "display name with brackets",
			id:          "id-123",
			displayName: "Resource [with brackets]",
			wantName:    "resource--with-bracketsx.id-123",
			wantErr:     false,
		},
		{
			name:        "display name with dots",
			id:          "id-123",
			displayName: "service.btp.sap",
			wantName:    "service.btp.sap.id-123",
			wantErr:     false,
		},
		{
			name:        "long display name (len > 63)",
			id:          "id-123",
			displayName: "this-is-a-long-display-name-that-should-be-truncated-to-fit-kubernetes-naming-constraints",
			wantName:    "this-is-a-long-display-name-that-should-be-truncated-to-fit-kub.id-123",
			wantErr:     false,
		},
		{
			name:        "display name starting with number",
			id:          "id-123",
			displayName: "123-resource",
			wantName:    "x123-resource.id-123",
			wantErr:     false,
		},
		{
			name:        "id starting with number",
			id:          "123-id",
			displayName: "resource-123",
			wantName:    "resource-123.x123-id",
			wantErr:     false,
		},
		{
			name:        "display name with multiple consecutive special chars",
			id:          "id-123",
			displayName: "resource___---test",
			wantName:    "resource------test.id-123",
			wantErr:     false,
		},
		{
			name:        "empty strings for all parameters",
			id:          "",
			displayName: "",
			wantName:    UndefinedName,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, err := GenerateK8sResourceName(tt.id, tt.displayName)

			if tt.wantErr {
				r.Error(err, "expected an error")
			} else {
				r.NoError(err, "unexpected error")
			}

			r.Equal(tt.wantName, gotName, "resource name mismatch")
		})
	}
}
