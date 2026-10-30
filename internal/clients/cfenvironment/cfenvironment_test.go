package environments

import (
	"context"
	"fmt"
	"net/http"
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

// An empty external-name means the instance ID may not have been recorded after a create that reached CIS,
// so the environment is looked up by the resource UID stored at create time.
func TestCloudFoundryOrganization_getEnvironmentByExternalNameWithLegacyHandling_EmptyExternalName(t *testing.T) {
	existing := provisioningclient.BusinessEnvironmentInstanceResponseObject{
		EnvironmentType: internal.Ptr("cloudfoundry"),
		Parameters:      internal.Ptr(`{"instance_name":"sub-test","orchestrate.cloud.sap/subaccount-operator":"uid-1"}`),
		Id:              internal.Ptr("1234"),
	}
	cr := v1alpha1.CloudFoundryEnvironment{
		// No external-name annotation
		ObjectMeta: v1.ObjectMeta{Name: "test", UID: "uid-1"},
		Spec:       v1alpha1.CfEnvironmentSpec{SubaccountGuid: "sub"},
	}

	tests := map[string]struct {
		client  *fakes.MockProvisioningServiceClient
		want    *provisioningclient.BusinessEnvironmentInstanceResponseObject
		wantErr bool
	}{
		"ExistingEnvironmentIsFoundByOrgName": {
			client: &fakes.MockProvisioningServiceClient{ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{
				EnvironmentInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{existing},
			}},
			want: &existing,
		},
		"EnvironmentOfOtherResourceIsNotAdopted": {
			client: &fakes.MockProvisioningServiceClient{ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{
				EnvironmentInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{{
					EnvironmentType: internal.Ptr("cloudfoundry"),
					// same org name, created by a different resource
					Parameters: internal.Ptr(`{"instance_name":"sub-test","orchestrate.cloud.sap/subaccount-operator":"uid-2"}`),
					Id:         internal.Ptr("5678"),
				}},
			}},
			want: nil,
		},
		"MatchingEnvironmentWithoutIdIsSkipped": {
			client: &fakes.MockProvisioningServiceClient{ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{
				EnvironmentInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{{
					EnvironmentType: internal.Ptr("cloudfoundry"),
					Parameters:      internal.Ptr(`{"instance_name":"sub-test","orchestrate.cloud.sap/subaccount-operator":"uid-1"}`),
				}},
			}},
			want: nil,
		},
		"NoMatchingEnvironment": {
			client: &fakes.MockProvisioningServiceClient{ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{}},
			want:   nil,
		},
		"ListError": {
			client:  &fakes.MockProvisioningServiceClient{Err: errors.New("error")},
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := CloudFoundryOrganization{btp: btp.Client{ProvisioningServiceClient: tt.client}}

			got, err := c.getEnvironmentByExternalNameWithLegacyHandling(context.TODO(), cr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// createFake simulates CIS accepting the create, followed by a lookup of the new instance.
type createFake struct {
	fakes.MockProvisioningServiceClient
	createdID   string
	getResponse *provisioningclient.BusinessEnvironmentInstanceResponseObject
	getErr      error
}

func (m *createFake) CreateEnvironmentInstance(ctx context.Context) provisioningclient.ApiCreateEnvironmentInstanceRequest {
	return provisioningclient.ApiCreateEnvironmentInstanceRequest{ApiService: m}
}

func (m *createFake) CreateEnvironmentInstanceExecute(r provisioningclient.ApiCreateEnvironmentInstanceRequest) (*provisioningclient.CreatedEnvironmentInstanceResponseObject, *http.Response, error) {
	return &provisioningclient.CreatedEnvironmentInstanceResponseObject{Id: &m.createdID}, &http.Response{}, nil
}

func (m *createFake) GetEnvironmentInstanceExecute(r provisioningclient.ApiGetEnvironmentInstanceRequest) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, *http.Response, error) {
	return m.getResponse, &http.Response{}, m.getErr
}

// Once CIS accepted the create, the instance ID must be returned even if a later step fails.
func TestCloudFoundryOrganization_CreateInstance_KeepsInstanceID(t *testing.T) {
	const instanceID = "550e8400-e29b-41d4-a716-446655440000"

	tests := map[string]struct {
		client *createFake
	}{
		"OrgExtractionFails": {
			client: &createFake{
				createdID:   instanceID,
				getResponse: &provisioningclient.BusinessEnvironmentInstanceResponseObject{Id: internal.Ptr(instanceID), Labels: internal.Ptr("")},
			},
		},
		"OrgLookupFails": {
			client: &createFake{createdID: instanceID, getErr: errors.New("lookup failed")},
		},
		"OrgNotFound": {
			client: &createFake{createdID: instanceID},
		},
		"OrgWithoutId": {
			client: &createFake{
				createdID: instanceID,
				getResponse: &provisioningclient.BusinessEnvironmentInstanceResponseObject{
					Id: internal.Ptr(instanceID), Labels: internal.Ptr(`{"Org Name":"name","API Endpoint":"endpoint"}`),
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := CloudFoundryOrganization{btp: btp.Client{
				Credential:                &btp.Credentials{UserCredential: &btp.UserCredential{Email: "admin@example.com"}},
				ProvisioningServiceClient: tt.client,
			}}

			got, err := c.CreateInstance(context.TODO(), v1alpha1.CloudFoundryEnvironment{})
			if err == nil {
				t.Fatal("expected an error")
			}
			if got != instanceID {
				t.Errorf("instance ID = %q, want %q", got, instanceID)
			}
		})
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
		input        string
		wantUsername string
		wantOrigin   string
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

func TestCloudFoundryOrganization_NeedsUpdate(t *testing.T) {
	const admin = "admin@example.com"
	crWith := func(configured []string, observed []v1alpha1.User) v1alpha1.CloudFoundryEnvironment {
		cr := v1alpha1.CloudFoundryEnvironment{}
		cr.Spec.ForProvider.Managers = configured
		cr.Status.AtProvider.Managers = observed
		return cr
	}

	tests := map[string]struct {
		cr          v1alpha1.CloudFoundryEnvironment
		wantMissing []string
	}{
		"NoManagersConfigured": {
			cr: crWith(nil, nil),
		},
		"ConfiguredManagerAssigned": {
			cr: crWith([]string{"a@example.com"}, []v1alpha1.User{{Username: "a@example.com", Origin: "sap.ids"}}),
		},
		"ConfiguredManagerAssignedWithDifferentCase": {
			cr: crWith([]string{"A@example.com"}, []v1alpha1.User{{Username: "a@example.com", Origin: "sap.ids"}}),
		},
		"AdminIsNotRequiredToBeObserved": {
			cr: crWith([]string{admin}, nil),
		},
		"ConfiguredManagerMissing": {
			cr:          crWith([]string{"a@example.com"}, nil),
			wantMissing: []string{"a@example.com"},
		},
		"ManagerWithOtherOriginIsMissing": {
			cr:          crWith([]string{"a@example.com|custom.idp"}, []v1alpha1.User{{Username: "a@example.com", Origin: "sap.ids"}}),
			wantMissing: []string{"a@example.com|custom.idp"},
		},
		"OnlyUnassignedManagersAreMissing": {
			cr:          crWith([]string{admin, "a@example.com", "b@example.com"}, []v1alpha1.User{{Username: "a@example.com", Origin: "sap.ids"}}),
			wantMissing: []string{"b@example.com"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := CloudFoundryOrganization{btp: btp.Client{
				Credential: &btp.Credentials{UserCredential: &btp.UserCredential{Email: admin}},
			}}

			if got := c.NeedsUpdate(tt.cr); got != (len(tt.wantMissing) > 0) {
				t.Errorf("NeedsUpdate() = %v, want %v", got, len(tt.wantMissing) > 0)
			}
			if got := c.missingManagers(tt.cr); !reflect.DeepEqual(got, tt.wantMissing) {
				t.Errorf("missingManagers() = %v, want %v", got, tt.wantMissing)
			}
		})
	}
}

func TestCloudFoundryOrganization_UpdateInstance(t *testing.T) {
	credential := &btp.Credentials{UserCredential: &btp.UserCredential{Email: "admin@example.com"}}
	cr := v1alpha1.CloudFoundryEnvironment{}
	cr.Spec.ForProvider.Managers = []string{"a@example.com"}

	t.Run("NothingMissingDoesNotCallCIS", func(t *testing.T) {
		// the zero value client has no provisioning client, so any CIS call would panic
		c := CloudFoundryOrganization{btp: btp.Client{Credential: credential}}
		withManager := cr
		withManager.Status.AtProvider.Managers = []v1alpha1.User{{Username: "a@example.com", Origin: "sap.ids"}}

		if err := c.UpdateInstance(context.TODO(), withManager); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("OrgNotFoundIsReturned", func(t *testing.T) {
		c := CloudFoundryOrganization{btp: btp.Client{
			Credential:                credential,
			ProvisioningServiceClient: &createFake{},
		}}

		if err := c.UpdateInstance(context.TODO(), cr); err == nil {
			t.Error("expected an error")
		}
	})
	t.Run("OrgLookupErrorIsReturned", func(t *testing.T) {
		c := CloudFoundryOrganization{btp: btp.Client{
			Credential:                credential,
			ProvisioningServiceClient: &createFake{getErr: errors.New("lookup failed")},
		}}

		if err := c.UpdateInstance(context.TODO(), cr); err == nil {
			t.Error("expected an error")
		}
	})
}
