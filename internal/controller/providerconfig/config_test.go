package providerconfig

import (
	"context"
	"maps"
	"testing"

	cp_xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	test2 "github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	trackingtest "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	"github.com/sap/crossplane-provider-btp/test/e2e"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	mg.ProviderConfigReferencer = &fake.LegacyProviderConfigReferencer{Ref: &cp_xpv1.Reference{Name: "any"}}
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

func (tr *tracker) Track(ctx context.Context, mg LegacyManaged) error { return nil }

// --- LoadDestinationCredentials ---

// fakeKubeClient returns a fake client pre-loaded with a single secret.
func fakeKubeClientWithSecret(name, namespace string, data map[string][]byte) client.Client {
	secret := &v1.Secret{}
	secret.Name = name
	secret.Namespace = namespace
	secret.Data = data
	return ctrlfake.NewClientBuilder().WithObjects(secret).Build()
}

func makeDestProviderConfig(secretName, secretKey string) *v1alpha1.ProviderConfig {
	pc := &v1alpha1.ProviderConfig{}
	ref := &cp_xpv1.SecretKeySelector{
		SecretReference: cp_xpv1.SecretReference{
			Name:      secretName,
			Namespace: "default",
		},
		Key: secretKey,
	}
	pc.Spec.DestinationServiceSecret = &v1alpha1.ProviderCredentials{
		Source: cp_xpv1.CredentialsSourceSecret,
		CommonCredentialSelectors: cp_xpv1.CommonCredentialSelectors{
			SecretRef: ref,
		},
	}
	return pc
}

func TestLoadDestinationCredentials_FlatJSON(t *testing.T) {
	// Format A: single key containing a JSON object with all four fields.
	secretData := map[string][]byte{
		"credentials": []byte(`{"clientid":"id1","clientsecret":"secret1","tokenurl":"https://token.example.com","uri":"https://api.example.com"}`),
	}
	kube := fakeKubeClientWithSecret("dest-secret", "default", secretData)
	pc := makeDestProviderConfig("dest-secret", "credentials")

	raw, err := LoadDestinationCredentials(context.Background(), kube, pc)
	assert.NoError(t, err)
	assert.Contains(t, string(raw), "clientid")
	assert.Contains(t, string(raw), "id1")
}

func TestLoadDestinationCredentials_FlatKeys(t *testing.T) {
	// Format B: service binding flat keys — no single JSON key, fields are individual keys.
	secretData := map[string][]byte{
		"clientid":     []byte("id2"),
		"clientsecret": []byte("secret2"),
		"tokenurl":     []byte("https://token.example.com"),
		"uri":          []byte("https://api.example.com"),
		"type":         []byte("destination"),      // extra metadata key from service binding
		"instance_name": []byte("dest-instance"),  // extra metadata key
	}
	kube := fakeKubeClientWithSecret("dest-binding-secret", "default", secretData)
	// Key is empty — signals flat-key format
	pc := makeDestProviderConfig("dest-binding-secret", "")

	raw, err := LoadDestinationCredentials(context.Background(), kube, pc)
	assert.NoError(t, err)
	assert.Contains(t, string(raw), "clientid")
	assert.Contains(t, string(raw), "id2")
	assert.Contains(t, string(raw), "tokenurl")
	assert.Contains(t, string(raw), "uri")
}

func TestLoadDestinationCredentials_NotConfigured(t *testing.T) {
	pc := &v1alpha1.ProviderConfig{}
	_, err := LoadDestinationCredentials(context.Background(), nil, pc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "destinationCredentials")
}

func TestLoadDestinationCredentials_MissingKey(t *testing.T) {
	secretData := map[string][]byte{
		"other-key": []byte(`{"foo":"bar"}`),
	}
	kube := fakeKubeClientWithSecret("dest-secret", "default", secretData)
	pc := makeDestProviderConfig("dest-secret", "credentials") // key doesn't exist

	_, err := LoadDestinationCredentials(context.Background(), kube, pc)
	assert.Error(t, err)
}

func TestLoadDestinationCredentials_FlatKeys_TokenUrl(t *testing.T) {
	// Format B with token_url key (Destination Service binding format).
	secretData := map[string][]byte{
		"clientid":     []byte("id3"),
		"clientsecret": []byte("secret3"),
		"token_url":    []byte("https://token.example.com"),
		"uri":          []byte("https://api.example.com"),
	}
	kube := fakeKubeClientWithSecret("dest-binding-secret", "default", secretData)
	pc := makeDestProviderConfig("dest-binding-secret", "")

	raw, err := LoadDestinationCredentials(context.Background(), kube, pc)
	assert.NoError(t, err)
	assert.Contains(t, string(raw), "tokenurl")
	assert.Contains(t, string(raw), "id3")
}

func TestAssembleDestinationCredJSON_MissingRequiredKeys(t *testing.T) {
	// Base uses token_url (Destination Service format) to verify both key variants.
	requiredKeys := []string{"clientid", "clientsecret", "token_url", "uri"}
	base := map[string][]byte{
		"clientid":     []byte("id"),
		"clientsecret": []byte("secret"),
		"token_url":    []byte("https://token.example.com"),
		"uri":          []byte("https://api.example.com"),
	}
	for _, missing := range requiredKeys {
		t.Run("missing_"+missing, func(t *testing.T) {
			data := make(map[string][]byte, len(base))
			maps.Copy(data, base)
			delete(data, missing)

			kube := fakeKubeClientWithSecret("dest-secret", "default", data)
			pc := makeDestProviderConfig("dest-secret", "") // empty key = Format B

			_, err := LoadDestinationCredentials(context.Background(), kube, pc)
			assert.Error(t, err)
			// token_url and tokenurl are normalized — error always says "tokenurl"
			expectedKey := missing
			if missing == "token_url" {
				expectedKey = "tokenurl"
			}
			assert.Contains(t, err.Error(), expectedKey)
		})
	}
}
