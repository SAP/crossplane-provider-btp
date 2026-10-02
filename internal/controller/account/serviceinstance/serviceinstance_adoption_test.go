package serviceinstance

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

// findFake serves the adoption lookup. The embedded interface is nil, so a
// call to any recovery method panics: adoption must only use Find*.
type findFake struct {
	smClient.SemanticLookuper

	match smClient.InstanceMatch
	found bool
	err   error

	gotName string
	calls   int
}

func (f *findFake) FindServiceInstance(_ context.Context, name string) (smClient.InstanceMatch, bool, error) {
	f.calls++
	f.gotName = name
	return f.match, f.found, f.err
}

func adoptingInstance(annotation string) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}
	cr.SetName("cr-name")
	cr.Spec.ForProvider.Name = "my-instance"
	// The spec declares the plan by name; TestAdopt resolves it to
	// "plan-declared". Status caches the plan resolved for an earlier spec,
	// which adopt must never trust.
	cr.Spec.ForProvider.OfferingName = "hana-cloud"
	cr.Spec.ForProvider.PlanName = "hana"
	cr.Status.AtProvider.ServiceplanID = "plan-stale"
	if annotation != "" {
		cr.SetAnnotations(map[string]string{adoption.Annotation: annotation})
	}
	return cr
}

func TestAdopt(t *testing.T) {
	const guid = "80540c06-2955-4bce-9c43-ad78fecc7f62"
	errBoom := errors.New("boom")

	type args struct {
		cr        *v1alpha1.ServiceInstance
		lookup    *findFake
		lookupErr error
		planErr   error
	}
	type want struct {
		err          error
		externalName string
		lookupCalls  int
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NotOptedIn": {
			reason: "Without the annotation, nothing is looked up.",
			args:   args{cr: adoptingInstance(""), lookup: &findFake{}},
			want:   want{},
		},
		"NoMatchCreates": {
			reason: "\"Import if it exists\": when nothing matches, the instance is created as usual.",
			args:   args{cr: adoptingInstance("true"), lookup: &findFake{found: false}},
			want:   want{lookupCalls: 1},
		},
		"MatchOnDeclaredPlanIsAdopted": {
			reason: "A name match on the declared plan is adopted and the reconcile requeued against it.",
			args: args{cr: adoptingInstance("true"), lookup: &findFake{
				found: true, match: smClient.InstanceMatch{ID: guid, PlanID: "plan-declared"},
			}},
			want: want{err: adoption.ErrRequeueAfterAdoption, externalName: guid, lookupCalls: 1},
		},
		"LegacyExternalNameIsLookedUp": {
			reason: "An external-name equal to metadata.name is a default left by older provider versions, not an identifier; the lookup runs and replaces it.",
			args: args{
				cr: func() *v1alpha1.ServiceInstance {
					cr := adoptingInstance("true")
					meta.SetExternalName(cr, cr.GetName())
					return cr
				}(),
				lookup: &findFake{found: true, match: smClient.InstanceMatch{ID: guid, PlanID: "plan-declared"}},
			},
			want: want{err: adoption.ErrRequeueAfterAdoption, externalName: guid, lookupCalls: 1},
		},
		"MatchOnStaleCachedPlanIsRefused": {
			reason: "The check is against the plan the spec declares now. Status still caches the plan of an earlier spec; trusting it would keep a fixed spec refused, or adopt against the old plan.",
			args: args{cr: adoptingInstance("true"), lookup: &findFake{
				found: true, match: smClient.InstanceMatch{ID: guid, PlanID: "plan-stale"},
			}},
			want: want{err: errPlanMismatch(adoptingInstance("true"), smClient.InstanceMatch{ID: guid, PlanID: "plan-stale"}, "plan-declared"), lookupCalls: 1},
		},
		"PlanResolutionFails": {
			reason: "Without the declared plan the match cannot be verified, so the reconcile fails rather than adopts or creates.",
			args: args{cr: adoptingInstance("true"), planErr: errBoom, lookup: &findFake{
				found: true, match: smClient.InstanceMatch{ID: guid, PlanID: "plan-declared"},
			}},
			want: want{err: errors.Wrap(errBoom, errAdoptPlan), lookupCalls: 1},
		},
		"MatchOnOtherPlanIsRefused": {
			reason: "Two services can share an instance name; adopting one on another plan would bind the wrong service, so it is refused without creating.",
			args: args{cr: adoptingInstance("true"), lookup: &findFake{
				found: true, match: smClient.InstanceMatch{ID: guid, PlanID: "plan-other"},
			}},
			want: want{err: errPlanMismatch(adoptingInstance("true"), smClient.InstanceMatch{ID: guid, PlanID: "plan-other"}, "plan-declared"), lookupCalls: 1},
		},
		"LookupFails": {
			reason: "A failed lookup must fail the reconcile; reporting the instance missing would create a duplicate.",
			args:   args{cr: adoptingInstance("true"), lookup: &findFake{err: errBoom}},
			want:   want{err: errors.Wrap(errBoom, errAdoptLookup), lookupCalls: 1},
		},
		"LookupClientFails": {
			reason: "Without a lookup client the user's request cannot be honoured, so the reconcile fails rather than creates.",
			args:   args{cr: adoptingInstance("true"), lookup: &findFake{}, lookupErr: errBoom},
			want:   want{err: errors.Wrap(errBoom, errAdoptLookup)},
		},
		"InvalidAnnotation": {
			reason: "A typo in the annotation must surface before anything is created.",
			args:   args{cr: adoptingInstance("yes"), lookup: &findFake{}},
			want:   want{err: errors.New(`annotation btp.sap.crossplane.io/lookup must be "true" or "false", got "yes"`)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				kube: &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
				newServicePlanInitializerFn: func() Initializer {
					return &InitializerMock{planID: "plan-declared", planErr: tc.args.planErr}
				},
				newAdminLookuperFn: func(context.Context, *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error) {
					return tc.args.lookup, func() {}, tc.args.lookupErr
				},
			}

			err := c.adopt(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nadopt(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.externalName, meta.GetExternalName(tc.args.cr)); diff != "" {
				t.Errorf("%s\nadopt(...): -want external-name, +got external-name:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.lookupCalls, tc.args.lookup.calls); diff != "" {
				t.Errorf("%s\nadopt(...): -want lookups, +got lookups:\n%s", tc.reason, diff)
			}
		})
	}
}

