package entitlement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	"golang.org/x/sync/singleflight"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

func TestFilterEntitledServiceByName(t *testing.T) {

	type args struct {
		payload     *entclient.EntitledAndAssignedServicesResponseObject
		serviceName string
	}

	type want struct {
		o   *entclient.EntitledServicesResponseObject
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"find entitled service": {
			reason: "found by matching name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					EntitledServices: []entclient.EntitledServicesResponseObject{
						{
							Name: internal.Ptr("postgresql-db"),
						},
					},
				},
				serviceName: "postgresql-db",
			},
			want: want{
				o: &entclient.EntitledServicesResponseObject{
					Name: internal.Ptr("postgresql-db"),
				},
				err: nil,
			},
		},
		"unknown entitled service": {
			reason: "entitled service with not found",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					EntitledServices: []entclient.EntitledServicesResponseObject{
						{
							Name: internal.Ptr("postgresql-db"),
						},
					},
				},
				serviceName: "postgresql-db-never-existed",
			},
			want: want{
				err: errors.Errorf(errServiceNotFoundByName, "postgresql-db-never-existed"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				got, err := filterEntitledServiceByName(tc.args.payload, tc.args.serviceName)

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServiceByName(...): -want error, +got error:\n%s\n", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServiceByName(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			},
		)
	}

}

func TestFilterEntitledServicePlan(t *testing.T) {

	type args struct {
		payload entclient.EntitledServicesResponseObject
		key     ExternalNameKey
	}

	type want struct {
		o   *entclient.ServicePlanResponseObject
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"find service plan": {
			reason: "found by matching name",
			args: args{
				payload: entclient.EntitledServicesResponseObject{
					ServicePlans: []entclient.ServicePlanResponseObject{
						{
							Name: internal.Ptr("default"),
						},
					},
				},
				key: ExternalNameKey{ServicePlanName: "default"},
			},
			want: want{
				o: &entclient.ServicePlanResponseObject{
					Name: internal.Ptr("default"),
				},
				err: nil,
			},
		},
		"unknown service plan": {
			reason: "service plan with name not found",
			args: args{
				payload: entclient.EntitledServicesResponseObject{
					ServicePlans: []entclient.ServicePlanResponseObject{
						{
							Name: internal.Ptr("default"),
						},
					},
				},
				key: ExternalNameKey{ServicePlanName: "default-plan-never-existed"},
			},
			want: want{
				o:   nil,
				err: errors.Errorf(errServicePlanNotFoundByName, "default-plan-never-existed"),
			},
		},
		"qualifier mismatch": {
			reason: "plan name matches but the unique identifier does not, so the error names the qualifier",
			args: args{
				payload: entclient.EntitledServicesResponseObject{
					ServicePlans: []entclient.ServicePlanResponseObject{
						{
							Name:             internal.Ptr("hana"),
							UniqueIdentifier: internal.Ptr("hana-cloud-hana"),
						},
					},
				},
				key: ExternalNameKey{
					ServicePlanName:             "hana",
					ServicePlanUniqueIdentifier: internal.Ptr("hana-cloud-hana-sap_eu-de-1"),
				},
			},
			want: want{
				o: nil,
				err: errors.Errorf(errServicePlanNotFoundByQualifier,
					"hana", "hana-cloud-hana-sap_eu-de-1"),
			},
		},
		"qualifier selects among duplicate plan names": {
			reason: "duplicate plan names are disambiguated by the unique identifier",
			args: args{
				payload: entclient.EntitledServicesResponseObject{
					ServicePlans: []entclient.ServicePlanResponseObject{
						{
							Name:             internal.Ptr("hana"),
							UniqueIdentifier: internal.Ptr("hana-cloud-hana"),
						},
						{
							Name:             internal.Ptr("hana"),
							UniqueIdentifier: internal.Ptr("hana-cloud-hana-sap_eu-de-1"),
						},
					},
				},
				key: ExternalNameKey{
					ServicePlanName:             "hana",
					ServicePlanUniqueIdentifier: internal.Ptr("hana-cloud-hana-sap_eu-de-1"),
				},
			},
			want: want{
				o: &entclient.ServicePlanResponseObject{
					Name:             internal.Ptr("hana"),
					UniqueIdentifier: internal.Ptr("hana-cloud-hana-sap_eu-de-1"),
				},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				got, err := filterEntitledServicePlan(&tc.args.payload, tc.args.key)

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServicePlan(...): -want error, +got error:\n%s\n", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServicePlan(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			},
		)
	}
}

func TestFindAssignedServicePlan(t *testing.T) {
	type args struct {
		payload *entclient.EntitledAndAssignedServicesResponseObject
		key     ExternalNameKey
	}

	type want struct {
		o   *entclient.AssignedServicePlanSubaccountDTO
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"not found service": {
			reason: "could not match service name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("srv-1"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name: internal.Ptr("plan-A"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("0000-0000-0000-0000"),
										},
									},
								},
							},
						},
					},
				},
				key: ExternalNameKey{
					SubaccountGUID:  "0000-0000-0000-0000",
					ServiceName:     "srv-2",
					ServicePlanName: "plan-A",
				},
			},
			want: want{
				o:   nil,
				err: nil,
			},
		},
		"not found service plan": {
			reason: "could match name, but not plan name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("srv-1"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name: internal.Ptr("plan-A"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("0000-0000-0000-0000"),
										},
									},
								},
							},
						},
					},
				},
				key: ExternalNameKey{
					SubaccountGUID:  "0000-0000-0000-0000",
					ServiceName:     "srv-1",
					ServicePlanName: "plan-B",
				},
			},
			want: want{
				o:   nil,
				err: nil,
			},
		},
		"found service plan": {
			reason: "matching name and planname",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("srv-1"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name: internal.Ptr("plan-A"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("0000-0000-0000-0000"),
										},
									},
								},
							},
						},
					},
				},
				key: ExternalNameKey{
					SubaccountGUID:  "0000-0000-0000-0000",
					ServiceName:     "srv-1",
					ServicePlanName: "plan-A",
				},
			},
			want: want{
				o: &entclient.AssignedServicePlanSubaccountDTO{
					EntityId: internal.Ptr("0000-0000-0000-0000"),
				},
				err: nil,
			},
		},
		"not found ambiguous service plan": {
			reason: "matched name and planname, but not unique planname ",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("srv-1"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name:             internal.Ptr("plan-A"),
									UniqueIdentifier: internal.Ptr("plan-A-A"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("0000-0000-0000-0000"),
										},
									},
								},
							},
						},
					},
				},
				key: ExternalNameKey{
					SubaccountGUID:              "0000-0000-0000-0000",
					ServicePlanUniqueIdentifier: internal.Ptr("plan-A-B"),
					ServicePlanName:             "plan-A",
					ServiceName:                 "srv-1",
				},
			},
			want: want{
				o:   nil,
				err: nil,
			},
		},
		"found ambiguous service plan": {
			reason: "matched name, planname and given unique name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("srv-1"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name:             internal.Ptr("plan-A"),
									UniqueIdentifier: internal.Ptr("plan-A-A"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("0000-0000-0000-0000"),
										},
									},
								},
								{
									Name:             internal.Ptr("plan-A"),
									UniqueIdentifier: internal.Ptr("plan-A-B"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("1111-1111-1111-1111"),
										},
									},
								},
							},
						},
					},
				},
				key: ExternalNameKey{
					SubaccountGUID:              "1111-1111-1111-1111",
					ServicePlanUniqueIdentifier: internal.Ptr("plan-A-B"),
					ServicePlanName:             "plan-A",
					ServiceName:                 "srv-1",
				},
			},
			want: want{
				o: &entclient.AssignedServicePlanSubaccountDTO{
					EntityId: internal.Ptr("1111-1111-1111-1111"),
				},
				err: nil,
			},
		},
		"duplicate plan names for same subaccount, qualifier selects correct assignment": {
			reason: "two plans share both a name and a subaccount; only the qualifier disambiguates them",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{
							Name: internal.Ptr("hana-cloud"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name:             internal.Ptr("hana"),
									UniqueIdentifier: internal.Ptr("region-a"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("sub-1"),
											Amount:   internal.Ptr(float32(1)),
										},
									},
								},
								{
									Name:             internal.Ptr("hana"),
									UniqueIdentifier: internal.Ptr("region-b"),
									AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
										{
											EntityId: internal.Ptr("sub-1"),
											Amount:   internal.Ptr(float32(2)),
										},
									},
								},
							},
						},
					},
				},
				key: ExternalNameKey{
					SubaccountGUID:              "sub-1",
					ServiceName:                 "hana-cloud",
					ServicePlanName:             "hana",
					ServicePlanUniqueIdentifier: internal.Ptr("region-b"),
				},
			},
			want: want{
				o: &entclient.AssignedServicePlanSubaccountDTO{
					EntityId: internal.Ptr("sub-1"),
					Amount:   internal.Ptr(float32(2)),
				},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				entClient := EntitlementsClient{}
				got, err := entClient.findAssignedServicePlan(tc.args.payload, tc.args.key)

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.findAssignedServicePlan(...): -want error, +got error:\n%s\n", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.findAssignedServicePlan(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			},
		)
	}
}

