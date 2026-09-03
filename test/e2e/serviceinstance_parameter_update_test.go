//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/e2e-framework/klient/wait"

	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
)

// TestServiceInstance_ParameterUpdate is the regression guard for issue #962:
// a parameters-only update on a ServiceInstance must reach the BTP backend.
//
// It is verified using one-mds/sap-integration (master data integration): a plan
// proven to support in-place parameter updates (instances_retrievable=true,
// plan_updateable=true) and to echo parameters back through
// GetServiceInstanceParameters.
func TestServiceInstance_ParameterUpdate(t *testing.T) {
	const (
		siName = "e2e-si-paramupdate"
		smName = "e2e-sm-si-paramupdate"
	)

	feature := features.New("ServiceInstance parameter-only update reaches backend (#962)").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/serviceinstance_paramupdate")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				si := v1alpha1.ServiceInstance{
					ObjectMeta: metav1.ObjectMeta{Name: siName, Namespace: cfg.Namespace()},
				}
				waitForResource(&si, cfg, t, wait.WithTimeout(15*time.Minute))
				return ctx
			},
		).
		Assess(
			"parameters-only update is applied to the BTP backend", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				si := MustGetResource(t, cfg, siName, nil, &v1alpha1.ServiceInstance{})
				instanceID := si.Status.AtProvider.ID
				if instanceID == "" {
					t.Fatal("ServiceInstance has no backend ID; setup did not complete")
				}

				// Flip businessSystemId probe-1 -> probe-2 (the exact repro from
				// issue #962 / #888-B). Keep enableTenantDeletion so teardown can
				// still delete the one-mds instance afterwards.
				si.Spec.ForProvider.Parameters = runtime.RawExtension{Raw: []byte(`{"businessSystemId":"probe-2","enableTenantDeletion":true}`)}
				if err := cfg.Client().Resources().Update(ctx, si); err != nil {
					t.Fatalf("failed to update ServiceInstance parameters: %v", err)
				}

				smClient := configureServiceManagerAPIClient(t, cfg, MustGetResource(t, cfg, smName, nil, &v1beta1.ServiceManager{}))
				smCfg := smClient.GetConfig()
				paramsURL := fmt.Sprintf("%s://%s/v1/service_instances/%s/parameters", smCfg.Scheme, smCfg.Host, instanceID)

				// Poll the backend, not the CR. Under the bug this never flips
				// and the test fails on timeout; with the fix the Update reaches
				// the broker and the value becomes "probe-2".
				deadline := time.Now().Add(10 * time.Minute)
				for {
					got, err := fetchInstanceParameters(ctx, smCfg.HTTPClient, paramsURL)
					if err == nil && got["businessSystemId"] == "probe-2" {
						return ctx
					}
					if time.Now().After(deadline) {
						t.Fatalf("backend parameters never reflected the update (#962 regression): got %v, err=%v", got, err)
					}
					time.Sleep(15 * time.Second)
				}
			},
		).
		Teardown(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				si := MustGetResource(t, cfg, siName, nil, &v1alpha1.ServiceInstance{})
				AwaitResourceDeletionOrFail(ctx, t, cfg, si, wait.WithTimeout(time.Minute*10))

				DeleteResourcesIgnoreMissing(ctx, t, cfg, "serviceinstance_paramupdate", wait.WithTimeout(time.Minute*10))
				return ctx
			},
		).Feature()

	testenv.Test(t, feature)
}

// fetchInstanceParameters GETs the Service Manager instance-parameters endpoint
// with the client's authed HTTP client and decodes into map[string]any. The
// generated GetServiceInstanceParameters returns map[string]string, which fails
// to unmarshal when a parameter is a bool (e.g. enableTenantDeletion).
func fetchInstanceParameters(ctx context.Context, hc *http.Client, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
