package servicemanager

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

func adoptingServiceManager(annotation, planID string) *apisv1beta1.ServiceManager {
	cr := NewServiceManager("cr-name")
	meta.SetExternalName(cr, "")
	if annotation != "" {
		meta.AddAnnotations(cr, map[string]string{adoption.Annotation: annotation})
	}
	if planID != "" {
		cr.Status.AtProvider.DataSourceLookup = &apisv1beta1.DataSourceLookup{ServiceManagerPlanID: planID}
	}
	return cr
}

func TestAdopt(t *testing.T) {
	errBoom := errors.New("boom")

	type want struct {
		err          error
		externalName string
		lookedUp     bool
	}

	cases := map[string]struct {
		reason string
		cr     *apisv1beta1.ServiceManager
		lookup *smLookuperFake
		want   want
	}{
		"NotOptedIn": {
			reason: "Without the annotation, nothing is looked up.",
			cr:     adoptingServiceManager("", "plan-1"),
			lookup: &smLookuperFake{},
			want:   want{},
		},
		"NoMatchCreates": {
			reason: "\"Import if it exists\": when nothing matches, the instance and binding are created as usual.",
			cr:     adoptingServiceManager("true", "plan-1"),
			lookup: &smLookuperFake{byNameFound: false},
			want:   want{lookedUp: true},
		},
		"InstanceAndBindingAreAdopted": {
			reason: "A complete pair is adopted under the compound external-name.",
			cr:     adoptingServiceManager("true", "plan-1"),
			lookup: &smLookuperFake{byNameFound: true, byName: sm.InstanceMatch{ID: "si-1", PlanID: "plan-1"}, found: true, siID: "si-1", sbID: "sb-1"},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: "si-1/sb-1", lookedUp: true},
		},
		"InstanceWithoutBindingIsAdopted": {
			reason: "An instance without its binding is adopted as the bare instance ID, the phase-1 state from which Create adds the binding.",
			cr:     adoptingServiceManager("true", "plan-1"),
			lookup: &smLookuperFake{byNameFound: true, byName: sm.InstanceMatch{ID: "si-1", PlanID: "plan-1"}, found: true, siID: "si-1"},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: "si-1", lookedUp: true},
		},
		"SameNameOnOtherPlanIsRefused": {
			reason: "An instance with the managed name on another plan is not this pair; it is refused rather than treated as missing, which would create a second instance.",
			cr:     adoptingServiceManager("true", "plan-1"),
			lookup: &smLookuperFake{byNameFound: true, byName: sm.InstanceMatch{ID: "si-1", PlanID: "plan-other"}},
			want:   want{err: errors.New(`refusing to adopt service instance "managed-service-manager" (si-1): it runs on service plan plan-other, the spec declares plan-1; instance names are unique, so it can neither be adopted nor created under this name: declare the plan it runs on to adopt it, or set another serviceInstanceName to create a new one`), lookedUp: true},
		},
		"UnresolvedPlanIsRefused": {
			reason: "The plan scopes the lookup to the managed admin instance; without it the reconcile fails rather than creates.",
			cr:     adoptingServiceManager("true", ""),
			lookup: &smLookuperFake{},
			want:   want{err: errors.New(errAdoptNoPlan)},
		},
		"LookupFails": {
			reason: "A failed lookup must fail the reconcile; reporting the resources missing would create duplicates.",
			cr:     adoptingServiceManager("true", "plan-1"),
			lookup: &smLookuperFake{byNameErr: errBoom},
			want:   want{err: errors.Wrap(errBoom, `cannot look up service instance "managed-service-manager" to adopt`), lookedUp: true},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				kube:               &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
				newAdminLookuperFn: smFactory(tc.lookup),
			}

			err := c.adopt(context.Background(), tc.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nadopt(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.externalName, meta.GetExternalName(tc.cr)); diff != "" {
				t.Errorf("%s\nadopt(...): -want external-name, +got external-name:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.lookedUp, tc.lookup.gotName != ""); diff != "" {
				t.Errorf("%s\nadopt(...): -want looked up, +got looked up:\n%s", tc.reason, diff)
			}
			if tc.lookup.gotPlan != "" {
				got := tc.lookup.gotPlan + " " + tc.lookup.gotSI + " " + tc.lookup.gotSB
				want := "plan-1 " + apisv1beta1.DefaultServiceInstanceName + " " + apisv1beta1.DefaultServiceBindingName
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("%s\nadopt(...): -want lookup key, +got lookup key:\n%s", tc.reason, diff)
				}
			}
		})
	}
}

// countingInitializer records whether Connect built the terraform clients.
type countingInitializer struct{ calls int }

func (i *countingInitializer) ConnectResources(context.Context, *apisv1beta1.ServiceManager) (sm.ITfClient, error) {
	i.calls++
	return &TfClientFake{}, nil
}

// TestConnectAdoptsBeforeBuildingClient pins where adoption happens: clients
// built before the external-name is set would carry upjet state for an empty
// ID and observe the adopted pair as missing, so Create would duplicate it.
func TestConnectAdoptsBeforeBuildingClient(t *testing.T) {
	initializer := &countingInitializer{}
	c := connector{
		kube:            &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
		resourcetracker: testutils.NewResourceTrackerMock(),
		newClientInitalizerFn: func() sm.ITfClientInitializer {
			return initializer
		},
		newAdminLookuperFn: smFactory(&smLookuperFake{
			byNameFound: true, byName: sm.InstanceMatch{ID: "si-1", PlanID: "plan-1"},
			found: true, siID: "si-1", sbID: "sb-1",
		}),
	}
	cr := adoptingServiceManager("true", "plan-1")
	cr.Spec.ForProvider.SubaccountGuid = "sa-1"

	_, err := c.Connect(context.Background(), cr)

	if diff := cmp.Diff(adoption.ErrRequeueAfterAdoption, err, test.EquateErrors()); diff != "" {
		t.Errorf("Connect(...): -want error, +got error:\n%s", diff)
	}
	if diff := cmp.Diff(0, initializer.calls); diff != "" {
		t.Errorf("Connect(...) must not build the terraform clients in the adopting reconcile: -want calls, +got calls:\n%s", diff)
	}
	if diff := cmp.Diff("si-1/sb-1", meta.GetExternalName(cr)); diff != "" {
		t.Errorf("Connect(...): -want external-name, +got external-name:\n%s", diff)
	}
}
