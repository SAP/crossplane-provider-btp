//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
// Design constraints:
//   - The GlobalAccount Go type is DELETED from apis/ on this branch (the "to"
//     version). The test therefore cannot import accountv1alpha1.GlobalAccount —
//     it would not compile. Instead the CR is built as an *unstructured.Unstructured
//     with the GVK baked into local constants below, so the production code stays
//     clean and the test still exercises the from-version CRD at runtime.
//   - The CR is NOT passed to WithResourceDirectories(): that set is imported
//     into and verified against BOTH provider versions and torn down at the end.
//     The to-version has no GlobalAccount CRD, so a shared fixture would break
//     post-upgrade VerifyResources + teardown. We create the CR manually in a
//     pre-upgrade assessment.
//   - SkipDefaultResourceVerification() is required: the default "resources
//     become healthy" assertion is meaningless once the CRD is dropped.
//   - The CR is deleted defensively post-upgrade (tolerating a kind that is no
//     longer served), since there is no shared directory for teardown to clean.
const gaRemovalCRName = "upgrade-test-ga-removal"

var (
	// from = the last released version (still ships the deprecated GlobalAccount
	//        CRD), matching what a real upgrading customer runs.
	// to   = this branch (local build) where GlobalAccount is removed.
	gaRemovalFromTag = "v1.11.0"
	gaRemovalToTag   = "local"

	// GlobalAccount GVK, baked in locally because the Go type no longer exists
	// on this branch. Mirrors what apis/account/v1alpha1 exposed before removal.
	gaGVK = schema.GroupVersionKind{
		Group:   "account.btp.sap.crossplane.io",
		Version: "v1alpha1",
		Kind:    "GlobalAccount",
	}
	gaListGVK = schema.GroupVersionKind{
		Group:   "account.btp.sap.crossplane.io",
		Version: "v1alpha1",
		Kind:    "GlobalAccountList",
	}
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

				if err := r.Create(ctx, newGlobalAccountCR()); err != nil {
					t.Fatalf("create GlobalAccount CR (from-version): %v", err)
				}

				// We do NOT gate on the CR reaching Ready before upgrade. The
				// from-version provider (v1.11.0) is a prebuilt image we cannot
				// patch, and its GlobalAccount Observe fails to decode the live
				// accounts-service response (globalAccountGUID wrongly marked
				// required in that release's vendored client), so the CR stays
				// Synced=False there. That is a pre-existing bug in the released
				// image, unrelated to the CRD-removal question this spike tests:
				// orphaning behaviour depends only on the CR EXISTING at upgrade
				// time, not on it being Ready. We just confirm it is stored, then
				// log its conditions once for the record.
				got := newGlobalAccountObj()
				if err := r.Get(ctx, gaRemovalCRName, "", got); err != nil {
					t.Fatalf("GlobalAccount CR not retrievable after create: %v", err)
				}
				klog.V(4).Infof("[371] pre-upgrade GlobalAccount stored; conditions: %s", formatConditions(got))
				return ctx
			},
		).
		WithCustomPostUpgradeAssessment(
			"observe CRD + orphaned CR after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r := cfg.Client().Resources()

				// 1. Is the GlobalAccount kind still served by the apiserver?
				list := &unstructured.UnstructuredList{}
				list.SetGroupVersionKind(gaListGVK)
				listErr := r.List(ctx, list)
				crdServed := listErr == nil
				if listErr != nil && meta.IsNoMatchError(listErr) {
					crdServed = false
				}
				findings["crd_still_served"] = boolStr(crdServed)

				// 2. Is the previously-created CR still stored (orphaned) or gone?
				got := newGlobalAccountObj()
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

				// 3. Can the user still DELETE the orphaned CR once its controller
				//    is gone? A finalizer left by the old controller could block
				//    removal (nothing runs to clear it). Issue the delete, then
				//    poll: does the CR actually disappear, or hang Terminating?
				//    This answers #371's "what can you recommend to customer" item.
				if getErr == nil {
					fins := got.GetFinalizers()
					findings["cr_finalizers"] = strings.Join(fins, ",")
					if len(fins) == 0 {
						findings["cr_finalizers"] = "(none)"
					}

					if delErr := r.Delete(ctx, got); delErr != nil {
						findings["cr_delete"] = "delete call failed: " + delErr.Error()
					} else {
						// Poll up to 60s for the object to actually be gone.
						deleted := false
						deadline := 12
						for i := 0; i < deadline; i++ {
							probe := newGlobalAccountObj()
							e := r.Get(ctx, gaRemovalCRName, "", probe)
							if apierrors.IsNotFound(e) {
								deleted = true
								break
							}
							time.Sleep(5 * time.Second)
						}
						if deleted {
							findings["cr_delete"] = "deleted cleanly"
						} else {
							findings["cr_delete"] = "STILL PRESENT after delete (finalizer blocks removal)"
						}
					}
					klog.V(4).Infof("[371] orphan delete: finalizers=%s result=%s",
						findings["cr_finalizers"], findings["cr_delete"])
				}
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())

	klog.Infof("[371] SPIKE FINDINGS: crd_still_served=%s cr_state=%s cr_finalizers=%s cr_delete=%s",
		findings["crd_still_served"], findings["cr_state"], findings["cr_finalizers"], findings["cr_delete"])
}

// newGlobalAccountObj returns an empty unstructured object typed as GlobalAccount,
// ready to be filled by a client Get.
func newGlobalAccountObj() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gaGVK)
	return u
}

// newGlobalAccountCR builds a GlobalAccount CR wired to the "default"
// ProviderConfig that the framework creates. external-name points at the real
// global account subdomain from the GLOBAL_ACCOUNT env var (loaded in TestMain).
func newGlobalAccountCR() *unstructured.Unstructured {
	u := newGlobalAccountObj()
	u.SetName(gaRemovalCRName)
	u.SetAnnotations(map[string]string{
		"crossplane.io/external-name": globalAccount,
	})
	// The from-version GlobalAccountParameters is empty; only providerConfigRef
	// is meaningful on spec.
	_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{
		"providerConfigRef": map[string]interface{}{"name": "default"},
	}, "spec")
	return u
}

// formatConditions renders status.conditions as "Type=Status(Reason): Message; ..."
// so a non-Ready CR's reconcile error is visible in the test log.
func formatConditions(u *unstructured.Unstructured) string {
	conds, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if err != nil || !found || len(conds) == 0 {
		return "(no conditions)"
	}
	parts := make([]string, 0, len(conds))
	for _, c := range conds {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		s := fmt.Sprintf("%v=%v(%v)", m["type"], m["status"], m["reason"])
		if msg, _ := m["message"].(string); msg != "" {
			s += ": " + msg
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
