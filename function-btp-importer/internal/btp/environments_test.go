package btp

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

// withEnvironments replaces the fixture's environments list.
func withEnvironments(envs ...btptest.Environment) func(btptest.Config) btptest.Config {
	return func(c btptest.Config) btptest.Config {
		c.Environments = envs
		return c
	}
}

func TestFindKymaEnvironment(t *testing.T) {
	type args struct {
		setup    func(btptest.Config) btptest.Config
		name     string
		planName string
	}
	type want struct {
		id  string
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SingleMatchReturnsID": {
			reason: "one Kyma environment matching name and plan — id returned",
			args: args{
				setup: withEnvironments(
					btptest.Environment{ID: "kyma-uuid-1", Name: "my-kyma", EnvironmentType: "kyma", PlanName: "aws"},
					btptest.Environment{ID: "cf-uuid-1", Name: "my-kyma", EnvironmentType: "cloudfoundry", PlanName: "standard"},
				),
				name: "my-kyma", planName: "aws",
			},
			want: want{id: "kyma-uuid-1"},
		},
		"DisambiguatesByPlan": {
			reason: "two Kyma environments with same name but different plans — correct one returned",
			args: args{
				setup: withEnvironments(
					btptest.Environment{ID: "kyma-aws", Name: "my-kyma", EnvironmentType: "kyma", PlanName: "aws"},
					btptest.Environment{ID: "kyma-gcp", Name: "my-kyma", EnvironmentType: "kyma", PlanName: "gcp"},
				),
				name: "my-kyma", planName: "gcp",
			},
			want: want{id: "kyma-gcp"},
		},
		"NoMatchReturnsEmpty": {
			reason: "no Kyma environment matching name and plan — empty string, no error",
			args:   args{name: "other", planName: "aws"},
			want:   want{id: ""},
		},
		"AmbiguousMatchReturnsError": {
			reason: "two Kyma environments with same name and plan — error returned",
			args: args{
				setup: withEnvironments(
					btptest.Environment{ID: "uuid-1", Name: "dup", EnvironmentType: "kyma", PlanName: "aws"},
					btptest.Environment{ID: "uuid-2", Name: "dup", EnvironmentType: "kyma", PlanName: "aws"},
				),
				name: "dup", planName: "aws",
			},
			want: want{err: cmpopts.AnyError},
		},
		"ServerError": {
			reason: "5xx response — error returned",
			args:   args{setup: withFaults(btptest.Fault{Status: http.StatusInternalServerError}), name: "my-kyma", planName: "aws"},
			want:   want{err: cmpopts.AnyError},
		},
		"MalformedResponse": {
			reason: "200 with invalid JSON body — error returned",
			args:   args{setup: withFaults(btptest.Fault{Body: `}not valid json{`}), name: "my-kyma", planName: "aws"},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down, name: "my-kyma", planName: "aws"},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			got, err := NewProvisioningClient(srv.Client(), srv.URL).FindKymaEnvironment(context.Background(), tc.args.name, tc.args.planName)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nFindKymaEnvironment(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.id, got); diff != "" {
				t.Errorf("%s\nFindKymaEnvironment(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestFindCloudFoundryEnvironment(t *testing.T) {
	type args struct {
		setup func(btptest.Config) btptest.Config
		name  string
	}
	type want struct {
		id  string
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SingleMatchReturnsID": {
			reason: "one CF environment matching name — id returned",
			args: args{
				setup: withEnvironments(
					btptest.Environment{ID: "cf-uuid-1", Name: "my-cf", EnvironmentType: "cloudfoundry"},
					btptest.Environment{ID: "kyma-uuid-1", Name: "my-cf", EnvironmentType: "kyma", PlanName: "aws"},
				),
				name: "my-cf",
			},
			want: want{id: "cf-uuid-1"},
		},
		"NoMatchReturnsEmpty": {
			reason: "no CF environment matching name — empty string, no error",
			args:   args{name: "other"},
			want:   want{id: ""},
		},
		"MultipleMatchesReturnsError": {
			reason: "two CF environments with same name — error returned (no plan to disambiguate)",
			args: args{
				setup: withEnvironments(
					btptest.Environment{ID: "uuid-1", Name: "dup", EnvironmentType: "cloudfoundry"},
					btptest.Environment{ID: "uuid-2", Name: "dup", EnvironmentType: "cloudfoundry"},
				),
				name: "dup",
			},
			want: want{err: cmpopts.AnyError},
		},
		"ServerError": {
			reason: "5xx response — error returned",
			args:   args{setup: withFaults(btptest.Fault{Status: http.StatusInternalServerError}), name: "my-cf"},
			want:   want{err: cmpopts.AnyError},
		},
		"MalformedResponse": {
			reason: "200 with invalid JSON body — error returned",
			args:   args{setup: withFaults(btptest.Fault{Body: `}not valid json{`}), name: "my-cf"},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down, name: "my-cf"},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			got, err := NewProvisioningClient(srv.Client(), srv.URL).FindCloudFoundryEnvironment(context.Background(), tc.args.name)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nFindCloudFoundryEnvironment(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.id, got); diff != "" {
				t.Errorf("%s\nFindCloudFoundryEnvironment(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
