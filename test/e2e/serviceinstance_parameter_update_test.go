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

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// TestServiceInstance_ParameterUpdate covers two parameter-update regressions:
// #962, a parameters-only update must reach the BTP backend, verified on
// one-mds/sap-integration (a plan with instances_retrievable=true,
// plan_updateable=true that echoes parameters back through
// GetServiceInstanceParameters); and #968, an update the broker rejects must
// hold the resource unhealthy instead of flickering, verified on xsuaa, whose
// xsappname is immutable once provisioned.
func TestServiceInstance_ParameterUpdate(t *testing.T) {
	const (
		siName       = "e2e-si-paramupdate"
		siRejectName = "e2e-si-rejectedupdate"
		smName       = "e2e-sm-si-paramupdate"
	)

	feature := features.New("ServiceInstance parameter updates: applied when accepted (#962), held unhealthy when rejected (#968)").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("serviceinstance_paramupdate"))
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
		Assess(
			"rejected parameter update holds the resource unhealthy (#968)", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// Wait here so xsuaa readiness failures belong to this assessment and do not
				// delay the independent #962 assessment.
				siReject := v1alpha1.ServiceInstance{
					ObjectMeta: metav1.ObjectMeta{Name: siRejectName, Namespace: cfg.Namespace()},
				}
				waitForResource(&siReject, cfg, t, wait.WithTimeout(15*time.Minute))
				if t.Failed() {
					t.FailNow()
				}

				si := MustGetResource(t, cfg, siRejectName, nil, &v1alpha1.ServiceInstance{})

				var params map[string]any
				if err := json.Unmarshal(si.Spec.ForProvider.Parameters.Raw, &params); err != nil {
					t.Fatalf("failed to decode ServiceInstance parameters: %v", err)
				}
				appName, ok := params["xsappname"].(string)
				if !ok {
					t.Fatalf("fixture must carry a string xsappname, got %v", params["xsappname"])
				}
				// xsappname is immutable, so the broker rejects the PATCH.
				params["xsappname"] = appName + "-b"
				raw, err := json.Marshal(params)
				if err != nil {
					t.Fatalf("failed to encode ServiceInstance parameters: %v", err)
				}
				si.Spec.ForProvider.Parameters = runtime.RawExtension{Raw: raw}
				if err := cfg.Client().Resources().Update(ctx, si); err != nil {
					t.Fatalf("failed to update ServiceInstance parameters: %v", err)
				}

				lastAsyncOp := xpv1.ConditionType(ujresource.TypeLastAsyncOperation)

				// Under the bug the lookup key was "/TF-<name>", the Get missed
				// and nothing was ever written. ObservedGeneration pins the
				// condition to the update just made, so a stale one left by an
				// earlier run on a reused cluster is not accepted.
				deadline := time.Now().Add(10 * time.Minute)
				for {
					cur := MustGetResource(t, cfg, siRejectName, nil, &v1alpha1.ServiceInstance{})
					cond := cur.GetCondition(lastAsyncOp)
					if cond.Status == corev1.ConditionFalse && cond.ObservedGeneration == si.Generation {
						if cond.Reason != ujresource.ReasonAsyncUpdateFailure {
							t.Fatalf("expected LastAsyncOperation reason %q, got %q (message %q)",
								ujresource.ReasonAsyncUpdateFailure, cond.Reason, cond.Message)
						}
						t.Logf("broker rejection recorded on the CR: reason=%q message=%q", cond.Reason, cond.Message)
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("rejected update never surfaced on the ServiceInstance (#968 regression): LastAsyncOperation status=%q reason=%q",
							cond.Status, cond.Reason)
					}
					time.Sleep(15 * time.Second)
				}

				// Flicker guard (#967/#968): the rejection must not only be
				// recorded once, it must keep the resource unhealthy. #968 names
				// all three conditions the user reads.
				holdUntil := time.Now().Add(3 * time.Minute)
				for time.Now().Before(holdUntil) {
					time.Sleep(30 * time.Second)
					cur := MustGetResource(t, cfg, siRejectName, nil, &v1alpha1.ServiceInstance{})
					ready := cur.GetCondition(xpv1.TypeReady)
					if ready.Status != corev1.ConditionFalse || string(ready.Reason) != "AsyncOperationFailed" {
						t.Fatalf("expected Ready=False/AsyncOperationFailed while the broker rejection stands, got status=%q reason=%q message=%q",
							ready.Status, ready.Reason, ready.Message)
					}
					if synced := cur.GetCondition(xpv1.TypeSynced); synced.Status != corev1.ConditionFalse {
						t.Fatalf("expected Synced=False while the broker rejection stands, got status=%q reason=%q message=%q",
							synced.Status, synced.Reason, synced.Message)
					}
					if cond := cur.GetCondition(lastAsyncOp); cond.Status != corev1.ConditionFalse {
						t.Fatalf("LastAsyncOperation stopped reporting the rejection: status=%q reason=%q", cond.Status, cond.Reason)
					}
				}
				return ctx
			},
		).
		Teardown(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				si := MustGetResource(t, cfg, siName, nil, &v1alpha1.ServiceInstance{})
				AwaitResourceDeletionOrFail(ctx, t, cfg, si, wait.WithTimeout(time.Minute*10))
				siReject := MustGetResource(t, cfg, siRejectName, nil, &v1alpha1.ServiceInstance{})
				AwaitResourceDeletionOrFail(ctx, t, cfg, siReject, wait.WithTimeout(time.Minute*10))

				// crsPath, not a bare directory name: GetObjectsToImport resolves
				// the path against the working directory and silently matches
				// nothing otherwise, leaving the fixtures behind.
				DeleteResourcesIgnoreMissing(ctx, t, cfg, crsPath("serviceinstance_paramupdate"), wait.WithTimeout(time.Minute*10))
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
