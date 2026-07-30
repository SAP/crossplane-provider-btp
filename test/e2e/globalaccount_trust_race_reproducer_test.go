//go:build e2e

// Reproducer for the intra-run Mode B collision on GlobalaccountTrustConfiguration.
//
// See ~/SAPDevelop/_tracking/crossplane-provider-btp/analysis-globalaccount-trust-inconsistent-result.md
// for the full diagnosis. Short version: when a bare CR (only IdentityProvider set)
// creates the BTP trust config and gets deleted at the K8s level without waiting
// for BTP-side deletion, the leaked BTP config carries the Platform IdP's
// backend-derived Name / Description. A subsequent Create with plan-supplied
// Name / Description hits terraform-plugin-framework's plan-vs-state check and
// fails with "Provider produced inconsistent result after apply", after which
// upjet's prevent_destroy guard wedges the resource.
//
// This file exists to prove that diagnosis is right. When run against a real
// BTP test global account with a Platform-type IdP at $IDP_URL, the test
// SHOULD PASS by asserting the failure signature on the second Create.
//
// Run:
//   make test-acceptance testFilter=TestReproducer_ModeB_IntraRunCollision
//
// Manual cleanup: the test prints the trust configuration's origin on Phase 1
// completion. If Phase 2 wedges, delete the BTP-side config with:
//   btp delete security/trust <origin> --global-account <ga-subdomain>
//
// Env knobs (all optional):
//   REPRO_ITERATIONS       max iterations to attempt (default 3). The test
//                          returns success on the first iteration that reproduces.
//   REPRO_PHASE_GAP_MS     milliseconds to sleep between Phase 1 K8s deletion
//                          and Phase 2 Create (default 0). Larger values give
//                          BTP more time to complete cleanup — use to test
//                          sensitivity of the wedge to cleanup lag.

package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	meta_api "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// inconsistentResultSubstring is the exact substring produced by
// terraform-plugin-framework when the post-apply state diverges from the plan.
// Matches the failure log from CI run 28998384410.
const inconsistentResultSubstring = "Provider produced inconsistent result after apply"

// preventDestroySubstring is the follow-up wedge signature — the resource is
// now stuck because upjet unconditionally injects lifecycle.prevent_destroy = true.
const preventDestroySubstring = "prevent_destroy set, but the plan calls for it to be destroyed"

func TestReproducer_ModeB_IntraRunCollision(t *testing.T) {
	idpURL := envvar.GetOrPanic(IDP_URL_ENV_KEY)
	iterations := envIntOr("REPRO_ITERATIONS", 3)
	phaseGap := envDurationMsOr("REPRO_PHASE_GAP_MS", 0)

	f := features.New("modeb-intra-run-collision").
		WithLabel("kind", "GlobalaccountTrustConfiguration").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Register the API types on the runtime scheme, mirroring what
			// Test_TrustConfiguration_v1alpha1's Setup does.
			r, _ := res.New(cfg.Client().RESTConfig())
			_ = meta_api.AddToScheme(r.GetScheme())
			return ctx
		}).
		Assess("intra-run-collision-reproduces",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				t.Logf("reproducer: idpURL=%s, iterations=%d, phaseGap=%s",
					idpURL, iterations, phaseGap)

				var lastErr error
				for i := 1; i <= iterations; i++ {
					t.Logf("=== iteration %d/%d ===", i, iterations)
					err := runModeBIteration(ctx, t, cfg, idpURL, i, phaseGap)
					if err == nil {
						t.Logf("iteration %d: reproduced Mode B successfully", i)
						return ctx
					}
					lastErr = err
					t.Logf("iteration %d did not reproduce: %v", i, err)
				}
				t.Fatalf("failed to reproduce Mode B after %d iterations: %v",
					iterations, lastErr)
				return ctx
			}).
		Feature()

	testenv.Test(t, f)
}

