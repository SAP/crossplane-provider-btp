package servicemanager

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/pkg/errors"
	saops "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

func TestLookup(t *testing.T) {
	type args struct {
		CreateV2MockFn func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error)
		DeleteV2MockFn func(name string) (*http.Response, error)
		GetV2MockFn    func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error)

		PlanLookupMockFn func() (string, error)
	}
	type want struct {
		err          bool
		id           string
		deleteCalled bool
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "BindingLookupFailure",
			args: args{
				GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return nil, response(500), errors.New("GetBindingError")
				},
			},
			want: want{
				err: true,
			},
		},
		{
			name: "BindingCreateFailure",
			args: args{
				GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return nil, response(500), errors.New("CreateBindingError")
				},
			},
			want: want{
				err: true,
			},
		},
		{
			name: "ServicePlanLookupFailure",
			args: args{
				GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return adminBindingV2(), response(200), nil
				},
				PlanLookupMockFn: func() (string, error) {
					return "", errors.New("PlanLookupError")
				},
				DeleteV2MockFn: func(name string) (*http.Response, error) {
					return response(200), nil
				},
			}, want: want{
				err:          true,
				deleteCalled: true,
			},
		},
		{
			name: "BindingDeleteFailure",
			args: args{
				GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return adminBindingV2(), response(200), nil
				},
				PlanLookupMockFn: func() (string, error) {
					return "someId", nil
				},
				DeleteV2MockFn: func(name string) (*http.Response, error) {
					return response(500), errors.New("DeleteBindingError")
				},
			},
			want: want{
				err:          true,
				id:           "someId",
				deleteCalled: true,
			},
		},
		{
			name: "SuccessFromFoundSMInstance",
			args: args{
				GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return adminBindingV2(), response(200), nil
				},
				PlanLookupMockFn: func() (string, error) {
					return "someId", nil
				},
			},
			want: want{
				err:          false,
				id:           "someId",
				deleteCalled: false,
			},
		},
		{
			name: "SuccessFromCreatedSMInstance",
			args: args{
				GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
					return adminBindingV2(), response(200), nil
				},
				PlanLookupMockFn: func() (string, error) {
					return "someId", nil
				},
				DeleteV2MockFn: func(name string) (*http.Response, error) {
					return response(200), nil
				},
			},
			want: want{
				err:          false,
				id:           "someId",
				deleteCalled: true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			accountService := &SubaccountServiceFake{
				CreateV2MockFn: tc.args.CreateV2MockFn,
				DeleteV2MockFn: tc.args.DeleteV2MockFn,
				GetV2MockFn:    tc.args.GetV2MockFn,
			}

			smClient := ServiceManagerInstanceProxyClient{
				accountService,
				func(ctx context.Context, credentials *BindingCredentials) (PlanIdResolver, error) {
					return &PlanIdResolverFake{
						PlanLookupMockFn: tc.args.PlanLookupMockFn,
					}, nil
				},
			}
			planID, err := smClient.ServiceManagerPlanIDByName(context.TODO(), "", "", "test-binding")

			if tc.want.err != (err != nil) {
				t.Errorf("Unexpected error return; Expected error: %v, Returned: %v", tc.want.err, err)
			}
			if tc.want.id != planID {
				t.Errorf("Unexpected returned PlanID; Expected: %s, Returned: %s", tc.want.id, planID)
			}
			if tc.want.deleteCalled != accountService.AdminBindingV2DeleteCalled {
				t.Errorf("Unexpected delete call attempts: Expected call: %v, Was Called: %v", tc.want.deleteCalled, accountService.AdminBindingV2DeleteCalled)
			}
		})
	}
}

