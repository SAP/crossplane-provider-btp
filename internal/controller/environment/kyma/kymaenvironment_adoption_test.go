package kyma

import (
	"context"
	"net/http"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/kyma/fake"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

func adoptingKyma(annotation string) *v1alpha1.KymaEnvironment {
	cr := &v1alpha1.KymaEnvironment{}
	cr.SetName("cr-name")
	cr.Spec.ForProvider.Name = internal.Ptr("my-kyma")
	cr.Spec.ForProvider.PlanName = "aws"
	if annotation != "" {
		cr.SetAnnotations(map[string]string{adoption.Annotation: annotation})
	}
	return cr
}

func TestObserveOptInAdoption(t *testing.T) {
	const guid = "9a4f3c2e-1b7d-4e8a-9c6f-2d5b8e1a7f30"
	errBoom := errors.New("boom")

	instance := func(planName string) provisioningclient.BusinessEnvironmentInstanceResponseObject {
		return provisioningclient.BusinessEnvironmentInstanceResponseObject{Id: internal.Ptr(guid), PlanName: internal.Ptr(planName)}
	}

	type find struct {
		instance provisioningclient.BusinessEnvironmentInstanceResponseObject
		found    bool
		err      error
	}
	type want struct {
		obs          managed.ExternalObservation
		err          error
		externalName string
		lookups      int
	}

	cases := map[string]struct {
		reason string
		cr     *v1alpha1.KymaEnvironment
		find   find
		want   want
	}{
		"NotOptedIn": {
			reason: "Without the annotation, Observe must not look anything up.",
			cr:     adoptingKyma(""),
			want:   want{obs: managed.ExternalObservation{ResourceExists: false}},
		},
		"NoMatchCreates": {
			reason: "\"Import if it exists\": when nothing matches, the environment is created as usual.",
			cr:     adoptingKyma("true"),
			find:   find{found: false},
			want:   want{obs: managed.ExternalObservation{ResourceExists: false}, lookups: 1},
		},
		"MatchOnDeclaredPlanIsAdopted": {
			reason: "A name match on the declared plan is adopted and the reconcile requeued against it.",
			cr:     adoptingKyma("true"),
			find:   find{instance: instance("aws"), found: true},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: guid, lookups: 1},
		},
		"MatchOnOtherPlanIsRefused": {
			reason: "The plan fixes the hyperscaler and cannot change; adopting a cluster on another plan could never converge, so it is refused without creating.",
			cr:     adoptingKyma("true"),
			find:   find{instance: instance("azure"), found: true},
			want:   want{err: errPlanMismatch("my-kyma", guid, "azure", "aws"), lookups: 1},
		},
		"LookupFails": {
			reason: "A failed lookup must fail the reconcile; reporting the environment missing would create a duplicate.",
			cr:     adoptingKyma("true"),
			find:   find{err: errBoom},
			want:   want{err: errors.Wrap(errBoom, errAdoptLookup), lookups: 1},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			lookups := 0
			client := fake.MockClient{
				MockFindInstance: func(_ context.Context, cr *v1alpha1.KymaEnvironment) (provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
					lookups++
					return tc.find.instance, tc.find.found, tc.find.err
				},
			}
			e := external{
				client:     client,
				httpClient: http.DefaultClient,
				kube:       &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
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
			if diff := cmp.Diff(tc.want.lookups, lookups); diff != "" {
				t.Errorf("%s\nObserve(...): -want lookups, +got lookups:\n%s", tc.reason, diff)
			}
		})
	}
}
