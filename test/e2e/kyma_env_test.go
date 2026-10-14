//go:build e2e_long

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	kymaModuleClient "github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	kymaModuleImportBindingRefName = "kyma-module-import-binding"
	kymaModuleName                 = "cloud-manager-module"
	kymaModuleExternalName         = "cloud-manager"
	kymaModuleChannelTimeout       = 15 * time.Minute
)

func TestKymaEnvironment(t *testing.T) {
	var manifestDir = crsPath("kyma_env")
	crudFeature := features.New("BTP Kyma Environment and Module Controllers").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, manifestDir)
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())
				return ctx
			},
		).
		Assess(
			"Await resources to become synced",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				if err := resources.WaitForResourcesToBeSynced(ctx, cfg, manifestDir, nil, wait.WithTimeout(time.Minute*50)); err != nil {
					t.Fatal(err)
				}
				return ctx
			},
		).
		Assess(
			"verify a channel update is applied by the remote Kyma cluster",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				clusterScope := ""
				module, err := GetResource(cfg, kymaModuleName, &clusterScope, &v1alpha1.KymaModule{})
				if err != nil {
					t.Fatalf("failed to get KymaModule %q: %v", kymaModuleName, err)
				}

				originalChannel := "regular"
				if module.Spec.ForProvider.Channel != nil {
					originalChannel = *module.Spec.ForProvider.Channel
				}
				if originalChannel != "regular" {
					t.Fatalf("test fixture must start on channel regular, got %q", originalChannel)
				}

				if err := waitForManagedKymaModuleChannel(ctx, cfg, originalChannel, 5*time.Minute); err != nil {
					t.Fatalf("remote Kyma module did not start on %q: %v", originalChannel, err)
				}
				originalKymaSpec, err := assertRemoteKymaModuleChannel(ctx, cfg, originalChannel)
				if err != nil {
					t.Fatalf("unexpected starting channel on remote Kyma: %v", err)
				}

				fastChannel := "fast"
				module.Spec.ForProvider.Channel = &fastChannel
				if err := cfg.Client().Resources().Update(ctx, module); err != nil {
					t.Fatalf("failed to request KymaModule channel update to %q: %v", fastChannel, err)
				}

				if err := waitForManagedKymaModuleChannel(ctx, cfg, fastChannel, kymaModuleChannelTimeout); err != nil {
					t.Fatalf("Kyma did not report module channel %q after the update: %v", fastChannel, err)
				}
				fastKymaSpec, err := assertRemoteKymaModuleChannel(ctx, cfg, fastChannel)
				if err != nil {
					t.Fatalf("channel %q was not applied on the remote Kyma cluster: %v", fastChannel, err)
				}
				expectedFastSpec, err := kymaSpecWithModuleChannel(originalKymaSpec, kymaModuleExternalName, fastChannel)
				if err != nil {
					t.Fatalf("failed to prepare expected Kyma spec after the channel update: %v", err)
				}
				if diff := cmp.Diff(expectedFastSpec, fastKymaSpec); diff != "" {
					t.Fatalf("Kyma spec changed outside the requested module channel (-want +got):\n%s", diff)
				}

				// Restore the fixture value so the test leaves the managed resource
				// converged before the normal teardown deletes the Kyma environment.
				module, err = GetResource(cfg, kymaModuleName, &clusterScope, &v1alpha1.KymaModule{})
				if err != nil {
					t.Fatalf("failed to refresh KymaModule %q before restoring its channel: %v", kymaModuleName, err)
				}
				module.Spec.ForProvider.Channel = &originalChannel
				if err := cfg.Client().Resources().Update(ctx, module); err != nil {
					t.Fatalf("failed to restore KymaModule channel %q: %v", originalChannel, err)
				}
				if err := waitForManagedKymaModuleChannel(ctx, cfg, originalChannel, kymaModuleChannelTimeout); err != nil {
					t.Fatalf("Kyma did not return to module channel %q after restoring it: %v", originalChannel, err)
				}
				restoredKymaSpec, err := assertRemoteKymaModuleChannel(ctx, cfg, originalChannel)
				if err != nil {
					t.Fatalf("restored channel %q was not applied on the remote Kyma cluster: %v", originalChannel, err)
				}
				if diff := cmp.Diff(originalKymaSpec, restoredKymaSpec); diff != "" {
					t.Fatalf("Kyma spec did not return to its original configuration after restoring the channel (-want +got):\n%s", diff)
				}

				return ctx
			},
		).
		Assess(
			"Check Resources Delete",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.DeleteResources(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*50))
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).
		Teardown(resources.DumpManagedResources).
		Feature()

	testenv.Test(t, crudFeature)
}

