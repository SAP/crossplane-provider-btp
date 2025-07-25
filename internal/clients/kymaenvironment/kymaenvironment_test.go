package environments

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	client "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnvironmentsApiHandler_GetEnvironments(t *testing.T) {
	tests := []struct {
		name                string
		mockEnvironmentsApi *MockProvisioningServiceClient
		mockCr              v1alpha1.KymaEnvironment

		wantErr        error
		wantInitialize bool
		wantResponse   *client.BusinessEnvironmentInstanceResponseObject
	}{
		{
			name: "APIerror",
			mockEnvironmentsApi: &MockProvisioningServiceClient{
				err:         errors.New("apiError"),
				apiResponse: nil,
			},

			wantErr: errors.New("apiError"),
		},
		{
			name: "EmptyResponse",
			mockEnvironmentsApi: &MockProvisioningServiceClient{
				err:         nil,
				apiResponse: &client.BusinessEnvironmentInstancesResponseCollection{},
			},
			wantErr:      nil,
			wantResponse: nil,
		},
		{
			name: "SuccessByExternalNameLookup",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"crossplane.io/external-name": "1234"},
				},
			},
			mockEnvironmentsApi: &MockProvisioningServiceClient{
				err: nil,
				apiResponse: &client.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []client.BusinessEnvironmentInstanceResponseObject{
						{
							Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
							Id:         internal.Ptr("1234"),
						},
					},
				},
			},
			wantErr:        nil,
			wantInitialize: false,
			wantResponse: &client.BusinessEnvironmentInstanceResponseObject{
				Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
				Id:         internal.Ptr("1234"),
			},
		},
		{
			name: "SuccessByNameAndTypeLookup",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Name:        "kyma",
					Annotations: map[string]string{"crossplane.io/external-name": "kyma"},
				},
			},
			mockEnvironmentsApi: &MockProvisioningServiceClient{
				err: nil,
				apiResponse: &client.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []client.BusinessEnvironmentInstanceResponseObject{
						{
							Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
							Name:       internal.Ptr("kyma"),
							Id:         internal.Ptr("1234"),
							Type:       internal.Ptr(btp.KymaEnvironmentType().Identifier),
						},
					},
				},
			},
			wantErr:        nil,
			wantInitialize: true,
			wantResponse: &client.BusinessEnvironmentInstanceResponseObject{
				Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
				Name:       internal.Ptr("kyma"),
				Id:         internal.Ptr("1234"),
				Type:       internal.Ptr(btp.KymaEnvironmentType().Identifier),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uut := KymaEnvironments{
				btp: btp.Client{ProvisioningServiceClient: tc.mockEnvironmentsApi},
			}

			res, lateInitialize, err := uut.DescribeInstance(context.TODO(), tc.mockCr)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nGetEnvironment(...): -want error, +got error:\n%s\n", diff)
			}

			if diff := cmp.Diff(tc.wantResponse, res); diff != "" {
				t.Errorf("\nGetEnvironment(...): -want, +got:\n%s\n", diff)
			}

			if diff := cmp.Diff(tc.wantInitialize, lateInitialize, test.EquateErrors()); diff != "" {
				t.Errorf("\nGetEnvironment(...): -want initialize, +got initialize:\n%s\n", diff)
			}
		})
	}
}
