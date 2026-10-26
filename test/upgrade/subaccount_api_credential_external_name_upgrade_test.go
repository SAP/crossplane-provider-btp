//go:build upgrade

package upgrade

import (
	"context"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	sacFromCustomTag             = "v1.3.0"
	sacToCustomTag               = "local"
	sacCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/subaccountExternalName"),
		upgradeCRsPath("customCRs/subaccountApiCredentialExternalName"),
	}
)

// Test_SubaccountApiCredential_External_Name verifies that legacy name-only
// SubaccountApiCredential external-name annotations are migrated to the ADR
// compound key "<subaccount-id>/<name>" during provider upgrades.
// This test ensures that:
// 1. The pre-upgrade external-name exists in the legacy name-only format
// 2. After upgrade, the external-name is migrated to the compound-key format
// 3. The credential-name segment is preserved, so the existing external resource remains targeted
// 4. The resource is held for a human provenance approval after upgrade
func Test_SubaccountApiCredential_External_Name(t *testing.T) {
	const sacName = "upgrade-test-extn-sac"
	// Feature phases share this snapshot; approval must never target a new MR
	// incarnation or a different credential after the provider upgrade.
	var original credentialUpgradeIdentity

	upgradeTest := NewCustomUpgradeTest("subaccount-api-credential-external-name-test").
		FromVersion(sacFromCustomTag).
		ToVersion(sacToCustomTag).
		WithResourceDirectories(sacCustomResourceDirectories).
		// Generic Ready checks would block before the explicit approval step.
		SkipDefaultResourceVerification().
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource: %v", err)
				}
				if err := wait.For(
					conditions.New(r).ResourceMatch(sac, func(object k8s.Object) bool {
						resource := object.(*securityv1alpha1.SubaccountApiCredential)
						return resource.GetCondition(xpv1.TypeSynced).Status == "True" && resource.GetCondition(xpv1.TypeReady).Status == "True"
					}),
					wait.WithTimeout(10*time.Minute),
					wait.WithContext(ctx),
				); err != nil {
					t.Fatalf("SubaccountApiCredential did not become managed by the source provider before upgrade: %v; %s", err, credentialUpgradeDiagnostics(sac))
				}

				var err error
				original, err = captureCredentialUpgradeIdentity(sac)
				if err != nil {
					t.Fatalf("Cannot capture original credential identity: %v; %s", err, credentialUpgradeDiagnostics(sac))
				}
				klog.V(4).Infof("Pre-upgrade credential external name: %s; expected compound identity: %s", original.name, original.externalName())
				return ctx
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				kube := cfg.Client().Resources().GetControllerRuntimeClient()
				key := client.ObjectKey{Name: sacName} // SubaccountApiCredential is cluster-scoped.
				if original.uid == "" {
					t.Fatal("Original credential identity was not captured; refusing approval")
				}

				// Resume only updates pause metadata; it does not wait for a provider
				// reconciliation. Wait for normalization AND the UID-bound approval
				// gate, not Ready=True (which requires the approval below).
				if _, err := waitForCredentialUpgrade(ctx, kube, key, original.uid, 10*time.Minute,
					func(cr *securityv1alpha1.SubaccountApiCredential) bool {
						return credentialAwaitingLegacyApproval(cr, original)
					}); err != nil {
					t.Fatalf("Expected migration to %q and an explicit legacy approval gate: %v", original.externalName(), err)
				}
				klog.V(4).Infof("Post-upgrade credential reached legacy approval gate for %s", original.externalName())

				// This fixture was created and verified under the source provider.
				// Model the explicit operator intervention only after seeing its gate.
				approval, err := approveLegacyUpgradeCredential(ctx, kube, key, original)
				if err != nil {
					t.Fatalf("Failed to apply the verified legacy-approval step: %v", err)
				}
				if _, err := waitForCredentialUpgrade(ctx, kube, key, original.uid, 10*time.Minute,
					func(cr *securityv1alpha1.SubaccountApiCredential) bool {
						return credentialUpgradeReady(cr, original, approval)
					}); err != nil {
					t.Fatalf("Legacy credential did not resume normal reconciliation with its original identity and approval: %v", err)
				}
				klog.V(4).Info("Legacy approval resumed management; normal teardown can clean up the test credential")
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
