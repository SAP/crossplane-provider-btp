package providerconfig

import (
	"context"
	"testing"

	cp_xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	test2 "github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	trackingtest "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	"github.com/sap/crossplane-provider-btp/test/e2e"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	btpOpSecret = map[string][]byte{
		".metadata":              []byte("{\n  \"credentialProperties\": [\n    {\n      \"name\": \"endpoints\",\n      \"format\": \"json\"\n    },\n    {\n      \"name\": \"grant_type\",\n      \"format\": \"text\"\n    },\n    {\n      \"name\": \"sap.cloud.service\",\n      \"format\": \"text\"\n    },\n    {\n      \"name\": \"uaa\",\n      \"format\": \"json\"\n    }\n  ],\n  \"metaDataProperties\": [\n    {\n      \"name\": \"instance_name\",\n      \"format\": \"text\"\n    },\n    {\n      \"name\": \"instance_guid\",\n      \"format\": \"text\"\n    },\n    {\n      \"name\": \"plan\",\n      \"format\": \"text\"\n    },\n    {\n      \"name\": \"label\",\n      \"format\":\n      \"text\"\n    },\n    {\n      \"name\": \"type\",\n      \"format\": \"text\"\n    }\n  ]\n}"),
		"endpoints":              []byte("{\"accounts_service_url\":\"xxx\",\"cloud_automation_url\":\"xxx\",\"entitlements_service_url\":\"xxx\",\"events_service_url\":\"xxx\",\"external_provider_registry_url\":\"xxx\",\"metadata_service_url\":\"xxx\",\"order_processing_url\":\"xxx\",\"provisioning_service_url\":\"xxx\",\"saas_registry_service_url\":\"xxx\"}"),
		"grant_type":             []byte("client_credentials"),
		"instance_external_name": []byte("cis-tests"),
		"instance_guid":          []byte("xxx"),
		"instance_name":          []byte("cis-tests"),
		"label":                  []byte("cis"),
		"plan":                   []byte("central"),
		"sap.cloud.service":      []byte("xxx"),
		"type":                   []byte("cis"),
		"uaa":                    []byte("{\"apiurl\":\"xxx\",\"clientid\":\"xxx\",\"clientsecret\":\"xxx\",\"credential-type\":\"binding-secret\",\"identityzone\":\"xxx\",\"identityzone id\":\"xxx\",\"sburl\":\"xxx\",\"subaccountid\":\"xxx\",\"tenantid\":\"xxx\",\"tenantmode\":\"shared\",\"uaadomain\":\"xxx\",\"url\":\"xxx\",\"verificationkey\":\"xxx\",\"xsappname\":\"xxx\",\"xsmasterappname\":\"xxx\",\"zoneid\":\"xxx\"}"),
	}
	btpCustomSecret = map[string][]byte{
		"data": []byte("{\"endpoints\": {\"accounts_service_url\": \"xxx\", \"cloud_automation_url\": \"xxx\", \"entitlements_service_url\": \"xxx\",      \"events_service_url\": \"xxx\",      \"external_provider_registry_url\": \"xxx\",      \"metadata_service_url\": \"xxx\",      \"order_processing_url\": \"xxx\",      \"provisioning_service_url\": \"xxx\",      \"saas_registry_service_url\": \"xxx\"    },    \"grant_type\": \"client_credentials\",    \"sap.cloud.service\": \"xxx\",    \"uaa\": {      \"apiurl\": \"xxx\",      \"clientid\": \"xxx\",      \"clientsecret\": \"xxx\",      \"credential-type\": \"binding-secret\",      \"identityzone\": \"xxx\",      \"identityzoneid\": \"xxx\",      \"sburl\": \"xxx\",      \"subaccountid\": \"xxx\",      \"tenantid\": \"xxx\",      \"tenantmode\": \"shared\",      \"uaadomain\": \"xxx\",      \"url\": \"xxx\",      \"verificationkey\": \"xxx\", \"xsappname\": \"xxx\", \"xsmasterappname\": \"xxx\", \"zoneid\": \"xxx\"}}"),
	}
	smSecret = map[string][]byte{
		"credentials": []byte("{\"email\": \"1@sap.com\",\"username\": \"xxx\",\"password\": \"xxx\"}"),
	}
)

const (
	secretNameSM  = "sa-secret"
	secretNameCIS = "cis-secret"
)

// This test ensures that the different secret source data is unified as expected and that a client can be initialized from it
func TestCreateClient(t *testing.T) {
	tests := []struct {
		name string
		// fake data injected from kube secret lookup
		cisSecretData map[string][]byte
		smSecretData  map[string][]byte
	}{
		{
			name:          "TestBtpOperatorFormat",
			cisSecretData: btpOpSecret,
			smSecretData:  smSecret,
		},
		{
			name:          "TestCustomFormat",
			cisSecretData: btpCustomSecret,
			smSecretData:  smSecret,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kube := mockClient(btpOpSecret)
			c, err := CreateClient(context.Background(), fakeResource(), kube, &tracker{}, btp.NewBTPClient, trackingtest.NoOpReferenceResolverTracker{})
			assert.Nil(t, err)
			assert.NotEqual(t, c, btp.Client{})
		})
	}
}

func fakeResource() *e2e.FakeManaged {
	var mg = e2e.FakeManaged{}
	mg.ProviderConfigReferencer = &fake.ProviderConfigReferencer{Ref: &cp_xpv1.Reference{Name: "any"}}
	return &mg
}
func mockClient(secretData map[string][]byte) *test2.MockClient {
	mockClient := test2.MockClient{
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			switch v := obj.(type) {
			case *v1alpha1.ProviderConfig:
				fakeProviderConfig(fakeProviderConfig(v))
			case *v1.Secret:
				switch key.Name {
				case secretNameCIS:
					v.Data = secretData
				case secretNameSM:
					v.Data = smSecret
				}
			}
			return nil
		},
	}
	return &mockClient
}

func fakeProviderConfig(pc *v1alpha1.ProviderConfig) *v1alpha1.ProviderConfig {
	pc.Spec = v1alpha1.ProviderConfigSpec{
		CISSecret: v1alpha1.ProviderCredentials{
			Source: "Secret",
			CommonCredentialSelectors: cp_xpv1.CommonCredentialSelectors{
				SecretRef: &cp_xpv1.SecretKeySelector{
					SecretReference: cp_xpv1.SecretReference{
						Name:      secretNameCIS,
						Namespace: "Namespace",
					},
					Key: "data",
				},
			},
		},
		ServiceAccountSecret: v1alpha1.ProviderCredentials{
			Source: "Secret",
			CommonCredentialSelectors: cp_xpv1.CommonCredentialSelectors{
				SecretRef: &cp_xpv1.SecretKeySelector{
					SecretReference: cp_xpv1.SecretReference{
						Name:      secretNameSM,
						Namespace: "Namespace",
					},
					Key: "credentials",
				},
			},
		},
	}
	pc.Status = v1alpha1.ProviderConfigStatus{}
	return pc
}

type tracker struct{}

func (tr *tracker) Track(ctx context.Context, mg resource.Managed) error { return nil }
