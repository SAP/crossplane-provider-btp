package serviceinstanceclient

import (
	"context"
	"testing"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestParameterReadbackOptOutMapping(t *testing.T) {
	cr := &v1alpha1.ServiceInstance{}
	cr.SetAnnotations(map[string]string{tfclient.ParameterReadbackAnnotation: "false"})
	resource, err := (&ServiceInstanceMapper{}).TfResource(context.Background(), cr, fake.NewClientBuilder().Build())
	if err != nil {
		t.Fatal(err)
	}
	if resource.GetAnnotations()[tfclient.ParameterReadbackAnnotation] != "false" {
		t.Fatal("write-only choice must reach the embedded provider")
	}
}
