//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
)

var (
	smCreateName = "e2e-sm-servicemanager"
	smImportName = "sm-import-test"
)

func TestServiceManagerCreationFlow(t *testing.T) {
	crudFeatureSuite := features.New("ServiceManager Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/servicemanager/create_flow")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

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

			AwaitResourceDeletionOrFail(ctx, t, cfg, sm, wait.WithTimeout(time.Minute*5))

			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, "servicemanager/create_flow", wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}

func TestServiceManagerImport(t *testing.T) {
	importFeatureSuite := features.New("ServiceManager Import Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/servicemanager/import/environment")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				// Wait for the subaccount to be ready before creating ServiceManager
				waitForResource(&v1alpha1.Subaccount{
					ObjectMeta: metav1.ObjectMeta{Name: "sm-import-sa-test", Namespace: cfg.Namespace()},
				}, cfg, t, wait.WithTimeout(15*time.Minute))

				// This will create the external resource but not delete it when we remove the k8s resource
				sm := &v1beta1.ServiceManager{
					ObjectMeta: metav1.ObjectMeta{
						Name:      smImportName + "-create",
						Namespace: cfg.Namespace(),
					},
					Spec: v1beta1.ServiceManagerSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ManagementPolicies: []xpv1.ManagementAction{
								xpv1.ManagementActionObserve,
								xpv1.ManagementActionCreate,
								xpv1.ManagementActionUpdate,
								xpv1.ManagementActionLateInitialize,
							},
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      smImportName + "-create",
								Namespace: cfg.Namespace(),
							},
						},
						ForProvider: v1beta1.ServiceManagerParameters{
							SubaccountRef: &xpv1.Reference{Name: "sm-import-sa-test"},
						},
					},
				}

				err := cfg.Client().Resources().Create(ctx, sm)
				if err != nil {
					t.Errorf("Failed to create ServiceManager for import preparation: %v", err)
				}

				waitForResource(sm, cfg, t, wait.WithTimeout(7*time.Minute))

				createdSM := &v1beta1.ServiceManager{}
				err = cfg.Client().Resources().Get(ctx, smImportName+"-create", cfg.Namespace(), createdSM)
				if err != nil {
					t.Errorf("Failed to get created ServiceManager: %v", err)
				}

				actualExternalName := createdSM.GetAnnotations()["crossplane.io/external-name"]
				t.Logf("Using external name for import: %s", actualExternalName)

				err = cfg.Client().Resources().Delete(ctx, createdSM)
				if err != nil {
					t.Errorf("Failed to delete ServiceManager: %v", err)
				}

				AwaitResourceDeletionOrFail(ctx, t, cfg, createdSM, wait.WithTimeout(time.Minute*2))

				ctx = context.WithValue(ctx, "actualExternalName", actualExternalName)

				return ctx
			},
		).
		Assess(
			"Check Imported ServiceManager gets healthy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				actualExternalName := ctx.Value("actualExternalName").(string)

				sm := &v1beta1.ServiceManager{
					ObjectMeta: metav1.ObjectMeta{
						Name:      smImportName,
						Namespace: cfg.Namespace(),
						Annotations: map[string]string{
							"crossplane.io/external-name": actualExternalName,
						},
					},
					Spec: v1beta1.ServiceManagerSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionObserve},
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      smImportName,
								Namespace: cfg.Namespace(),
							},
						},
						ForProvider: v1beta1.ServiceManagerParameters{
							SubaccountRef: &xpv1.Reference{Name: "sm-import-sa-test"},
						},
					},
				}

				err := cfg.Client().Resources().Create(ctx, sm)
				if err != nil {
					t.Errorf("Failed to create import ServiceManager: %v", err)
				}

				waitForResource(sm, cfg, t)

				importedSM := &v1beta1.ServiceManager{}
				MustGetResource(t, cfg, smImportName, nil, importedSM)

				if importedSM.Status.AtProvider.Status != v1beta1.ServiceManagerBound {
					t.Error("ServiceManager Status not as expected")
				}

				assertServiceManagerSecret(t, ctx, cfg, importedSM)

				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sm := &v1beta1.ServiceManager{}
			MustGetResource(t, cfg, smImportName, nil, sm)

			// Allow resource to be deleted for teardown
			sm.Spec.ResourceSpec.ManagementPolicies = []xpv1.ManagementAction{xpv1.ManagementActionDelete, xpv1.ManagementActionObserve}
			if err := cfg.Client().Resources().Update(ctx, sm); err != nil {
				t.Errorf("Failed to update ServiceManager deletion policy: %v", err)
			}

			resources.AwaitResourceDeletionOrFail(ctx, t, cfg, sm)

			DeleteResourcesIgnoreMissing(ctx, t, cfg, "servicemanager/import/environment", wait.WithTimeout(time.Minute*15))
			return ctx
		},
	).Feature()

	testenv.Test(t, importFeatureSuite)
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