func TestFilterEntitledServices(t *testing.T) {
	type args struct {
		payload *entclient.EntitledAndAssignedServicesResponseObject
		key     ExternalNameKey
	}

	type want struct {
		o   *entclient.ServicePlanResponseObject
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"find service plan": {
			reason: "found by matching name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					EntitledServices: []entclient.EntitledServicesResponseObject{
						{

							Name: internal.Ptr("postgresql-db"),
							ServicePlans: []entclient.ServicePlanResponseObject{
								{
									Name: internal.Ptr("default"),
								},
							},
						},
					},
				},
				key: ExternalNameKey{
					ServiceName:     "postgresql-db",
					ServicePlanName: "default",
				},
			},
			want: want{
				o: &entclient.ServicePlanResponseObject{
					Name: internal.Ptr("default"),
				},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				got, err := filterEntitledServices(tc.args.payload, tc.args.key)

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServices(...): -want error, +got error:\n%s\n", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServices(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			},
		)
	}
}

func TestDescribeInstanceQualifier(t *testing.T) {
	const (
		subaccountGUID = "duplicate-name-subaccount"
		serviceName    = "hana-cloud"
		servicePlan    = "hana"
		qualifierA     = "region-a"
		qualifierB     = "region-b"
	)

	// One shared response carries two "hana" plans distinguished only by their
	// unique identifier, mirroring the real-world duplicate-name scenario: both
	// the entitled and assigned side list "region-a" first and "region-b"
	// second, with a different Unlimited/Amount per variant.
	response := entclient.EntitledAndAssignedServicesResponseObject{
		EntitledServices: []entclient.EntitledServicesResponseObject{
			{
				Name: internal.Ptr(serviceName),
				ServicePlans: []entclient.ServicePlanResponseObject{
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierA),
						Unlimited:        internal.Ptr(false),
					},
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierB),
						Unlimited:        internal.Ptr(true),
					},
				},
			},
		},
		AssignedServices: []entclient.AssignedServiceResponseObject{
			{
				Name: internal.Ptr(serviceName),
				ServicePlans: []entclient.AssignedServicePlanResponseObject{
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierA),
						AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
							{
								EntityId: internal.Ptr(subaccountGUID),
								Amount:   internal.Ptr(float32(1)),
							},
						},
					},
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierB),
						AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
							{
								EntityId: internal.Ptr(subaccountGUID),
								Amount:   internal.Ptr(float32(2)),
							},
						},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encoding stub GetDirectoryAssignments response: %v", err)
		}
	}))
	defer server.Close()

	cfg := entclient.NewConfiguration()
	cfg.Servers = entclient.ServerConfigurations{{URL: server.URL}}
	api := entclient.NewAPIClient(cfg)
	c := EntitlementsClient{btp: btp.Client{EntitlementsServiceClient: api.ManageAssignedEntitlementsAPI}}

	cases := map[string]struct {
		qualifier     *string
		wantAmount    *float32
		wantUnlimited *bool
	}{
		"qualifier region-a selects the region-a variant": {
			qualifier:     internal.Ptr(qualifierA),
			wantAmount:    internal.Ptr(float32(1)),
			wantUnlimited: internal.Ptr(false),
		},
		"qualifier region-b selects the region-b variant": {
			qualifier:     internal.Ptr(qualifierB),
			wantAmount:    internal.Ptr(float32(2)),
			wantUnlimited: internal.Ptr(true),
		},
		"no qualifier keeps first-name-match behavior": {
			qualifier:     nil,
			wantAmount:    internal.Ptr(float32(1)),
			wantUnlimited: internal.Ptr(false),
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				key := ExternalNameKey{
					SubaccountGUID:              subaccountGUID,
					ServiceName:                 serviceName,
					ServicePlanName:             servicePlan,
					ServicePlanUniqueIdentifier: tc.qualifier,
				}

				got, err := c.DescribeInstance(context.Background(), key)
				if err != nil {
					t.Fatalf("DescribeInstance(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(tc.wantAmount, got.Assignment.Amount); diff != "" {
					t.Errorf("DescribeInstance(...).Assignment.Amount: -want, +got:\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantUnlimited, got.EntitledServicePlan.Unlimited); diff != "" {
					t.Errorf("DescribeInstance(...).EntitledServicePlan.Unlimited: -want, +got:\n%s", diff)
				}
			},
		)
	}
}