// TestReproducer_DeleteTiming isolates a single question: after the Crossplane
// provider reports a GlobalaccountTrustConfiguration fully deleted (K8s object
// gone — which, per crossplane-runtime, happens only after a reconcile Observe
// returned ResourceExists=false and the finalizer was removed), is the BTP-side
// trust configuration still present?
//
// It does NOT create a second CR. Its only job is to emit absolute UTC
// timestamps around the delete boundary, marked with a grep-able prefix, so an
// external `btp list security/trust` monitor (run in parallel) can be
// time-correlated against them.
//
// Run:
//   make test-acceptance testFilter=TestReproducer_DeleteTiming
//
// In a second terminal, before starting the test, run the monitor (see
// hack/monitor-btp-trust.sh) targeting the same global account. Then compare:
//   - PROVIDER_DELETE_CONFIRMED timestamp (this test) vs
//   - the last "PRESENT" and first "ABSENT" timestamps (monitor).
// If PRESENT timestamps exist after PROVIDER_DELETE_CONFIRMED, the BTP config
// outlived the provider's delete — confirming eventual-consistency leakage.
//
// The origin is printed as REPRO_ORIGIN=<origin> so the monitor can filter.
// After the test, delete the leftover BTP config manually:
//   btp delete security/trust <origin>
func TestReproducer_DeleteTiming(t *testing.T) {
	idpURL := envvar.GetOrPanic(IDP_URL_ENV_KEY)

	f := features.New("delete-timing").
		WithLabel("kind", "GlobalaccountTrustConfiguration").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r, _ := res.New(cfg.Client().RESTConfig())
			_ = meta_api.AddToScheme(r.GetScheme())
			return ctx
		}).
		Assess("measure-delete-boundary",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				ns := cfg.Namespace()
				client := cfg.Client().Resources()
				name := "repro-delete-timing"

				stamp := func(marker, extra string) {
					t.Logf("REPRO_MARK %s ts=%s %s",
						marker, time.Now().UTC().Format(time.RFC3339Nano), extra)
				}

				cr := &v1alpha1.GlobalaccountTrustConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
					Spec: v1alpha1.GlobalaccountTrustConfigurationSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &xpv1.Reference{Name: "default"},
						},
						ForProvider: v1alpha1.GlobalaccountTrustConfigurationParameters{
							IdentityProvider: &idpURL,
						},
					},
				}

				stamp("CREATE_START", fmt.Sprintf("idp=%s", idpURL))
				if err := client.Create(ctx, cr); err != nil {
					t.Fatalf("create: %v", err)
				}
				if err := wait.For(
					conditions.New(client).ResourceMatch(cr, isReadyTrue),
					wait.WithTimeout(5*time.Minute),
				); err != nil {
					t.Fatalf("did not reach Ready=True: %v", err)
				}

				origin := ""
				if latest, err := getTrust(ctx, client, name, ns); err == nil {
					origin = deref(latest.Status.AtProvider.Origin)
				}
				// Emit the origin on its own line so the monitor can grep it.
				t.Logf("REPRO_ORIGIN=%s", origin)
				stamp("READY", fmt.Sprintf("origin=%s", origin))

				// Begin delete. The K8s object disappears only after the
				// reconciler's Observe reports ResourceExists=false and removes
				// the finalizer — i.e. after terraform destroy reports done.
				stamp("DELETE_START", fmt.Sprintf("origin=%s", origin))
				if err := client.Delete(ctx, cr); err != nil && !apierrors.IsNotFound(err) {
					t.Fatalf("delete: %v", err)
				}
				if err := wait.For(
					conditions.New(client).ResourceDeleted(cr),
					wait.WithTimeout(5*time.Minute),
					wait.WithInterval(2*time.Second),
				); err != nil {
					t.Fatalf("K8s object not deleted in time: %v", err)
				}
				// At this point the provider considers the resource fully gone.
				stamp("PROVIDER_DELETE_CONFIRMED", fmt.Sprintf("origin=%s", origin))

				// Hold the test open briefly so the parallel monitor keeps
				// sampling past the delete boundary. Tunable; default 120s.
				hold := envDurationSecOr("REPRO_HOLD_SEC", 120*time.Second)
				stamp("HOLD_START", fmt.Sprintf("dur=%s origin=%s", hold, origin))
				time.Sleep(hold)
				stamp("HOLD_END", fmt.Sprintf("origin=%s", origin))

				t.Logf("REPRO_MARK NOTE ts=%s manual cleanup if still present: btp delete security/trust %s",
					time.Now().UTC().Format(time.RFC3339Nano), origin)
				return ctx
			}).
		Feature()

	testenv.Test(t, f)
}

