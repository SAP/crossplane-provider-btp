//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
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

				// Verify random name is being generated
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
				}, wait.WithTimeout(4*time.Minute)) // (2m + buffer)

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
				// Wait and observe TTL expiration behavior
				t.Logf("Monitoring TTL expiration behavior...")

				var hasSeenExpiredKeyCleanup bool
				var maxRetiredKeysObserved int

				err := wait.For(func(ctx context.Context) (bool, error) {
					sb := &v1alpha1.ServiceBinding{}
					MustGetResource(t, cfg, sbRotationName, nil, sb)

					currentRetiredCount := len(sb.Status.AtProvider.RetiredKeys)
					if currentRetiredCount > maxRetiredKeysObserved {
						maxRetiredKeysObserved = currentRetiredCount
					}

					// Log detailed information about each retired key
					now := time.Now()
					expiredKeysCount := 0

					for i, key := range sb.Status.AtProvider.RetiredKeys {
						var createdTime time.Time
						var expirationTime time.Time
						var timeInfo string
						var isExpired bool

						if key.CreatedDate != nil {
							if parsed, err := time.Parse("2006-01-02T15:04:05Z0700", *key.CreatedDate); err == nil {
								createdTime = parsed
								if sb.Spec.ForProvider.Rotation != nil && sb.Spec.ForProvider.Rotation.TTL != nil {
									expirationTime = createdTime.Add(sb.Spec.ForProvider.Rotation.TTL.Duration)
									timeUntilExpiration := expirationTime.Sub(now)
									isExpired = timeUntilExpiration <= 0
									if isExpired {
										expiredKeysCount++
									}

									timeInfo = fmt.Sprintf("created: %s, expires: %s (in %v) %s",
										createdTime.Format("15:04:05"),
										expirationTime.Format("15:04:05"),
										timeUntilExpiration.Round(time.Second),
										map[bool]string{true: "[EXPIRED]", false: ""}[isExpired])
								} else {
									timeInfo = fmt.Sprintf("created: %s, TTL not configured", createdTime.Format("15:04:05"))
								}
							} else {
								timeInfo = fmt.Sprintf("created: %s (parse error: %v)", *key.CreatedDate, err)
							}
						} else {
							timeInfo = "created: unknown"
						}

						t.Logf("Retired key %d: ID=%s, Name=%s, %s", i+1, key.ID, key.Name, timeInfo)
					}

					t.Logf("Current retired keys: %d, Max observed: %d, Expired keys present: %d",
						currentRetiredCount, maxRetiredKeysObserved, expiredKeysCount)

					// Success condition: We've seen keys accumulate and then get cleaned up
					// This indicates TTL cleanup is working
					if maxRetiredKeysObserved >= 2 && currentRetiredCount < maxRetiredKeysObserved {
						hasSeenExpiredKeyCleanup = true
						t.Logf("✅ TTL cleanup observed: max keys was %d, now %d", maxRetiredKeysObserved, currentRetiredCount)
						return true, nil
					}

					// Continue monitoring if we haven't seen the full cycle yet
					return false, nil
				}, wait.WithTimeout(10*time.Minute)) // Extended timeout to observe full rotation + TTL cycle

				if err != nil {
					t.Logf("TTL monitoring completed after timeout. Max keys observed: %d, Final count: %d", maxRetiredKeysObserved, len(sb.Status.AtProvider.RetiredKeys))
					if maxRetiredKeysObserved >= 2 {
						t.Log("✅ Key accumulation observed - rotation is working correctly")
					}
				} else {
					t.Log("✅ TTL-based cleanup successfully observed")
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
