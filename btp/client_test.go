package btp

import (
	"strings"
	"testing"

	"github.com/sap/crossplane-provider-btp/internal"
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