// runModeBIteration executes one full attempt:
//
//  1. Phase 1: apply a bare CR (only IdentityProvider set). Wait for Ready=True.
//     Print the observed BTP origin. Delete the K8s object; wait only for the
//     K8s object to disappear, deliberately not for BTP-side deletion.
//  2. Optional REPRO_PHASE_GAP_MS sleep — tune down to hit the race harder.
//  3. Phase 2: apply a full CR (IdentityProvider + Name + Description) for the
//     same $IDP_URL. Poll the Synced condition looking for the exact
//     "inconsistent result" substring within 5 minutes.
//
// Returns nil on successful reproduction (Phase 2 wedged with the expected
// signature). Returns an error if Phase 2 reached Ready=True (no wedge) or if
// the observed error is not the expected signature.
//
// On any exit path (success or failure), the BTP-side trust config is left
// behind by design — cleanup is manual (see file header).
func runModeBIteration(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	idpURL string,
	iter int,
	phaseGap time.Duration,
) error {
	ns := cfg.Namespace()
	client := cfg.Client().Resources()

	phase1Name := fmt.Sprintf("repro-modeb-p1-%d", iter)
	phase2Name := fmt.Sprintf("repro-modeb-p2-%d", iter)

	// Phase 1: bare CR — only IdentityProvider. This mimics
	// TestGlobalaccountTrustConfigurationImportFlow's Phase-1 setup shape.
	phase1 := &v1alpha1.GlobalaccountTrustConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      phase1Name,
			Namespace: ns,
		},
		Spec: v1alpha1.GlobalaccountTrustConfigurationSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: v1alpha1.GlobalaccountTrustConfigurationParameters{
				IdentityProvider: &idpURL,
			},
		},
	}

	t.Logf("phase 1: creating bare CR %s (IdentityProvider only)", phase1Name)
	if err := client.Create(ctx, phase1); err != nil {
		return fmt.Errorf("phase 1 create: %w", err)
	}

	// Wait for Ready=True. If Phase 1 itself fails, the reproducer can't
	// proceed — surface as an iteration error, try again.
	if err := wait.For(
		conditions.New(client).ResourceMatch(phase1, isReadyTrue),
		wait.WithTimeout(5*time.Minute),
	); err != nil {
		return fmt.Errorf("phase 1 did not reach Ready=True: %w", err)
	}

	// Print the origin so a human can `btp delete` if the reproducer wedges.
	if latest, err := getTrust(ctx, client, phase1Name, ns); err == nil {
		t.Logf("phase 1: created BTP trust config; origin=%q, atProvider.Name=%q, atProvider.Description=%q",
			deref(latest.Status.AtProvider.Origin),
			deref(latest.Status.AtProvider.Name),
			deref(latest.Status.AtProvider.Description),
		)
	}

	// Delete the K8s object; wait only for the K8s side to disappear.
	// This deliberately does NOT verify BTP-side deletion — that lag is
	// the whole reason Mode B fires.
	t.Logf("phase 1: deleting K8s object %s (not waiting for BTP)", phase1Name)
	if err := client.Delete(ctx, phase1); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("phase 1 delete: %w", err)
	}
	if err := wait.For(
		conditions.New(client).ResourceDeleted(phase1),
		wait.WithTimeout(3*time.Minute),
	); err != nil {
		return fmt.Errorf("phase 1 K8s cleanup: %w", err)
	}

	if phaseGap > 0 {
		t.Logf("phase gap: sleeping %s before Phase 2", phaseGap)
		time.Sleep(phaseGap)
	}

	// Phase 2: full CR — IdentityProvider + plan-supplied Name + Description.
	// This mimics Test_TrustConfiguration_v1alpha1's fixture.
	planName := fmt.Sprintf("%d-repro-modeb-trust", iter)
	planDesc := "reproducer-custom-description"

	phase2 := &v1alpha1.GlobalaccountTrustConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      phase2Name,
			Namespace: ns,
		},
		Spec: v1alpha1.GlobalaccountTrustConfigurationSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: v1alpha1.GlobalaccountTrustConfigurationParameters{
				IdentityProvider: &idpURL,
				Name:             &planName,
				Description:      &planDesc,
			},
		},
	}

	t.Logf("phase 2: creating full CR %s (name=%q, description=%q)",
		phase2Name, planName, planDesc)
	if err := client.Create(ctx, phase2); err != nil {
		return fmt.Errorf("phase 2 create: %w", err)
	}

	// Poll Synced condition for the exact "inconsistent result" substring.
	// Timeout matches the standard test's 7-minute Assess timeout.
	got, err := waitForSyncedFalseWithMessage(
		ctx, client, phase2, inconsistentResultSubstring, 7*time.Minute,
	)
	if err == nil {
		t.Logf("phase 2: reproduced! Synced=False message:\n  %s", got)
		return nil
	}

	// Fallback signature: after "inconsistent result", subsequent observes
	// switch to the prevent_destroy wedge. If we didn't see the primary
	// signature but did see the wedge, that still counts as reproduction.
	got2, err2 := waitForSyncedFalseWithMessage(
		ctx, client, phase2, preventDestroySubstring, 1*time.Minute,
	)
	if err2 == nil {
		t.Logf("phase 2: reproduced via prevent_destroy wedge; message:\n  %s", got2)
		return nil
	}

	// Neither signature — check whether Phase 2 unexpectedly reached Ready=True.
	// That means BTP cleanup was fast enough this iteration and no divergence
	// occurred. Report as "did not reproduce" so the outer loop can retry.
	latest, gerr := getTrust(ctx, client, phase2Name, ns)
	if gerr == nil && isReadyTrue(latest) {
		return fmt.Errorf("phase 2 reached Ready=True — no wedge this iteration")
	}
	return fmt.Errorf(
		"phase 2 did not surface expected signature within timeout: primary=%v, fallback=%v",
		err, err2,
	)
}

