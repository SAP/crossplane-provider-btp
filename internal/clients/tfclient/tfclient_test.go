package tfclient

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTerraformSetupBuilder(t *testing.T) {

	kubeStub := func(err error) client.Client {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(err, func(obj client.Object) error {
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
		err          error
		setupCreated bool
	}

	cases := map[string]struct {
		args    args
		want    want
		mockErr error
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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			mockClient := kubeStub(tc.mockErr)

			tfSetup := TerraformSetupBuilder(tc.args.version, tc.args.providerSource, tc.args.providerVersion, tc.args.disableTracking)

			if tc.want.setupCreated != (tfSetup != nil) {
				t.Errorf("expected terraform setup to be created: %t, tfSetup %t", tc.want.setupCreated, tfSetup != nil)
			}

			ctx := context.Background()
			//mg := TODO, managed resource zum testen

			_, err := tfSetup(ctx, mockClient, mg)

			if tc.want.err != nil {
				if err == nil || !errors.Is(err, tc.want.err) {
					t.Errorf("expected error: %v, tfSetup Error: %v", tc.want.err, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// GetProviderConfigReference
// NewProviderConfigUsageTracker Fehler
// Unmarshal Fehler
// CommonCredentialsExtractor