func TestUpdateInstanceUsesKey(t *testing.T) {
	cases := map[string]struct {
		key ExternalNameKey
	}{
		"qualifier present for four-segment key": {
			key: ExternalNameKey{
				SubaccountGUID:              "key-subaccount",
				ServiceName:                 "key-service",
				ServicePlanName:             "key-plan",
				ServicePlanUniqueIdentifier: internal.Ptr("key-qualifier"),
			},
		},
		"qualifier absent for three-segment key": {
			key: ExternalNameKey{
				SubaccountGUID:  "key-subaccount",
				ServiceName:     "key-service",
				ServicePlanName: "key-plan",
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var gotPayload entclient.SubaccountServicePlansRequestPayloadCollection
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
						t.Errorf("decoding SetServicePlans request body: %v", err)
					}
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				cfg := entclient.NewConfiguration()
				cfg.Servers = entclient.ServerConfigurations{{URL: server.URL}}
				api := entclient.NewAPIClient(cfg)
				c := EntitlementsClient{btp: btp.Client{EntitlementsServiceClient: api.ManageAssignedEntitlementsAPI}}

				// cr's spec identity and quota deliberately disagree with key and
				// Status.AtProvider.Required, proving UpdateInstance sources identity
				// from key and writes the aggregate Required, never cr.Spec.ForProvider.
				cr := &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:              "cr-subaccount-should-be-ignored",
							ServiceName:                 "cr-service-should-be-ignored",
							ServicePlanName:             "cr-plan-should-be-ignored",
							ServicePlanUniqueIdentifier: internal.Ptr("cr-qualifier-should-be-ignored"),
							Amount:                      internal.Ptr(99),
							Enable:                      internal.Ptr(false),
						},
					},
					Status: v1alpha1.EntitlementStatus{
						AtProvider: &v1alpha1.EntitlementObservation{
							Required: &v1alpha1.EntitlementSummary{
								Amount: internal.Ptr(5),
								Enable: internal.Ptr(true),
							},
						},
					},
				}

				if err := c.UpdateInstance(context.Background(), tc.key, cr); err != nil {
					t.Fatalf("UpdateInstance(...): unexpected error: %v", err)
				}

				if len(gotPayload.SubaccountServicePlans) != 1 {
					t.Fatalf("SetServicePlans payload: want 1 assignment, got %d", len(gotPayload.SubaccountServicePlans))
				}
				plan := gotPayload.SubaccountServicePlans[0]

				if diff := cmp.Diff(tc.key.ServiceName, plan.ServiceName); diff != "" {
					t.Errorf("SetServicePlans payload ServiceName: -want, +got:\n%s", diff)
				}
				if diff := cmp.Diff(tc.key.ServicePlanName, plan.ServicePlanName); diff != "" {
					t.Errorf("SetServicePlans payload ServicePlanName: -want, +got:\n%s", diff)
				}
				if diff := cmp.Diff(tc.key.ServicePlanUniqueIdentifier, plan.ServicePlanUniqueIdentifier); diff != "" {
					t.Errorf("SetServicePlans payload ServicePlanUniqueIdentifier: -want, +got:\n%s", diff)
				}
				if len(plan.AssignmentInfo) != 1 {
					t.Fatalf("SetServicePlans payload AssignmentInfo: want 1 entry, got %d", len(plan.AssignmentInfo))
				}
				if diff := cmp.Diff(tc.key.SubaccountGUID, plan.AssignmentInfo[0].SubaccountGUID); diff != "" {
					t.Errorf("SetServicePlans payload SubaccountGUID: -want, +got:\n%s", diff)
				}
				if diff := cmp.Diff(internal.Ptr(float32(5)), plan.AssignmentInfo[0].Amount); diff != "" {
					t.Errorf("SetServicePlans payload Amount: -want, +got:\n%s\n(must be the aggregate status.atProvider.required.amount (5), never cr.Spec.ForProvider.Amount (99))", diff)
				}
				if diff := cmp.Diff(internal.Ptr(true), plan.AssignmentInfo[0].Enable); diff != "" {
					t.Errorf("SetServicePlans payload Enable: -want, +got:\n%s\n(must be the aggregate status.atProvider.required.enable (true), never cr.Spec.ForProvider.Enable (false))", diff)
				}
			},
		)
	}
}

// TestDeleteSkipsAutoAssigned verifies DeleteInstance never calls
// SetServicePlans when Assigned.AutoAssigned is true: BTP documents such
// assignments as unremovable by admin action. AutoAssign (user intent) is
// unaffected; see TestUpdateInstanceUsesKey for that write path.
func TestDeleteSkipsAutoAssigned(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("DeleteInstance issued %s %s for an AutoAssigned entitlement; BTP documents this assignment as unremovable", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	c, closeServer := newTestEntitlementsClient(t, handler)
	defer closeServer()

	cr := &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Amount: internal.Ptr(5),
			},
		},
		Status: v1alpha1.EntitlementStatus{
			AtProvider: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Amount: internal.Ptr(0),
				},
				Assigned: &v1alpha1.Assignable{
					Amount:       internal.Ptr(5),
					AutoAssigned: true,
				},
			},
		},
	}
	key := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if err := c.DeleteInstance(context.Background(), key, cr); err != nil {
		t.Fatalf("DeleteInstance(...): unexpected error: %v", err)
	}
}

// TestDeleteMissingAssignmentOK verifies DeleteInstance treats a nil
// Status.AtProvider.Assigned as success: the desired end state (no
// assignment) is already reached, whether a sibling's Delete removed it
// first or it never existed.
func TestDeleteMissingAssignmentOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("DeleteInstance issued %s %s for an entitlement with no external assignment; an absent assignment is already the desired end state", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	c, closeServer := newTestEntitlementsClient(t, handler)
	defer closeServer()

	cr := &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Amount: internal.Ptr(5),
			},
		},
		Status: v1alpha1.EntitlementStatus{
			AtProvider: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Amount: internal.Ptr(5),
				},
				Assigned: nil,
			},
		},
	}
	key := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if err := c.DeleteInstance(context.Background(), key, cr); err != nil {
		t.Fatalf("DeleteInstance(...): unexpected error: %v", err)
	}
}

