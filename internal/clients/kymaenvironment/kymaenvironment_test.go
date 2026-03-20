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
	"github.com/sap/crossplane-provider-btp/internal/clients/fakes"
	client "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnvironmentsApiHandler_GetEnvironments(t *testing.T) {
	// Valid UUID for testing GUID-based lookups
	testUUID := "12345678-1234-1234-1234-123456789012"

	tests := []struct {
		name                string
		mockEnvironmentsApi *fakes.MockProvisioningServiceClient
		mockCr              v1alpha1.KymaEnvironment

		wantErr      error
		wantResponse *client.BusinessEnvironmentInstanceResponseObject
	}{
		{
			name: "EmptyExternalName",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kyma",
					// No external-name annotation
				},
			},
			mockEnvironmentsApi: &fakes.MockProvisioningServiceClient{
				Err: nil,
				ApiResponse: &client.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []client.BusinessEnvironmentInstanceResponseObject{
						{
							Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
							Id:         internal.Ptr(testUUID),
						},
					},
				},
			},
			wantErr:      nil,
			wantResponse: nil, // Empty external-name returns nil (needs creation)
		},
		{
			name: "APIerror",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"crossplane.io/external-name": testUUID},
				},
			},
			mockEnvironmentsApi: &fakes.MockProvisioningServiceClient{
				Err:         errors.New("apiError"),
				ApiResponse: nil,
			},

			wantErr: errors.New("apiError"),
		},
		{
			name: "NotFoundByGUID",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"crossplane.io/external-name": testUUID},
				},
			},
			mockEnvironmentsApi: &fakes.MockProvisioningServiceClient{
				Err:         nil,
				ApiResponse: &client.BusinessEnvironmentInstancesResponseCollection{},
			},
			wantErr:      nil,
			wantResponse: nil, // 404 returns nil (drift scenario)
		},
		{
			name: "SuccessByGUIDLookup",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{"crossplane.io/external-name": testUUID},
				},
			},
			mockEnvironmentsApi: &fakes.MockProvisioningServiceClient{
				Err: nil,
				ApiResponse: &client.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []client.BusinessEnvironmentInstanceResponseObject{
						{
							Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
							Id:         internal.Ptr(testUUID),
						},
					},
				},
			},
			wantErr: nil,
			wantResponse: &client.BusinessEnvironmentInstanceResponseObject{
				Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
				Id:         internal.Ptr(testUUID),
			},
		},
		{
			name: "LegacyLookupByNameAndType",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Name:        "kyma",
					Annotations: map[string]string{"crossplane.io/external-name": "kyma"}, // non-UUID triggers legacy lookup
				},
			},
			mockEnvironmentsApi: &fakes.MockProvisioningServiceClient{
				Err: nil,
				ApiResponse: &client.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []client.BusinessEnvironmentInstanceResponseObject{
						{
							Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
							Name:       internal.Ptr("kyma"),
							Id:         internal.Ptr(testUUID),
							Type:       internal.Ptr(btp.KymaEnvironmentType().Identifier),
						},
					},
				},
			},
			wantErr: nil,
			wantResponse: &client.BusinessEnvironmentInstanceResponseObject{
				Parameters: internal.Ptr("{\"name\":\"kyma\"}"),
				Name:       internal.Ptr("kyma"),
				Id:         internal.Ptr(testUUID),
				Type:       internal.Ptr(btp.KymaEnvironmentType().Identifier),
			},
		},
		{
			name: "LegacyLookupByForProviderName",
			mockCr: v1alpha1.KymaEnvironment{
				ObjectMeta: v1.ObjectMeta{
					Name:        "wrong-lookup-name",
					Annotations: map[string]string{"crossplane.io/external-name": "right-lookup-name"}, // non-UUID triggers legacy lookup
				},
				Spec: v1alpha1.KymaEnvironmentSpec{
					ForProvider: v1alpha1.KymaEnvironmentParameters{
						Name: internal.Ptr("right-lookup-name"),
					},
				},
			},
			mockEnvironmentsApi: &fakes.MockProvisioningServiceClient{
				Err: nil,
				ApiResponse: &client.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []client.BusinessEnvironmentInstanceResponseObject{
						{
							Parameters: internal.Ptr("{\"name\":\"right-lookup-name\"}"),
							Name:       internal.Ptr("right-lookup-name"),
							Id:         internal.Ptr(testUUID),
							Type:       internal.Ptr(btp.KymaEnvironmentType().Identifier),
						},
					},
				},
			},
			wantErr: nil,
			wantResponse: &client.BusinessEnvironmentInstanceResponseObject{
				Parameters: internal.Ptr("{\"name\":\"right-lookup-name\"}"),
				Name:       internal.Ptr("right-lookup-name"),
				Id:         internal.Ptr(testUUID),
				Type:       internal.Ptr(btp.KymaEnvironmentType().Identifier),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uut := KymaEnvironments{
				btp: btp.Client{ProvisioningServiceClient: tc.mockEnvironmentsApi},
			}

			res, err := uut.DescribeInstance(context.TODO(), tc.mockCr)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nGetEnvironment(...): -want error, +got error:\n%s\n", diff)
			}

			if diff := cmp.Diff(tc.wantResponse, res); diff != "" {
				t.Errorf("\nGetEnvironment(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}
