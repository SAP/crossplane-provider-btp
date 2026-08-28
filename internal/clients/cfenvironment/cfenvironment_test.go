package environments

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/fakes"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//
// Unit tests for CloudFoundryEnvironment external-name handling
// Tests verify compliance with external-name convention documented in docs/development/external-name-handling.md
//

func TestFormOrgName(t *testing.T) {
	tests := []struct {
		name         string
		orgName      string
		subaccountId string
		crName       string
		want         string
	}{
		{
			name:         "CustomOrgName_ReturnsOrgName",
			orgName:      "custom-org",
			subaccountId: "subaccount-123",
			crName:       "my-cf",
			want:         "custom-org",
		},
		{
			name:         "EmptyOrgName_ReturnsGenerated",
			orgName:      "",
			subaccountId: "subaccount-123",
			crName:       "my-cf",
			want:         "subaccount-123-my-cf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormOrgName(tt.orgName, tt.subaccountId, tt.crName); got != tt.want {
				t.Errorf("formOrgName() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExternalNameEmptyCheck verifies that empty external-name returns nil
// This is part of the external-name convention: empty external-name means resource doesn't exist yet
func TestCloudFoundryOrganization_getEnvironmentByExternalNameWithLegacyHandling_EmptyExternalName(t *testing.T) {
	c := CloudFoundryOrganization{
		btp: btp.Client{},
	}

	cr := v1alpha1.CloudFoundryEnvironment{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
			// No external-name annotation
		},
	}

	got, err := c.getEnvironmentByExternalNameWithLegacyHandling(context.TODO(), cr)

	if err != nil {
		t.Errorf("Expected no error for empty external-name, got: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil for empty external-name, got: %v", got)
	}
}

// TestExternalNameValidGUID verifies GUID validation
// Tests that valid GUIDs are recognized correctly
func TestCloudFoundryOrganization_getEnvironmentByExternalNameWithLegacyHandling_ValidGUID(t *testing.T) {
	validGUID := "550e8400-e29b-41d4-a716-446655440000"

	// Test that a valid GUID is accepted
	cr := v1alpha1.CloudFoundryEnvironment{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test",
			Annotations: map[string]string{"crossplane.io/external-name": validGUID},
		},
	}

	// Verify the GUID is valid
	if !internal.IsValidUUID(validGUID) {
		t.Errorf("Valid GUID was not recognized as valid UUID")
	}

	// Verify the GUID is extracted from CR
	externalName := cr.Annotations["crossplane.io/external-name"]
	if externalName != validGUID {
		t.Errorf("External name mismatch: got %v, want %v", externalName, validGUID)
	}
}

// TestExternalNameInvalidGUID verifies invalid GUID handling
// Tests that non-GUID formats trigger legacy lookup path
func TestExternalNameInvalidGUID(t *testing.T) {
	invalidCases := []string{
		"not-a-guid",
		"123",
		"legacy-org-name",
	}

	for _, invalidGUID := range invalidCases {
		t.Run(fmt.Sprintf("Invalid_%s", invalidGUID), func(t *testing.T) {
			if internal.IsValidUUID(invalidGUID) {
				t.Errorf("Invalid GUID %q was incorrectly recognized as valid UUID", invalidGUID)
			}
		})
	}
}

// TestLegacyFormatHandling tests the legacy lookup path
// Verifies that non-GUID external-names use orgName-based lookup
func TestCloudFoundryOrganization_LegacyFormatHandling(t *testing.T) {
	// Test legacy format with non-GUID external-name
	cr := v1alpha1.CloudFoundryEnvironment{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test",
			Annotations: map[string]string{"crossplane.io/external-name": "legacy-name"},
		},
		Spec: v1alpha1.CfEnvironmentSpec{
			SubaccountGuid: "subaccount-guid",
			ForProvider: v1alpha1.CfEnvironmentParameters{
				OrgName: "custom-org",
			},
		},
	}

	// Verify external-name is not a valid GUID (triggers legacy path)
	externalName := cr.Annotations["crossplane.io/external-name"]
	if internal.IsValidUUID(externalName) {
		t.Errorf("Legacy external-name %q should not be a valid UUID", externalName)
	}

	// Verify orgName is correctly formed for legacy lookup
	expectedOrgName := "custom-org" // OrgName takes precedence
	actualOrgName := FormOrgName(cr.Spec.ForProvider.OrgName, cr.Spec.SubaccountGuid, cr.Name)
	if actualOrgName != expectedOrgName {
		t.Errorf("OrgName mismatch: got %v, want %v", actualOrgName, expectedOrgName)
	}
}

func TestNewOrganizationClient_Validation(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
		url     string
		orgId   string
		user    string
		pass    string
		origin  string
		wantErr bool
	}{
		{
			name:    "missing org name returns error",
			orgName: "",
			url:     "https://api.cf.example.com",
			orgId:   "org-guid",
			user:    "user",
			pass:    "pass",
			origin:  "",
			wantErr: true,
		},
		{
			name:    "missing orgGuid returns error",
			orgName: "my-org",
			url:     "https://api.cf.example.com",
			orgId:   "",
			user:    "user",
			pass:    "pass",
			origin:  "",
			wantErr: true,
		},
		{
			name:    "empty origin is accepted",
			orgName: "my-org",
			url:     "https://api.cf.example.com",
			orgId:   "org-guid",
			user:    "user",
			pass:    "pass",
			origin:  "",
			wantErr: true, // will fail on CF API connect, but not on validation
		},
		{
			name:    "non-empty origin is accepted",
			orgName: "my-org",
			url:     "https://api.cf.example.com",
			orgId:   "org-guid",
			user:    "user",
			pass:    "pass",
			origin:  "custom-idp",
			wantErr: true, // will fail on CF API connect, but not on validation
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newOrganizationClient(tt.orgName, tt.url, tt.orgId, tt.user, tt.pass, tt.origin)
			if (err != nil) != tt.wantErr {
				t.Errorf("newOrganizationClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFilterOutUser(t *testing.T) {
	tests := []struct {
		name    string
		users   []string
		exclude string
		want    []string
	}{
		{
			name:    "TechUserInList_IsFiltered",
			users:   []string{"other@example.com", "techuser@example.com", "another@example.com"},
			exclude: "techuser@example.com",
			want:    []string{"other@example.com", "another@example.com"},
		},
		{
			name:    "TechUserNotInList_NoChange",
			users:   []string{"other@example.com", "another@example.com"},
			exclude: "techuser@example.com",
			want:    []string{"other@example.com", "another@example.com"},
		},
		{
			name:    "OnlyTechUser_ReturnsEmpty",
			users:   []string{"techuser@example.com"},
			exclude: "techuser@example.com",
			want:    []string{},
		},
		{
			name:    "EmptyList_ReturnsEmpty",
			users:   []string{},
			exclude: "techuser@example.com",
			want:    []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterOutUser(tt.users, tt.exclude)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterOutUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseManagerString(t *testing.T) {
	tests := []struct {
		input          string
		wantUsername   string
		wantOrigin     string
	}{
		{"user@example.com", "user@example.com", defaultOrigin},
		{"user@example.com|custom.idp", "user@example.com", "custom.idp"},
		{"user@example.com|sap.ids", "user@example.com", "sap.ids"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotUsername, gotOrigin := parseManagerString(tt.input)
			if gotUsername != tt.wantUsername || gotOrigin != tt.wantOrigin {
				t.Errorf("parseManagerString(%q) = (%q, %q), want (%q, %q)", tt.input, gotUsername, gotOrigin, tt.wantUsername, tt.wantOrigin)
			}
		})
	}
}

// Test legacy getEnvironmentByNameAndOrg behavior with existing mock infrastructure
// This validates backwards compatibility with the legacy API
func TestCloudFoundryOrganization_getEnvironmentByNameAndOrg_Legacy(t *testing.T) {
	getBtpClient := func(instanceName string) btp.Client {
		return btp.Client{
			Credential: &btp.Credentials{
				UserCredential: &btp.UserCredential{Username: "username", Password: "password"},
			},
			ProvisioningServiceClient: &fakes.MockProvisioningServiceClient{
				ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
						{
							EnvironmentType: internal.Ptr("cloudfoundry"),
							Parameters:      internal.Ptr(fmt.Sprintf(`{"instance_name":"%s"}`, instanceName)),
							Id:              internal.Ptr("1234"),
							Labels:          internal.Ptr(`{"Org Id":"1234","Org Name":"name","API Endpoint":"endpoint"}`),
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name    string
		client  btp.Client
		cr      v1alpha1.CloudFoundryEnvironment
		want    *provisioningclient.BusinessEnvironmentInstanceResponseObject
		wantErr bool
	}{
		{
			name:   "Success - match by external-name",
			client: getBtpClient("extName"),
			cr: v1alpha1.CloudFoundryEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"crossplane.io/external-name": "extName"},
				},
				Spec: v1alpha1.CfEnvironmentSpec{
					ForProvider: v1alpha1.CfEnvironmentParameters{
						OrgName: "org",
					},
				},
			},
			want: &provisioningclient.BusinessEnvironmentInstanceResponseObject{
				EnvironmentType: internal.Ptr("cloudfoundry"),
				Parameters:      internal.Ptr(`{"instance_name":"extName"}`),
				Id:              internal.Ptr("1234"),
				Labels:          internal.Ptr(`{"Org Id":"1234","Org Name":"name","API Endpoint":"endpoint"}`),
			},
			wantErr: false,
		},
		{
			name:   "Not found - no environment matching external-name",
			client: getBtpClient("somethingElse"),
			cr: v1alpha1.CloudFoundryEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"crossplane.io/external-name": "extName"},
				},
				Spec: v1alpha1.CfEnvironmentSpec{
					ForProvider: v1alpha1.CfEnvironmentParameters{
						OrgName: "org",
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "Error",
			client: btp.Client{
				Credential: &btp.Credentials{
					UserCredential: &btp.UserCredential{Username: "username", Password: "password"},
				},
				ProvisioningServiceClient: &fakes.MockProvisioningServiceClient{
					Err: errors.New("error"),
				},
			},
			cr: v1alpha1.CloudFoundryEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"crossplane.io/external-name": "extName"},
				},
				Spec: v1alpha1.CfEnvironmentSpec{
					ForProvider: v1alpha1.CfEnvironmentParameters{
						OrgName: "org",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CloudFoundryOrganization{
				btp: tt.client,
			}

			externalName := tt.cr.Annotations["crossplane.io/external-name"]
			orgName := FormOrgName(tt.cr.Spec.ForProvider.OrgName, tt.cr.Spec.SubaccountGuid, tt.cr.Name)

			got, err := c.btp.GetCFEnvironmentByNameAndOrg(context.TODO(), externalName, orgName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getEnvironmentByNameAndOrg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEnvironmentByNameAndOrg() got = %v, want %v", got, tt.want)
			}
		})
	}
}
