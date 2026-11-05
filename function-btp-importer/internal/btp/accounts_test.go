package btp

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

func TestFindSubaccount(t *testing.T) {
	type args struct {
		setup     func(btptest.Config) btptest.Config
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
		"MatchBySubdomainAndRegion": {
			reason: "matching subdomain+region → return guid",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.Subaccounts = append(c.Subaccounts, btptest.Subaccount{GUID: "sa-uuid-2", Subdomain: "other-sub", Region: "eu10"})
					return c
				},
				subdomain: "my-sub", region: "eu10",
			},
			want: want{guid: "sa-uuid-1"},
		},
		"NoMatchReturnsEmpty": {
			reason: "no matching subdomain → empty string, no error",
			args:   args{subdomain: "missing-sub", region: "eu10"},
			want:   want{guid: ""},
		},
		"MultipleMatchesReturnsFirst": {
			reason: "two entries with same subdomain+region → return first (subdomains are unique per BTP)",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.Subaccounts = []btptest.Subaccount{
						{GUID: "sa-uuid-1", Subdomain: "dup", Region: "eu10"},
						{GUID: "sa-uuid-2", Subdomain: "dup", Region: "eu10"},
					}
					return c
				},
				subdomain: "dup", region: "eu10",
			},
			want: want{guid: "sa-uuid-1"},
		},
		"RegionMismatchSkipped": {
			reason: "matching subdomain but different region → no match",
			args:   args{subdomain: "my-sub", region: "us10"},
			want:   want{guid: ""},
		},
		"ServerError": {
			reason: "5xx response → error returned",
			args:   args{setup: withFaults(btptest.Fault{Status: http.StatusInternalServerError}), subdomain: "my-sub", region: "eu10"},
			want:   want{err: cmpopts.AnyError},
		},
		"MalformedResponse": {
			reason: "200 with invalid JSON body → error returned",
			args:   args{setup: withFaults(btptest.Fault{Body: `}not valid json{`}), subdomain: "my-sub", region: "eu10"},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable → error returned",
			args:   args{setup: down, subdomain: "my-sub", region: "eu10"},
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
