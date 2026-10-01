package subaccount

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
)

func adoptingSubaccount(annotation string) *apisv1alpha1.Subaccount {
	cr := &apisv1alpha1.Subaccount{}
	cr.SetName("cr-name")
	cr.Spec.ForProvider.Subdomain = "my-subdomain"
	cr.Spec.ForProvider.Region = "eu10"
	if annotation != "" {
		cr.SetAnnotations(map[string]string{adoption.Annotation: annotation})
	}
	return cr
}

func TestObserveOptInAdoption(t *testing.T) {
	const guid = "b808193b-e1ee-4001-abec-f920134cca60"
	errBoom := errors.New("boom")

	type want struct {
		obs          managed.ExternalObservation
		err          error
		externalName string
		lookupCalls  int
	}

	cases := map[string]struct {
		reason string
		cr     *apisv1alpha1.Subaccount
		acc    *MockAccountsApiAccessor
		want   want
	}{
		"NotOptedIn": {
			reason: "Without the annotation, Observe must not look anything up (recovery's own lookup is gated on a Create attempt).",
			cr:     adoptingSubaccount(""),
			acc:    &MockAccountsApiAccessor{},
			want:   want{obs: managed.ExternalObservation{ResourceExists: false}},
		},
		"NoMatchCreates": {
			reason: "\"Import if it exists\": when nothing matches, the subaccount is created as usual.",
			cr:     adoptingSubaccount("true"),
			acc:    &MockAccountsApiAccessor{lookupFound: false},
			want:   want{obs: managed.ExternalObservation{ResourceExists: false}, lookupCalls: 1},
		},
		"MatchInDeclaredRegionIsAdopted": {
			reason: "The subdomain is unique in the global account; a match in the declared region is adopted.",
			cr:     adoptingSubaccount("true"),
			acc:    &MockAccountsApiAccessor{lookupFound: true, lookupGuid: guid, lookupRegion: "eu10"},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: guid, lookupCalls: 1},
		},
		"MatchInOtherRegionIsRefused": {
			reason: "Region is immutable; adopting a subaccount from another region could never converge, so it is refused without creating.",
			cr:     adoptingSubaccount("true"),
			acc:    &MockAccountsApiAccessor{lookupFound: true, lookupGuid: guid, lookupRegion: "us10"},
			want:   want{err: errRegionMismatch("my-subdomain", guid, "us10", "eu10"), lookupCalls: 1},
		},
		"LookupFails": {
			reason: "A failed lookup must fail the reconcile; reporting the subaccount missing would create a duplicate.",
			cr:     adoptingSubaccount("true"),
			acc:    &MockAccountsApiAccessor{lookupErr: errBoom},
			want:   want{err: errors.Wrap(errBoom, errAdoptLookup), lookupCalls: 1},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				Client:           &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
				accountsAccessor: tc.acc,
			}

			obs, err := e.Observe(context.Background(), tc.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nObserve(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.obs, obs); diff != "" {
				t.Errorf("%s\nObserve(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.externalName, meta.GetExternalName(tc.cr)); diff != "" {
				t.Errorf("%s\nObserve(...): -want external-name, +got external-name:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.lookupCalls, tc.acc.lookupCalls); diff != "" {
				t.Errorf("%s\nObserve(...): -want lookups, +got lookups:\n%s", tc.reason, diff)
			}
		})
	}
}
