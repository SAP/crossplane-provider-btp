package servicemanager

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sap/crossplane-provider-btp/internal"
	smclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

// instancesAPIFake fakes the generated ServiceInstancesAPI by embedding the
// interface (so only the methods used by the lookuper need to be implemented)
// and routing GetAllServiceInstances back to itself.
type instancesAPIFake struct {
	smclient.ServiceInstancesAPI
	listFn func() (*smclient.ServiceInstanceResponseList, *http.Response, error)
}

func (f *instancesAPIFake) GetAllServiceInstances(ctx context.Context) smclient.ApiGetAllServiceInstancesRequest {
	return smclient.ApiGetAllServiceInstancesRequest{ApiService: f}
}

func (f *instancesAPIFake) GetAllServiceInstancesExecute(r smclient.ApiGetAllServiceInstancesRequest) (*smclient.ServiceInstanceResponseList, *http.Response, error) {
	return f.listFn()
}

// bindingsAPIFake fakes the generated ServiceBindingsAPI the same way.
type bindingsAPIFake struct {
	smclient.ServiceBindingsAPI
	listFn func() (*smclient.ServiceBindingResponseList, *http.Response, error)
}

func (f *bindingsAPIFake) GetAllServiceBindings(ctx context.Context) smclient.ApiGetAllServiceBindingsRequest {
	return smclient.ApiGetAllServiceBindingsRequest{ApiService: f}
}

func (f *bindingsAPIFake) GetAllServiceBindingsExecute(r smclient.ApiGetAllServiceBindingsRequest) (*smclient.ServiceBindingResponseList, *http.Response, error) {
	return f.listFn()
}

func instanceList(ids ...string) *smclient.ServiceInstanceResponseList {
	items := make([]smclient.ListedServiceInstanceResponseObject, 0, len(ids))
	for _, id := range ids {
		items = append(items, smclient.ListedServiceInstanceResponseObject{Id: internal.Ptr(id)})
	}
	return &smclient.ServiceInstanceResponseList{Items: items}
}

func bindingList(ids ...string) *smclient.ServiceBindingResponseList {
	items := make([]smclient.ListedServiceBindingResponseObject, 0, len(ids))
	for _, id := range ids {
		items = append(items, smclient.ListedServiceBindingResponseObject{Id: internal.Ptr(id)})
	}
	return &smclient.ServiceBindingResponseList{Items: items}
}

func TestLookupServiceInstance(t *testing.T) {
	tests := []struct {
		name      string
		list      *smclient.ServiceInstanceResponseList
		wantGUID  string
		wantFound bool
		wantErr   bool
	}{
		{name: "no match", list: instanceList(), wantFound: false},
		{name: "single match", list: instanceList("si-1"), wantGUID: "si-1", wantFound: true},
		{name: "multiple matches -> error", list: instanceList("si-1", "si-2"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sm := &ServiceManagerClient{
				ServiceInstancesAPI: &instancesAPIFake{listFn: func() (*smclient.ServiceInstanceResponseList, *http.Response, error) {
					return tc.list, nil, nil
				}},
			}
			guid, _, found, err := sm.LookupServiceInstance(context.TODO(), "some-name")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if found != tc.wantFound || guid != tc.wantGUID {
				t.Errorf("got (%q,%v), want (%q,%v)", guid, found, tc.wantGUID, tc.wantFound)
			}
		})
	}
}

func TestLookupServiceBinding(t *testing.T) {
	tests := []struct {
		name      string
		list      *smclient.ServiceBindingResponseList
		wantGUID  string
		wantFound bool
		wantErr   bool
	}{
		{name: "no match", list: bindingList(), wantFound: false},
		{name: "single match", list: bindingList("sb-1"), wantGUID: "sb-1", wantFound: true},
		{name: "multiple matches -> error", list: bindingList("sb-1", "sb-2"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sm := &ServiceManagerClient{
				ServiceBindingsAPI: &bindingsAPIFake{listFn: func() (*smclient.ServiceBindingResponseList, *http.Response, error) {
					return tc.list, nil, nil
				}},
			}
			guid, _, found, err := sm.LookupServiceBinding(context.TODO(), "si-1", "some-name")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if found != tc.wantFound || guid != tc.wantGUID {
				t.Errorf("got (%q,%v), want (%q,%v)", guid, found, tc.wantGUID, tc.wantFound)
			}
		})
	}
}

