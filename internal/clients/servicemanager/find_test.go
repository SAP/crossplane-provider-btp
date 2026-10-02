package servicemanager

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sap/crossplane-provider-btp/internal"
	smclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

func TestFindServiceInstance(t *testing.T) {
	created := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		list      *smclient.ServiceInstanceResponseList
		apiErr    error
		want      InstanceMatch
		wantFound bool
		wantErr   bool
	}{
		{name: "no match", list: instanceList()},
		{
			// Adoption verifies the plan, so the match carries the plan ID.
			name: "single match carries id, plan and creation time",
			list: &smclient.ServiceInstanceResponseList{Items: []smclient.ListedServiceInstanceResponseObject{
				{Id: internal.Ptr("si-1"), ServicePlanId: internal.Ptr("plan-1"), CreatedAt: &created},
			}},
			want:      InstanceMatch{ID: "si-1", PlanID: "plan-1", CreatedAt: created},
			wantFound: true,
		},
		{name: "multiple matches -> error", list: instanceList("si-1", "si-2"), wantErr: true},
		{name: "api error -> error, never not-found", apiErr: errors.New("boom"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sm := &ServiceManagerClient{
				ServiceInstancesAPI: &instancesAPIFake{listFn: func() (*smclient.ServiceInstanceResponseList, *http.Response, error) {
					return tc.list, nil, tc.apiErr
				}},
			}
			got, found, err := sm.FindServiceInstance(context.TODO(), "my-instance")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if found != tc.wantFound || got != tc.want {
				t.Errorf("got (%+v,%v), want (%+v,%v)", got, found, tc.want, tc.wantFound)
			}
		})
	}
}

func TestFindServiceBinding(t *testing.T) {
	created := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		list      *smclient.ServiceBindingResponseList
		want      BindingMatch
		wantFound bool
		wantErr   bool
	}{
		{
			name: "exact match",
			list: &smclient.ServiceBindingResponseList{Items: []smclient.ListedServiceBindingResponseObject{
				{Id: internal.Ptr("sb-1"), CreatedAt: &created},
			}},
			want:      BindingMatch{ID: "sb-1", CreatedAt: created},
			wantFound: true,
		},
		// Unlike recovery, adoption must not treat a rotated "<name>-<suffix>"
		// binding as the named one: a miss is final after one query.
		{name: "no exact match does not fall back to rotated names", list: bindingList()},
		{name: "multiple matches -> error", list: bindingList("sb-1", "sb-2"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			sm := &ServiceManagerClient{
				ServiceBindingsAPI: &bindingsAPIFake{listFn: func() (*smclient.ServiceBindingResponseList, *http.Response, error) {
					calls++
					return tc.list, nil, nil
				}},
			}
			got, found, err := sm.FindServiceBinding(context.TODO(), "si-1", "my-binding")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if found != tc.wantFound || got != tc.want {
				t.Errorf("got (%+v,%v), want (%+v,%v)", got, found, tc.want, tc.wantFound)
			}
			if calls != 1 {
				t.Errorf("binding list calls = %d, want 1", calls)
			}
		})
	}
}

// pairLookuperFake serves AdoptablePair. The embedded interface is nil, so an
// unexpected method call panics.
type pairLookuperFake struct {
	SemanticLookuper

	byName      InstanceMatch
	byNameFound bool
	byNameErr   error

	siID, sbID string
	pairFound  bool
	pairErr    error
	pairCalls  int
}

func (f *pairLookuperFake) FindServiceInstance(context.Context, string) (InstanceMatch, bool, error) {
	return f.byName, f.byNameFound, f.byNameErr
}

func (f *pairLookuperFake) LookupInstanceAndBinding(context.Context, string, string, string) (string, string, time.Time, bool, error) {
	f.pairCalls++
	return f.siID, f.sbID, time.Time{}, f.pairFound, f.pairErr
}

func TestAdoptablePair(t *testing.T) {
	onPlan := InstanceMatch{ID: "si-1", PlanID: "plan-1"}

	tests := []struct {
		name            string
		lookup          *pairLookuperFake
		want            string
		wantFound       bool
		wantErrContains string
		wantPairLookups int
	}{
		{name: "no instance by that name", lookup: &pairLookuperFake{}},
		{
			// Instance names are unique: a same-name instance on another plan can
			// neither be adopted nor make room for a create, so it is refused.
			name:            "same name on another plan is refused",
			lookup:          &pairLookuperFake{byNameFound: true, byName: InstanceMatch{ID: "si-1", PlanID: "plan-other"}},
			wantErrContains: "refusing to adopt service instance",
		},
		{
			name:            "instance and binding",
			lookup:          &pairLookuperFake{byNameFound: true, byName: onPlan, pairFound: true, siID: "si-1", sbID: "sb-1"},
			want:            "si-1/sb-1",
			wantFound:       true,
			wantPairLookups: 1,
		},
		{
			// The bare instance ID is the phase-1 state from which Create adds the binding.
			name:            "instance without binding",
			lookup:          &pairLookuperFake{byNameFound: true, byName: onPlan, pairFound: true, siID: "si-1"},
			want:            "si-1",
			wantFound:       true,
			wantPairLookups: 1,
		},
		{
			name:            "name lookup fails -> error, never not-found",
			lookup:          &pairLookuperFake{byNameErr: errors.New("boom")},
			wantErrContains: `cannot look up service instance "managed-service-manager" to adopt`,
		},
		{
			name:            "pair lookup fails -> error, never not-found",
			lookup:          &pairLookuperFake{byNameFound: true, byName: onPlan, pairErr: errors.New("boom")},
			wantErrContains: `cannot look up binding "managed-service-manager-binding"`,
			wantPairLookups: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, found, err := AdoptablePair(context.TODO(), tc.lookup, "plan-1", "managed-service-manager", "managed-service-manager-binding")
			if tc.wantErrContains == "" && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErrContains)) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErrContains)
			}
			if found != tc.wantFound || got != tc.want {
				t.Errorf("got (%q,%v), want (%q,%v)", got, found, tc.want, tc.wantFound)
			}
			if tc.lookup.pairCalls != tc.wantPairLookups {
				t.Errorf("pair lookups = %d, want %d", tc.lookup.pairCalls, tc.wantPairLookups)
			}
		})
	}
}

func TestQuote(t *testing.T) {
	// Verified against the Service Manager API: a single quote inside a value
	// is escaped by doubling it; unescaped it is a 400 parse error, and a
	// backslash escape parses but matches nothing.
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain value", in: "my-instance", want: "'my-instance'"},
		{name: "single quote is doubled", in: "o'q", want: "'o''q'"},
		{name: "every quote is doubled", in: "'a''b'", want: "'''a''''b'''"},
		{name: "empty value", in: "", want: "''"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := quote(tc.in); got != tc.want {
				t.Errorf("quote(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
