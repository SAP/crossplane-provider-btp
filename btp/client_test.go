package btp

import (
	"strings"
	"testing"

	"github.com/sap/crossplane-provider-btp/internal"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func TestNewBTPClient(t *testing.T) {

	tests := []struct {
		name                     string
		cisSecretData            []byte
		serviceAccountSecretData []byte
		wantErr                  *string
	}{
		{
			name:                     "sucessfully create new btp client",
			cisSecretData:            []byte("{\"endpoints\": {\"accounts_service_url\": \"xxx\", \"cloud_automation_url\": \"xxx\", \"entitlements_service_url\": \"xxx\",      \"events_service_url\": \"xxx\",      \"external_provider_registry_url\": \"xxx\",      \"metadata_service_url\": \"xxx\",      \"order_processing_url\": \"xxx\",      \"provisioning_service_url\": \"xxx\",      \"saas_registry_service_url\": \"xxx\"    },    \"grant_type\": \"client_credentials\",    \"sap.cloud.service\": \"xxx\",    \"uaa\": {      \"apiurl\": \"xxx\",      \"clientid\": \"xxx\",      \"clientsecret\": \"xxx\",      \"credential-type\": \"binding-secret\",      \"identityzone\": \"xxx\",      \"identityzoneid\": \"xxx\",      \"sburl\": \"xxx\",      \"subaccountid\": \"xxx\",      \"tenantid\": \"xxx\",      \"tenantmode\": \"shared\",      \"uaadomain\": \"xxx\",      \"url\": \"xxx\",      \"verificationkey\": \"xxx\", \"xsappname\": \"xxx\", \"xsmasterappname\": \"xxx\", \"zoneid\": \"xxx\"}}"),
			serviceAccountSecretData: []byte("{\"email\": \"1@sap.com\",\"username\": \"xxx\",\"password\": \"xxx\"}"),
			wantErr:                  nil,
		},
		{
			name:                     "fail on invalid json",
			cisSecretData:            []byte("{\"endpoints\": {\"accounts_service_url\": \"xxx\", \"cloud_automation_url\": \"xxx\", \"entitlements_service_url\": \"xxx\",      \"events_service_url\": \"xxx\",      \"external_provider_registry_url\": \"xxx\",      \"metadata_service_url\": \"xxx\",      \"order_processing_url\": \"xxx\",      \"provisioning_service_url\": \"xxx\",      \"saas_registry_service_url\": \"xxx\"    },    \"grant_type\": \"client_credentials\",    \"sap.cloud.service\": \"xxx\",    \"uaa\": {      \"apiurl\": \"xxx\",      \"clientid\": \"xxx\",      \"clientsecret\": \"xxx\",      \"credential-type\": \"binding-secret\",      \"identityzone\": \"xxx\",      \"identityzoneid\": \"xxx\",      \"sburl\": \"xxx\",      \"subaccountid\": \"xxx\",      \"tenantid\": \"xxx\",      \"tenantmode\": \"shared\",      \"uaadomain\": \"xxx\",      \"url\": \"xxx\",      \"verificationkey\": \"xxx\", \"xsappname\": \"xxx\", \"xsmasterappname\": \"xxx\", \"zoneid\": \"xxx\"}}"),
			serviceAccountSecretData: []byte("{\"email\": \"1@sap.com\",\"username\": \"xxx\",\"password\": \"xx\"x\"}"),
			wantErr:                  internal.Ptr(errCouldNotParseUserCredential),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				_, err := NewBTPClient(tt.cisSecretData, tt.serviceAccountSecretData)
				if err != nil && tt.wantErr == nil {
					t.Errorf("unexpected error output: %s", err)
				}
				if err != nil && !strings.Contains(err.Error(), internal.Val(tt.wantErr)) {
					t.Errorf("error does not contain wanted error message: %s", err)
				}
			},
		)
	}
}

func TestFindCFEnvironmentByNameAndOrg(t *testing.T) {
	tests := []struct {
		name         string
		envInstances []provisioningclient.BusinessEnvironmentInstanceResponseObject
		instanceName string
		orgName      string
		wantID       *string
		wantErr      bool
	}{
		{
			name: "Match By ID happy path",
			envInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				{
					Id:              internal.Ptr("env-id-123"),
					EnvironmentType: internal.Ptr("cloudfoundry"),
				},
			},
			instanceName: "env-id-123",
			orgName:      "test-org",
			wantID:       internal.Ptr("env-id-123"),
			wantErr:      false,
		},
		{
			name: "Match by instance name in Parameters",
			envInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				{
					Id:              internal.Ptr("env-456"),
					EnvironmentType: internal.Ptr("cloudfoundry"),
					Parameters:      internal.Ptr(`{"instance_name": "my-cf-env"}`),
				},
			},
			instanceName: "my-cf-env",
			orgName:      "test-org",
			wantID:       internal.Ptr("env-456"),
			wantErr:      false,
		},
		{
			name: "Not found - no match",
			envInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				{
					Id:              internal.Ptr("other-env"),
					EnvironmentType: internal.Ptr("cloudfoundry"),
					Parameters:      internal.Ptr(`{"instance_name": "different-name"}`),
				},
			},
			instanceName: "nonexistent",
			orgName:      "",
			wantID:       nil,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := findCFEnvironment(tt.envInstances, tt.instanceName, tt.orgName)
			if (err != nil) != tt.wantErr {
				t.Errorf("findCFEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Check result
			if tt.wantID == nil {
				if result != nil {
					t.Errorf("findCFEnvironment() expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("findCFEnvironment() expected result with ID %s, got nil", *tt.wantID)
				} else if result.Id == nil || *result.Id != *tt.wantID {
					t.Errorf("findCFEnvironment() got = %v, want %s", result.Id, *tt.wantID)
				}
			}
		})
	}
}
