package tfclient

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTerraformSetupBuilder(t *testing.T) {

	kubeStub := func(err error, secretData []byte) client.Client {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(err, func(obj client.Object) error {
				if secret, ok := obj.(*v1.Secret); ok {
					secret.Data = map[string][]byte{"service-account.json": secretData}
				}
				return nil
			}),
		}
	}

	type args struct {
		version         string
		providerSource  string
		providerVersion string
		disableTracking bool
	}
	type want struct {
		err           error
		setupCreated  bool
		username      string
		password      string
		globalAccount string
		cliServerUrl  string
	}

	cases := map[string]struct {
		args           args
		want           want
		mockSecretData []byte
		mockErr        error
	}{
		"connect tfclient with tracking": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: false,
			},
			want: want{
				err:          nil,
				setupCreated: true,
			},
		},
		"connect tfclient without tracking": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: true,
			},
			want: want{
				err:          nil,
				setupCreated: true,
			},
		},
		"failed to resolve provider config reference": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: false,
			},
			want: want{
				err:          errors.New(errGetProviderConfig),
				setupCreated: false,
			},
			mockErr: errors.New(errGetProviderConfig),
		},
		"connect tfclient with tracking and valid secret data": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: false,
			},
			want: want{
				err:           nil,
				setupCreated:  true,
				username:      "testUser",
				password:      "testPassword",
				globalAccount: "testAccount",
				cliServerUrl:  "<https://cli.server.url>",
			},
			mockSecretData: []byte(`{"username":"testUser","password":"testPassword"}`),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			mockClient := kubeStub(tc.mockErr, tc.mockSecretData)

			tfSetup := TerraformSetupBuilder(tc.args.version, tc.args.providerSource, tc.args.providerVersion, tc.args.disableTracking)
			ctx := context.Background()

			if tc.want.setupCreated != (tfSetup != nil) {
				t.Errorf("expected terraform setup to be created: %t, tfSetup %t", tc.want.setupCreated, tfSetup != nil)
			}

			//mg := TODO, managed resource zum testen

			setup, err := tfSetup(ctx, mockClient, mg)

			if tc.want.setupCreated {
				if setup.Configuration == nil {
					t.Errorf("expected setup to be created with configuration, but got nil")
				} else {
					if username, ok := setup.Configuration["username"]; !ok || username != tc.want.username {
						t.Errorf("expected username: %v, got: %v", tc.want.username, username)
					}
					if password, ok := setup.Configuration["password"]; !ok || password != tc.want.password {
						t.Errorf("expected password: %v, got: %v", tc.want.password, password)
					}
					if globalAccount, ok := setup.Configuration["globalaccount"]; !ok || globalAccount != tc.want.globalAccount {
						t.Errorf("expected globalaccount: %v, got: %v", tc.want.globalAccount, globalAccount)
					}
					if cliServerUrl, ok := setup.Configuration["cli_server_url"]; !ok || cliServerUrl != tc.want.cliServerUrl {
						t.Errorf("expected cli_server_url: %v, got: %v", tc.want.cliServerUrl, cliServerUrl)
					}
				}
			}

			if tc.want.err != nil {
				if err == nil || !errors.Is(err, tc.want.err) {
					t.Errorf("expected error: %v, tfSetup Error: %v", tc.want.err, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if setup.Configuration["username"] != tc.want.username {
				t.Errorf("expected username: %v, got: %v", tc.want.username, setup.Configuration["username"])
			}
			if setup.Configuration["password"] != tc.want.password {
				t.Errorf("expected password: %v, got: %v", tc.want.password, setup.Configuration["password"])
			}
			if setup.Configuration["globalaccount"] != tc.want.globalAccount {
				t.Errorf("expected globalaccount: %v, got: %v", tc.want.globalAccount, setup.Configuration["globalaccount"])
			}
			if setup.Configuration["cli_server_url"] != tc.want.cliServerUrl {
				t.Errorf("expected cli_server_url: %v, got: %v", tc.want.cliServerUrl, setup.Configuration["cli_server_url"])
			}
		})
	}
}

// GetProviderConfigReference
// NewProviderConfigUsageTracker Fehler
// Unmarshal Fehler
// CommonCredentialsExtractor
