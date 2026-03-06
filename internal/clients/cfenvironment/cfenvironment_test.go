package environments

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/fakes"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// crWithManagers returns a CloudFoundryEnvironment CR with the given managers.
func _(specManagers []string, statusManagers []string) v1alpha1.CloudFoundryEnvironment {
	return v1alpha1.CloudFoundryEnvironment{
		Spec: v1alpha1.CfEnvironmentSpec{
			ForProvider: v1alpha1.CfEnvironmentParameters{
				Managers: specManagers,
			},
		},
		Status: v1alpha1.EnvironmentStatus{
			AtProvider: v1alpha1.CfEnvironmentObservation{
				Managers: toUserSlice(statusManagers),
			},
		},
	}
}

// toUserSlice converts a slice of strings to a slice of v1alpha1.User.
func toUserSlice(ss []string) []v1alpha1.User {
	us := make([]v1alpha1.User, 0)
	for _, s := range ss {
		us = append(us, v1alpha1.User{Username: s, Origin: "sap.ids"})
	}
	return us
}

func TestNewOrganizationClient_Validation(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
		url     string
		orgId   string
		user    string
		pass    string
		origin  string
		wantErr bool
	}{
		{
			name:    "missing org name returns error",
			orgName: "",
			url:     "https://api.cf.example.com",
			orgId:   "org-guid",
			user:    "user",
			pass:    "pass",
			origin:  "",
			wantErr: true,
		},
		{
			name:    "missing orgGuid returns error",
			orgName: "my-org",
			url:     "https://api.cf.example.com",
			orgId:   "",
			user:    "user",
			pass:    "pass",
			origin:  "",
			wantErr: true,
		},
		{
			name:    "empty origin is accepted",
			orgName: "my-org",
			url:     "https://api.cf.example.com",
			orgId:   "org-guid",
			user:    "user",
			pass:    "pass",
			origin:  "",
			wantErr: true, // will fail on CF API connect, but not on validation
		},
		{
			name:    "non-empty origin is accepted",
			orgName: "my-org",
			url:     "https://api.cf.example.com",
			orgId:   "org-guid",
			user:    "user",
			pass:    "pass",
			origin:  "custom-idp",
			wantErr: true, // will fail on CF API connect, but not on validation
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newOrganizationClient(tt.orgName, tt.url, tt.orgId, tt.user, tt.pass, tt.origin)
			if (err != nil) != tt.wantErr {
				t.Errorf("newOrganizationClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCloudFoundryOrganization_getEnvironmentByNameAndOrg(t *testing.T) {
	getBtpClient := func(instanceName string) btp.Client {
		return btp.Client{
			Credential: &btp.Credentials{
				UserCredential: &btp.UserCredential{Username: "username", Password: "password"},
			},
			ProvisioningServiceClient: &fakes.MockProvisioningServiceClient{
				ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{
					EnvironmentInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
						{
							EnvironmentType: internal.Ptr("cloudfoundry"),
							Parameters:      internal.Ptr(fmt.Sprintf("{\"instance_name\":\"%s\"}", instanceName)),
							Id:              internal.Ptr("1234"),
							Labels:          internal.Ptr("{\"Org Id\":\"1234\",\"Org Name\":\"name\",\"API Endpoint\":\"endpoint\"}"),
						},
					},
				},
			},
		}
	}

	type fields struct {
		btp btp.Client
	}
	type args struct {
		ctx context.Context
		cr  v1alpha1.CloudFoundryEnvironment
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *provisioningclient.BusinessEnvironmentInstanceResponseObject
		wantErr bool
	}{
		{
			name: "Error",
			fields: fields{
				btp: btp.Client{
					Credential: &btp.Credentials{
						UserCredential: &btp.UserCredential{Username: "username", Password: "password"},
					},
					ProvisioningServiceClient: &fakes.MockProvisioningServiceClient{
						Err: errors.New("error"),
					},
				},
			},
			args: args{
				cr: v1alpha1.CloudFoundryEnvironment{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{"crossplane.io/external-name": "extName"},
					},
					Spec: v1alpha1.CfEnvironmentSpec{
						ForProvider: v1alpha1.CfEnvironmentParameters{
							OrgName: "org",
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Not found - no environment matching external-name",
			fields: fields{
				btp: getBtpClient("somethingElse"),
			},
			args: args{
				cr: v1alpha1.CloudFoundryEnvironment{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{"crossplane.io/external-name": "extName"},
					},
					Spec: v1alpha1.CfEnvironmentSpec{
						ForProvider: v1alpha1.CfEnvironmentParameters{
							OrgName: "org",
						},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "Success - match by external-name",
			fields: fields{
				btp: getBtpClient("extName"),
			},
			args: args{
				cr: v1alpha1.CloudFoundryEnvironment{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{"crossplane.io/external-name": "extName"},
					},
					Spec: v1alpha1.CfEnvironmentSpec{
						ForProvider: v1alpha1.CfEnvironmentParameters{
							OrgName: "org",
						},
					},
				},
			},
			want: &provisioningclient.BusinessEnvironmentInstanceResponseObject{
				EnvironmentType: internal.Ptr("cloudfoundry"),
				Parameters:      internal.Ptr("{\"instance_name\":\"extName\"}"),
				Id:              internal.Ptr("1234"),
				Labels:          internal.Ptr("{\"Org Id\":\"1234\",\"Org Name\":\"name\",\"API Endpoint\":\"endpoint\"}"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CloudFoundryOrganization{
				btp: tt.fields.btp,
			}
			got, err := c.getEnvironmentByNameAndOrg(tt.args.ctx, tt.args.cr)
			if (err != nil) != tt.wantErr {
				t.Errorf("getEnvironmentByNameAndOrg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEnvironmentByNameAndOrg() got = %v, want %v", got, tt.want)
			}
		})
	}
}
