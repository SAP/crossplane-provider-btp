package servicemanager

import (
	"context"
	"net/http"
	"testing"

	"github.com/pkg/errors"
	saops "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

func TestLookup(t *testing.T) {
	type args struct {
		CreateMockFn func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error)
		DeleteMockFn func() (*http.Response, error)
		GetMockFn    func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error)

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
				GetMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
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
				GetMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
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
				GetMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return adminBinding(), response(200), nil
				},
				PlanLookupMockFn: func() (string, error) {
					return "", errors.New("PlanLookupError")
				},
			}, want: want{
				err: true,
			},
		},
		{
			name: "BindingDeleteFailure",
			args: args{
				GetMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return adminBinding(), response(200), nil
				},
				PlanLookupMockFn: func() (string, error) {
					return "someId", nil
				},
				DeleteMockFn: func() (*http.Response, error) {
					return response(500), errors.New("DeleteBindingError")
				},
			},
			want: want{
				err:          true,
				deleteCalled: true,
			},
		},
		{
			name: "SuccessFromFoundSMInstance",
			args: args{
				GetMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return adminBinding(), response(200), nil
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
				GetMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return nil, response(404), errors.New("GetBindingError")
				},
				CreateMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
					return adminBinding(), response(200), nil
				},
				PlanLookupMockFn: func() (string, error) {
					return "someId", nil
				},
				DeleteMockFn: func() (*http.Response, error) {
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
				CreateMockFn: tc.args.CreateMockFn,
				DeleteMockFn: tc.args.DeleteMockFn,
				GetMockFn:    tc.args.GetMockFn,
			}

			smClient := ServiceManagerInstanceProxyClient{
				accountService,
				func(ctx context.Context, credentials *BindingCredentials) (PlanIdResolver, error) {
					return &PlanIdResolverFake{
						PlanLookupMockFn: tc.args.PlanLookupMockFn,
					}, nil
				},
			}
			planID, err := smClient.ServiceManagerPlanIDByName(context.TODO(), "", "")

			if tc.want.err != (err != nil) {
				t.Errorf("Unexpected error return; Expected error: %v, Returned: %v", tc.want.err, err)
			}
			if tc.want.id != planID {
				t.Errorf("Unexpected returned PlanID; Expected: %s, Returned: %s", tc.want.id, planID)
			}
			if tc.want.deleteCalled != accountService.AdminBindingDeleteCalled {
				t.Errorf("Unexpected delete call attempts: Expected call: %v, Was Called: %v", tc.want.deleteCalled, accountService.AdminBindingDeleteCalled)
			}
		})
	}
}

func response(code int) *http.Response {
	return &http.Response{StatusCode: code}
}

func adminBinding() *saops.ServiceManagerBindingResponseObject {
	// since we mock all other components, this only needs to be != nil
	return saops.NewServiceManagerBindingResponseObject()
}
