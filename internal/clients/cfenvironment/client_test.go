package environments

import (
	"reflect"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

func TestGetConnectionDetailsConflictingSpellings(t *testing.T) {
	tests := []struct {
		name   string
		labels string
	}{
		{name: "organization ID values differ", labels: `{"Org ID":"id-marker-one","Org ID:":"id-marker-two"}`},
		{name: "endpoint values differ", labels: `{"API Endpoint":"endpoint-marker-one","API Endpoint:":"endpoint-marker-two"}`},
		{name: "empty ID conflicts with value", labels: `{"Org ID":"","Org ID:":"id-marker"}`},
		{name: "empty endpoint conflicts with value", labels: `{"API Endpoint":"","API Endpoint:":"endpoint-marker"}`},
		{name: "value ID conflicts with empty", labels: `{"Org ID":"id-marker","Org ID:":""}`},
		{name: "value endpoint conflicts with empty", labels: `{"API Endpoint":"endpoint-marker","API Endpoint:":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetConnectionDetails(instance(withLabels(tt.labels)))
			if err == nil {
				t.Fatalf("GetConnectionDetails() = %v, want conflict error", got)
			}
			if len(got) != 0 {
				t.Errorf("GetConnectionDetails() returned partial details: %v", got)
			}
			for _, marker := range []string{"id-marker-one", "id-marker-two", "endpoint-marker-one", "endpoint-marker-two", "id-marker", "endpoint-marker"} {
				if strings.Contains(err.Error(), marker) {
					t.Errorf("error leaked label value %q: %v", marker, err)
				}
			}
		})
	}
}

func TestGetConnectionDetailsIdenticalEmptyDuplicates(t *testing.T) {
	labels := `{"Org ID":"","Org ID:":"","API Endpoint":"","API Endpoint:":""}`
	got, err := GetConnectionDetails(instance(withLabels(labels)))
	if err != nil {
		t.Fatalf("GetConnectionDetails() error = %v", err)
	}
	for _, key := range []string{v1alpha1.ResourceOrgId, v1alpha1.ResourceAPIEndpoint} {
		value, ok := got[key]
		if !ok || len(value) != 0 {
			t.Errorf("GetConnectionDetails()[%q] = %q, present %v; want present empty value", key, value, ok)
		}
	}
}

func TestGetConnectionDetailsIdenticalDuplicatesPreserveRawLabels(t *testing.T) {
	labels := ` { "Org ID:" : "same-id", "API Endpoint" : "same-endpoint", "Org ID" : "same-id", "Org Name" : "my-org", "API Endpoint:" : "same-endpoint", "Other" : "unchanged" } `
	got, err := GetConnectionDetails(instance(withLabels(labels)))
	if err != nil {
		t.Fatalf("GetConnectionDetails() error = %v", err)
	}
	want := managed.ConnectionDetails{
		v1alpha1.ResourceOrgName:       []byte("my-org"),
		v1alpha1.ResourceOrgId:         []byte("same-id"),
		v1alpha1.ResourceAPIEndpoint:   []byte("same-endpoint"),
		providerv1alpha1.RawBindingKey: []byte(labels),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetConnectionDetails() = %v, want %v", got, want)
	}
}

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
				v1alpha1.ResourceOrgName:       []byte("name"),
				v1alpha1.ResourceOrgId:         []byte("uuid"),
				v1alpha1.ResourceAPIEndpoint:   []byte("url"),
				providerv1alpha1.RawBindingKey: []byte("{\"API Endpoint\":\"url\",\"Org Name\":\"name\",\"Org ID\":\"uuid\"}"),
			},
			wantErr: false,
		},
		{
			name: "Labels mixed format from issue 1175 - Org ID:, API Endpoint",
			args: args{
				instance: instance(withLabels(`{"Org ID:":"mixed-uuid","API Endpoint":"https://api.cf.example.com","Org Name":"my-org"}`)),
			},
			want: managed.ConnectionDetails{
				v1alpha1.ResourceOrgName:       []byte("my-org"),
				v1alpha1.ResourceOrgId:         []byte("mixed-uuid"),
				v1alpha1.ResourceAPIEndpoint:   []byte("https://api.cf.example.com"),
				providerv1alpha1.RawBindingKey: []byte(`{"Org ID:":"mixed-uuid","API Endpoint":"https://api.cf.example.com","Org Name":"my-org"}`),
			},
			wantErr: false,
		},
		{
			name: "Labels unknown, only raw connection details",
			args: args{
				instance: instance(withLabels("{\"asd\":\"url\",\"asdf\":\"name\",\"bar\":\"uuid\"}")),
			},
			want: managed.ConnectionDetails{
				providerv1alpha1.RawBindingKey: []byte("{\"asd\":\"url\",\"asdf\":\"name\",\"bar\":\"uuid\"}"),
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
		{
			name: "Labels old format with colons - API Endpoint:, Org ID:",
			args: args{
				instance: instance(withLabels("{\"API Endpoint:\":\"https://api.cf.example.com\",\"Org Name\":\"my-org\",\"Org ID:\":\"old-uuid\"}")),
			},
			want: managed.ConnectionDetails{
				v1alpha1.ResourceOrgName:       []byte("my-org"),
				v1alpha1.ResourceOrgId:         []byte("old-uuid"),
				v1alpha1.ResourceAPIEndpoint:   []byte("https://api.cf.example.com"),
				providerv1alpha1.RawBindingKey: []byte("{\"API Endpoint:\":\"https://api.cf.example.com\",\"Org Name\":\"my-org\",\"Org ID:\":\"old-uuid\"}"),
			},
			wantErr: false,
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