func assertRemoteKymaModuleChannel(ctx context.Context, cfg *envconf.Config, expected string) (map[string]interface{}, error) {
	specChannel, statusChannel, spec, err := observeRemoteKymaModuleChannels(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if specChannel != expected || statusChannel != expected {
		return nil, fmt.Errorf("remote Kyma reports spec.modules channel %q and status.modules channel %q, want both %q", specChannel, statusChannel, expected)
	}
	return spec, nil
}

func observeRemoteKymaModuleChannels(ctx context.Context, cfg *envconf.Config) (string, string, map[string]interface{}, error) {
	bindingSecret := &corev1.Secret{}
	namespace := "default"
	if err := cfg.Client().Resources().Get(ctx, "kyma-binding", namespace, bindingSecret); err != nil {
		return "", "", nil, fmt.Errorf("failed to get KymaEnvironmentBinding kubeconfig secret: %w", err)
	}

	kubeconfig := bindingSecret.Data[v1alpha1.KymaEnvironmentBindingKey]
	if len(kubeconfig) == 0 {
		return "", "", nil, fmt.Errorf("KymaEnvironmentBinding secret has no %q key", v1alpha1.KymaEnvironmentBindingKey)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to parse KymaEnvironmentBinding kubeconfig: %w", err)
	}

	remoteClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to connect to the remote Kyma cluster: %w", err)
	}

	kyma := &unstructured.Unstructured{}
	kyma.SetGroupVersionKind(kymaModuleClient.GVKKyma)
	if err := remoteClient.Get(ctx, types.NamespacedName{Name: kymaModuleClient.DefaultKymaName, Namespace: kymaModuleClient.DefaultKymaNamespace}, kyma); err != nil {
		return "", "", nil, fmt.Errorf("failed to get Kyma CR from the remote cluster: %w", err)
	}

	spec, _, err := unstructured.NestedMap(kyma.Object, "spec")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read spec from the remote Kyma CR: %w", err)
	}
	specModules, _, err := unstructured.NestedSlice(kyma.Object, "spec", "modules")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read spec.modules from the remote Kyma CR: %w", err)
	}
	specChannel, err := findKymaModuleChannel(specModules, kymaModuleExternalName)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to find the module in remote Kyma spec: %w", err)
	}

	statusModules, _, err := unstructured.NestedSlice(kyma.Object, "status", "modules")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read status.modules from the remote Kyma CR: %w", err)
	}
	statusChannel, err := findKymaModuleChannel(statusModules, kymaModuleExternalName)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to find the module in remote Kyma status: %w", err)
	}

	return specChannel, statusChannel, spec, nil
}

func kymaSpecWithModuleChannel(spec map[string]interface{}, moduleName, channel string) (map[string]interface{}, error) {
	expected := runtime.DeepCopyJSONValue(spec).(map[string]interface{})
	modules, found, err := unstructured.NestedSlice(expected, "modules")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("Kyma spec has no modules field")
	}
	for _, rawModule := range modules {
		module, ok := rawModule.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := module["name"].(string)
		if name == moduleName {
			module["channel"] = channel
			if err := unstructured.SetNestedSlice(expected, modules, "modules"); err != nil {
				return nil, err
			}
			return expected, nil
		}
	}
	return nil, fmt.Errorf("module %q was not found in Kyma spec", moduleName)
}

