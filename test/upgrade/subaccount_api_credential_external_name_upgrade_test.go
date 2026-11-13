//go:build upgrade

package upgrade

import (
	"bytes"
	"context"
	"strings"
	"testing"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type subaccountCredentialUpgradeSnapshot struct {
	externalName    string
	uid             string
	subaccountID    string
	credentialName  string
	secretName      string
	secretNamespace string
	secretData      map[string][]byte
}

var credentialUpgradeSecretKeys = []string{
	"attribute.client_id",
	"attribute.client_secret",
	"attribute.api_url",
	"attribute.token_url",
}

var credentialUpgradeExtraSecretData = map[string][]byte{
	"attribute.read_only":     []byte("synthetic-extra-not-observed-state"),
	"attribute.unknown":       []byte("synthetic-unknown-extra"),
	"attribute.name":          []byte("must-not-retarget"),
	"attribute.subaccount_id": []byte("must-not-retarget-subaccount"),
	"unprefixed-extra":        []byte("preserve-unprefixed-extra"),
}

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
// 4. The resource remains healthy after upgrade
func Test_SubaccountApiCredential_External_Name(t *testing.T) {
	const sacName = "upgrade-test-extn-sac"

	upgradeTest := NewCustomUpgradeTest("subaccount-api-credential-external-name-test").
		FromVersion(sacFromCustomTag).
		ToVersion(sacToCustomTag).
		WithResourceDirectories(sacCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource: %v", err)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}
				if sac.Status.AtProvider.SubaccountID == nil || *sac.Status.AtProvider.SubaccountID == "" {
					t.Fatal("status.atProvider.subaccountId is empty before upgrade")
				}
				if sac.Status.AtProvider.Name == nil || *sac.Status.AtProvider.Name == "" {
					t.Fatal("status.atProvider.name is empty before upgrade")
				}
				if sac.GetWriteConnectionSecretToReference() == nil {
					t.Fatal("write connection Secret reference is missing before upgrade")
				}
				secret := &corev1.Secret{}
				secretRef := sac.GetWriteConnectionSecretToReference()
				if err := r.Get(ctx, secretRef.Name, secretRef.Namespace, secret); err != nil {
					t.Fatalf("get connection Secret before upgrade: %v", err)
				}
				for key, value := range credentialUpgradeExtraSecretData {
					secret.Data[key] = append([]byte(nil), value...)
				}
				if err := r.Update(ctx, secret); err != nil {
					t.Fatalf("seed extra connection Secret data before upgrade: %v", err)
				}
				secretSnapshot := make(map[string][]byte, len(secret.Data))
				for key, value := range secret.Data {
					secretSnapshot[key] = append([]byte(nil), value...)
				}
				for _, key := range credentialUpgradeSecretKeys {
					if len(secretSnapshot[key]) == 0 {
						t.Fatalf("connection Secret is missing nonempty key %q before upgrade", key)
					}
				}

				klog.V(4).Infof("Pre-upgrade SubaccountApiCredential external name: %s", externalName)
				state := subaccountCredentialUpgradeSnapshot{
					externalName:    externalName,
					uid:             string(sac.UID),
					subaccountID:    *sac.Status.AtProvider.SubaccountID,
					credentialName:  *sac.Status.AtProvider.Name,
					secretName:      secretRef.Name,
					secretNamespace: secretRef.Namespace,
					secretData:      secretSnapshot,
				}
				return context.WithValue(ctx, "preUpgradeSacState", state)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource after upgrade: %v", err)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade SubaccountApiCredential external name: %s", externalName)

				preUpgradeState, ok := ctx.Value("preUpgradeSacState").(subaccountCredentialUpgradeSnapshot)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade SubaccountApiCredential state from context")
				}
				preUpgradeExternalName := preUpgradeState.externalName
				if string(sac.UID) != preUpgradeState.uid {
					t.Fatalf("SubaccountApiCredential UID changed during upgrade: before=%q after=%q", preUpgradeState.uid, sac.UID)
				}
				if sac.Status.AtProvider.SubaccountID == nil || *sac.Status.AtProvider.SubaccountID != preUpgradeState.subaccountID {
					t.Fatalf("subaccount identity changed during upgrade: before=%q after=%v", preUpgradeState.subaccountID, sac.Status.AtProvider.SubaccountID)
				}
				if sac.Status.AtProvider.Name == nil || *sac.Status.AtProvider.Name != preUpgradeState.credentialName {
					t.Fatalf("credential name changed during upgrade: before=%q after=%v", preUpgradeState.credentialName, sac.Status.AtProvider.Name)
				}

				parts := strings.SplitN(externalName, "/", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Fatalf("Post-upgrade external-name %q is not in compound-key format", externalName)
				}
				if parts[0] != preUpgradeState.subaccountID || parts[1] != preUpgradeState.credentialName {
					t.Fatalf("compound external name changed identity: before annotation=%q, expected components=%q/%q, after=%q", preUpgradeExternalName, preUpgradeState.subaccountID, preUpgradeState.credentialName, externalName)
				}

				secretRef := sac.GetWriteConnectionSecretToReference()
				if secretRef == nil {
					t.Fatal("write connection Secret reference is missing after upgrade")
				}
				if secretRef.Name != preUpgradeState.secretName || secretRef.Namespace != preUpgradeState.secretNamespace {
					t.Fatalf("connection Secret destination changed during upgrade: before=%s/%s after=%s/%s", preUpgradeState.secretNamespace, preUpgradeState.secretName, secretRef.Namespace, secretRef.Name)
				}
				secret := &corev1.Secret{}
				if err := r.Get(ctx, secretRef.Name, secretRef.Namespace, secret); err != nil {
					t.Fatalf("get connection Secret after upgrade: %v", err)
				}
				beforeSecret := preUpgradeState.secretData
				for _, key := range credentialUpgradeSecretKeys {
					if !bytes.Equal(beforeSecret[key], secret.Data[key]) {
						t.Errorf("connection Secret key %q changed during upgrade", key)
					}
				}
				for key := range credentialUpgradeExtraSecretData {
					if !bytes.Equal(beforeSecret[key], secret.Data[key]) {
						t.Errorf("extra connection Secret key %q changed during upgrade", key)
					}
				}
				if sac.Status.AtProvider.ReadOnly == nil {
					t.Error("typed ReadOnly observation is missing after upgrade")
				}

				klog.V(4).Info("Compound identity and connection data correctly preserved after upgrade")
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
