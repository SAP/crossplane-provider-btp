package subscription

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	"github.com/sap/crossplane-provider-btp/internal/clients/subscription"
)

func adoptingSubscription(annotation string) *v1alpha1.Subscription {
	cr := &v1alpha1.Subscription{}
	cr.SetName("cr-name")
	cr.Spec.ForProvider.AppName = "my-app"
	cr.Spec.ForProvider.PlanName = "standard"
	if annotation != "" {
		cr.SetAnnotations(map[string]string{adoption.Annotation: annotation})
	}
	return cr
}

func TestObserveOptInAdoption(t *testing.T) {
	errBoom := errors.New("boom")

	type want struct {
		obs          managed.ExternalObservation
		err          error
		externalName string
		gotKey       string
	}

	cases := map[string]struct {
		reason string
		cr     *v1alpha1.Subscription
		api    *MockApiHandler
		want   want
	}{
		"NotOptedIn": {
			reason: "Without the annotation, Observe must not look anything up.",
			cr:     adoptingSubscription(""),
			api:    &MockApiHandler{},
			want:   want{obs: managed.ExternalObservation{ResourceExists: false}},
		},
		"NotSubscribedCreates": {
			reason: "\"Import if it exists\": when the app is not subscribed, the subscription is created as usual.",
			cr:     adoptingSubscription("true"),
			api:    &MockApiHandler{returnGet: nil},
			want:   want{obs: managed.ExternalObservation{ResourceExists: false}, gotKey: "my-app/standard"},
		},
		"SubscribedIsAdopted": {
			reason: "An existing subscription is adopted under the appName/planName key the provider itself uses.",
			cr:     adoptingSubscription("true"),
			api:    &MockApiHandler{returnGet: &subscription.SubscriptionGet{}},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: "my-app/standard", gotKey: "my-app/standard"},
		},
		"LookupFails": {
			reason: "A failed lookup must fail the reconcile; reporting the subscription missing would subscribe again.",
			cr:     adoptingSubscription("true"),
			api:    &MockApiHandler{returnErr: errBoom},
			want:   want{err: errors.Wrap(errBoom, errAdoptLookup), gotKey: "my-app/standard"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				kube:       &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
				apiHandler: tc.api,
				typeMapper: &MockTypeMapper{},
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
			if diff := cmp.Diff(tc.want.gotKey, tc.api.gotGetKey); diff != "" {
				t.Errorf("%s\nObserve(...): -want looked-up key, +got looked-up key:\n%s", tc.reason, diff)
			}
		})
	}
}