// TestConnectAdoptsBeforeBuildingClient pins where adoption happens: a client
// built before the external-name is set would carry upjet state for an empty
// ID and observe the adopted instance as missing, so Create would duplicate it.
func TestConnectAdoptsBeforeBuildingClient(t *testing.T) {
	const guid = "80540c06-2955-4bce-9c43-ad78fecc7f62"

	creator := &TfProxyClientCreatorMock{}
	c := connector{
		kube:                        &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
		clientConnector:             creator,
		newServicePlanInitializerFn: func() Initializer { return &InitializerMock{planID: "plan-declared"} },
		resourcetracker:             testutils.NewResourceTrackerMock(),
		newAdminLookuperFn: func(context.Context, *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error) {
			return &findFake{found: true, match: smClient.InstanceMatch{ID: guid, PlanID: "plan-declared"}}, func() {}, nil
		},
	}
	cr := adoptingInstance("true")

	_, err := c.Connect(context.Background(), cr)

	if diff := cmp.Diff(adoption.ErrRequeueAfterAdoption, err, test.EquateErrors()); diff != "" {
		t.Errorf("Connect(...): -want error, +got error:\n%s", diff)
	}
	if diff := cmp.Diff(0, creator.calls); diff != "" {
		t.Errorf("Connect(...) must not build the terraform client in the adopting reconcile: -want calls, +got calls:\n%s", diff)
	}
	if diff := cmp.Diff(guid, meta.GetExternalName(cr)); diff != "" {
		t.Errorf("Connect(...): -want external-name, +got external-name:\n%s", diff)
	}
}