// TestDeleteInstanceSoleNumericSendsZero verifies DeleteInstance fills in
// an explicit zero amount when Required.Amount is nil (the shape
// MergeRelatedEntitlements produces with no siblings left), releasing the
// full BTP quota instead of a computed sibling sum.
func TestDeleteInstanceSoleNumericSendsZero(t *testing.T) {
	var gotPayload entclient.SubaccountServicePlansRequestPayloadCollection
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Errorf("decoding SetServicePlans request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})
	c, closeServer := newTestEntitlementsClient(t, handler)
	defer closeServer()

	cr := &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Amount: internal.Ptr(5),
			},
		},
		Status: v1alpha1.EntitlementStatus{
			AtProvider: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Amount: nil,
				},
				Assigned: &v1alpha1.Assignable{
					Amount: internal.Ptr(5),
				},
			},
		},
	}
	key := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if err := c.DeleteInstance(context.Background(), key, cr); err != nil {
		t.Fatalf("DeleteInstance(...): unexpected error: %v", err)
	}

	if len(gotPayload.SubaccountServicePlans) != 1 {
		t.Fatalf("SetServicePlans payload: want 1 assignment, got %d", len(gotPayload.SubaccountServicePlans))
	}
	assignmentInfo := gotPayload.SubaccountServicePlans[0].AssignmentInfo
	if len(assignmentInfo) != 1 {
		t.Fatalf("SetServicePlans payload AssignmentInfo: want 1 entry, got %d", len(assignmentInfo))
	}
	if assignmentInfo[0].Amount == nil {
		t.Fatalf("SetServicePlans payload Amount: want 0, got nil")
	}
	if diff := cmp.Diff(float32(0), *assignmentInfo[0].Amount); diff != "" {
		t.Errorf("SetServicePlans payload Amount: -want, +got:\n%s", diff)
	}
}

// TestDeleteEnableMultiSiblingNoOp verifies DeleteInstance is a no-op for
// an enable-based entitlement when more than one sibling CR still shares
// the plan: nothing needs reducing while another sibling still needs it
// enabled.
func TestDeleteEnableMultiSiblingNoOp(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("DeleteInstance issued %s %s for an enable-based entitlement with remaining siblings; nothing should be written", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	c, closeServer := newTestEntitlementsClient(t, handler)
	defer closeServer()

	cr := &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Enable: internal.Ptr(true),
			},
		},
		Status: v1alpha1.EntitlementStatus{
			AtProvider: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Enable:            internal.Ptr(true),
					EntitlementsCount: internal.Ptr(2),
				},
				Assigned: &v1alpha1.Assignable{
					EntityState: "OK",
				},
			},
		},
	}
	key := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if err := c.DeleteInstance(context.Background(), key, cr); err != nil {
		t.Fatalf("DeleteInstance(...): unexpected error: %v", err)
	}
}

// TestDeleteAutoAssignSendsReduction verifies DeleteInstance still
// reduces the assignment when AutoAssign (user intent) is true but
// AutoAssigned (system-assigned) is false; the guard only checks
// AutoAssigned. Contrast TestDeleteSkipsAutoAssigned.
func TestDeleteAutoAssignSendsReduction(t *testing.T) {
	var gotPayload entclient.SubaccountServicePlansRequestPayloadCollection
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Errorf("decoding SetServicePlans request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})
	c, closeServer := newTestEntitlementsClient(t, handler)
	defer closeServer()

	cr := &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Amount: internal.Ptr(5),
			},
		},
		Status: v1alpha1.EntitlementStatus{
			AtProvider: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Amount: internal.Ptr(3),
				},
				Assigned: &v1alpha1.Assignable{
					AutoAssign:   true,
					AutoAssigned: false,
				},
			},
		},
	}
	key := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if err := c.DeleteInstance(context.Background(), key, cr); err != nil {
		t.Fatalf("DeleteInstance(...): unexpected error: %v", err)
	}

	if len(gotPayload.SubaccountServicePlans) != 1 {
		t.Fatalf("SetServicePlans payload: want 1 assignment, got %d", len(gotPayload.SubaccountServicePlans))
	}
	assignmentInfo := gotPayload.SubaccountServicePlans[0].AssignmentInfo
	if len(assignmentInfo) != 1 {
		t.Fatalf("SetServicePlans payload AssignmentInfo: want 1 entry, got %d", len(assignmentInfo))
	}
	if assignmentInfo[0].Amount == nil {
		t.Fatalf("SetServicePlans payload Amount: want 3, got nil")
	}
	if diff := cmp.Diff(float32(3), *assignmentInfo[0].Amount); diff != "" {
		t.Errorf("SetServicePlans payload Amount: -want, +got:\n%s", diff)
	}
}

