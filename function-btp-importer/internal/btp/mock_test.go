package btp

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

// The tests in this file validate the shared mock through the real clients,
// acquired exactly as production acquires them: the SM client via the
// Accounts admin-binding endpoint and OAuth against the mock's token endpoint,
// the others by URL. They pin that the mock's data reaches the clients in the
// shape the clients expect.

// newMockSMClient acquires an SM client against a fresh default mock exactly
// as production does, and returns the mock's URL alongside.
func newMockSMClient(t *testing.T) (*SMClient, string) {
	t.Helper()
	srv := btptest.Serve(t, btptest.Default())
	sm, err := NewSMClient(context.Background(), srv.Client(), srv.URL, "test-sub")
	if err != nil {
		t.Fatalf("NewSMClient() against mock: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	return sm, srv.URL
}

func TestMockBTPFindServiceInstance(t *testing.T) {
	type args struct {
		name string
	}
	type want struct {
		resource *SMResource
		err      error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Match": {
			reason: "the client's fieldQuery lookup finds the configured instance and reads its plan linkage",
			args:   args{name: "my-instance"},
			want:   want{resource: &SMResource{ID: "si-uuid-1", ServicePlanID: "plan-uuid-1"}},
		},
		"NoMatch": {
			reason: "a name absent from the mock data is a clean no-match, not an error",
			args:   args{name: "missing"},
			want:   want{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sm, _ := newMockSMClient(t)

			got, err := sm.FindServiceInstance(context.Background(), tc.args.name)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nFindServiceInstance(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.resource, got); diff != "" {
				t.Errorf("%s\nFindServiceInstance(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestMockBTPFindServiceBinding(t *testing.T) {
	type args struct {
		name string
	}
	type want struct {
		resource *SMResource
		err      error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Match": {
			reason: "the client's fieldQuery lookup finds the configured binding and reads its instance linkage",
			args:   args{name: "my-binding"},
			want:   want{resource: &SMResource{ID: "sb-uuid-1", ServiceInstanceID: "si-uuid-1"}},
		},
		"NoMatch": {
			reason: "a name absent from the mock data is a clean no-match, not an error",
			args:   args{name: "missing"},
			want:   want{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sm, _ := newMockSMClient(t)

			got, err := sm.FindServiceBinding(context.Background(), tc.args.name)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nFindServiceBinding(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.resource, got); diff != "" {
				t.Errorf("%s\nFindServiceBinding(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestMockBTPGetServicePlanIdentity(t *testing.T) {
	type args struct {
		planID string
	}
	type identity struct {
		offering string
		plan     string
	}
	type want struct {
		identity identity
		err      error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"KnownPlan": {
			reason: "plan and offering detail resolve to catalog names through the client, like real SM",
			args:   args{planID: "plan-uuid-1"},
			want:   want{identity: identity{offering: "hana-cloud", plan: "hana"}},
		},
		"UnknownPlan": {
			reason: "a plan ID absent from the mock data is a 404 the client surfaces as an error",
			args:   args{planID: "unknown"},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sm, _ := newMockSMClient(t)

			offering, plan, err := sm.GetServicePlanIdentity(context.Background(), tc.args.planID)
			got := identity{offering: offering, plan: plan}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nGetServicePlanIdentity(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.identity, got, cmp.AllowUnexported(identity{})); diff != "" {
				t.Errorf("%s\nGetServicePlanIdentity(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// TestMockBTPGetBindingCredentials is a focused test: the cloud-management
// credentials the mock serves point back at the mock itself (the client
// authenticates against that address), so the want depends on the server URL
// and is built in the test rather than tabled.
func TestMockBTPGetBindingCredentials(t *testing.T) {
	sm, mockURL := newMockSMClient(t)

	var want CISCredentials
	want.UAA.ClientID = "mock-cm-cid"
	want.UAA.ClientSecret = "mock-cm-csecret"
	want.UAA.URL = mockURL
	want.UAA.SubaccountID = "7b3f9a2e-4d1c-4f8a-b5e6-2c9d8f0a1b3e"
	want.Endpoints.AccountsServiceURL = mockURL
	want.Endpoints.ProvisioningServiceURL = mockURL

	got, err := sm.GetBindingCredentials(context.Background(), "my-binding")
	if err != nil {
		t.Fatalf("GetBindingCredentials(): unexpected error: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetBindingCredentials(): -want, +got:\n%s", diff)
	}
}

func TestMockBTPFindSubaccount(t *testing.T) {
	type args struct {
		subdomain string
		region    string
	}
	type want struct {
		guid string
		err  error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Match": {
			reason: "the client resolves the configured subaccount by subdomain and region",
			args:   args{subdomain: "my-sub", region: "eu10"},
			want:   want{guid: "sa-uuid-1"},
		},
		"RegionMismatchNoMatch": {
			reason: "the same subdomain in another region is a clean no-match",
			args:   args{subdomain: "my-sub", region: "us10"},
			want:   want{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := btptest.Serve(t, btptest.Default())

			got, err := NewAccountsClient(srv.Client(), srv.URL).FindSubaccount(context.Background(), tc.args.subdomain, tc.args.region)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nFindSubaccount(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.guid, got); diff != "" {
				t.Errorf("%s\nFindSubaccount(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestMockBTPFindEnvironment(t *testing.T) {
	// One Kyma and one CloudFoundry environment sharing a name, so each
	// lookup has to discriminate on environment type.
	sameName := withEnvironments(
		btptest.Environment{ID: "env-uuid-kyma-1", Name: "my-env", EnvironmentType: "kyma", PlanName: "aws"},
		btptest.Environment{ID: "env-uuid-cf-1", Name: "my-env", EnvironmentType: "cloudfoundry"},
	)

	type args struct {
		kymaName string // set for a Kyma lookup
		planName string
		cfName   string // set for a CloudFoundry lookup
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
		"KymaMatch": {
			reason: "the Kyma lookup picks the kyma-typed environment by name and plan, ignoring the same-named CF one",
			args:   args{kymaName: "my-env", planName: "aws"},
			want:   want{id: "env-uuid-kyma-1"},
		},
		"KymaPlanMismatchNoMatch": {
			reason: "a Kyma environment on a different plan is a clean no-match",
			args:   args{kymaName: "my-env", planName: "gcp"},
			want:   want{},
		},
		"CloudFoundryMatch": {
			reason: "the CF lookup picks the cloudfoundry-typed environment by name, ignoring the same-named Kyma one",
			args:   args{cfName: "my-env"},
			want:   want{id: "env-uuid-cf-1"},
		},
		"CloudFoundryNoMatch": {
			reason: "a name absent from the mock data is a clean no-match",
			args:   args{cfName: "missing"},
			want:   want{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := btptest.Serve(t, sameName(btptest.Default()))
			c := NewProvisioningClient(srv.Client(), srv.URL)

			var got string
			var err error
			if tc.args.cfName != "" {
				got, err = c.FindCloudFoundryEnvironment(context.Background(), tc.args.cfName)
			} else {
				got, err = c.FindKymaEnvironment(context.Background(), tc.args.kymaName, tc.args.planName)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nFind*Environment(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.id, got); diff != "" {
				t.Errorf("%s\nFind*Environment(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
