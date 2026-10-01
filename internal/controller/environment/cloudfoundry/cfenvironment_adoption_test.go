package cloudfoundry

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/cloudfoundry/fake"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

const cfID = "D75F70E0-E14B-4725-83E2-C36570FF6130"

// existingCF mirrors an environment as BTP reports it: the org name lives in
// the BTP labels, the environment name and landscape on the instance.
func existingCF(labels string) provisioningclient.BusinessEnvironmentInstanceResponseObject {
	return provisioningclient.BusinessEnvironmentInstanceResponseObject{
		Id:             internal.Ptr(cfID),
		Name:           internal.Ptr("jmt-dev-us2_cloudfoundry"),
		LandscapeLabel: internal.Ptr("cf-us20"),
		Labels:         internal.Ptr(labels),
	}
}

const orgLabels = `{"API Endpoint":"https://api.cf.us20.hana.ondemand.com","Org Name":"my-org","Org ID":"58e4278a-b156-4052-a1c4-d401395b71a3"}`

func adoptingCloudFoundry(annotation string, fp v1alpha1.CfEnvironmentParameters) *v1alpha1.CloudFoundryEnvironment {
	cr := &v1alpha1.CloudFoundryEnvironment{}
	cr.SetName("cr-name")
	cr.Spec.ForProvider = fp
	if annotation != "" {
		cr.SetAnnotations(map[string]string{adoption.Annotation: annotation})
	}
	return cr
}

func TestAdopt(t *testing.T) {
	errBoom := errors.New("boom")

	type find struct {
		instance provisioningclient.BusinessEnvironmentInstanceResponseObject
		found    bool
		err      error
	}
	type want struct {
		err          error
		externalName string
		lookups      int
	}

	cases := map[string]struct {
		reason string
		cr     *v1alpha1.CloudFoundryEnvironment
		find   find
		want   want
	}{
		"NotOptedIn": {
			reason: "Without the annotation, nothing is looked up.",
			cr:     adoptingCloudFoundry("", v1alpha1.CfEnvironmentParameters{}),
		},
		"NoEnvironmentCreates": {
			reason: "\"Import if it exists\": a subaccount without Cloud Foundry gets one created as usual.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{}),
			find:   find{found: false},
			want:   want{lookups: 1},
		},
		"SpecDeclaringNothingAdopts": {
			reason: "Every field is optional; a spec that declares no names states no intent to verify, and the subaccount's one environment is adopted.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{}),
			find:   find{instance: existingCF(orgLabels), found: true},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: cfID, lookups: 1},
		},
		"DeclaredFieldsMatchingAdopts": {
			reason: "When everything the spec declares matches the environment, it is adopted.",
			cr: adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{
				OrgName: "my-org", EnvironmentName: "jmt-dev-us2_cloudfoundry", Landscape: "cf-us20",
			}),
			find: find{instance: existingCF(orgLabels), found: true},
			want: want{err: adoption.ErrRequeueAfterAdoption, externalName: cfID, lookups: 1},
		},
		"OrgNameMismatchRefused": {
			reason: "The provider never updates a Cloud Foundry environment, so a declared org name that differs could never converge; adopting would leave the spec lying.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{OrgName: "other-org"}),
			find:   find{instance: existingCF(orgLabels), found: true},
			want: want{err: errIdentityMismatch(cfID, []string{
				`orgName: spec declares "other-org", environment has "my-org"`,
			}), lookups: 1},
		},
		"EnvironmentNameMismatchRefused": {
			reason: "A declared environment name that differs could never converge either.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{EnvironmentName: "other-env"}),
			find:   find{instance: existingCF(orgLabels), found: true},
			want: want{err: errIdentityMismatch(cfID, []string{
				`environmentName: spec declares "other-env", environment has "jmt-dev-us2_cloudfoundry"`,
			}), lookups: 1},
		},
		"LandscapeMismatchRefused": {
			reason: "An org cannot move between landscapes.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{Landscape: "cf-us20-001"}),
			find:   find{instance: existingCF(orgLabels), found: true},
			want: want{err: errIdentityMismatch(cfID, []string{
				`landscape: spec declares "cf-us20-001", environment has "cf-us20"`,
			}), lookups: 1},
		},
		"EveryMismatchIsReported": {
			reason: "All differences are reported at once, so the user fixes the spec in one go.",
			cr: adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{
				OrgName: "other-org", Landscape: "cf-us20-001",
			}),
			find: find{instance: existingCF(orgLabels), found: true},
			want: want{err: errIdentityMismatch(cfID, []string{
				`orgName: spec declares "other-org", environment has "my-org"`,
				`landscape: spec declares "cf-us20-001", environment has "cf-us20"`,
			}), lookups: 1},
		},
		"OrgNameUnreadableWhenDeclared": {
			reason: "A declared org name that cannot be read from the environment cannot be verified, so it is not adopted.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{OrgName: "my-org"}),
			find:   find{instance: existingCF(""), found: true},
			want:   want{err: errors.Wrap(errors.New("labels string is empty"), errAdoptOrgName), lookups: 1},
		},
		"OrgNameNotNeededWhenUndeclared": {
			reason: "Labels are only read to verify a declared org name; without one there is nothing to read.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{EnvironmentName: "jmt-dev-us2_cloudfoundry"}),
			find:   find{instance: existingCF(""), found: true},
			want:   want{err: adoption.ErrRequeueAfterAdoption, externalName: cfID, lookups: 1},
		},
		"LookupFails": {
			reason: "A failed lookup must fail the reconcile; reporting the environment missing would create a duplicate.",
			cr:     adoptingCloudFoundry("true", v1alpha1.CfEnvironmentParameters{}),
			find:   find{err: errBoom},
			want:   want{err: errors.Wrap(errBoom, errAdoptLookup), lookups: 1},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			lookups := 0
			client := fake.MockClient{
				MockFindInstance: func(v1alpha1.CloudFoundryEnvironment) (provisioningclient.BusinessEnvironmentInstanceResponseObject, bool, error) {
					lookups++
					return tc.find.instance, tc.find.found, tc.find.err
				},
			}
			e := external{client: client, kube: &test.MockClient{MockPatch: test.NewMockPatchFn(nil)}}

			err := e.adopt(context.Background(), tc.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nadopt(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.externalName, meta.GetExternalName(tc.cr)); diff != "" {
				t.Errorf("%s\nadopt(...): -want external-name, +got external-name:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.lookups, lookups); diff != "" {
				t.Errorf("%s\nadopt(...): -want lookups, +got lookups:\n%s", tc.reason, diff)
			}
		})
	}
}