func TestLookupInstanceAndBindingByPlan(t *testing.T) {
	tests := []struct {
		name        string
		instances   *smclient.ServiceInstanceResponseList
		bindings    *smclient.ServiceBindingResponseList
		wantSI      string
		wantSB      string
		wantFound   bool
		wantErr     bool
		skipBinding bool // when true, the binding API must not be consulted
	}{
		{name: "no instance", instances: instanceList(), wantFound: false, skipBinding: true},
		{name: "instance + binding", instances: instanceList("si-1"), bindings: bindingList("sb-1"), wantSI: "si-1", wantSB: "sb-1", wantFound: true},
		{name: "instance without binding yet", instances: instanceList("si-1"), bindings: bindingList(), wantSI: "si-1", wantSB: "", wantFound: true},
		{name: "multiple instances -> error", instances: instanceList("si-1", "si-2"), wantErr: true, skipBinding: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bindingCalled := false
			sm := &ServiceManagerClient{
				ServiceInstancesAPI: &instancesAPIFake{listFn: func() (*smclient.ServiceInstanceResponseList, *http.Response, error) {
					return tc.instances, nil, nil
				}},
				ServiceBindingsAPI: &bindingsAPIFake{listFn: func() (*smclient.ServiceBindingResponseList, *http.Response, error) {
					bindingCalled = true
					return tc.bindings, nil, nil
				}},
			}
			si, sb, _, found, err := sm.LookupInstanceAndBinding(context.TODO(), "plan-1", "managed-service-manager", "managed-service-manager-binding")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if si != tc.wantSI || sb != tc.wantSB || found != tc.wantFound {
				t.Errorf("got (%q,%q,%v), want (%q,%q,%v)", si, sb, found, tc.wantSI, tc.wantSB, tc.wantFound)
			}
			if tc.skipBinding && bindingCalled {
				t.Errorf("binding API should not have been consulted")
			}
		})
	}
}

func namedBinding(id, name string, ready bool) smclient.ListedServiceBindingResponseObject {
	return smclient.ListedServiceBindingResponseObject{Id: internal.Ptr(id), Name: internal.Ptr(name), Ready: internal.Ptr(ready)}
}

// TestLookupServiceBinding_Rotation covers the fallback for rotated bindings
// named "<name>-<random>" when no exact-name match exists.
func TestLookupServiceBinding_Rotation(t *testing.T) {
	tests := []struct {
		name      string
		exact     *smclient.ServiceBindingResponseList // returned for the exact "name eq" query
		fallback  *smclient.ServiceBindingResponseList // returned for the "service_instance_id eq" query
		wantGUID  string
		wantFound bool
		wantErr   bool
	}{
		{
			name:     "exact match wins",
			exact:    &smclient.ServiceBindingResponseList{Items: []smclient.ListedServiceBindingResponseObject{namedBinding("sb-exact", "octoroute", true)}},
			wantGUID: "sb-exact", wantFound: true,
		},
		{
			name:  "no exact, single ready rotated -> adopt",
			exact: &smclient.ServiceBindingResponseList{},
			fallback: &smclient.ServiceBindingResponseList{Items: []smclient.ListedServiceBindingResponseObject{
				namedBinding("sb-old", "octoroute-aaaaa", false),
				namedBinding("sb-live", "octoroute-bbbbb", true),
			}},
			wantGUID: "sb-live", wantFound: true,
		},
		{
			name:  "no exact, no ready rotated -> not found",
			exact: &smclient.ServiceBindingResponseList{},
			fallback: &smclient.ServiceBindingResponseList{Items: []smclient.ListedServiceBindingResponseObject{
				namedBinding("sb-1", "octoroute-aaaaa", false),
				namedBinding("sb-2", "octoroute-bbbbb", false),
			}},
			wantFound: false,
		},
		{
			name:  "no exact, multiple ready rotated -> ambiguous error",
			exact: &smclient.ServiceBindingResponseList{},
			fallback: &smclient.ServiceBindingResponseList{Items: []smclient.ListedServiceBindingResponseObject{
				namedBinding("sb-1", "octoroute-aaaaa", true),
				namedBinding("sb-2", "octoroute-bbbbb", true),
			}},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			sm := &ServiceManagerClient{
				ServiceBindingsAPI: &bindingsAPIFake{listFn: func() (*smclient.ServiceBindingResponseList, *http.Response, error) {
					calls++
					if calls == 1 {
						return tc.exact, nil, nil
					}
					return tc.fallback, nil, nil
				}},
			}
			guid, _, found, err := sm.LookupServiceBinding(context.TODO(), "si-1", "octoroute")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if found != tc.wantFound || guid != tc.wantGUID {
				t.Errorf("got (%q,%v), want (%q,%v)", guid, found, tc.wantGUID, tc.wantFound)
			}
		})
	}
}

