package environments

import (
	"reflect"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

func TestGetConnectionDetails(t *testing.T) {
	type args struct {
		instance *provisioningclient.BusinessEnvironmentInstanceResponseObject
	}
	tests := []struct {
		name    string
		args    args
		want    managed.ConnectionDetails
		wantErr bool
	}{
		{
			name: "Nil instance, returns empty connection details",
			args: args{
				instance: nil,
			},
			want:    managed.ConnectionDetails{},
			wantErr: false,
		},
		{
			name: "Labels Correct, good conenction details",
			args: args{
				instance: instance(withLabels("{\"API Endpoint\":\"url\",\"Org Name\":\"name\",\"Org ID\":\"uuid\"}")),
			},
			want: managed.ConnectionDetails{
				v1alpha1.ResourceOrgName:     []byte("name"),
				v1alpha1.ResourceOrgId:       []byte("uuid"),
				v1alpha1.ResourceAPIEndpoint: []byte("url"),
				v1alpha1.ResourceRaw:         []byte("{\"API Endpoint\":\"url\",\"Org Name\":\"name\",\"Org ID\":\"uuid\"}"),
			},
			wantErr: false,
		},
		{
			name: "Labels unknown, only raw connection details",
			args: args{
				instance: instance(withLabels("{\"asd\":\"url\",\"asdf\":\"name\",\"bar\":\"uuid\"}")),
			},
			want: managed.ConnectionDetails{
				v1alpha1.ResourceRaw: []byte("{\"asd\":\"url\",\"asdf\":\"name\",\"bar\":\"uuid\"}"),
			},
			wantErr: false,
		},
		{
			name: "Labels invalid json, empty connection details",
			args: args{
				instance: instance(withLabels("asdf:f")),
			},
			want:    managed.ConnectionDetails{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetConnectionDetails(tt.args.instance)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetConnectionDetails() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetConnectionDetails() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type instanceModifier func(*provisioningclient.BusinessEnvironmentInstanceResponseObject)

func withLabels(labels string) instanceModifier {
	return func(r *provisioningclient.BusinessEnvironmentInstanceResponseObject) { r.Labels = &labels }
}

func instance(m ...instanceModifier) *provisioningclient.BusinessEnvironmentInstanceResponseObject {
	cr := &provisioningclient.BusinessEnvironmentInstanceResponseObject{}
	for _, f := range m {
		f(cr)
	}
	return cr
}
