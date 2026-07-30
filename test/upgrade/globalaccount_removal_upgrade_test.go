//go:build upgrade

package upgrade

import (
	"context"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// Test_GlobalAccount_Removal is the spike for #371 / #161: what happens to an
// existing GlobalAccount CR when the provider is upgraded to a version that no
// longer ships the GlobalAccount CRD?
//
// It runs the exact #371 scenario — install provider A with the CRD, create a
// CR of A, upgrade to a provider build without the CRD — and records what the
// kube-apiserver and Crossplane package manager actually do. The test is
// observational: it logs findings rather than asserting a migration outcome,
// because the outcome is precisely what the spike must discover. See
//   ~/SAPDevelop/_tracking/crossplane-provider-btp/plan-161-remove-globalaccount.md
//
// Design constraints from the framework (custom_upgrade_framework_test.go):
//   - The GlobalAccount CR is NOT passed to WithResourceDirectories(): that set
//     is imported into and verified against BOTH provider versions and torn
//     down at the end. The to-version (this branch) has no GlobalAccount CRD,
//     so a shared fixture would break post-upgrade VerifyResources + teardown.
//     Instead we create the CR manually in a pre-upgrade assessment.
//   - SkipDefaultResourceVerification() is required: the default "resources
//     become healthy" assertion is meaningless once the CRD is dropped.
//   - The CR is deleted defensively in a post-upgrade assessment (tolerating a
//     kind that is no longer served), since there is no shared directory for
//     the framework teardown to clean.
const gaRemovalCRName = "upgrade-test-ga-removal"

var (
	// from = the last released version (still ships the deprecated GlobalAccount
	//        CRD), matching what a real upgrading customer runs.
	// to   = this branch (local build) where GlobalAccount is removed.
	gaRemovalFromTag = "v1.11.0"
	gaRemovalToTag   = "local"
)

func Test_GlobalAccount_Removal(t *testing.T) {
	findings := map[string]string{}

	upgradeTest := NewCustomUpgradeTest("globalaccount-removal-spike").
		FromVersion(gaRemovalFromTag).
		ToVersion(gaRemovalToTag).
		WithResourceDirectories([]string{}).
		SkipDefaultResourceVerification().
		WithVerifyTimeout(10 * time.Minute).
		WithCustomPreUpgradeAssessment(
			"create GlobalAccount CR against from-version schema",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r := cfg.Client().Resources()

				ga := newGlobalAccountCR()
				if err := r.Create(ctx, ga); err != nil {
					t.Fatalf("create GlobalAccount CR (from-version): %v", err)
				}

				// Wait until observed Ready so we upgrade from an established CR.
				got := &accountv1alpha1.GlobalAccount{}
				werr := waitFor(ctx, 5*time.Minute, func() (bool, error) {
					if err := r.Get(ctx, gaRemovalCRName, "", got); err != nil {
						return false, nil
					}
					return got.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue, nil
				})
				if werr != nil {
					t.Logf("GlobalAccount CR not Ready before upgrade (continuing): %v", werr)
				}
				klog.V(4).Infof("[371] pre-upgrade GlobalAccount CR ready=%v",
					got.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue)
				return ctx
			},
		).
		WithCustomPostUpgradeAssessment(
			"observe CRD + orphaned CR after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r := cfg.Client().Resources()

				// 1. Is the GlobalAccount kind still served by the apiserver?
				list := &accountv1alpha1.GlobalAccountList{}
				listErr := r.List(ctx, list)
				crdServed := listErr == nil
				if listErr != nil && meta.IsNoMatchError(listErr) {
					crdServed = false
				}
				findings["crd_still_served"] = boolStr(crdServed)

				// 2. Is the previously-created CR still stored (orphaned) or gone?
				got := &accountv1alpha1.GlobalAccount{}
				getErr := r.Get(ctx, gaRemovalCRName, "", got)
				switch {
				case getErr == nil:
					findings["cr_state"] = "still stored (orphaned), retrievable"
				case meta.IsNoMatchError(getErr):
					findings["cr_state"] = "kind no longer served (no REST mapping)"
				case apierrors.IsNotFound(getErr):
					findings["cr_state"] = "not found, removed"
				default:
					findings["cr_state"] = "unknown: " + getErr.Error()
				}
				klog.V(4).Infof("[371] post-upgrade crd_served=%s cr_state=%s",
					findings["crd_still_served"], findings["cr_state"])

				// 3. Defensive cleanup of the orphaned CR (best effort).
				if getErr == nil {
					if delErr := r.Delete(ctx, got); delErr != nil {
						t.Logf("cleanup: delete orphaned GlobalAccount CR: %v", delErr)
					}
				}
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())

	klog.Infof("[371] SPIKE FINDINGS: crd_still_served=%s cr_state=%s",
		findings["crd_still_served"], findings["cr_state"])
}

// newGlobalAccountCR builds a GlobalAccount CR wired to the "default"
// ProviderConfig that the framework creates. external-name points at the real
// global account subdomain from the GLOBAL_ACCOUNT env var (loaded in TestMain).
func newGlobalAccountCR() *accountv1alpha1.GlobalAccount {
	ga := &accountv1alpha1.GlobalAccount{}
	ga.SetGroupVersionKind(accountv1alpha1.GlobalAccountGroupVersionKind)
	ga.SetName(gaRemovalCRName)
	ga.SetAnnotations(map[string]string{
		"crossplane.io/external-name": globalAccount,
	})
	ga.Spec.ResourceSpec.ProviderConfigReference = &xpv1.Reference{Name: "default"}
	return ga
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// waitFor polls fn until it returns true or the timeout elapses.
func waitFor(ctx context.Context, timeout time.Duration, fn func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := fn()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}