func findKymaModuleChannel(modules []interface{}, moduleName string) (string, error) {
	for _, rawModule := range modules {
		module, ok := rawModule.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := module["name"].(string)
		if name != moduleName {
			continue
		}
		channel, ok := module["channel"].(string)
		if !ok {
			return "", fmt.Errorf("module %q has no string channel", moduleName)
		}
		return channel, nil
	}
	return "", fmt.Errorf("module %q was not found", moduleName)
}

func waitForManagedKymaModuleChannel(ctx context.Context, cfg *envconf.Config, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	clusterScope := ""
	var lastChannel string
	var lastErr error

	for {
		module, err := GetResource(cfg, kymaModuleName, &clusterScope, &v1alpha1.KymaModule{})
		lastErr = err
		if err == nil {
			lastChannel = module.Status.AtProvider.Channel
			if lastChannel == expected {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for KymaModule status channel %q (last channel: %q, last error: %v)", expected, lastChannel, lastErr)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

func TestKymaEnvironmentImportFlow(t *testing.T) {
	kymaImportName := "e2e-kyma-import-test"

	importTester := NewImportTester(
		&v1alpha1.KymaEnvironment{
			Spec: v1alpha1.KymaEnvironmentSpec{
				ForProvider: v1alpha1.KymaEnvironmentParameters{
					PlanName: "azure",
					Name:     &kymaImportName,
					Parameters: runtime.RawExtension{
						Object: &unstructured.Unstructured{
							Object: map[string]any{
								"region":         "westeurope",
								"administrators": []any{envvar.GetOrPanic(TECHNICAL_USER_EMAIL_ENV_KEY)},
							},
						},
					},
				},
				SubaccountRef: &xpv1.Reference{
					Name: "kyma-import-test-subaccount",
				},
				CloudManagementRef: &xpv1.Reference{
					Name: "cis-local-kyma-import",
				},
			},
		},
		kymaImportName,
		WithWaitDependentResourceTimeout[*v1alpha1.KymaEnvironment](wait.WithTimeout(15*time.Minute)),
		WithWaitCreateTimeout[*v1alpha1.KymaEnvironment](wait.WithTimeout(50*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.KymaEnvironment](wait.WithTimeout(50*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.KymaEnvironment](crsPath("kyma_env_import")),
	)

	importFeature := importTester.BuildTestFeature("BTP Kyma Environment Import Flow").Feature()

	testenv.Test(t, importFeature)
}

// TestKymaModuleImportFlow tests the import flow for KymaModule resource
// according to the External Name Handling ADR.
//
// This test verifies that:
// 1. A KymaModule can be created with its dependencies (KymaEnvironment + KymaEnvironmentBinding)
// 2. The external-name is properly set to the module name (e.g. "cloud-manager")
// 3. The resource can be imported using the external-name annotation
// 4. Imported resources transition to healthy state
func TestKymaModuleImportFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping kyma module import in short mode")
		return
	}

	kymaModuleImportName := "cloud-manager"

	importTester := NewImportTester(
		&v1alpha1.KymaModule{
			Spec: v1alpha1.KymaModuleSpec{
				ForProvider: v1alpha1.KymaModuleParameters{
					Name: kymaModuleImportName,
				},
				KymaEnvironmentBindingRef: &xpv1.Reference{
					Name: kymaModuleImportBindingRefName,
				},
			},
		},
		kymaModuleImportName,
		WithWaitDependentResourceTimeout[*v1alpha1.KymaModule](wait.WithTimeout(60*time.Minute)),
		WithWaitCreateTimeout[*v1alpha1.KymaModule](wait.WithTimeout(15*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.KymaModule](wait.WithTimeout(15*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.KymaModule](crsPath("kyma_module_import")),
	)

	importFeature := importTester.BuildTestFeature("BTP Kyma Module Import Flow").Feature()

	testenv.Test(t, importFeature)
}
