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

func TestFilterAssignedServiceByName(t *testing.T) {

	type args struct {
		payload     *entclient.EntitledAndAssignedServicesResponseObject
		serviceName string
	}

	type want struct {
		o   *entclient.AssignedServiceResponseObject
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"find assigned service": {
			reason: "found by matching name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{
							Name: internal.Ptr("postgresql-db"),
						},
					},
				},
				serviceName: "postgresql-db",
			},
			want: want{
				o: &entclient.AssignedServiceResponseObject{
					Name: internal.Ptr("postgresql-db"),
				},
				err: nil,
			},
		},
		"unknown assigned service": {
			reason: "assigned service with not found",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{
							Name: internal.Ptr("postgresql-db"),
						},
					},
				},
				serviceName: "postgresql-db-never-existed",
			},
			want: want{
				o:   nil,
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				got := filterAssignedServiceByName(tc.args.payload, tc.args.serviceName)

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.filterAssignedServiceByName(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			},
		)
	}
}

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

func TestFilterAssignedServicePlanByName(t *testing.T) {

	type args struct {
		payload         *entclient.AssignedServiceResponseObject
		servicePlanName string
	}

	type want struct {
		o   *entclient.AssignedServicePlanResponseObject
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
				payload: &entclient.AssignedServiceResponseObject{
					ServicePlans: []entclient.AssignedServicePlanResponseObject{
						{
							Name: internal.Ptr("default"),
						},
					},
				},
				servicePlanName: "default",
			},
			want: want{
				o: &entclient.AssignedServicePlanResponseObject{
					Name: internal.Ptr("default"),
				},
				err: nil,
			},
		},
		"unknown service plan": {
			reason: "service plan with name not found",
			args: args{
				payload: &entclient.AssignedServiceResponseObject{
					ServicePlans: []entclient.AssignedServicePlanResponseObject{
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
				got, err := filterAssignedServicePlanByName(tc.args.payload, tc.args.servicePlanName)

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.filterAssignedServicePlanByName(...): -want error, +got error:\n%s\n", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.filterAssignedServicePlanByName(...): -want, +got:\n%s\n", tc.reason, diff)
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

func TestFilterAssignedServices(t *testing.T) {
	type args struct {
		payload     *entclient.EntitledAndAssignedServicesResponseObject
		serviceName string
		servicePlan string
		cr          *v1alpha1.Entitlement
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
		"find service plan": {
			reason: "found by matching name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("postgresql-db"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name: internal.Ptr("default"),
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
				servicePlan: "default",
				serviceName: "postgresql-db",
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid: "0000-0000-0000-0000",
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
		"find service plan and unique ID": {
			reason: "found by matching name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("postgresql-db"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name:             internal.Ptr("default"),
									UniqueIdentifier: internal.Ptr("postgresql-db-aws"),
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
				servicePlan: "default",
				serviceName: "postgresql-db",
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:              "0000-0000-0000-0000",
							ServicePlanUniqueIdentifier: internal.Ptr("postgresql-db-aws"),
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
		"not matching unique ID": {
			reason: "could not find matching unique ID",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("postgresql-db"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name:             internal.Ptr("default"),
									UniqueIdentifier: internal.Ptr("postgresql-db-aws"),
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
				servicePlan: "default",
				serviceName: "postgresql-db",
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid:              "0000-0000-0000-0000",
							ServicePlanUniqueIdentifier: internal.Ptr("postgresql-db-azure"),
						},
					},
				},
			},
			want: want{
				o:   nil,
				err: errors.Errorf(errServiceUniqueName, "postgresql-db-azure"),
			},
		},
		"no unigue ID, not matching plan name": {
			reason: "could not match plan name",
			args: args{
				payload: &entclient.EntitledAndAssignedServicesResponseObject{
					AssignedServices: []entclient.AssignedServiceResponseObject{
						{

							Name: internal.Ptr("postgresql-db"),
							ServicePlans: []entclient.AssignedServicePlanResponseObject{
								{
									Name: internal.Ptr("default"),
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
				servicePlan: "definetly-not-default",
				serviceName: "postgresql-db",
				cr: &v1alpha1.Entitlement{
					Spec: v1alpha1.EntitlementSpec{
						ForProvider: v1alpha1.EntitlementParameters{
							SubaccountGuid: "0000-0000-0000-0000",
						},
					},
				},
			},
			want: want{
				o:   nil,
				err: errors.Errorf(errServicePlanNotFoundByName, "definetly-not-default"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				got, err := filterAssignedServices(tc.args.payload, tc.args.serviceName, tc.args.servicePlan, tc.args.cr)

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.filterAssignedServices(...): -want error, +got error:\n%s\n", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.filterAssignedServices(...): -want, +got:\n%s\n", tc.reason, diff)
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
