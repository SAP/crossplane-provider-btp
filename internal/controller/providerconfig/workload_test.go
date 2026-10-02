package providerconfig

import (
	"context"
	"encoding/json"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	trackingtest "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	var user btp.UserCredential
	json.Unmarshal(data, &user)
	if user.Email != "workload@example.com" || user.Idp != "test-origin" || user.Password != "" {
		t.Fatal("incorrect native identity metadata")
	}
}

func TestWorkloadUsesManualCISSecret(t *testing.T) {
	for _, grant := range []string{"client_credentials", "user_token"} {
		t.Run(grant, func(t *testing.T) {
			kube := &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				switch out := obj.(type) {
				case *v1alpha1.ProviderConfig:
					out.Spec = v1alpha1.ProviderConfigSpec{GlobalAccount: "test-ga", WorkloadIdentity: &v1alpha1.WorkloadIdentityConfiguration{TokenFile: "/token", IdentityProvider: "test-origin", UserEmail: "workload@example.com"}}
					out.Spec.CISSecret = v1alpha1.ProviderCredentials{Source: "Secret", CommonCredentialSelectors: xpv1.CommonCredentialSelectors{SecretRef: &xpv1.SecretKeySelector{SecretReference: xpv1.SecretReference{Name: "manual-cis", Namespace: "test"}, Key: "credentials"}}}
				case *corev1.Secret:
					if key.Name != "manual-cis" {
						t.Fatal("read user credentials")
					}
					out.Data = map[string][]byte{"credentials": []byte(`{"grant_type":"` + grant + `"}`)}
				}
				return nil
			}, MockList: test.NewMockListFn(nil)}
			called := false
			svc := func(cis, user []byte) (*btp.Client, error) {
				called = true
				var metadata btp.UserCredential
				json.Unmarshal(user, &metadata)
				if metadata.Email != "workload@example.com" || metadata.Password != "" {
					t.Fatal("incorrect native metadata")
				}
				var binding btp.CISCredential
				json.Unmarshal(cis, &binding)
				if binding.GrantType != "client_credentials" {
					t.Fatal("password grant reached native client")
				}
				return &btp.Client{}, nil
			}
			_, err := CreateClient(context.Background(), fakeResource(), kube, &tracker{}, svc, trackingtest.NoOpReferenceResolverTracker{})
			if grant == "client_credentials" && (err != nil || !called) {
				t.Fatalf("manual CIS coexistence failed: %v", err)
			}
			if grant == "user_token" && (err == nil || called) {
				t.Fatal("accepted native password grant")
			}
		})
	}
}
