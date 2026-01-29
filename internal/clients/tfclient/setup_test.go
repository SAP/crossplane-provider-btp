package tfclient

import (
	"context"
	"encoding/json"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testSecretName    = "test-secret"
	testSecretNS      = "test-namespace"
	testProviderName  = "test-provider"
	testUsername      = "test-user@example.com"
	testPassword      = "test-password"
	testGlobalAccount = "test-global-account"
	testCliServerURL  = "https://cli.example.com"
	testCustomIDP     = "custom-idp.example.com"
)

func TestTerraformSetupBuilder_ConditionalIDP(t *testing.T) {
	type args struct {
		origin string
	}
	type want struct {
		idpSet bool
		idp    string
		err    error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "WithoutCustomIDP",
			args: args{
				origin: "",
			},
			want: want{
				idpSet: false,
				err:    nil,
			},
		},
		{
			name: "WithCustomIDP",
			args: args{
				origin: testCustomIDP,
			},
			want: want{
				idpSet: true,
				idp:    testCustomIDP,
				err:    nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create test credentials
			userCred := btp.UserCredential{
				Username: testUsername,
				Password: testPassword,
				Idp:      tc.args.origin,
			}
			credJSON, err := json.Marshal(userCred)
			if err != nil {
				t.Fatalf("failed to marshal credentials: %v", err)
			}

			// Setup mock Kubernetes client
			kube := &test.MockClient{
				MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					switch v := obj.(type) {
					case *v1alpha1.ProviderConfig:
						pc := fakeProviderConfig(testProviderName, testSecretName, testSecretNS, testGlobalAccount, testCliServerURL)
						*v = *pc
					case *corev1.Secret:
						v.Data = map[string][]byte{
							"credentials": credJSON,
						}
					}
					return nil
				},
			}

			// Create fake managed resource
			mg := &fake.Managed{}
			mg.SetProviderConfigReference(&xpv1.Reference{Name: testProviderName})

			// Call TerraformSetupBuilder
			setupFn := TerraformSetupBuilder("1.5.0", "SAP/btp", "1.7.0")
			setup, err := setupFn(context.Background(), kube, mg)

			// Verify error
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("TerraformSetupBuilder() error = %v, want %v", err, tc.want.err)
			}
			if tc.want.err != nil {
				return
			}

			// Verify configuration
			if got := setup.Configuration["username"]; got != testUsername {
				t.Errorf("TerraformSetupBuilder() username = %v, want %v", got, testUsername)
			}
			if got := setup.Configuration["password"]; got != testPassword {
				t.Errorf("TerraformSetupBuilder() password = %v, want %v", got, testPassword)
			}
			if got := setup.Configuration["globalaccount"]; got != testGlobalAccount {
				t.Errorf("TerraformSetupBuilder() globalaccount = %v, want %v", got, testGlobalAccount)
			}
			if got := setup.Configuration["cli_server_url"]; got != testCliServerURL {
				t.Errorf("TerraformSetupBuilder() cli_server_url = %v, want %v", got, testCliServerURL)
			}

			// Verify IDP field
			idpValue, idpExists := setup.Configuration["idp"]
			if idpExists != tc.want.idpSet {
				t.Errorf("TerraformSetupBuilder() idp exists = %v, want %v", idpExists, tc.want.idpSet)
			}
			if tc.want.idpSet && idpValue != tc.want.idp {
				t.Errorf("TerraformSetupBuilder() idp = %v, want %v", idpValue, tc.want.idp)
			}
		})
	}
}

func TestTerraformSetupBuilderNoTracking_ConditionalIDP(t *testing.T) {
	type args struct {
		origin string
	}
	type want struct {
		idpSet bool
		idp    string
		err    error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "WithoutCustomIDP",
			args: args{
				origin: "",
			},
			want: want{
				idpSet: false,
				err:    nil,
			},
		},
		{
			name: "WithCustomIDP",
			args: args{
				origin: testCustomIDP,
			},
			want: want{
				idpSet: true,
				idp:    testCustomIDP,
				err:    nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create test credentials
			userCred := btp.UserCredential{
				Username: testUsername,
				Password: testPassword,
				Idp:      tc.args.origin,
			}
			credJSON, err := json.Marshal(userCred)
			if err != nil {
				t.Fatalf("failed to marshal credentials: %v", err)
			}

			// Setup mock Kubernetes client
			kube := &test.MockClient{
				MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					switch v := obj.(type) {
					case *v1alpha1.ProviderConfig:
						pc := fakeProviderConfig(testProviderName, testSecretName, testSecretNS, testGlobalAccount, testCliServerURL)
						*v = *pc
					case *corev1.Secret:
						v.Data = map[string][]byte{
							"credentials": credJSON,
						}
					}
					return nil
				},
			}

			// Create fake managed resource
			mg := &fake.Managed{}
			mg.SetProviderConfigReference(&xpv1.Reference{Name: testProviderName})

			// Call TerraformSetupBuilderNoTracking
			setupFn := TerraformSetupBuilderNoTracking("1.5.0", "SAP/btp", "1.7.0")
			setup, err := setupFn(context.Background(), kube, mg)

			// Verify error
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("TerraformSetupBuilderNoTracking() error = %v, want %v", err, tc.want.err)
			}
			if tc.want.err != nil {
				return
			}

			// Verify configuration
			if got := setup.Configuration["username"]; got != testUsername {
				t.Errorf("TerraformSetupBuilderNoTracking() username = %v, want %v", got, testUsername)
			}
			if got := setup.Configuration["password"]; got != testPassword {
				t.Errorf("TerraformSetupBuilderNoTracking() password = %v, want %v", got, testPassword)
			}
			if got := setup.Configuration["globalaccount"]; got != testGlobalAccount {
				t.Errorf("TerraformSetupBuilderNoTracking() globalaccount = %v, want %v", got, testGlobalAccount)
			}
			if got := setup.Configuration["cli_server_url"]; got != testCliServerURL {
				t.Errorf("TerraformSetupBuilderNoTracking() cli_server_url = %v, want %v", got, testCliServerURL)
			}

			// Verify IDP field
			idpValue, idpExists := setup.Configuration["idp"]
			if idpExists != tc.want.idpSet {
				t.Errorf("TerraformSetupBuilderNoTracking() idp exists = %v, want %v", idpExists, tc.want.idpSet)
			}
			if tc.want.idpSet && idpValue != tc.want.idp {
				t.Errorf("TerraformSetupBuilderNoTracking() idp = %v, want %v", idpValue, tc.want.idp)
			}
		})
	}
}

func fakeProviderConfig(name, secretName, secretNS, globalAccount, cliServerURL string) *v1alpha1.ProviderConfig {
	return &v1alpha1.ProviderConfig{
		Spec: v1alpha1.ProviderConfigSpec{
			GlobalAccount: globalAccount,
			CliServerUrl:  cliServerURL,
			ServiceAccountSecret: v1alpha1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{
							Name:      secretName,
							Namespace: secretNS,
						},
						Key: "credentials",
					},
				},
			},
		},
	}
}