// --- helpers ---

// isReadyTrue returns true when the managed resource has Ready=True.
// Signature matches sigs.k8s.io/e2e-framework's conditions.MatchFunc.
func isReadyTrue(o k8s.Object) bool {
	cr, ok := o.(*v1alpha1.GlobalaccountTrustConfiguration)
	if !ok {
		return false
	}
	c := cr.GetCondition(xpv1.TypeReady)
	return c.Status == corev1.ConditionTrue
}

// waitForSyncedFalseWithMessage polls the given managed resource until its
// Synced condition is False AND its message contains substr, or until timeout.
// Returns the observed message on match, else an error.
func waitForSyncedFalseWithMessage(
	ctx context.Context,
	client *res.Resources,
	obj *v1alpha1.GlobalaccountTrustConfiguration,
	substr string,
	timeout time.Duration,
) (string, error) {
	var observed string
	err := wait.For(
		conditions.New(client).ResourceMatch(obj, func(k k8s.Object) bool {
			cr, ok := k.(*v1alpha1.GlobalaccountTrustConfiguration)
			if !ok {
				return false
			}
			c := cr.GetCondition(xpv1.TypeSynced)
			if c.Status != corev1.ConditionFalse {
				return false
			}
			if strings.Contains(c.Message, substr) {
				observed = c.Message
				return true
			}
			return false
		}),
		wait.WithTimeout(timeout),
		wait.WithInterval(5*time.Second),
	)
	return observed, err
}

// getTrust fetches the current state of a GlobalaccountTrustConfiguration.
func getTrust(
	ctx context.Context,
	client *res.Resources,
	name, ns string,
) (*v1alpha1.GlobalaccountTrustConfiguration, error) {
	out := &v1alpha1.GlobalaccountTrustConfiguration{}
	if err := client.Get(ctx, name, ns, out); err != nil {
		return nil, err
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func envIntOr(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func envDurationMsOr(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms < 0 {
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

func envDurationSecOr(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	s, err := strconv.Atoi(v)
	if err != nil || s < 0 {
		return def
	}
	return time.Duration(s) * time.Second
}
