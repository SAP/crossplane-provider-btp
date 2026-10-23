//go:build upgrade

package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
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
// 1. The legacy managed resource is healthy before upgrade and has a complete Secret
// 2. Its external-name is normalized to the compound-key format without changing the credential-name segment
// 3. The existing Secret remains unchanged
// 4. The legacy resource becomes ownership-uncertain and is not silently promoted after upgrade
func Test_SubaccountApiCredential_External_Name(t *testing.T) {
	const sacName = "upgrade-test-extn-sac"

	upgradeTest := NewCustomUpgradeTest("subaccount-api-credential-external-name-test").
		FromVersion(sacFromCustomTag).
		ToVersion(sacToCustomTag).
		WithResourceDirectories(sacCustomResourceDirectories).
		SkipDefaultResourceVerification().
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{
					Name:      sacName,
					Namespace: cfg.Namespace(),
				}}
				r := cfg.Client().Resources()
				if err := wait.For(conditions.New(r).ResourceMatch(sac, func(object k8s.Object) bool {
					got, ok := object.(*securityv1alpha1.SubaccountApiCredential)
					return ok && got.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue && got.GetCondition(xpv1.TypeSynced).Status == corev1.ConditionTrue
				}), wait.WithTimeout(8*time.Minute), wait.WithContext(ctx)); err != nil {
					t.Fatalf("wait for pre-upgrade legacy credential health: %v", err)
				}

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource: %v", err)
				}
				if sac.GetCondition(xpv1.TypeReady).Status != corev1.ConditionTrue || sac.GetCondition(xpv1.TypeSynced).Status != corev1.ConditionTrue {
					t.Fatalf("legacy credential must be healthy before upgrade: Ready=%s Synced=%s", sac.GetCondition(xpv1.TypeReady).Status, sac.GetCondition(xpv1.TypeSynced).Status)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}

				secretRef := sac.GetWriteConnectionSecretToReference()
				if secretRef == nil || secretRef.Name == "" {
					t.Fatal("legacy credential has no connection Secret reference")
				}
				secret := &corev1.Secret{}
				if err := r.Get(ctx, secretRef.Name, secretRef.Namespace, secret); err != nil {
					t.Fatalf("get legacy connection Secret before upgrade: %v", err)
				}
				secretData := make(map[string][]byte, len(secret.Data))
				for key, value := range secret.Data {
					secretData[key] = append([]byte(nil), value...)
				}

				klog.V(4).Infof("Pre-upgrade SubaccountApiCredential external name: %s", externalName)
				ctx = context.WithValue(ctx, "preUpgradeSacSecretName", secretRef.Name)
				ctx = context.WithValue(ctx, "preUpgradeSacSecretNamespace", secretRef.Namespace)
				ctx = context.WithValue(ctx, "preUpgradeSacSecretData", secretData)
				return context.WithValue(ctx, "preUpgradeSacExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{
					Name:      sacName,
					Namespace: cfg.Namespace(),
				}}
				r := cfg.Client().Resources()
				if err := wait.For(conditions.New(r).ResourceMatch(sac, func(object k8s.Object) bool {
					got, ok := object.(*securityv1alpha1.SubaccountApiCredential)
					if !ok || got.Status.Lifecycle == nil || got.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseUncertain {
						return false
					}
					externalName := got.GetAnnotations()["crossplane.io/external-name"]
					parts := strings.SplitN(externalName, "/", 2)
					if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
						return false
					}
					subaccountID := ""
					if got.Spec.ForProvider.SubaccountID != nil {
						subaccountID = *got.Spec.ForProvider.SubaccountID
					}
					if subaccountID == "" && got.Status.AtProvider.SubaccountID != nil {
						subaccountID = *got.Status.AtProvider.SubaccountID
					}
					return subaccountID != "" && parts[0] == subaccountID
				}), wait.WithTimeout(5*time.Minute), wait.WithContext(ctx)); err != nil {
					t.Fatalf("wait for legacy ownership classification and normalized compound external-name: %v", err)
				}

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource after upgrade: %v", err)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade SubaccountApiCredential external name: %s", externalName)

				preUpgradeExternalName, ok := ctx.Value("preUpgradeSacExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				parts := strings.SplitN(externalName, "/", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Fatalf("Post-upgrade external-name %q is not in compound-key format", externalName)
				}
				if parts[1] != preUpgradeExternalName {
					t.Fatalf(
						"Credential-name segment changed during migration: before=%q, after compound=%q",
						preUpgradeExternalName,
						externalName,
					)
				}

				if sac.Status.Lifecycle == nil || sac.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseUncertain {
					t.Fatalf("legacy credential should remain ownership-uncertain until audited resolution; lifecycle=%#v", sac.Status.Lifecycle)
				}
				secretName, nameOK := ctx.Value("preUpgradeSacSecretName").(string)
				secretNamespace, namespaceOK := ctx.Value("preUpgradeSacSecretNamespace").(string)
				if !nameOK || !namespaceOK {
					t.Fatal("could not retrieve pre-upgrade Secret reference")
				}
				wantSecretData, ok := ctx.Value("preUpgradeSacSecretData").(map[string][]byte)
				if !ok {
					t.Fatal("could not retrieve pre-upgrade Secret data")
				}
				secret := &corev1.Secret{}
				if err := r.Get(ctx, secretName, secretNamespace, secret); err != nil {
					t.Fatalf("legacy connection Secret was not preserved after upgrade: %v", err)
				}
				if !reflect.DeepEqual(wantSecretData, secret.Data) {
					t.Fatalf("legacy connection Secret data changed during identity normalization")
				}

				lifecycle := sac.Status.Lifecycle
				if lifecycle.ManagedResourceUID != string(sac.UID) {
					t.Fatalf("uncertain lifecycle is not bound to current MR UID: receipt=%q UID=%q", lifecycle.ManagedResourceUID, sac.UID)
				}
				// Legacy classification intentionally leaves lifecycle identity unset.
				// Resolve against the verified compound external-name above.
				// This isolated upgrade fixture was created by v1.3.0 in a dedicated
				// test subaccount. The marker below is test evidence only; production
				// resolution must use independently retained provisioning/audit records.
				resolvedAt := metav1.NewTime(time.Now().UTC())
				resolvedLifecycle := &securityv1alpha1.SubaccountApiCredentialLifecycleStatus{
					ManagedResourceUID: string(sac.UID),
					SubaccountID:       parts[0],
					Name:               parts[1],
					Phase:              securityv1alpha1.SubaccountApiCredentialPhaseOwned,
					Reason:             "administrator-resolved-legacy",
					ResolvedBy:         "upgrade-test-administrator",
					ResolvedAt:         &resolvedAt,
					ResolutionEvidence: fmt.Sprintf("test-fixture:v1.3.0-create:%s:%s", sac.UID, externalName),
				}
				patchData, err := json.Marshal(map[string]interface{}{"status": map[string]interface{}{"lifecycle": resolvedLifecycle}})
				if err != nil {
					t.Fatalf("marshal audited legacy-resolution test patch: %v", err)
				}
				if err := r.PatchSubresource(ctx, sac, "status", k8s.Patch{PatchType: types.MergePatchType, Data: patchData}); err != nil {
					t.Fatalf("apply audited legacy-resolution status patch: %v", err)
				}

				if err := wait.For(conditions.New(r).ResourceMatch(sac, func(object k8s.Object) bool {
					got, ok := object.(*securityv1alpha1.SubaccountApiCredential)
					return ok && got.Status.Lifecycle != nil &&
						got.Status.Lifecycle.Phase == securityv1alpha1.SubaccountApiCredentialPhaseOwned &&
						got.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue &&
						got.GetCondition(xpv1.TypeSynced).Status == corev1.ConditionTrue
				}), wait.WithTimeout(5*time.Minute), wait.WithContext(ctx)); err != nil {
					t.Fatalf("wait for legacy resolution to resume healthy management: %v", err)
				}
				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("get resolved legacy credential: %v", err)
				}
				resolved := sac.Status.Lifecycle
				if resolved == nil || resolved.Phase != securityv1alpha1.SubaccountApiCredentialPhaseOwned ||
					resolved.ManagedResourceUID != string(sac.UID) || resolved.SubaccountID != parts[0] || resolved.Name != parts[1] ||
					resolved.Reason != "administrator-resolved-legacy" ||
					resolved.ResolvedBy != resolvedLifecycle.ResolvedBy || resolved.ResolvedAt == nil ||
					resolved.ResolutionEvidence != resolvedLifecycle.ResolutionEvidence {
					t.Fatalf("resolved lifecycle receipt did not persist as submitted: %#v", resolved)
				}
				if delta := resolved.ResolvedAt.Time.Sub(resolvedLifecycle.ResolvedAt.Time); delta < -time.Second || delta > time.Second {
					t.Fatalf("resolvedAt changed by more than API timestamp precision: got=%s want=%s", resolved.ResolvedAt, resolvedLifecycle.ResolvedAt)
				}
				if got := sac.GetAnnotations()["crossplane.io/external-name"]; got != externalName {
					t.Fatalf("external-name changed during legacy resolution: got %q, want %q", got, externalName)
				}
				secret = &corev1.Secret{}
				if err := r.Get(ctx, secretName, secretNamespace, secret); err != nil {
					t.Fatalf("legacy connection Secret was not preserved after resolution: %v", err)
				}
				if !reflect.DeepEqual(wantSecretData, secret.Data) {
					t.Fatalf("legacy connection Secret data changed during resolution")
				}

				klog.V(4).Info("Legacy resolution restored healthy management; identity and connection Secret were preserved")
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
