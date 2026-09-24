//go:build upgrade

package upgrade

import (
	"context"
	"strings"
	"testing"

	accountv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	smclient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	// v1.11.0 is the newest stable release (no -rc suffix) and already wrote the
	// compound key, so the upgrade must leave the annotation untouched. That makes
	// this a key-stability guard only: it does not exercise the zz_setup.go
	// initializer change, because that initializer fires only on an empty
	// annotation and these fixtures carry a real key by the time it runs. Covering
	// it would mean patching the annotation back to a fallback post-upgrade and
	// polling until the recovery path heals it, which is out of scope here rather
	// than impossible.
	smFromCustomTag             = "v1.11.0"
	smToCustomTag               = "local"
	smCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/serviceManagerExternalName"),
	}
)

// ADR(external-name): ServiceManager's key is the compound
// "<serviceInstanceID>/<serviceBindingID>" and has never changed format, so there
// is no migration to perform. Test_ServiceManager_External_Name pins that:
//
//  1. Before upgrade the annotation is already the two-UUID compound key.
//  2. After upgrade it is byte-identical. Dropping the default initializer and
//     adding format validation must not rewrite or reject an existing key.
//  3. status.atProvider still mirrors both halves, proving the upgraded provider
//     re-observed the same instance and binding rather than losing one.
func Test_ServiceManager_External_Name(t *testing.T) {
	const serviceManagerName = "upgrade-test-extn-sm"

	upgradeTest := NewCustomUpgradeTest("service-manager-external-name-test").
		FromVersion(smFromCustomTag).
		ToVersion(smToCustomTag).
		WithResourceDirectories(smCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sm := &accountv1beta1.ServiceManager{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, serviceManagerName, cfg.Namespace(), sm); err != nil {
					t.Fatalf("Failed to get ServiceManager resource: %v", err)
				}

				annotations := sm.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				// Must not be the crossplane-runtime default (metadata.name); the old
				// provider still ran the default initializer, so a CR stuck on that
				// value never completed its two-phase create.
				if externalName == sm.GetName() {
					t.Fatalf("Pre-upgrade external-name %q equals metadata.name, so the resource never reached a real compound key", externalName)
				}
				assertCompoundServiceManagerKey(t, sm, externalName, "before")

				klog.V(4).Infof("Pre-upgrade ServiceManager external name: %s", externalName)
				return context.WithValue(ctx, "preUpgradeSmExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sm := &accountv1beta1.ServiceManager{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, serviceManagerName, cfg.Namespace(), sm); err != nil {
					t.Fatalf("Failed to get ServiceManager resource after upgrade: %v", err)
				}

				annotations := sm.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}
				assertCompoundServiceManagerKey(t, sm, externalName, "after")

				preUpgradeExternalName, ok := ctx.Value("preUpgradeSmExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}
				if externalName != preUpgradeExternalName {
					t.Fatalf(
						"External name changed during upgrade: before=%q, after=%q (the format never migrated, so it must be preserved verbatim)",
						preUpgradeExternalName,
						externalName,
					)
				}

				klog.V(4).Infof("ServiceManager external name preserved across upgrade: %s", externalName)
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}

// assertCompoundServiceManagerKey checks the ADR compound form and that the
// observed status mirrors both halves of it.
func assertCompoundServiceManagerKey(t *testing.T, smCR *accountv1beta1.ServiceManager, externalName, phase string) {
	t.Helper()

	// The provider's own predicate, not a looser hand-rolled one: internal.IsValidUUID
	// alone admits the braced, urn:uuid: and unhyphenated spellings that the new
	// Observe() rejects, so asserting it would let a key through that the upgraded
	// provider then refuses to reconcile.
	if err := smclient.ValidateExternalName(smCR.GetName(), externalName); err != nil {
		t.Fatalf("Compound external name %q (%s upgrade) is not ADR-conformant: %v", externalName, phase, err)
	}
	// ValidateExternalName also accepts the phase-1 one-segment transient; a
	// fixture that reached Ready must be past that.
	parts := strings.Split(externalName, "/")
	if len(parts) != 2 {
		t.Fatalf(
			"External name %q %s upgrade is not in compound form \"<serviceInstanceID>/<serviceBindingID>\" (%d segments)",
			externalName, phase, len(parts),
		)
	}
	instanceID, bindingID := parts[0], parts[1]

	// Errorf, not Fatalf: the three are independent, so a run where both IDs
	// drifted reports both instead of only the first.
	if smCR.Status.AtProvider.Status != accountv1beta1.ServiceManagerBound {
		t.Errorf("ServiceManager status %s upgrade = %q, want %q", phase, smCR.Status.AtProvider.Status, accountv1beta1.ServiceManagerBound)
	}
	if smCR.Status.AtProvider.ServiceInstanceID != instanceID {
		t.Errorf(
			"status.atProvider.serviceInstanceID %s upgrade = %q, want the first key segment %q",
			phase, smCR.Status.AtProvider.ServiceInstanceID, instanceID,
		)
	}
	if smCR.Status.AtProvider.ServiceBindingID != bindingID {
		t.Errorf(
			"status.atProvider.serviceBindingID %s upgrade = %q, want the second key segment %q",
			phase, smCR.Status.AtProvider.ServiceBindingID, bindingID,
		)
	}
}
