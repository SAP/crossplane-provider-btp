//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	crossplane_meta "github.com/crossplane/crossplane-runtime/pkg/meta"
	meta_api "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/internal"
	saas_client "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-saas-provisioning-api-go/pkg"
	"sigs.k8s.io/e2e-framework/klient/wait"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	subscriptionCreateName = "sub-test"

	subscriptionImportName         = "sub-import-test"
	subscriptionCisImportName      = "e2e-sub-import-cis-local"
	subscriptionImportExternalName = "auditlog-viewer/free"
)

func TestSubscriptionCRUDFlow(t *testing.T) {
	t.Parallel()
	crudFeatureSuite := features.New("Subscription Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/subscription/create_flow")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta_api.AddToSchemeConcurrent(r.GetScheme())

				cm := v1alpha1.Subscription{
					ObjectMeta: metav1.ObjectMeta{Name: subscriptionCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&cm, cfg, t, wait.WithTimeout(15*time.Minute))
				return ctx
			},
		).
		Assess(
			"Check external Subscription is truly created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sub := &v1alpha1.Subscription{}
				MustGetResource(t, cfg, subscriptionCreateName, nil, sub)

				cis := &v1alpha1.CloudManagement{}
				MustGetResource(t, cfg, sub.Spec.CloudManagementSecret, internal.Ptr(sub.Spec.CloudManagementSecretNamespace), cis)

				apiClient := configureSaasProvisioningAPIClient(t, cfg, cis)
				assertSubscriptionAPIExists(t, apiClient, sub, true)
				return ctx
			},
		).
		Assess(
			"Check Updates are rejected", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sub := &v1alpha1.Subscription{}
				MustGetResource(t, cfg, subscriptionCreateName, nil, sub)

				changes := sub.DeepCopy()
				changes.Spec.ForProvider.AppName = "sapappstudio"
				changes.Spec.ForProvider.PlanName = "standard-edition"

				// changes should be rejected by K8s validation rules annotated in _types
				err := cfg.Client().Resources().Update(ctx, changes)
				if err == nil {
					t.Fatal("Expected validation error")
				}
				if !strings.Contains(err.Error(), "appName can't be updated once set") {
					t.Fatal("Expected validation error on appName")
				}
				if !strings.Contains(err.Error(), "planName can't be updated once set") {
					t.Fatal("Expected validation error on planName")
				}

				return ctx
			},
		).
		Assess(
			"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// k8s resource cleaned up?
				sub := &v1alpha1.Subscription{}
				MustGetResource(t, cfg, subscriptionCreateName, nil, sub)

				AwaitResourceDeletionOrFail(ctx, t, cfg, sub, wait.WithTimeout(time.Minute*10))

				// all external resources deleted?
				cis := &v1alpha1.CloudManagement{}
				MustGetResource(t, cfg, sub.Spec.CloudManagementSecret, internal.Ptr(sub.Spec.CloudManagementSecretNamespace), cis)

				apiClient := configureSaasProvisioningAPIClient(t, cfg, cis)
				assertSubscriptionAPIExists(t, apiClient, sub, false)

				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, "subscription/create_flow", wait.WithTimeout(time.Minute*15))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}

func TestSubscriptionImport(t *testing.T) {
	t.Parallel()
	crudFeatureSuite := features.New("Subscription Import Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/subscription/import/environment")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta_api.AddToSchemeConcurrent(r.GetScheme())

				// The local cloudmanagement instance is the requirement for the next steps, so we wait for it to be healthy
				waitForResource(&v1alpha1.CloudManagement{
					ObjectMeta: metav1.ObjectMeta{Name: subscriptionCisImportName, Namespace: cfg.Namespace()},
				}, cfg, t, wait.WithTimeout(15*time.Minute))

				cis := &v1alpha1.CloudManagement{}
				MustGetResource(t, cfg, subscriptionCisImportName, nil, cis)

				apiClient := configureSaasProvisioningAPIClient(t, cfg, cis)
				createSubscriptionAPI(t, apiClient, subscriptionImportExternalName)

				return ctx
			},
		).
		Assess(
			"Check Imported Subscription gets healthy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/subscription/import/resource")
				waitForResource(&v1alpha1.Subscription{
					ObjectMeta: metav1.ObjectMeta{Name: subscriptionImportName, Namespace: cfg.Namespace()},
				}, cfg, t)

				sub := &v1alpha1.Subscription{}
				MustGetResource(t, cfg, subscriptionImportName, nil, sub)

				if sub.Status.AtProvider.State == nil || *sub.Status.AtProvider.State != v1alpha1.SubscriptionStateSubscribed {
					t.Error("Subscription State not as expected")
				}
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sub := &v1alpha1.Subscription{}
			MustGetResource(t, cfg, subscriptionImportName, nil, sub)

			resources.AwaitResourceDeletionOrFail(ctx, t, cfg, sub)

			DeleteResourcesIgnoreMissing(ctx, t, cfg, "subscription/import/environment", wait.WithTimeout(time.Minute*15))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}

func assertSubscriptionAPIExists(t *testing.T, apiClient *saas_client.APIClient, cr *v1alpha1.Subscription, expectExist bool) {
	externalName := crossplane_meta.GetExternalName(cr)
	fragments := strings.Split(externalName, "/")
	request := apiClient.SubscriptionOperationsForAppConsumersAPI.GetEntitledApplication(context.TODO(), fragments[0]).PlanName(fragments[1])
	response, _, err := request.Execute()
	if err != nil {
		t.Errorf("Cannot verify existitance of subscription instance over API")
	}
	if expectExist && (response == nil || *response.State != v1alpha1.SubscriptionStateSubscribed) {
		t.Errorf("Error verifying existing instance")
	}
	if !expectExist && response != nil && *response.State != v1alpha1.SubscriptionStateNotSubscribed {
		t.Errorf("Error verifying not existing instance")
	}
}

func createSubscriptionAPI(t *testing.T, apiClient *saas_client.APIClient, externalName string) {
	fragments := strings.Split(externalName, "/")
	_, err := apiClient.SubscriptionOperationsForAppConsumersAPI.
		CreateSubscriptionAsync(context.TODO(), fragments[0]).
		CreateSubscriptionRequestPayload(saas_client.CreateSubscriptionRequestPayload{PlanName: internal.Ptr(fragments[1])}).
		Execute()

	if err != nil {
		t.Errorf("Cannot create subscription over API")
	}
}