// These failures occur after the Accounts API has created the temporary helper.
// Its deletion must still run, with usable credentials and a bounded context,
// and must not hide either the lookup failure or a cleanup failure.
func TestDynamicServiceInstanceCleanup(t *testing.T) {
	lookupErr := errors.New("plan lookup failed")
	cleanupErr := errors.New("admin binding delete failed")
	tests := []struct {
		name         string
		lookupErr    error
		lookupID     string
		cleanupErr   error
		cancelLookup bool
	}{
		{name: "lookup failure", lookupErr: lookupErr},
		{name: "partial lookup failure", lookupErr: lookupErr, lookupID: "partial-id"},
		{name: "canceled lookup", lookupErr: context.Canceled, cancelLookup: true},
		{name: "lookup succeeds after cancellation", cancelLookup: true},
		{name: "cleanup failure", cleanupErr: cleanupErr},
		{name: "lookup and cleanup fail", lookupErr: lookupErr, cleanupErr: cleanupErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			type contextKey struct{}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "credentials"))
			defer cancel()
			deleteCalls := 0
			api := &cleanupContextSubaccountService{
				SubaccountServiceFake: SubaccountServiceFake{
					CreateV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
						return adminBindingV2(), response(200), nil
					},
					GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
						return nil, response(404), nil
					},
				},
				deleteFn: func(deleteCtx context.Context, name string) (*http.Response, error) {
					deleteCalls++
					if deleteCtx.Err() != nil {
						t.Errorf("cleanup context is canceled: %v", deleteCtx.Err())
					}
					if deleteCtx.Value(contextKey{}) != "credentials" {
						t.Error("cleanup context lost authentication values")
					}
					deadline, ok := deleteCtx.Deadline()
					if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > time.Minute {
						t.Errorf("cleanup must have a bounded future deadline, got %v (set: %v)", deadline, ok)
					}
					return response(200), tc.cleanupErr
				},
			}
			client := ServiceManagerInstanceProxyClient{SubaccountOperationsAPI: api}
			id, err := client.dynamicServiceInstance(ctx, "subaccount", "test-binding", func(*BindingCredentials) (string, error) {
				if tc.cancelLookup {
					cancel()
				}
				if tc.lookupErr != nil {
					return tc.lookupID, tc.lookupErr
				}
				return "plan-id", nil
			})
			if deleteCalls != 1 {
				t.Errorf("expected one helper deletion, got %d", deleteCalls)
			}
			if tc.lookupErr != nil && !errors.Is(err, tc.lookupErr) {
				t.Errorf("lookup error was lost: %v", err)
			}
			if tc.cleanupErr != nil && !errors.Is(err, tc.cleanupErr) {
				t.Errorf("cleanup error was lost: %v", err)
			}
			if tc.lookupErr != nil && tc.cleanupErr == nil && err != tc.lookupErr {
				t.Errorf("successful cleanup must preserve the original lookup error, got %v", err)
			}
			if tc.lookupErr == nil && tc.cleanupErr == nil {
				if err != nil || id != "plan-id" {
					t.Errorf("expected successful lookup, got id %q, error %v", id, err)
				}
			} else if tc.lookupErr == nil && tc.cleanupErr != nil {
				// Lookup succeeded: plan ID must be preserved even when cleanup fails.
				if id != "plan-id" {
					t.Errorf("cleanup failure must not discard a successful plan ID, got %q", id)
				}
			} else if tc.lookupErr != nil && id != "" {
				t.Errorf("failed lookup must not return a plan ID, got %q", id)
			}
		})
	}
}

// Override the V2 delete request builder to inspect the context supplied to
// deletion; generated request fields are private and the shared fake drops the context.
type cleanupContextSubaccountService struct {
	SubaccountServiceFake
	deleteFn func(context.Context, string) (*http.Response, error)
}

func (s *cleanupContextSubaccountService) DeleteServiceManagerBindingV2(ctx context.Context, _ string, bindingName string) saops.ApiDeleteServiceManagerBindingV2Request {
	result, err := s.deleteFn(ctx, bindingName)
	s.DeleteV2MockFn = func(name string) (*http.Response, error) { return result, err }
	s.LastV2BindingName = bindingName
	return saops.ApiDeleteServiceManagerBindingV2Request{ApiService: &s.SubaccountServiceFake}
}

func response(code int) *http.Response {
	return &http.Response{StatusCode: code}
}

func adminBinding() *saops.ServiceManagerBindingResponseObject {
	return saops.NewServiceManagerBindingResponseObject()
}

func adminBindingV2() *saops.ServiceManagerBindingExtendedResponseObject {
	obj := saops.NewServiceManagerBindingExtendedResponseObject()
	clientid := "clientid"
	clientsecret := "secret"
	url := "https://accounts.example.com"
	smUrl := "https://sm.example.com"
	obj.Clientid = &clientid
	obj.Clientsecret = &clientsecret
	obj.Url = &url
	obj.SmUrl = &smUrl
	return obj
}

var _ PlanIdResolver = &PlanIdResolverFake{}

type PlanIdResolverFake struct {
	PlanLookupMockFn func() (string, error)
}

func (p *PlanIdResolverFake) PlanIDByName(ctx context.Context, offeringName, planName string, dataCenter string) (string, error) {
	return p.PlanLookupMockFn()
}