// TestLookupCreatedAtPropagation makes sure the BTP-reported created_at
// timestamp flows through each lookup method. This is what the ownership
// check (recovery.IsOwnedByCR) uses to distinguish our own lost-ID create
// from a brownfield resource.
func TestLookupCreatedAtPropagation(t *testing.T) {
	sample := time.Date(2026, 7, 15, 10, 30, 0, 0, time.UTC)

	instances := &smclient.ServiceInstanceResponseList{Items: []smclient.ListedServiceInstanceResponseObject{
		{Id: internal.Ptr("si-1"), CreatedAt: &sample},
	}}
	bindings := &smclient.ServiceBindingResponseList{Items: []smclient.ListedServiceBindingResponseObject{
		{Id: internal.Ptr("sb-1"), CreatedAt: &sample},
	}}

	t.Run("LookupServiceInstance propagates created_at", func(t *testing.T) {
		sm := &ServiceManagerClient{
			ServiceInstancesAPI: &instancesAPIFake{listFn: func() (*smclient.ServiceInstanceResponseList, *http.Response, error) {
				return instances, nil, nil
			}},
		}
		_, got, _, err := sm.LookupServiceInstance(context.TODO(), "any")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !got.Equal(sample) {
			t.Errorf("createdAt = %v, want %v", got, sample)
		}
	})

	t.Run("LookupServiceBinding propagates created_at", func(t *testing.T) {
		sm := &ServiceManagerClient{
			ServiceBindingsAPI: &bindingsAPIFake{listFn: func() (*smclient.ServiceBindingResponseList, *http.Response, error) {
				return bindings, nil, nil
			}},
		}
		_, got, _, err := sm.LookupServiceBinding(context.TODO(), "si-1", "any")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !got.Equal(sample) {
			t.Errorf("createdAt = %v, want %v", got, sample)
		}
	})

	t.Run("LookupInstanceAndBinding propagates the instance's created_at", func(t *testing.T) {
		sm := &ServiceManagerClient{
			ServiceInstancesAPI: &instancesAPIFake{listFn: func() (*smclient.ServiceInstanceResponseList, *http.Response, error) {
				return instances, nil, nil
			}},
			ServiceBindingsAPI: &bindingsAPIFake{listFn: func() (*smclient.ServiceBindingResponseList, *http.Response, error) {
				return bindings, nil, nil
			}},
		}
		_, _, got, _, err := sm.LookupInstanceAndBinding(context.TODO(), "plan-1", "managed-service-manager", "managed-service-manager-binding")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !got.Equal(sample) {
			t.Errorf("createdAt = %v, want %v", got, sample)
		}
	})
}
