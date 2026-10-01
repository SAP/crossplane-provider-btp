package servicebinding

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

// sbFindFake serves the adoption lookup. The embedded interface is nil, so a
// call to any recovery method panics: adoption must only use Find*.
type sbFindFake struct {
	smClient.SemanticLookuper

	match smClient.BindingMatch
	found bool
	err   error

	gotInstanceID string
	gotName       string
	calls         int
}

func (f *sbFindFake) FindServiceBinding(_ context.Context, serviceInstanceID, name string) (smClient.BindingMatch, bool, error) {
	f.calls++
	f.gotInstanceID = serviceInstanceID
	f.gotName = name
	return f.match, f.found, f.err
}

func adoptingBinding(annotation string, mod ...func(*v1alpha1.ServiceBinding)) *v1alpha1.ServiceBinding {
	cr := &v1alpha1.ServiceBinding{}
	cr.SetName("cr-name")
	cr.Spec.ForProvider.Name = "my-binding"
	cr.Spec.ForProvider.ServiceInstanceID = internal.Ptr("si-1")
	if annotation != "" {
		cr.SetAnnotations(map[string]string{adoption.Annotation: annotation})
	}
	for _, m := range mod {
		m(cr)
	}
	return cr
}

func TestAdopt(t *testing.T) {
	const guid = "80540c06-2955-4bce-9c43-ad78fecc7f62"
	errBoom := errors.New("boom")

	type want struct {
		err          error
		externalName string
		lookupCalls  int
	}

	cases := map[string]struct {
		reason string
		cr     *v1alpha1.ServiceBinding
		lookup *sbFindFake
		want   want
	}{
		"NotOptedIn": {
			reason: "Without the annotation, nothing is looked up.",
			cr:     adoptingBinding(""),
			lookup: &sbFindFake{},
			want:   want{},
		},
		"NoMatchCreates": {
			reason: "\"Import if it exists\": when nothing matches, the binding is created as usual.",
			cr:     adoptingBinding("true"),
			lookup: &sbFindFake{found: false},
			want:   want{lookupCalls: 1},
		},
		"MatchIsAdopted": {
			reason: "An exact name match under the resolved parent instance is adopted; the parent scope is the identity check.",
			cr:     adoptingBinding("true"),
			lookup: &sbFindFake{found: true, match: smClient.BindingMatch{ID: guid}},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: guid, lookupCalls: 1},
		},
		"RotationIsRefused": {
			reason: "A rotating binding's real name carries a generated suffix and its external-name is rewritten by rotation, so adopting by spec name cannot be correct; the combination is refused without creating.",
			cr: adoptingBinding("true", func(cr *v1alpha1.ServiceBinding) {
				cr.Spec.Rotation = &v1alpha1.RotationParameters{}
			}),
			lookup: &sbFindFake{found: true, match: smClient.BindingMatch{ID: guid}},
			want:   want{err: errors.New(errAdoptRotation)},
		},
		"UnresolvedParentIsRefused": {
			reason: "Without the parent instance ID the lookup has no scope; the reconcile fails rather than creates.",
			cr: adoptingBinding("true", func(cr *v1alpha1.ServiceBinding) {
				cr.Spec.ForProvider.ServiceInstanceID = nil
			}),
			lookup: &sbFindFake{},
			want:   want{err: errors.New(errAdoptNoParent)},
		},
		"LookupFails": {
			reason: "A failed lookup must fail the reconcile; reporting the binding missing would create a duplicate.",
			cr:     adoptingBinding("true"),
			lookup: &sbFindFake{err: errBoom},
			want:   want{err: errors.Wrap(errBoom, errAdoptLookup), lookupCalls: 1},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				kube: &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
				newAdminLookuperFn: func(context.Context, *v1alpha1.ServiceBinding) (smClient.SemanticLookuper, func(), error) {
					return tc.lookup, func() {}, nil
				},
			}

			err := c.adopt(context.Background(), tc.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nadopt(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.externalName, meta.GetExternalName(tc.cr)); diff != "" {
				t.Errorf("%s\nadopt(...): -want external-name, +got external-name:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.lookupCalls, tc.lookup.calls); diff != "" {
				t.Errorf("%s\nadopt(...): -want lookups, +got lookups:\n%s", tc.reason, diff)
			}
			if tc.lookup.calls > 0 {
				if diff := cmp.Diff("si-1/my-binding", tc.lookup.gotInstanceID+"/"+tc.lookup.gotName); diff != "" {
					t.Errorf("%s\nadopt(...): -want lookup key, +got lookup key:\n%s", tc.reason, diff)
				}
			}
		})
	}
}

// TestConnectAdoptsBeforeBuildingClient pins where adoption happens: a client
// built before the external-name is set would carry upjet state for an empty
// ID and observe the adopted binding as missing, so Create would duplicate it.
func TestConnectAdoptsBeforeBuildingClient(t *testing.T) {
	const guid = "80540c06-2955-4bce-9c43-ad78fecc7f62"

	factory := &MockServiceBindingClientFactory{}
	c := connector{
		kube:            &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
		resourcetracker: testutils.NewResourceTrackerMock(),
		clientFactory:   factory,
		newAdminLookuperFn: func(context.Context, *v1alpha1.ServiceBinding) (smClient.SemanticLookuper, func(), error) {
			return &sbFindFake{found: true, match: smClient.BindingMatch{ID: guid}}, func() {}, nil
		},
	}
	cr := adoptingBinding("true")

	_, err := c.Connect(context.Background(), cr)

	if diff := cmp.Diff(adoption.ErrRequeueAfterAdoption, err, test.EquateErrors()); diff != "" {
		t.Errorf("Connect(...): -want error, +got error:\n%s", diff)
	}
	if diff := cmp.Diff(0, len(factory.CreateClientCalls)); diff != "" {
		t.Errorf("Connect(...) must not build the terraform client in the adopting reconcile: -want calls, +got calls:\n%s", diff)
	}
	if diff := cmp.Diff(guid, meta.GetExternalName(cr)); diff != "" {
		t.Errorf("Connect(...): -want external-name, +got external-name:\n%s", diff)
	}
}
