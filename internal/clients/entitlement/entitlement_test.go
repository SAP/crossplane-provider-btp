package entitlement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/btp"
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

// TestDeleteSkipsAutoAssigned verifies DeleteInstance never calls
// SetServicePlans when Assigned.AutoAssigned is true: BTP documents such
// assignments as unremovable by admin action.
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

	if err := c.DeleteInstance(context.Background(), cr); err != nil {
		t.Fatalf("DeleteInstance(...): unexpected error: %v", err)
	}
}

// TestCreateSkipsAutoAssigned verifies CreateInstance (== UpdateInstance)
// never calls SetServicePlans for an AutoAssigned entitlement.
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

	if err := c.CreateInstance(context.Background(), cr); err != nil {
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

	if err := c.CreateInstance(context.Background(), cr); err != nil {
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
