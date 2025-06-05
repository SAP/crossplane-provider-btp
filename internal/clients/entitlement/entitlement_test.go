package entitlement

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/internal"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"

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

func TestFilterEntitledServicePlanByName(t *testing.T) {

	type args struct {
		payload         entclient.EntitledServicesResponseObject
		servicePlanName string
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
				servicePlanName: "default",
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
				servicePlanName: "default-plan-never-existed",
			},
			want: want{
				o:   nil,
				err: errors.Errorf(errServicePlanNotFoundByName, "default-plan-never-existed"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				got, err := filterEntitledServicePlanByName(&tc.args.payload, tc.args.servicePlanName)

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServicePlanByName(...): -want error, +got error:\n%s\n", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.filterEntitledServicePlanByName(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			},
		)
	}
}

func TestFindAssignedServicePlan(t *testing.T) {
	type args struct {
		payload *entclient.EntitledAndAssignedServicesResponseObject
		cr      *v1alpha1.Entitlement
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
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:  "0000-0000-0000-0000",
							ServicePlanName: "plan-A",
							ServiceName:     "srv-2",
						},
					},
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
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:  "0000-0000-0000-0000",
							ServicePlanName: "plan-B",
							ServiceName:     "srv-1",
						},
					},
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
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:  "0000-0000-0000-0000",
							ServicePlanName: "plan-A",
							ServiceName:     "srv-1",
						},
					},
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
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:              "0000-0000-0000-0000",
							ServicePlanUniqueIdentifier: internal.Ptr("plan-A-B"),
							ServicePlanName:             "plan-A",
							ServiceName:                 "srv-1",
						},
					},
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
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:              "1111-1111-1111-1111",
							ServicePlanUniqueIdentifier: internal.Ptr("plan-A-B"),
							ServicePlanName:             "plan-A",
							ServiceName:                 "srv-1",
						},
					},
				},
			},
			want: want{
				o: &entclient.AssignedServicePlanSubaccountDTO{
					EntityId: internal.Ptr("1111-1111-1111-1111"),
				},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				entClient := EntitlementsClient{}
				got, err := entClient.findAssignedServicePlan(tc.args.payload, tc.args.cr)

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
		payload     *entclient.EntitledAndAssignedServicesResponseObject
		serviceName string
		servicePlan string
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
				servicePlan: "default",
				serviceName: "postgresql-db",
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
				got, err := filterEntitledServices(tc.args.payload, tc.args.serviceName, tc.args.servicePlan)

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
