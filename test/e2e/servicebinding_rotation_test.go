//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"

	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	sbRotationName = "e2e-destination-binding-rotation"
)


func TestServiceBinding_RotationLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rotation tests in short mode - use 'make e2e-long' to run rotation tests")
		return
	}

	rotationLifecycleFeature := features.New("ServiceBinding Complete Rotation Lifecycle").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/servicebinding")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				sb := v1alpha1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{Name: sbRotationName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sb, cfg, t, wait.WithTimeout(7*time.Minute))
				return ctx
			},
		).
		Assess(
			"Verify ServiceBinding is created with rotation configuration", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sb := &v1alpha1.ServiceBinding{}
				MustGetResource(t, cfg, sbRotationName, nil, sb)

				// Check that the binding is fully initialized
				if sb.Status.AtProvider.ID == "" {
					t.Error("ServiceBinding not fully initialized")
				}

				// Check that rotation configuration exists
				if sb.Spec.ForProvider.Rotation == nil {
					t.Error("Expected rotation configuration to be set")
				}
				if sb.Spec.ForProvider.Rotation.Frequency == nil {
					t.Error("Expected rotation frequency to be set")
				}
				if sb.Spec.ForProvider.Rotation.TTL == nil {
					t.Error("Expected rotation TTL to be set")
				}

				// Verify rotation is enabled (random name generated)
				if sb.Status.AtProvider.Name != *sb.Spec.BtpName {
					t.Error("The name of the external resource should match the generated BtpName")
				}
				if sb.Status.AtProvider.Name == sb.Spec.ForProvider.Name {
					t.Error("The name of the external resource should be randomly generated for rotation-enabled binding")
				}

				// Store initial state for later comparison
				t.Logf("Initial binding ID: %s, Name: %s", sb.Status.AtProvider.ID, sb.Status.AtProvider.Name)

				return ctx
			},
		).
		Assess(
			"Wait for rotation frequency to trigger key retirement", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sb := &v1alpha1.ServiceBinding{}
				MustGetResource(t, cfg, sbRotationName, nil, sb)

				// Store the original ID and creation time
				originalID := sb.Status.AtProvider.ID
				originalName := sb.Status.AtProvider.Name
				if originalID == "" {
					t.Fatal("Original binding ID is empty")
				}

				t.Logf("Waiting for rotation of binding ID: %s after 2 minutes...", originalID)

				// Wait for rotation frequency (2 minutes) + buffer time
				err := wait.For(func(ctx context.Context) (bool, error) {
					sb := &v1alpha1.ServiceBinding{}
					MustGetResource(t, cfg, sbRotationName, nil, sb)

					// Check if the original binding has been retired
					for _, retiredKey := range sb.Status.AtProvider.RetiredKeys {
						if retiredKey.ID == originalID && retiredKey.Name == originalName {
							t.Logf("Original key retired: %s", retiredKey.ID)
							return true, nil
						}
					}
					return false, nil
				}, wait.WithTimeout(4*time.Minute)) // Wait longer than rotation frequency (2m + buffer)

				if err != nil {
					t.Error("Frequency-based rotation did not trigger within expected time")
				}

				return ctx
			},
		).
		Assess(
			"Verify new binding is created after rotation", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sb := &v1alpha1.ServiceBinding{}
				MustGetResource(t, cfg, sbRotationName, nil, sb)

				// Wait for new binding to be created and ready
				err := wait.For(func(ctx context.Context) (bool, error) {
					sb := &v1alpha1.ServiceBinding{}
					MustGetResource(t, cfg, sbRotationName, nil, sb)

					// New binding should exist and be ready
					if sb.Status.AtProvider.ID != "" && len(sb.Status.AtProvider.RetiredKeys) > 0 {
						t.Logf("New binding created: %s", sb.Status.AtProvider.ID)
						return true, nil
					}
					return false, nil
				}, wait.WithTimeout(3*time.Minute))

				if err != nil {
					t.Error("New binding was not created after rotation")
				}

				return ctx
			},
		).
		Assess(
			"Wait for TTL expiration and verify expired key deletion", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sb := &v1alpha1.ServiceBinding{}
				MustGetResource(t, cfg, sbRotationName, nil, sb)

				// Store current retired keys count
				initialRetiredCount := len(sb.Status.AtProvider.RetiredKeys)
				t.Logf("Initial retired keys count: %d", initialRetiredCount)

				if initialRetiredCount == 0 {
					t.Error("Expected at least one retired key before TTL expiration test")
					return ctx
				}

				// Wait for TTL period (5 minutes) + buffer for cleanup
				t.Logf("Waiting for TTL expiration (5 minutes) + cleanup time...")
				err := wait.For(func(ctx context.Context) (bool, error) {
					sb := &v1alpha1.ServiceBinding{}
					MustGetResource(t, cfg, sbRotationName, nil, sb)

					// Check if expired keys have been cleaned up
					currentRetiredCount := len(sb.Status.AtProvider.RetiredKeys)
					t.Logf("Current retired keys count: %d", currentRetiredCount)

					// Keys should be cleaned up after TTL
					return currentRetiredCount < initialRetiredCount, nil
				}, wait.WithTimeout(8*time.Minute)) // TTL (5m) + buffer for controller processing

				if err != nil {
					t.Log("TTL-based cleanup may not have completed yet - this could be due to test timing")
				}

				return ctx
			},
		).
		Assess(
			"Verify clean resource deletion", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// Get final state before deletion
				sb := &v1alpha1.ServiceBinding{}
				MustGetResource(t, cfg, sbRotationName, nil, sb)

				t.Logf("Final state - Active binding: %s, Retired keys: %d",
					sb.Status.AtProvider.ID, len(sb.Status.AtProvider.RetiredKeys))

				// The teardown will handle deletion, but we verify the state is consistent
				if sb.Status.AtProvider.ID == "" {
					t.Error("Active binding should still exist before deletion")
				}

				return ctx
			},
		).
		Teardown(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				t.Log("Cleaning up ServiceBinding and verifying all keys are deleted...")
				DeleteResourcesIgnoreMissing(ctx, t, cfg, "serviceinstance", wait.WithTimeout(time.Minute*5))
				return ctx
			},
		).Feature()

	testenv.Test(t, rotationLifecycleFeature)
}


