package servicemanager

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/internal"
	smclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

func TestListServiceInstances(t *testing.T) {
	cases := map[string]struct {
		reason string
		list   *smclient.ServiceInstanceResponseList
		want   []ServiceInstanceRef
	}{
		"Empty": {
			reason: "a subaccount without service instances yields no refs, not an error",
			list:   &smclient.ServiceInstanceResponseList{},
			want:   []ServiceInstanceRef{},
		},
		"MapsEveryField": {
			reason: "the operator-facing fields must all survive the mapping",
			list: &smclient.ServiceInstanceResponseList{Items: []smclient.ListedServiceInstanceResponseObject{
				{
					Id:            internal.Ptr("id-1"),
					Name:          internal.Ptr("objectstore-a"),
					ServicePlanId: internal.Ptr("plan-1"),
					Ready:         internal.Ptr(true),
				},
				{
					Id:            internal.Ptr("id-2"),
					Name:          internal.Ptr("objectstore-b"),
					ServicePlanId: internal.Ptr("plan-2"),
					Ready:         internal.Ptr(false),
				},
			}},
			want: []ServiceInstanceRef{
				{ID: "id-1", Name: "objectstore-a", ServicePlanID: "plan-1", Ready: true},
				{ID: "id-2", Name: "objectstore-b", ServicePlanID: "plan-2", Ready: false},
			},
		},
		"NilFieldsAreEmptyStrings": {
			reason: "the generated model uses pointers throughout; an absent field must not panic",
			list: &smclient.ServiceInstanceResponseList{Items: []smclient.ListedServiceInstanceResponseObject{
				{Id: internal.Ptr("id-1")},
			}},
			want: []ServiceInstanceRef{{ID: "id-1"}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sm := &ServiceManagerClient{
				ServiceInstancesAPI: &instancesAPIFake{listFn: func() (*smclient.ServiceInstanceResponseList, *http.Response, error) {
					return tc.list, nil, nil
				}},
			}

			got, err := sm.ListServiceInstances(context.Background())
			if err != nil {
				t.Fatalf("\n%s\nunexpected error: %v", tc.reason, err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\n-want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// TestListServiceInstancesWrapsAPIError pins that the BTP-side message is
// surfaced rather than the opaque generated-client error, so a failed
// enumeration still tells an operator something.
func TestListServiceInstancesWrapsAPIError(t *testing.T) {
	sm := &ServiceManagerClient{
		ServiceInstancesAPI: &instancesAPIFake{listFn: func() (*smclient.ServiceInstanceResponseList, *http.Response, error) {
			return nil, nil, newSMOpenAPIError(
				smclient.Error{
					Error:       internal.Ptr("InsufficientScope"),
					Description: internal.Ptr("not authorized to list service instances"),
				}, nil, "403 Forbidden")
		}},
	}

	_, err := sm.ListServiceInstances(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not authorized to list service instances") {
		t.Errorf("expected the BTP description to be surfaced, got: %v", err)
	}
}