// TestDeleteQualifierPayloadShape verifies that the qualifier chosen by
// DescribeInstance's plan selection also decides DeleteInstance's payload
// shape (amount-zero vs enable:false), driving both duplicate-qualifier
// plan variants through real selection into a real deletion payload.
func TestDeleteQualifierPayloadShape(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	const (
		subaccountGUID     = "delete-composition-subaccount"
		serviceName        = "widget-service"
		servicePlan        = "widget"
		qualifierNumeric   = "region-numeric"
		qualifierUnlimited = "region-unlimited"
	)

	// One shared response carries two "widget" plans distinguished only by
	// their unique identifier: region-numeric is a numeric plan
	// (Unlimited=false), region-unlimited is unlimited (Unlimited=true).
	// Each side assigns exactly this subaccount, so DeleteInstance's
	// Assigned-nil early-return never fires.
	response := entclient.EntitledAndAssignedServicesResponseObject{
		EntitledServices: []entclient.EntitledServicesResponseObject{
			{
				Name: internal.Ptr(serviceName),
				ServicePlans: []entclient.ServicePlanResponseObject{
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierNumeric),
						Unlimited:        internal.Ptr(false),
					},
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierUnlimited),
						Unlimited:        internal.Ptr(true),
					},
				},
			},
		},
		AssignedServices: []entclient.AssignedServiceResponseObject{
			{
				Name: internal.Ptr(serviceName),
				ServicePlans: []entclient.AssignedServicePlanResponseObject{
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierNumeric),
						AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
							{EntityId: internal.Ptr(subaccountGUID), Amount: internal.Ptr(float32(5))},
						},
					},
					{
						Name:             internal.Ptr(servicePlan),
						UniqueIdentifier: internal.Ptr(qualifierUnlimited),
						AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
							{EntityId: internal.Ptr(subaccountGUID)},
						},
					},
				},
			},
		},
	}

	cases := map[string]struct {
		qualifier  string
		wantAmount *float32
		wantEnable *bool
	}{
		"numeric variant sends amount-zero, never enable:false": {
			qualifier:  qualifierNumeric,
			wantAmount: internal.Ptr(float32(0)),
			wantEnable: nil,
		},
		"unlimited variant sends enable:false, never a leaked amount": {
			qualifier:  qualifierUnlimited,
			wantAmount: nil,
			wantEnable: internal.Ptr(false),
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				var gotPayload entclient.SubaccountServicePlansRequestPayloadCollection
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.Method {
					case http.MethodGet:
						w.Header().Set("Content-Type", "application/json")
						if err := json.NewEncoder(w).Encode(response); err != nil {
							t.Errorf("encoding stub GetDirectoryAssignments response: %v", err)
						}
					case http.MethodPut:
						if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
							t.Errorf("decoding SetServicePlans request body: %v", err)
						}
						w.WriteHeader(http.StatusOK)
					default:
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					}
				}))
				defer server.Close()

				cfg := entclient.NewConfiguration()
				cfg.HTTPClient = server.Client()
				cfg.Servers = entclient.ServerConfigurations{{URL: server.URL}}
				api := entclient.NewAPIClient(cfg)
				c := NewEntitlementsClient(btp.Client{EntitlementsServiceClient: api.ManageAssignedEntitlementsAPI})

				key := ExternalNameKey{
					SubaccountGUID:              subaccountGUID,
					ServiceName:                 serviceName,
					ServicePlanName:             servicePlan,
					ServicePlanUniqueIdentifier: internal.Ptr(tc.qualifier),
				}

				// Selection: DescribeInstance picks the plan variant
				// matching this key's qualifier, exactly as the
				// controller's Observe does immediately before a real
				// Delete.
				instance, err := c.DescribeInstance(context.Background(), key)
				if err != nil {
					t.Fatalf("DescribeInstance(...): unexpected error: %v", err)
				}

				// No sibling CRs remain for this service/plan/qualifier,
				// the shape MergeRelatedEntitlements produces once the
				// deleted CR itself is excluded -- Required.Amount and
				// .Enable both come back nil, EntitlementsCount 0.
				observation, err := GenerateObservation(instance, &v1alpha1.EntitlementList{})
				if err != nil {
					t.Fatalf("GenerateObservation(...): unexpected error: %v", err)
				}

				cr := &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							Amount: internal.Ptr(5),
						},
					},
					Status: v1alpha1.EntitlementStatus{AtProvider: observation},
				}

				// Deletion: instance.EntitledServicePlan.Unlimited from
				// the selection above is what hasNumericQuota reads to
				// choose the payload shape asserted below.
				if err := c.DeleteInstance(context.Background(), key, cr); err != nil {
					t.Fatalf("DeleteInstance(...): unexpected error: %v", err)
				}

				if len(gotPayload.SubaccountServicePlans) != 1 {
					t.Fatalf("SetServicePlans payload: want 1 assignment, got %d", len(gotPayload.SubaccountServicePlans))
				}
				assignmentInfo := gotPayload.SubaccountServicePlans[0].AssignmentInfo
				if len(assignmentInfo) != 1 {
					t.Fatalf("SetServicePlans payload AssignmentInfo: want 1 entry, got %d", len(assignmentInfo))
				}
				if diff := cmp.Diff(tc.wantAmount, assignmentInfo[0].Amount); diff != "" {
					t.Errorf("SetServicePlans payload Amount: -want, +got:\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantEnable, assignmentInfo[0].Enable); diff != "" {
					t.Errorf("SetServicePlans payload Enable: -want, +got:\n%s", diff)
				}
			},
		)
	}
}

// TestCreateSkipsAutoAssigned verifies CreateInstance (== UpdateInstance)
// never calls SetServicePlans for an AutoAssigned entitlement, reachable
// via needsCreate's PROCESSING_FAILED retry path without this guard.
func TestCreateSkipsAutoAssigned(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("CreateInstance issued %s %s for an AutoAssigned entitlement; BTP documents this assignment as unremovable", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	c, closeServer := newTestEntitlementsClient(t, handler)
	defer closeServer()

	cr := &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Amount: internal.Ptr(5),
			},
		},
		Status: v1alpha1.EntitlementStatus{
			AtProvider: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Amount: internal.Ptr(5),
				},
				Assigned: &v1alpha1.Assignable{
					AutoAssigned: true,
				},
			},
		},
	}
	key := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if err := c.CreateInstance(context.Background(), key, cr); err != nil {
		t.Fatalf("CreateInstance(...): unexpected error: %v", err)
	}
}

// TestCreateWritesWhenAutoAssignOnly verifies CreateInstance still writes
// when AutoAssign (user intent) is true but AutoAssigned (system-assigned)
// is false: the guard must not suppress writes driven by AutoAssign.
func TestCreateWritesWhenAutoAssignOnly(t *testing.T) {
	requestSeen := make(chan struct{}, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Non-blocking: a duplicate write must never block this handler
		// goroutine, which would hang the test instead of failing with a diff.
		select {
		case requestSeen <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	})
	c, closeServer := newTestEntitlementsClient(t, handler)
	defer closeServer()

	cr := &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Amount: internal.Ptr(5),
			},
		},
		Status: v1alpha1.EntitlementStatus{
			AtProvider: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Amount: internal.Ptr(5),
				},
				Assigned: &v1alpha1.Assignable{
					AutoAssign:   true,
					AutoAssigned: false,
				},
			},
		},
	}
	key := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if err := c.CreateInstance(context.Background(), key, cr); err != nil {
		t.Fatalf("CreateInstance(...): unexpected error: %v", err)
	}

	select {
	case <-requestSeen:
	case <-time.After(2 * time.Second):
		t.Fatalf("CreateInstance(...): expected a SetServicePlans request for an AutoAssign (not AutoAssigned) entitlement, got none")
	}
}

// newTestEntitlementsClient wires an EntitlementsClient to an httptest
// server running handler, returning the client and a func to close the
// server.
func newTestEntitlementsClient(t *testing.T, handler http.Handler) (*EntitlementsClient, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	cfg := entclient.NewConfiguration()
	cfg.HTTPClient = server.Client()
	cfg.Servers = []entclient.ServerConfiguration{{URL: server.URL}}
	api := entclient.NewAPIClient(cfg)
	client := NewEntitlementsClient(btp.Client{
		EntitlementsServiceClient: api.ManageAssignedEntitlementsAPI,
	})
	return client, server.Close
}

func resetDescribeState() {
	describeCache.Range(func(key, _ any) bool {
		describeCache.Delete(key)
		return true
	})
	describeGroup = singleflight.Group{}
}

// describeStub is an http.Handler stub for GetDirectoryAssignments. It
// counts requests in arrival order, reports each arrival on arrived
// before gating, and serves respFn's response for the nth request; the
// first gate requests block until releaseAll is called.
type describeStub struct {
	t      *testing.T
	respFn func(n int) entclient.EntitledAndAssignedServicesResponseObject
	gate   int

	arrived     chan int
	release     chan struct{}
	releaseOnce sync.Once

	mu    sync.Mutex
	count int
}

