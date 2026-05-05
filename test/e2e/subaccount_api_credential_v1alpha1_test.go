//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	xpmeta "github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sobj "sigs.k8s.io/e2e-framework/klient/k8s"
	kres "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	sacCreateName = "sac-subaccountapicredentials"
)

func TestSubaccountApiCredentialsStandalone(t *testing.T) {
	var manifestDir = "testdata/crs/SubaccountApiCredentialsStandalone"
	crudFeature := features.New("SubaccountApiCredentials Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, manifestDir)
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				sac := v1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Name: sacCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sac, cfg, t, wait.WithTimeout(time.Minute*7))
				return ctx
			},
		).
		Assess(
			"Await resources to become synced",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacCreateName, nil, sac)

				assertApiCredentialSecret(t, ctx, cfg, sac)

				return ctx
			},
		).
		Assess(
			"Check Resources Delete",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// k8s resource cleaned up?
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacCreateName, nil, sac)

				AwaitResourceDeletionOrFail(ctx, t, cfg, sac, wait.WithTimeout(time.Minute*5))
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeature)
}

func assertApiCredentialSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, sac *v1alpha1.SubaccountApiCredential) {
	secretName := sac.GetWriteConnectionSecretToReference().Name
	secretNS := sac.GetWriteConnectionSecretToReference().Namespace
	secret := &corev1.Secret{}
	err := cfg.Client().Resources().Get(ctx, secretName, secretNS, secret)
	if err != nil {
		t.Error("Error while loading expected secret from Ref")
	}
	// secret contains correct structure
	if _, ok := secret.Data["attribute.client_secret"]; !ok {
		t.Error("Secret not in proper format")
	}
}

// TestSubaccountApiCredentialOrphanImportFails verifies that importing an orphaned
// SubaccountApiCredential results in a connection secret with an empty/missing client_secret.
// BTP only returns client_secret at creation time — a Terraform read succeeds but does not
// return the secret, so the connection secret will not contain a usable client_secret.
// This is expected and documented behavior (ADR exception).
// See: https://github.com/SAP/crossplane-provider-btp/issues/553
func TestSubaccountApiCredentialOrphanImportFails(t *testing.T) {
	var orphanManifestDir = "testdata/crs/SubaccountApiCredentialOrphanImport"
	var orphanSacName = "sac-orphan-import"
	importName := NewID("sac-orphan-reimport", BUILD_ID)

	orphanImportFeature := features.New("SubaccountApiCredential Orphan Import Has Missing Client Secret").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r, _ := kres.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				// Create the credential via crossplane using its own subaccount (no conflict with Standalone test)
				resources.ImportResources(ctx, t, cfg, orphanManifestDir)
				sac := &v1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Name: orphanSacName, Namespace: cfg.Namespace()},
				}
				waitForResource(sac, cfg, t, wait.WithTimeout(time.Minute*7))

				// Read back to get the external name (credential name, not a GUID — ADR exception)
				MustGetResource(t, cfg, orphanSacName, nil, sac)
				externalName := xpmeta.GetExternalName(sac)
				ctx = context.WithValue(ctx, importFeatureContextKey, externalName)
				// Also store the subaccount ref so the import CR can find the credential
				ctx = context.WithValue(ctx, "sacSubaccountRef", sac.Spec.ForProvider.SubaccountRef)

				// Orphan: set management policy to not-delete so BTP resource survives CR deletion
				sac.Spec.ManagementPolicies = []xpv1.ManagementAction{
					xpv1.ManagementActionObserve,
					xpv1.ManagementActionCreate,
					xpv1.ManagementActionUpdate,
					xpv1.ManagementActionLateInitialize,
				}
				if err := cfg.Client().Resources().Update(ctx, sac); err != nil {
					t.Fatalf("Failed to update management policy: %v", err)
				}
				AwaitResourceDeletionOrFail(ctx, t, cfg, sac, wait.WithTimeout(time.Minute*5))

				return ctx
			},
		).
		Assess(
			"Import of orphaned credential results in missing client_secret in connection secret",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				externalName := ctx.Value(importFeatureContextKey).(string)
				subaccountRef, _ := ctx.Value("sacSubaccountRef").(*xpv1.Reference)

				// Re-create the CR with the credential name as external-name to trigger import
				readOnly := false
				importSac := &v1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{
						Name:      importName,
						Namespace: cfg.Namespace(),
					},
					Spec: v1alpha1.SubaccountApiCredentialSpec{
						ForProvider: v1alpha1.SubaccountApiCredentialParameters{
							ReadOnly:      &readOnly,
							SubaccountRef: subaccountRef,
						},
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      importName + "-secret",
								Namespace: cfg.Namespace(),
							},
						},
					},
				}
				xpmeta.SetExternalName(importSac, externalName)

				if err := cfg.Client().Resources().Create(ctx, importSac); err != nil {
					t.Fatalf("Failed to create import CR: %v", err)
				}

				// Wait for the resource to become synced (Available). BTP returns the credential
				// metadata on read but NOT the client_secret, so the resource will become Available
				// but the connection secret will have an empty/missing client_secret.
				c := conditions.New(cfg.Client().Resources())
				err := wait.For(c.ResourceMatch(importSac, func(object k8sobj.Object) bool {
					cr := object.(*v1alpha1.SubaccountApiCredential)
					cond := cr.GetCondition(xpv1.TypeReady)
					return cond.Status == corev1.ConditionTrue
				}), wait.WithTimeout(time.Minute*5))

				if err != nil {
					t.Logf("Import CR did not become Available within timeout: %v — checking connection secret anyway", err)
				}

				// Verify the connection secret is missing a usable client_secret.
				// BTP only returns client_secret at creation time, so importing always produces an
				// empty or absent client_secret, making the imported credential unusable.
				secret := &corev1.Secret{}
				if getErr := cfg.Client().Resources().Get(ctx, importName+"-secret", cfg.Namespace(), secret); getErr != nil {
					// No secret written at all is also acceptable — the credential is unusable either way
					t.Logf("Connection secret not found (imported credential has no usable client_secret): %v", getErr)
					return ctx
				}
				clientSecret := secret.Data["attribute.client_secret"]
				if len(clientSecret) > 0 {
					t.Errorf("Expected client_secret to be empty/missing after import, but got a non-empty value — BTP unexpectedly returned the secret")
				}

				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			importSac := &v1alpha1.SubaccountApiCredential{}
			if err := cfg.Client().Resources().Get(ctx, importName, cfg.Namespace(), importSac); err == nil {
				AwaitResourceDeletionOrFail(ctx, t, cfg, importSac, wait.WithTimeout(time.Minute*5))
			}
			DeleteResourcesIgnoreMissing(ctx, t, cfg, orphanManifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, orphanImportFeature)
}
