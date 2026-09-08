//go:build upgrade

package upgrade

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	accountv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	smPlanNameFromTag = "v1.13.0"
	smPlanNameToTag   = "local"
	// Two cohorts (v1alpha1 + v1beta1 ServiceManager, each with its own subaccount)
	// in one dir so both migrate through the same upgrade in a single cluster.
	smPlanNameResourceDirectories = []string{
		upgradeCRsPath("customCRs/serviceManagerPlanName"),
	}
)

// Test_ServiceManager_PlanName_Upgrade checks that a ServiceManager keeps its
// service plan across the v1.13.0 -> v2 upgrade, for both API versions. The v2
// default planName changed from service-operator-access to subaccount-admin
// (#925), and the two versions resolve planName differently:
//
//   - v1alpha1 has no planName field, so the plan resolves to the runtime
//     DefaultPlanName. An instance created on the old default must not be updated
//     to the new one on upgrade: the service plan is immutable in BTP, so an
//     in-place update calls update_instance and BTP rejects it with
//     "update_instance is not supported", leaving the SM Synced=False. The provider
//     reports up-to-date on a plan-only diff and heals status.dataSourceLookup back
//     to the live plan (#941). This cohort must stay Synced+Ready.
//
//   - v1beta1 pins planName immutable at admission, so it persists and is never
//     re-resolved. This cohort must stay Synced+Ready and keep its plan. Its
//     planName is service-operator-access (v1.13.0's default, not the v2 default),
//     so a regression that re-resolved planName would flip it and fail here.
func Test_ServiceManager_PlanName_Upgrade(t *testing.T) {
	const serviceManagerName = "upgrade-test-planflip-sm"
	const serviceManagerV1beta1Name = "upgrade-test-planflip-sm-v1beta1"

	upgradeTest := NewCustomUpgradeTest("service-manager-planname-test").
		FromVersion(smPlanNameFromTag).
		ToVersion(smPlanNameToTag).
		WithResourceDirectories(smPlanNameResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify both ServiceManagers healthy before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sm := &accountv1alpha1.ServiceManager{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, serviceManagerName, cfg.Namespace(), sm); err != nil {
					t.Fatalf("Failed to get v1alpha1 ServiceManager before upgrade: %v", err)
				}

				// The default resource verification already waited for Ready+Synced;
				// recheck here to confirm the pre-upgrade state is the one we migrate
				// (healthy on the old default plan).
				assertServiceManagerV1alpha1Healthy(t, sm, "before")

				// The v1beta1 SM must also be healthy going in.
				smb := &accountv1beta1.ServiceManager{}
				if err := r.Get(ctx, serviceManagerV1beta1Name, cfg.Namespace(), smb); err != nil {
					t.Fatalf("Failed to get v1beta1 ServiceManager before upgrade: %v", err)
				}
				assertServiceManagerV1beta1Healthy(t, smb, "before")

				klog.V(4).Infof("Pre-upgrade ServiceManagers %q and %q are Synced and Ready", serviceManagerName, serviceManagerV1beta1Name)
				return ctx
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify both ServiceManagers stay healthy and keep their plan after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r := cfg.Client().Resources()

				// The v1alpha1 SM must stay Synced+Ready. A plan re-resolution shows up
				// as Synced=False with the reconcile error "update_instance is not
				// supported". Gate on Synced AND Ready; the pre-upgrade Ready could
				// otherwise mask a post-upgrade Synced=False.
				sm := &accountv1alpha1.ServiceManager{}
				if err := r.Get(ctx, serviceManagerName, cfg.Namespace(), sm); err != nil {
					t.Fatalf("Failed to get v1alpha1 ServiceManager after upgrade: %v", err)
				}
				waitSyncedAndReady(ctx, t, cfg, sm, serviceManagerName, "v1alpha1")

				// status.dataSourceLookup must hold the live instance plan. Read as
				// v1beta1 (storage version) to see the full observation.
				assertPlanMatchesLiveInstance(ctx, t, cfg, serviceManagerName)

				// The v1beta1 SM must stay Synced+Ready and keep its explicit plan.
				smb := &accountv1beta1.ServiceManager{}
				if err := r.Get(ctx, serviceManagerV1beta1Name, cfg.Namespace(), smb); err != nil {
					t.Fatalf("Failed to get v1beta1 ServiceManager after upgrade: %v", err)
				}
				waitSyncedAndReady(ctx, t, cfg, smb, serviceManagerV1beta1Name, "v1beta1")
				assertPlanMatchesLiveInstance(ctx, t, cfg, serviceManagerV1beta1Name)

				klog.V(4).Infof("Post-upgrade: v1alpha1 %q and v1beta1 %q stayed Synced and Ready", serviceManagerName, serviceManagerV1beta1Name)
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}

// waitSyncedAndReady blocks until the object reports Synced=True AND Ready=True,
// failing with the actual Synced condition (naming the cause, not just a timeout)
// if the deadline passes. obj must expose GetCondition (both SM API versions do).
func waitSyncedAndReady(ctx context.Context, t *testing.T, cfg *envconf.Config, obj k8s.Object, name, label string) {
	t.Helper()
	r := cfg.Client().Resources()
	conditioned := func(o k8s.Object) conditionReader { return o.(conditionReader) }
	if err := wait.For(
		conditions.New(r).ResourceMatch(obj, func(o k8s.Object) bool {
			synced := conditioned(o).GetCondition(xpv1.TypeSynced)
			ready := conditioned(o).GetCondition(xpv1.TypeReady)
			return synced.Status == corev1.ConditionTrue && ready.Status == corev1.ConditionTrue
		}),
		wait.WithTimeout(globalVerifyTimeout),
	); err != nil {
		_ = r.Get(ctx, name, cfg.Namespace(), obj)
		synced := conditioned(obj).GetCondition(xpv1.TypeSynced)
		t.Fatalf(
			"%s ServiceManager %q did not stay Synced+Ready after upgrade: %v; Synced=%s reason=%s message=%q",
			label, name, err, synced.Status, synced.Reason, synced.Message,
		)
	}
}

// conditionReader is the subset of both ServiceManager API versions that exposes
// crossplane conditions, so waitSyncedAndReady works for either.
type conditionReader interface {
	GetCondition(xpv1.ConditionType) xpv1.Condition
}

// assertPlanMatchesLiveInstance reads the SM as v1beta1 (storage version) and
// checks status.dataSourceLookup carries a plan ID and the SM is Bound. The live
// instance ID is not queryable from the CR, so this only confirms the plan ID is
// non-empty and the SM stayed bound to its instance.
func assertPlanMatchesLiveInstance(ctx context.Context, t *testing.T, cfg *envconf.Config, name string) {
	t.Helper()
	sm := &accountv1beta1.ServiceManager{}
	if err := cfg.Client().Resources().Get(ctx, name, cfg.Namespace(), sm); err != nil {
		t.Fatalf("Failed to read v1beta1 ServiceManager %q for plan assertion: %v", name, err)
	}
	if sm.Status.AtProvider.DataSourceLookup == nil || sm.Status.AtProvider.DataSourceLookup.ServiceManagerPlanID == "" {
		t.Errorf("ServiceManager %q: expected a persisted dataSourceLookup.serviceManagerPlanID after upgrade, got none", name)
	}
	if sm.Status.AtProvider.Status != accountv1beta1.ServiceManagerBound {
		t.Errorf("ServiceManager %q: status.atProvider.status = %q, want %q (should stay bound to its live instance)",
			name, sm.Status.AtProvider.Status, accountv1beta1.ServiceManagerBound)
	}
}

// assertServiceManagerV1alpha1Healthy checks a v1alpha1 ServiceManager is both
// Synced and Ready. Errorf (not Fatalf) on each condition so a run where both
// drifted reports both.
func assertServiceManagerV1alpha1Healthy(t *testing.T, sm *accountv1alpha1.ServiceManager, phase string) {
	t.Helper()

	synced := sm.GetCondition(xpv1.TypeSynced)
	ready := sm.GetCondition(xpv1.TypeReady)
	if synced.Status != corev1.ConditionTrue {
		t.Errorf("ServiceManager Synced %s upgrade = %s (reason %s, message %q), want True",
			phase, synced.Status, synced.Reason, synced.Message)
	}
	if ready.Status != corev1.ConditionTrue {
		t.Errorf("ServiceManager Ready %s upgrade = %s (reason %s), want True",
			phase, ready.Status, ready.Reason)
	}
	// A bound ServiceManager mirrors both instance and binding IDs in status; if it
	// is bound, the pre-upgrade state is the one we mean to migrate.
	if sm.Status.AtProvider.Status != "" && sm.Status.AtProvider.Status != accountv1alpha1.ServiceManagerBound {
		t.Errorf("ServiceManager status.atProvider.status %s upgrade = %q, want %q or empty",
			phase, sm.Status.AtProvider.Status, accountv1alpha1.ServiceManagerBound)
	}
}

// assertServiceManagerV1beta1Healthy is the v1beta1 counterpart, used for the
// control cohort. Errorf (not Fatalf) per condition so a drift reports fully.
func assertServiceManagerV1beta1Healthy(t *testing.T, sm *accountv1beta1.ServiceManager, phase string) {
	t.Helper()

	synced := sm.GetCondition(xpv1.TypeSynced)
	ready := sm.GetCondition(xpv1.TypeReady)
	if synced.Status != corev1.ConditionTrue {
		t.Errorf("v1beta1 ServiceManager Synced %s upgrade = %s (reason %s, message %q), want True",
			phase, synced.Status, synced.Reason, synced.Message)
	}
	if ready.Status != corev1.ConditionTrue {
		t.Errorf("v1beta1 ServiceManager Ready %s upgrade = %s (reason %s), want True",
			phase, ready.Status, ready.Reason)
	}
	if sm.Status.AtProvider.Status != "" && sm.Status.AtProvider.Status != accountv1beta1.ServiceManagerBound {
		t.Errorf("v1beta1 ServiceManager status.atProvider.status %s upgrade = %q, want %q or empty",
			phase, sm.Status.AtProvider.Status, accountv1beta1.ServiceManagerBound)
	}
}