func newDescribeStub(t *testing.T, respFn func(n int) entclient.EntitledAndAssignedServicesResponseObject) *describeStub {
	t.Helper()
	return &describeStub{
		t:       t,
		respFn:  respFn,
		arrived: make(chan int, 8),
		release: make(chan struct{}),
	}
}

func (s *describeStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.count++
	n := s.count
	s.mu.Unlock()

	s.arrived <- n
	if n <= s.gate {
		<-s.release
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.respFn(n)); err != nil {
		s.t.Errorf("encoding stub GetDirectoryAssignments response for request %d: %v", n, err)
	}
}

// releaseAll unblocks every gated request, current or future. It is
// idempotent (sync.Once), so tests can call it both to trigger the
// release under test and defensively via t.Cleanup on every exit path.
func (s *describeStub) releaseAll() {
	s.releaseOnce.Do(func() { close(s.release) })
}

func (s *describeStub) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// waitArrived blocks until request number want reaches the server, failing
// the test if that doesn't happen within one second.
func (s *describeStub) waitArrived(t *testing.T, want int) {
	t.Helper()
	select {
	case n := <-s.arrived:
		if n != want {
			t.Fatalf("request arrival order: want request %d next, got %d", want, n)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out after 1s waiting for request %d to reach the server", want)
	}
}

func emptyAssignmentsResponse(int) entclient.EntitledAndAssignedServicesResponseObject {
	return entclient.EntitledAndAssignedServicesResponseObject{}
}

// waitForFlights runs before resetDescribeState's t.Cleanup (LIFO order)
// and waits up to a second for every tracked goroutine to leave
// fetchAssignments, so a t.Fatalf-triggered runtime.Goexit can't leave one
// racing resetDescribeState's describeGroup overwrite under -race.
func waitForFlights(t *testing.T, stub *describeStub, wg *sync.WaitGroup) {
	t.Helper()
	t.Cleanup(func() {
		stub.releaseAll()
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	})
}

// TestDescribeFreshBypassesCache primes the ordinary TTL cache, changes
// what the server would return, and asserts a fresh read triggers a
// second HTTP request and returns the new response, not the cached one.
func TestDescribeFreshBypassesCache(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	stub := newDescribeStub(t, func(n int) entclient.EntitledAndAssignedServicesResponseObject {
		// The second response is distinguishable from the first so a fresh
		// read that wrongly served the cached first response is detectable
		// by content, not just by request count.
		return entclient.EntitledAndAssignedServicesResponseObject{
			FetchErrorFromExternalProviderRegistry: internal.Ptr(n > 1),
		}
	})
	client, closeServer := newTestEntitlementsClient(t, stub)
	t.Cleanup(closeServer)

	key := ExternalNameKey{
		SubaccountGUID:  "fresh-bypass-subaccount",
		ServiceName:     "fresh-bypass-service",
		ServicePlanName: "fresh-bypass-plan",
	}
	ctx := context.Background()

	primed, err := client.fetchAssignments(ctx, key, false)
	if err != nil {
		t.Fatalf("priming fetchAssignments(...): unexpected error: %v", err)
	}
	if got := stub.requestCount(); got != 1 {
		t.Fatalf("after priming: want 1 HTTP request, got %d", got)
	}
	if internal.Val(primed.FetchErrorFromExternalProviderRegistry) {
		t.Fatalf("primed response: want the first (unflagged) response, got the second")
	}

	// Sanity check: an ordinary read is served from cache, not a second
	// request, so the fresh assertions below actually exercise a cache that
	// is populated.
	if cached, err := client.fetchAssignments(ctx, key, false); err != nil {
		t.Fatalf("ordinary fetchAssignments(...) after priming: unexpected error: %v", err)
	} else if got := stub.requestCount(); got != 1 {
		t.Fatalf("ordinary read after priming: want a cache hit (1 HTTP request total), got %d", got)
	} else if cached != primed {
		t.Fatalf("ordinary read after priming: want the cached response, got a different object")
	}

	fresh, err := client.fetchAssignments(ctx, key, true)
	if err != nil {
		t.Fatalf("fetchAssignments(..., fresh=true): unexpected error: %v", err)
	}

	if got := stub.requestCount(); got != 2 {
		t.Fatalf("fresh read: want a second HTTP request (2 total), got %d", got)
	}
	if !internal.Val(fresh.FetchErrorFromExternalProviderRegistry) {
		t.Fatalf("fresh read: want the second (flagged) response, got the stale cached one")
	}
}

// TestFreshReadDoesNotJoinOrdinaryFlight blocks an in-flight ordinary
// request, then starts a fresh request for the same key and asserts it
// reaches the server before the ordinary request is released. It fails if
// the "fresh|" flight-key prefix is dropped, because the fresh call would
// then join the blocked ordinary call instead of issuing its own request.
func TestFreshReadDoesNotJoinOrdinaryFlight(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	stub := newDescribeStub(t, emptyAssignmentsResponse)
	stub.gate = 1 // hold the first (ordinary) request open until released
	client, closeServer := newTestEntitlementsClient(t, stub)
	t.Cleanup(closeServer)

	var wg sync.WaitGroup
	waitForFlights(t, stub, &wg)

	key := ExternalNameKey{
		SubaccountGUID:  "fresh-isolation-subaccount",
		ServiceName:     "fresh-isolation-service",
		ServicePlanName: "fresh-isolation-plan",
	}
	ctx := context.Background()

	ordinaryErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.fetchAssignments(ctx, key, false)
		ordinaryErr <- err
	}()
	stub.waitArrived(t, 1) // ordinary request reached the server and is now blocked

	freshErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.fetchAssignments(ctx, key, true)
		freshErr <- err
	}()

	// A fresh read on its own flight key must reach the server without
	// waiting for the blocked ordinary request to be released. If it instead
	// joined the ordinary key, this would time out because request 2 would
	// never be sent.
	stub.waitArrived(t, 2)

	stub.releaseAll() // release the blocked ordinary request

	select {
	case err := <-ordinaryErr:
		if err != nil {
			t.Fatalf("ordinary fetchAssignments(...): unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out after 1s waiting for the ordinary call to return")
	}
	select {
	case err := <-freshErr:
		if err != nil {
			t.Fatalf("fresh fetchAssignments(...): unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out after 1s waiting for the fresh call to return")
	}

	if got := stub.requestCount(); got != 2 {
		t.Fatalf("want 2 HTTP requests (ordinary + fresh, no coalescing), got %d", got)
	}
}

// TestConcurrentFreshReadsCoalesce blocks the first of two concurrent fresh
// requests for the same key and asserts the second never talks to the
// server (they coalesce via singleflight), then releases and asserts both
// callers return successfully.
func TestConcurrentFreshReadsCoalesce(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	stub := newDescribeStub(t, emptyAssignmentsResponse)
	stub.gate = 1 // hold the first fresh request open until released
	client, closeServer := newTestEntitlementsClient(t, stub)
	t.Cleanup(closeServer)

	var wg sync.WaitGroup
	waitForFlights(t, stub, &wg)

	key := ExternalNameKey{
		SubaccountGUID:  "fresh-coalesce-subaccount",
		ServiceName:     "fresh-coalesce-service",
		ServicePlanName: "fresh-coalesce-plan",
	}
	ctx := context.Background()

	firstErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.fetchAssignments(ctx, key, true)
		firstErr <- err
	}()
	stub.waitArrived(t, 1) // first fresh request reached the server and is now blocked

	secondErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.fetchAssignments(ctx, key, true)
		secondErr <- err
	}()

	// A wrongly-uncoalesced duplicate gets a bounded window to prove itself
	// before the first request releases; the request-count assertion below
	// is what actually proves coalescing.
	select {
	case n := <-stub.arrived:
		t.Fatalf("unexpected HTTP request #%d while the first fresh request was still blocked; concurrent fresh reads must coalesce", n)
	case <-time.After(100 * time.Millisecond):
	}

	stub.releaseAll()

	select {
	case err := <-firstErr:
		if err != nil {
			t.Fatalf("first fresh fetchAssignments(...): unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out after 1s waiting for the first fresh call to return")
	}
	select {
	case err := <-secondErr:
		if err != nil {
			t.Fatalf("second fresh fetchAssignments(...): unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out after 1s waiting for the second fresh call to return")
	}

	if got := stub.requestCount(); got != 1 {
		t.Fatalf("want concurrent fresh reads to coalesce into 1 HTTP request, got %d", got)
	}
}

// TestFreshReadPopulatesOrdinaryCache calls a fresh read once and then an
// ordinary read for the same key, asserting only one HTTP request is made
// in total: the fresh read's response must populate the ordinary cache.
func TestFreshReadPopulatesOrdinaryCache(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	stub := newDescribeStub(t, emptyAssignmentsResponse)
	client, closeServer := newTestEntitlementsClient(t, stub)
	t.Cleanup(closeServer)

	key := ExternalNameKey{
		SubaccountGUID:  "fresh-populate-subaccount",
		ServiceName:     "fresh-populate-service",
		ServicePlanName: "fresh-populate-plan",
	}
	ctx := context.Background()

	if _, err := client.fetchAssignments(ctx, key, true); err != nil {
		t.Fatalf("fetchAssignments(..., fresh=true): unexpected error: %v", err)
	}
	if got := stub.requestCount(); got != 1 {
		t.Fatalf("after fresh read: want 1 HTTP request, got %d", got)
	}

	if _, err := client.fetchAssignments(ctx, key, false); err != nil {
		t.Fatalf("fetchAssignments(..., fresh=false) after fresh read: unexpected error: %v", err)
	}
	if got := stub.requestCount(); got != 1 {
		t.Fatalf("ordinary read after fresh read: want a cache hit (1 HTTP request total), got %d", got)
	}

	if cached := describeCacheGet(key.CacheKey()); cached == nil {
		t.Fatalf("describeCacheGet(%q): want an entry populated by the fresh read, got nil", key.CacheKey())
	}
}

// TestDescribeFreshCallsThroughToBTP drives the exported
// DescribeInstance/DescribeInstanceFresh methods (other fresh-read tests
// call fetchAssignments directly), verifying DescribeInstanceFresh still
// bypasses the cache and returns an *Instance shaped like DescribeInstance's.
func TestDescribeFreshCallsThroughToBTP(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	const (
		subaccountGUID = "fresh-exported-subaccount"
		serviceName    = "fresh-exported-service"
		servicePlan    = "fresh-exported-plan"
	)

	// A minimal payload with one entitled plan and one matching assignment,
	// modeled on TestDescribeInstanceQualifier's fixture, so both
	// DescribeInstance and DescribeInstanceFresh can return a non-error
	// *Instance for it.
	response := entclient.EntitledAndAssignedServicesResponseObject{
		EntitledServices: []entclient.EntitledServicesResponseObject{
			{
				Name: internal.Ptr(serviceName),
				ServicePlans: []entclient.ServicePlanResponseObject{
					{
						Name:      internal.Ptr(servicePlan),
						Unlimited: internal.Ptr(false),
					},
				},
			},
		},
		AssignedServices: []entclient.AssignedServiceResponseObject{
			{
				Name: internal.Ptr(serviceName),
				ServicePlans: []entclient.AssignedServicePlanResponseObject{
					{
						Name: internal.Ptr(servicePlan),
						AssignmentInfo: []entclient.AssignedServicePlanSubaccountDTO{
							{
								EntityId: internal.Ptr(subaccountGUID),
								Amount:   internal.Ptr(float32(3)),
							},
						},
					},
				},
			},
		},
	}

	stub := newDescribeStub(t, func(int) entclient.EntitledAndAssignedServicesResponseObject {
		return response
	})
	client, closeServer := newTestEntitlementsClient(t, stub)
	t.Cleanup(closeServer)

	key := ExternalNameKey{
		SubaccountGUID:  subaccountGUID,
		ServiceName:     serviceName,
		ServicePlanName: servicePlan,
	}
	ctx := context.Background()

	ordinary, err := client.DescribeInstance(ctx, key)
	if err != nil {
		t.Fatalf("DescribeInstance(...): unexpected error: %v", err)
	}
	if got := stub.requestCount(); got != 1 {
		t.Fatalf("after DescribeInstance: want 1 HTTP request, got %d", got)
	}

	fresh, err := client.DescribeInstanceFresh(ctx, key)
	if err != nil {
		t.Fatalf("DescribeInstanceFresh(...): unexpected error: %v", err)
	}
	if got := stub.requestCount(); got != 2 {
		t.Fatalf("DescribeInstanceFresh(...): want a second HTTP request (2 total) bypassing the cache DescribeInstance populated, got %d", got)
	}

	if diff := cmp.Diff(ordinary, fresh); diff != "" {
		t.Errorf("DescribeInstanceFresh(...) Instance shape: -DescribeInstance, +DescribeInstanceFresh:\n%s", diff)
	}
}

// TestOlderWriteNeverClobbersFreshWrite blocks an ordinary request after
// it reaches the server, lets a fresh request for the same key complete
// and cache first, then releases the ordinary request and asserts its
// now-stale response can't overwrite the fresh entry - the regression
// describeCacheStore's recency guard fixes, since ordinary and fresh
// flights use distinct singleflight keys and can finish in either order.
func TestOlderWriteNeverClobbersFreshWrite(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	stub := newDescribeStub(t, func(n int) entclient.EntitledAndAssignedServicesResponseObject {
		// Request 2 (the fresh read) is distinguishable from request 1 (the
		// gated ordinary read) by content, not just by count, so a wrongly
		// clobbered cache entry is detectable regardless of which request
		// stored last.
		return entclient.EntitledAndAssignedServicesResponseObject{
			FetchErrorFromExternalProviderRegistry: internal.Ptr(n == 2),
		}
	})
	stub.gate = 1 // hold the first (ordinary) request open until released
	client, closeServer := newTestEntitlementsClient(t, stub)
	t.Cleanup(closeServer)

	var wg sync.WaitGroup
	waitForFlights(t, stub, &wg)

	key := ExternalNameKey{
		SubaccountGUID:  "recency-guard-subaccount",
		ServiceName:     "recency-guard-service",
		ServicePlanName: "recency-guard-plan",
	}
	ctx := context.Background()

	ordinaryErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.fetchAssignments(ctx, key, false)
		ordinaryErr <- err
	}()
	stub.waitArrived(t, 1) // ordinary request reached the server and is now blocked

	// The fresh read is issued strictly after the ordinary request (the
	// server only sees it once the ordinary request already occupies the
	// server) but, because it is not gated, completes and stores before the
	// ordinary request is released below.
	fresh, err := client.fetchAssignments(ctx, key, true)
	if err != nil {
		t.Fatalf("fetchAssignments(..., fresh=true): unexpected error: %v", err)
	}
	if !internal.Val(fresh.FetchErrorFromExternalProviderRegistry) {
		t.Fatalf("fresh read: want the second (flagged) response, got the first")
	}
	if got := stub.requestCount(); got != 2 {
		t.Fatalf("after fresh read: want 2 HTTP requests, got %d", got)
	}
	if cached := describeCacheGet(key.CacheKey()); cached == nil || !internal.Val(cached.FetchErrorFromExternalProviderRegistry) {
		t.Fatalf("describeCacheGet(%q) after fresh read: want the fresh (flagged) entry, got %+v", key.CacheKey(), cached)
	}

	stub.releaseAll() // release the blocked ordinary request

	select {
	case err := <-ordinaryErr:
		if err != nil {
			t.Fatalf("ordinary fetchAssignments(...): unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out after 1s waiting for the ordinary call to return")
	}

	// The ordinary request's response was issued before the fresh
	// request's, so even though it is stored after the fresh entry is
	// already cached, it must not overwrite it.
	cached := describeCacheGet(key.CacheKey())
	if cached == nil {
		t.Fatalf("describeCacheGet(%q) after ordinary release: want an entry, got nil", key.CacheKey())
	}
	if !internal.Val(cached.FetchErrorFromExternalProviderRegistry) {
		t.Fatalf("describeCacheGet(%q) after ordinary release: want the fresh (flagged) entry to survive, got the older ordinary response", key.CacheKey())
	}
}

// TestDescribeCacheGetExpiresEntries pins describeCacheGet's TTL branch:
// an entry issued longer ago than describeCacheT must read as a miss AND
// be evicted, while one inside the window is still served and retained.
// Neither direction of a broken comparison surfaces as an error anywhere
// else - a never-expiring cache silently serves stale entitlement state,
// and a never-caching one silently reverts the payload reduction the
// cache exists for. issuedAt is backdated directly instead of slept for,
// so the test is deterministic and instant.
func TestDescribeCacheGetExpiresEntries(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	response := &entclient.EntitledAndAssignedServicesResponseObject{
		FetchErrorFromExternalProviderRegistry: internal.Ptr(true),
	}

	describeCacheStore("ttl-inside", response, time.Now().Add(-describeCacheT+time.Second))
	if got := describeCacheGet("ttl-inside"); got == nil {
		t.Error(`describeCacheGet("ttl-inside"): entry issued within describeCacheT, want a hit, got nil`)
	}
	if _, ok := describeCache.Load("ttl-inside"); !ok {
		t.Error("describeCache after an in-window read: want the entry retained, got it evicted")
	}

	describeCacheStore("ttl-expired", response, time.Now().Add(-describeCacheT-time.Second))
	if got := describeCacheGet("ttl-expired"); got != nil {
		t.Errorf(`describeCacheGet("ttl-expired"): entry older than describeCacheT, want nil, got %+v`, got)
	}
	// Eviction, not just the miss: a retained expired entry would leak one
	// stale value per key for the lifetime of the process.
	if _, ok := describeCache.Load("ttl-expired"); ok {
		t.Error("describeCache after an expired read: want the entry evicted, got it retained")
	}
}

// TestDescribeCacheStoreNewestWins races two writers against an already
// populated key so that both reach describeCacheStore's CompareAndSwap and
// one of them must lose and retry. Seeding matters: on an empty key the
// first writer's LoadOrStore stores and returns, so only one writer ever
// reaches the CAS and the retry is unreachable. A populated key is the
// steady-state shape - a Create's fresh read and a sibling's Observe both
// landing on an entry the previous poll already cached.
//
// The surviving entry is fixed by issuedAt ordering under every legal
// interleaving (if the newest lands first the loser's retry re-reads it and
// gives up on the recency guard; if the older lands first the newest's retry
// swaps it out), so repeating the race probes the retry without flaking.
func TestDescribeCacheStoreNewestWins(t *testing.T) {
	resetDescribeState()
	t.Cleanup(resetDescribeState)

	const key = "cas-race"
	newest := &entclient.EntitledAndAssignedServicesResponseObject{
		FetchErrorFromExternalProviderRegistry: internal.Ptr(true),
	}
	middle := &entclient.EntitledAndAssignedServicesResponseObject{
		FetchErrorFromExternalProviderRegistry: internal.Ptr(false),
	}
	seed := &entclient.EntitledAndAssignedServicesResponseObject{
		FetchErrorFromExternalProviderRegistry: internal.Ptr(false),
	}

	for i := range 1000 {
		issued := time.Now()
		// Both racers are newer than the seed, so both clear the recency
		// guard and contend on the CAS instead of returning early.
		describeCache.Delete(key)
		describeCacheStore(key, seed, issued)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			describeCacheStore(key, middle, issued.Add(time.Second))
		}()
		go func() {
			defer wg.Done()
			describeCacheStore(key, newest, issued.Add(2*time.Second))
		}()
		wg.Wait()

		cached := describeCacheGet(key)
		if cached == nil {
			t.Fatalf("iteration %d: describeCacheGet(%q): want an entry, got nil", i, key)
		}
		if !internal.Val(cached.FetchErrorFromExternalProviderRegistry) {
			t.Fatalf("iteration %d: describeCacheGet(%q): want the newest write to win, got an older one", i, key)
		}
	}
}
