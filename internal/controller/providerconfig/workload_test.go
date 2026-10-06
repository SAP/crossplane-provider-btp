package providerconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
)

func TestWorkloadNativeMetadata(t *testing.T) {
	pc := &v1alpha1.ProviderConfig{Spec: v1alpha1.ProviderConfigSpec{GlobalAccount: "test-ga", WorkloadIdentity: &v1alpha1.WorkloadIdentityConfiguration{TokenFile: "/token", IdentityProvider: "test-origin", UserEmail: "workload@example.com"}}}
	pc.Spec.CISSecret.Source = "Secret"
	if err := ValidateWorkloadIdentity(pc); err != nil {
		t.Fatal(err)
	}
	data, err := loadSaCredentials(context.Background(), nil, pc)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	json.Unmarshal(data, &fields)
	for _, key := range []string{"TokenFile", "IASURL", "IASClientID", "IASResource"} {
		if _, exists := fields[key]; exists {
			t.Fatal("workload config serialized as user credentials")
		}
	}
	var user btp.UserCredential
	json.Unmarshal(data, &user)
	if user.Email != "workload@example.com" || user.Idp != "test-origin" || user.Password != "" {
		t.Fatal("incorrect native identity metadata")
	}
}

func TestExplicitWorkloadClientBypassesLegacyFactory(t *testing.T) {
	for _, grant := range []string{"client_credentials", "user_token"} {
		pc := &v1alpha1.ProviderConfig{Spec: v1alpha1.ProviderConfigSpec{GlobalAccount: "ga", WorkloadIdentity: &v1alpha1.WorkloadIdentityConfiguration{TokenFile: "/no-file-read-at-construction", UserEmail: "workload@example.com", IdentityProvider: "origin", IASURL: "https://ias.example", IASClientID: "consumer", IASResource: "dependency"}}}
		cis := []byte(`{"grant_type":"` + grant + `","uaa":{"clientid":"client","clientsecret":"secret","url":"https://uaa.example"},"endpoints":{"accounts_service_url":"https://api.example","entitlements_service_url":"https://api.example","provisioning_service_url":"https://api.example"}}`)
		legacy := func([]byte, []byte) (*btp.Client, error) {
			t.Fatal("workload configuration passed through legacy Secret factory")
			return nil, nil
		}
		got, err := NewConfiguredClient(pc, cis, nil, legacy)
		if err != nil {
			t.Fatal(err)
		}
		if got.Credential.WorkloadIdentity == nil || got.Credential.WorkloadIdentity.TokenFile != pc.Spec.WorkloadIdentity.TokenFile {
			t.Fatal("explicit workload metadata missing")
		}
	}
}

func TestLegacyClientRejectsWorkloadKeysBeforeFactory(t *testing.T) {
	pc := &v1alpha1.ProviderConfig{}
	legacy := func([]byte, []byte) (*btp.Client, error) {
		t.Fatal("invalid Secret reached client construction")
		return nil, nil
	}
	if _, err := NewConfiguredClient(pc, nil, []byte(`{"tokenFile":"/should-not-read","iasUrl":"https://should-not-call.example"}`), legacy); err == nil {
		t.Fatal("accepted workload keys")
	}
	called := false
	legacy = func(cis, user []byte) (*btp.Client, error) { called = true; return &btp.Client{}, nil }
	if _, err := NewConfiguredClient(pc, nil, []byte(`{"email":"user@example.com","password":"secret"}`), legacy); err != nil || !called {
		t.Fatalf("legacy path changed: %v", err)
	}
}
