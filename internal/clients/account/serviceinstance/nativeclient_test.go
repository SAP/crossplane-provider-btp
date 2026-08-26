package serviceinstanceclient

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	smopenapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

func httpResp(code int) *http.Response {
	return &http.Response{StatusCode: code}
}

func TestNativeCreate(t *testing.T) {
	const guid = "550e8400-e29b-41d4-a716-446655440000"

	type want struct {
		id      string
		err     bool
		payload *smopenapi.CreateByPlanID
		async   bool
	}

	cases := map[string]struct {
		cr     *v1alpha1.ServiceInstance
		params map[string]interface{}
		resp   *smopenapi.CreatedServiceInstanceResponseObject
		apiErr error
		want   want
	}{
		"Success": {
			cr: &v1alpha1.ServiceInstance{
				Spec: v1alpha1.ServiceInstanceSpec{ForProvider: v1alpha1.ServiceInstanceParameters{
					Name:   "my-instance",
					Labels: map[string][]*string{"team": {internal.Ptr("a")}},
				}},
				Status: v1alpha1.ServiceInstanceStatus{AtProvider: v1alpha1.ServiceInstanceObservation{ServiceplanID: "plan-1"}},
			},
			params: map[string]interface{}{"foo": "bar"},
			resp:   &smopenapi.CreatedServiceInstanceResponseObject{Id: internal.Ptr(guid)},
			want: want{
				id:    guid,
				async: true,
				payload: &smopenapi.CreateByPlanID{
					Name:          "my-instance",
					ServicePlanId: "plan-1",
					Parameters:    map[string]interface{}{"foo": "bar"},
					Labels:        &map[string][]string{"team": {"a"}},
				},
			},
		},
		"ApiError": {
			cr: &v1alpha1.ServiceInstance{
				Spec: v1alpha1.ServiceInstanceSpec{ForProvider: v1alpha1.ServiceInstanceParameters{Name: "x"}},
			},
			apiErr: &smopenapi.GenericOpenAPIError{},
			want:   want{err: true},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var gotPayload smopenapi.CreateServiceInstanceRequestPayload
			var gotAsync bool
			c := &ServiceInstanceClient{
				createFn: func(ctx context.Context, payload smopenapi.CreateServiceInstanceRequestPayload, async bool) (*smopenapi.CreatedServiceInstanceResponseObject, *http.Response, error) {
					gotPayload = payload
					gotAsync = async
					return tc.resp, httpResp(202), tc.apiErr
				},
			}

			id, err := c.Create(context.Background(), tc.cr, tc.params)
			if tc.want.err {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.want.id {
				t.Errorf("id = %q, want %q", id, tc.want.id)
			}
			if gotAsync != tc.want.async {
				t.Errorf("async = %v, want %v", gotAsync, tc.want.async)
			}
			if diff := cmp.Diff(tc.want.payload, gotPayload.CreateByPlanID); diff != "" {
				t.Errorf("payload mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNativeObserve(t *testing.T) {
	const guid = "550e8400-e29b-41d4-a716-446655440000"

	cases := map[string]struct {
		resp       *smopenapi.ServiceInstanceResponseObject
		httpRes    *http.Response
		apiErr     error
		wantExists bool
		wantErr    bool
	}{
		"Found": {
			resp:       &smopenapi.ServiceInstanceResponseObject{Id: internal.Ptr(guid)},
			httpRes:    httpResp(200),
			wantExists: true,
		},
		"NotFound404": {
			httpRes:    httpResp(404),
			apiErr:     &smopenapi.GenericOpenAPIError{},
			wantExists: false,
		},
		"OtherError": {
			httpRes: httpResp(500),
			apiErr:  &smopenapi.GenericOpenAPIError{},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &ServiceInstanceClient{
				getFn: func(ctx context.Context, id string) (*smopenapi.ServiceInstanceResponseObject, *http.Response, error) {
					return tc.resp, tc.httpRes, tc.apiErr
				},
			}
			res, err := c.Observe(context.Background(), guid)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Exists != tc.wantExists {
				t.Errorf("exists = %v, want %v", res.Exists, tc.wantExists)
			}
			if tc.wantExists && res.Instance == nil {
				t.Errorf("expected non-nil instance when exists")
			}
			if !tc.wantExists && res.Instance != nil {
				t.Errorf("expected nil instance when not exists")
			}
		})
	}
}

func TestNativeUpdate(t *testing.T) {
	const guid = "550e8400-e29b-41d4-a716-446655440000"

	cr := &v1alpha1.ServiceInstance{
		Spec: v1alpha1.ServiceInstanceSpec{ForProvider: v1alpha1.ServiceInstanceParameters{
			Name:   "my-instance",
			Shared: internal.Ptr(true),
			Labels: map[string][]*string{
				"team": {internal.Ptr("blue")}, // changed value
				"env":  {internal.Ptr("prod")}, // new key
			},
		}},
		Status: v1alpha1.ServiceInstanceStatus{AtProvider: v1alpha1.ServiceInstanceObservation{ServiceplanID: "plan-1"}},
	}
	observed := &smopenapi.ServiceInstanceResponseObject{
		Shared: internal.Ptr(false), // spec wants true => shared PATCH expected
		Labels: &map[string][]string{
			"team": {"red"},  // will be re-added with new value
			"old":  {"gone"}, // will be removed
		},
	}
	params := map[string]interface{}{"k": "v"}

	type call struct {
		payload smopenapi.UpdateServiceInstanceRequestPayload
		async   bool
	}
	var calls []call
	c := &ServiceInstanceClient{
		updateFn: func(ctx context.Context, id string, payload smopenapi.UpdateServiceInstanceRequestPayload, async bool) (*smopenapi.UpdatedServiceInstanceResponseObject, *http.Response, error) {
			calls = append(calls, call{payload: payload, async: async})
			return &smopenapi.UpdatedServiceInstanceResponseObject{}, httpResp(202), nil
		},
	}

	if err := c.Update(context.Background(), guid, cr, params, observed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Shared drifts (spec=true, observed=false), so this reconcile applies ONLY
	// the synchronous shared-only PATCH; general fields are deferred to the next
	// reconcile once the share operation has settled.
	if len(calls) != 1 {
		t.Fatalf("expected 1 update call (shared-only) when shared drifts, got %d", len(calls))
	}

	shared := calls[0]
	if shared.async {
		t.Errorf("expected shared update async=false")
	}
	if internal.Val(shared.payload.Shared) != true {
		t.Errorf("shared = %v, want true", internal.Val(shared.payload.Shared))
	}
	// Shared PATCH must carry ONLY the shared property.
	if shared.payload.Name != nil || shared.payload.ServicePlanId != nil ||
		shared.payload.Parameters != nil || shared.payload.Labels != nil {
		t.Errorf("shared update must carry shared alone, got %+v", shared.payload)
	}
}

// TestNativeUpdate_General verifies the general async PATCH (name, plan,
// params, labels) when shared does not drift.
func TestNativeUpdate_General(t *testing.T) {
	const guid = "550e8400-e29b-41d4-a716-446655440000"

	cr := &v1alpha1.ServiceInstance{
		Spec: v1alpha1.ServiceInstanceSpec{ForProvider: v1alpha1.ServiceInstanceParameters{
			Name:   "my-instance",
			Shared: internal.Ptr(true), // matches observed => no shared PATCH
			Labels: map[string][]*string{
				"team": {internal.Ptr("blue")}, // changed value
				"env":  {internal.Ptr("prod")}, // new key
			},
		}},
		Status: v1alpha1.ServiceInstanceStatus{AtProvider: v1alpha1.ServiceInstanceObservation{ServiceplanID: "plan-1"}},
	}
	observed := &smopenapi.ServiceInstanceResponseObject{
		Shared: internal.Ptr(true),
		Labels: &map[string][]string{
			"team": {"red"},  // will be re-added with new value
			"old":  {"gone"}, // will be removed
		},
	}
	params := map[string]interface{}{"k": "v"}

	var got smopenapi.UpdateServiceInstanceRequestPayload
	var gotAsync bool
	var calls int
	c := &ServiceInstanceClient{
		updateFn: func(ctx context.Context, id string, payload smopenapi.UpdateServiceInstanceRequestPayload, async bool) (*smopenapi.UpdatedServiceInstanceResponseObject, *http.Response, error) {
			got = payload
			gotAsync = async
			calls++
			return &smopenapi.UpdatedServiceInstanceResponseObject{}, httpResp(202), nil
		},
	}

	if err := c.Update(context.Background(), guid, cr, params, observed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 general update call, got %d", calls)
	}
	if !gotAsync {
		t.Errorf("expected async=true")
	}
	if got.Shared != nil {
		t.Errorf("general update must not carry shared, got %v", *got.Shared)
	}
	if internal.Val(got.Name) != "my-instance" {
		t.Errorf("name = %q, want my-instance", internal.Val(got.Name))
	}
	if internal.Val(got.ServicePlanId) != "plan-1" {
		t.Errorf("service_plan_id = %q, want plan-1", internal.Val(got.ServicePlanId))
	}
	if diff := cmp.Diff(params, got.Parameters); diff != "" {
		t.Errorf("parameters mismatch (-want +got):\n%s", diff)
	}

	// Label ops: expect add(env), add(team), remove(old) — sorted add keys then remove keys.
	ops := map[string]*smopenapi.Label{}
	for i := range got.Labels {
		l := got.Labels[i]
		ops[internal.Val(l.Key)] = &l
	}
	if l, ok := ops["env"]; !ok || internal.Val(l.Op) != labelOpAdd {
		t.Errorf("expected add op for env, got %+v", l)
	}
	if l, ok := ops["team"]; !ok || internal.Val(l.Op) != labelOpAdd {
		t.Errorf("expected add op for team, got %+v", l)
	}
	if l, ok := ops["old"]; !ok || internal.Val(l.Op) != labelOpRemove {
		t.Errorf("expected remove op for old, got %+v", l)
	}
}

func TestNativeUpdate_SharedUnmanaged(t *testing.T) {
	// spec.Shared == nil => Shared must not be set in the payload.
	cr := &v1alpha1.ServiceInstance{
		Spec: v1alpha1.ServiceInstanceSpec{ForProvider: v1alpha1.ServiceInstanceParameters{Name: "x"}},
	}
	var got smopenapi.UpdateServiceInstanceRequestPayload
	c := &ServiceInstanceClient{
		updateFn: func(ctx context.Context, id string, payload smopenapi.UpdateServiceInstanceRequestPayload, async bool) (*smopenapi.UpdatedServiceInstanceResponseObject, *http.Response, error) {
			got = payload
			return &smopenapi.UpdatedServiceInstanceResponseObject{}, httpResp(202), nil
		},
	}
	if err := c.Update(context.Background(), "id", cr, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Shared != nil {
		t.Errorf("expected Shared nil when unmanaged, got %v", *got.Shared)
	}
}

// TestNativeUpdate_SharedNoDrift ensures no extra synchronous shared PATCH is
// issued when the managed shared value already matches the observed instance.
func TestNativeUpdate_SharedNoDrift(t *testing.T) {
	cr := &v1alpha1.ServiceInstance{
		Spec: v1alpha1.ServiceInstanceSpec{ForProvider: v1alpha1.ServiceInstanceParameters{
			Name:   "x",
			Shared: internal.Ptr(true),
		}},
	}
	observed := &smopenapi.ServiceInstanceResponseObject{Shared: internal.Ptr(true)}

	var calls int
	c := &ServiceInstanceClient{
		updateFn: func(ctx context.Context, id string, payload smopenapi.UpdateServiceInstanceRequestPayload, async bool) (*smopenapi.UpdatedServiceInstanceResponseObject, *http.Response, error) {
			calls++
			return &smopenapi.UpdatedServiceInstanceResponseObject{}, httpResp(202), nil
		},
	}
	if err := c.Update(context.Background(), "id", cr, nil, observed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected only the general update (1 call) when shared matches, got %d", calls)
	}
}

func TestNativeDelete(t *testing.T) {
	cases := map[string]struct {
		httpRes *http.Response
		apiErr  error
		wantErr bool
	}{
		"Success": {
			httpRes: httpResp(202),
		},
		"NotFound404": {
			httpRes: httpResp(404),
			apiErr:  &smopenapi.GenericOpenAPIError{},
			wantErr: false,
		},
		"OtherError": {
			httpRes: httpResp(500),
			apiErr:  &smopenapi.GenericOpenAPIError{},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var gotAsync bool
			c := &ServiceInstanceClient{
				deleteFn: func(ctx context.Context, id string, async bool) (map[string]interface{}, *http.Response, error) {
					gotAsync = async
					return nil, tc.httpRes, tc.apiErr
				},
			}
			err := c.Delete(context.Background(), "id")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !gotAsync {
				t.Errorf("expected async=true")
			}
		})
	}
}

func TestToStringSliceMap(t *testing.T) {
	in := map[string][]*string{
		"a": {internal.Ptr("1"), nil, internal.Ptr("2")},
		"b": nil,
	}
	got := toStringSliceMap(in)
	want := map[string][]string{
		"a": {"1", "2"},
		"b": {},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if toStringSliceMap(nil) != nil {
		t.Errorf("expected nil for nil input")
	}
}

func TestToPtrSliceMap(t *testing.T) {
	in := map[string][]string{"a": {"1", "2"}}
	got := ToPtrSliceMap(in)
	if len(got["a"]) != 2 || internal.Val(got["a"][0]) != "1" || internal.Val(got["a"][1]) != "2" {
		t.Errorf("unexpected result: %+v", got)
	}
	if ToPtrSliceMap(nil) != nil {
		t.Errorf("expected nil for nil input")
	}
}

func TestDiffLabels_NoChange(t *testing.T) {
	desired := map[string][]*string{"team": {internal.Ptr("a"), internal.Ptr("b")}}
	observed := map[string][]string{"team": {"b", "a"}} // same set, different order
	if ops := diffLabels(desired, observed); len(ops) != 0 {
		t.Errorf("expected no label ops, got %+v", ops)
	}
}

// TestDiffLabels_IgnoresReservedObserved ensures the SM-managed subaccount_id
// label (returned on the instance GET but not user-settable) never produces a
// remove op, which BTP rejects with "Modifying is not allowed for label".
func TestDiffLabels_IgnoresReservedObserved(t *testing.T) {
	desired := map[string][]*string{} // spec declares no labels
	observed := observedLabels(&smopenapi.ServiceInstanceResponseObject{
		Labels: &map[string][]string{
			"subaccount_id": {"550e8400-e29b-41d4-a716-446655440000"},
		},
	})
	if ops := diffLabels(desired, observed); len(ops) != 0 {
		t.Errorf("expected no label ops for reserved-only observed labels, got %+v", ops)
	}
}

func TestObservedLabels_DropsReserved(t *testing.T) {
	got := observedLabels(&smopenapi.ServiceInstanceResponseObject{
		Labels: &map[string][]string{
			"subaccount_id": {"sa"},
			"team":          {"a"},
		},
	})
	want := map[string][]string{"team": {"a"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestFilterReservedLabels(t *testing.T) {
	in := map[string][]*string{
		"subaccount_id": {internal.Ptr("sa")},
		"team":          {internal.Ptr("a")},
	}
	got := FilterReservedLabels(in)
	if _, ok := got["subaccount_id"]; ok {
		t.Errorf("expected subaccount_id to be filtered out, got %+v", got)
	}
	if _, ok := got["team"]; !ok {
		t.Errorf("expected team to be retained, got %+v", got)
	}
	if FilterReservedLabels(nil) != nil {
		t.Errorf("expected nil for nil input")
	}
}

func TestTimeToMetav1(t *testing.T) {
	if TimeToMetav1(nil) != nil {
		t.Errorf("expected nil for nil input")
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := TimeToMetav1(&now)
	want := metav1.NewTime(now)
	if got == nil || !got.Equal(&want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestIsNotFound(t *testing.T) {
	if isNotFound(nil) {
		t.Errorf("nil response should not be not-found")
	}
	if !isNotFound(httpResp(404)) {
		t.Errorf("404 should be not-found")
	}
	if isNotFound(httpResp(200)) {
		t.Errorf("200 should not be not-found")
	}
}