// TestCreateAdminBindingSurfacesAPIBody asserts that createAdminBindingV2 routes
// transport errors through specifyAccountsAPIError so the BTP API body is surfaced
// instead of the opaque "<status> <reason>" string.
func TestCreateAdminBindingSurfacesAPIBody(t *testing.T) {
	accountService := &SubaccountServiceFake{
		GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
			return nil, response(404), errors.New("not found")
		},
		CreateV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
			return nil, response(500), create500Error()
		},
	}
	smClient := ServiceManagerInstanceProxyClient{
		accountService,
		func(ctx context.Context, credentials *BindingCredentials) (PlanIdResolver, error) {
			return nil, nil
		},
	}

	_, err := smClient.ServiceManagerPlanIDByName(context.TODO(), "", "", "test-binding")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "internal server error") || !strings.Contains(err.Error(), "Code 500") {
		t.Errorf("expected SM API body to be surfaced, got: %v", err)
	}
}

// create500Error builds a *saops.GenericOpenAPIError carrying an
// ApiExceptionResponseObject so we can assert specifyAccountsAPIError surfaces the
// structured body. Mirrors subaccount_test.go's helper.
func create500Error() error {
	apiExceptionError := saops.NewApiExceptionResponseObjectError()
	apiExceptionError.SetCode(500)
	apiExceptionError.SetMessage("internal server error")

	apiException := saops.NewApiExceptionResponseObject(*apiExceptionError)

	err := &saops.GenericOpenAPIError{}
	errValue := reflect.ValueOf(err).Elem()

	if modelField := errValue.FieldByName("model"); modelField.IsValid() {
		reflect.NewAt(modelField.Type(), unsafe.Pointer(modelField.UnsafeAddr())).
			Elem().Set(reflect.ValueOf(*apiException))
	}
	if errorField := errValue.FieldByName("error"); errorField.IsValid() {
		reflect.NewAt(errorField.Type(), unsafe.Pointer(errorField.UnsafeAddr())).
			Elem().SetString("500 Internal Server Error")
	}
	return err
}

func TestTempBindingName(t *testing.T) {
	tests := []struct {
		crName    string
		namespace string
		wantValue string
	}{
		{crName: "my-sm", namespace: "default", wantValue: "crossplane-tmp-my-sm-default"},
		{crName: "a", namespace: "b", wantValue: "crossplane-tmp-a-b"},
		// long name must be truncated to 63 chars
		{crName: strings.Repeat("x", 40), namespace: strings.Repeat("y", 40), wantValue: "crossplane-tmp-" + strings.Repeat("x", 40) + "-" + strings.Repeat("y", 7)},
	}
	for _, tc := range tests {
		got := TempBindingName(tc.crName, tc.namespace)
		if len(got) > 63 {
			t.Errorf("TempBindingName(%q, %q): result too long: %d chars", tc.crName, tc.namespace, len(got))
		}
		if got != tc.wantValue {
			t.Errorf("TempBindingName(%q, %q): got %q, want %q", tc.crName, tc.namespace, got, tc.wantValue)
		}
		if !strings.HasPrefix(got, "crossplane-tmp-") {
			t.Errorf("TempBindingName(%q, %q): missing prefix, got %q", tc.crName, tc.namespace, got)
		}
	}
}

func TestOrphanedBindingReuse(t *testing.T) {
	// Simulate: named binding already exists in BTP (orphan from previous reconcile).
	// ServiceManagerPlanIDByName must reuse it without calling Create (which would 409).
	createCalled := false

	accountService := &SubaccountServiceFake{
		GetV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
			return adminBindingV2(), response(200), nil
		},
		CreateV2MockFn: func(name string) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
			createCalled = true
			return nil, response(409), errors.New("binding already exists")
		},
		DeleteV2MockFn: func(name string) (*http.Response, error) {
			t.Errorf("delete must not be called when existing binding is reused")
			return response(200), nil
		},
	}

	smClient := ServiceManagerInstanceProxyClient{
		accountService,
		func(ctx context.Context, credentials *BindingCredentials) (PlanIdResolver, error) {
			return &PlanIdResolverFake{
				PlanLookupMockFn: func() (string, error) { return "plan-id", nil },
			}, nil
		},
	}

	planID, err := smClient.ServiceManagerPlanIDByName(context.Background(), "subaccount", "standard", "crossplane-tmp-my-sm-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if planID != "plan-id" {
		t.Errorf("expected plan-id, got %q", planID)
	}
	if createCalled {
		t.Error("Create must not be called when binding already exists")
	}
}
