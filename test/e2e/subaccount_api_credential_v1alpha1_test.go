//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/cluster/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"

	"sigs.k8s.io/e2e-framework/klient/wait"
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

// TestSubaccountApiCredentialOrphanImport verifies External-Name ADR compliance for
// SubaccountApiCredential: after provisioning, the crossplane.io/external-name annotation
// reflects the BTP credential name (via GetExternalNameFn reading `name` from Terraform
// state). The connection secret must also contain a valid client_secret.
//
// Note: the BTP Terraform provider does not implement ImportState for this resource type.
// Setting the external-name annotation before creation would trigger Upjet's terraform-import
// path, which always fails for this resource. The external-name is therefore set by the
// provider after successful creation, not by the user before creation.
//
// See: https://github.com/SAP/crossplane-provider-btp/issues/553
func TestSubaccountApiCredentialOrphanImport(t *testing.T) {
	var orphanManifestDir = "testdata/crs/SubaccountApiCredentialOrphanImport"
	// Prefix "sac-adr-" ensures the BTP credential name starts with a letter.
	// The SAC is created in Setup (not Assess) so the controller picks it up
	// reliably while Crossplane is already actively reconciling resources.
	sacName := "sac-adr-" + BUILD_ID

	orphanImportFeature := features.New("SubaccountApiCredential External Name ADR Compliance").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				// Apply the subaccount first. The SAC is created programmatically after
				// the subaccount is Ready to avoid exponential back-off on SubaccountRef
				// resolution.
				resources.ImportResources(ctx, t, cfg, orphanManifestDir)

				waitForResource(&accountv1alpha1.Subaccount{
					ObjectMeta: metav1.ObjectMeta{Name: "sac-orphan-subaccount", Namespace: cfg.Namespace()},
				}, cfg, t, wait.WithTimeout(time.Minute*12))

				// Create the SAC in Setup so the controller picks it up while Crossplane
				// is already actively reconciling. Do NOT set the external-name annotation
				// before creation — that would trigger Upjet's terraform-import path, which
				// always fails because the BTP Terraform provider does not implement
				// ImportState for btp_subaccount_api_credential.
				readOnly := false
				sac := &v1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Name: sacName, Namespace: cfg.Namespace()},
					Spec: v1alpha1.SubaccountApiCredentialSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      sacName + "-secret",
								Namespace: cfg.Namespace(),
							},
						},
						ForProvider: v1alpha1.SubaccountApiCredentialParameters{
							ReadOnly:      &readOnly,
							SubaccountRef: &xpv1.Reference{Name: "sac-orphan-subaccount"},
						},
					},
				}
				if err := cfg.Client().Resources().Create(ctx, sac); err != nil {
					t.Fatalf("Failed to create SAC: %v", err)
				}
				waitForResource(sac, cfg, t, wait.WithTimeout(time.Minute*8))

				return ctx
			},
		).
		Assess(
			"SAC external-name annotation reflects BTP credential name after provisioning (ADR compliance)",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacName, nil, sac)

				// ADR compliance: after provisioning, GetExternalNameFn reads `name` from
				// the Terraform state and writes it back to the annotation.
				if externalName := xpmeta.GetExternalName(sac); externalName == "" {
					t.Error("External name ADR compliance: annotation is empty after provisioning — GetExternalNameFn did not run")
				}

				// Verify the connection secret contains a valid client_secret.
				assertApiCredentialSecret(t, ctx, cfg, sac)

				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sac := &v1alpha1.SubaccountApiCredential{}
			if err := cfg.Client().Resources().Get(ctx, sacName, cfg.Namespace(), sac); err == nil {
				AwaitResourceDeletionOrFail(ctx, t, cfg, sac, wait.WithTimeout(time.Minute*5))
			}
			DeleteResourcesIgnoreMissing(ctx, t, cfg, orphanManifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, orphanImportFeature)
}
