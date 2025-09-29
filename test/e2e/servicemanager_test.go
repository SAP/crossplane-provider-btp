//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	smCreateName = "e2e-sm-servicemanager"
)

func TestServiceManagerCreationFlow(t *testing.T) {
	t.Parallel()
	crudFeatureSuite := features.New("ServiceManager Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/servicemanager/create_flow")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToSchemeConcurrent(r.GetScheme())

				sm := v1beta1.ServiceManager{
					ObjectMeta: metav1.ObjectMeta{Name: smCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sm, cfg, t, wait.WithTimeout(7*time.Minute))
				return ctx
			},
		).
		Assess(
			"Check ServiceManager Resources are fully created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sm := &v1beta1.ServiceManager{}
				MustGetResource(t, cfg, smCreateName, nil, sm)
				// Status bound?
				if sm.Status.AtProvider.Status != v1alpha1.ServiceManagerBound {
					t.Error("Binding status not set as expected")
				}

				assertServiceManagerSecret(t, ctx, cfg, sm)

				return ctx
			},
		).Assess(
		"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// k8s resource cleaned up?
			sm := &v1beta1.ServiceManager{}
			MustGetResource(t, cfg, smCreateName, nil, sm)

			AwaitResourceDeletionOrFail(ctx, t, cfg, sm, wait.WithTimeout(time.Minute*10))

			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, "servicemanager/create_flow", wait.WithTimeout(time.Minute*10))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}

func assertServiceManagerSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, cm *v1beta1.ServiceManager) {
	secretName := cm.GetWriteConnectionSecretToReference().Name
	secretNS := cm.GetWriteConnectionSecretToReference().Namespace
	secret := &corev1.Secret{}
	err := cfg.Client().Resources().Get(ctx, secretName, secretNS, secret)
	if err != nil {
		t.Error("Error while loading expected secret from Ref")
	}
	// secret contains correct structure
	if _, ok := secret.Data["tokenurl"]; !ok {
		t.Error("Secret not in proper format")
	}
}

//
//func createAPIInstance(t *testing.T, apiClient *servicemanager.APIClient, externalName string) *string {
//	request := apiClient.ServiceInstancesAPI.CreateServiceInstance(context.TODO())
//	parameters := map[string]string{"grantType": "clientCredentials"}
//
//	createCisLocalInstanceRequest := servicemanager.CreateServiceInstanceRequestPayload{
//		CreateByOfferingAndPlanName: &servicemanager.CreateByOfferingAndPlanName{
//			Name:                externalName,
//			ServiceOfferingName: "cis",
//			ServicePlanName:     "local",
//			Parameters:          &parameters,
//		},
//		CreateByPlanID: nil,
//	}
//
//	request = request.CreateServiceInstanceRequestPayload(createCisLocalInstanceRequest)
//	request = request.Async(false)
//	response, _, err := request.Execute()
//	if err != nil {
//		t.Errorf("Cannot create cis instance over API")
//		return nil
//	}
//	return response.Id
//}
//
//func createAPIBinding(t *testing.T, apiClient *servicemanager.APIClient, externalName string, serviceInstanceId *string) *string {
//	request := apiClient.ServiceBindingsAPI.CreateServiceBinding(context.TODO())
//	createCisLocalBindingRequest := servicemanager.CreateServiceBindingRequestPayload{
//		Name:              externalName,
//		ServiceInstanceId: *serviceInstanceId,
//		Parameters:        nil,
//		BindResource:      nil,
//	}
//	request = request.CreateServiceBindingRequestPayload(createCisLocalBindingRequest)
//	request = request.Async(false)
//	res, _, err := request.Execute()
//
//	if err != nil {
//		t.Errorf("Cannot create cis binding over API")
//	}
//	return res.Id
//}
