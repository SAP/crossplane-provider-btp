//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerTest "github.com/sap/crossplane-provider-btp/test"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// TestSubaccountApiCredentialProviderRestart verifies the same MR survives losing
// the provider process's entire Terraform operation store. The connection Secret
// is created normally and never seeded, removed or repaired by this test. BTP
// Read cannot return client_secret: the cold provider must restore it from the
// surviving Secret, preserve it through Read, and publish complete data.
// This test restarts the shared provider and must not run in parallel with other
// tests or workloads in a reused cluster; use a disposable E2E environment.
func TestSubaccountApiCredentialProviderRestart(t *testing.T) {
	const manifestDir = "SubaccountApiCredentialOrphanImport"
	const timeout = 8 * time.Minute
	name := "sac-restart-" + BUILD_ID
	secretName := name + "-secret"
	var before *securityv1alpha1.SubaccountApiCredential
	var beforeSecret *corev1.Secret

	feature := features.New("SubaccountApiCredential Provider Restart").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cleanup := []providerTest.CredentialAdoptionCleanupAction{{
				Name: "resume restart credential before cleanup", Timeout: 30 * time.Second,
				Run: func(ctx context.Context) error {
					err := setCredentialRestartPausedE2E(ctx, cfg, name, false)
					if apierrors.IsNotFound(err) {
						return nil
					}
					return err
				},
			}}
			cleanup = append(cleanup, credentialCleanupActionsE2E(cfg, manifestDir, nil, []string{name}, []credentialConnectionSecretOwnerE2E{{secretName, name}})...)
			providerTest.RegisterCredentialAdoptionCleanup(t, ctx, cleanup...)

			resources.ImportResources(ctx, t, cfg, crsPath(manifestDir))
			waitForResource(&accountv1alpha1.Subaccount{
				ObjectMeta: metav1.ObjectMeta{Name: "sac-orphan-subaccount", Namespace: cfg.Namespace()},
			}, cfg, t, wait.WithTimeout(12*time.Minute))
			created := createCredentialForAdoptionE2E(ctx, t, cfg, name, secretName, nil)
			if !waitForCredentialReadyAndSyncedE2E(ctx, t, cfg, created.Name, timeout) {
				t.FailNow()
			}
			before = getCredentialForAdoptionE2E(ctx, t, cfg, name)
			assertCredentialIdentityE2E(t, before)
			assertCompleteCredentialSecretE2E(t, ctx, cfg, before)
			beforeSecret = getCredentialSecretE2E(ctx, t, cfg, before)
			if t.Failed() {
				t.FailNow()
			}
			return ctx
		}).
		Assess("replace provider processes without changing the MR or Secret", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Observe a non-success condition before restart. Otherwise the health
			// wait could immediately pass on Ready/Synced persisted by the old process.
			if err := setCredentialRestartPausedE2E(ctx, cfg, name, true); err != nil {
				t.Fatalf("pause credential before restart: %v", err)
			}
			paused := conditions.New(cfg.Client().Resources()).ResourceMatch(
				&securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.Namespace()}},
				func(object k8s.Object) bool {
					credential, ok := object.(*securityv1alpha1.SubaccountApiCredential)
					if !ok {
						return false
					}
					synced := credential.GetCondition(xpv1.TypeSynced)
					return xpmeta.IsPaused(credential) && synced.Status == corev1.ConditionFalse && synced.Reason == xpv1.ReconcilePaused().Reason
				},
			)
			if err := wait.For(paused, wait.WithContext(ctx), wait.WithTimeout(timeout)); err != nil {
				t.Fatalf("credential did not acknowledge pause: %v; %s", err, credentialAdoptionDiagnosticsE2E(ctx, cfg, name))
			}
			runtime, err := providerTest.NewProviderRuntime(cfg.Client().RESTConfig())
			if err != nil {
				t.Fatalf("initialize provider restart client: %v", err)
			}
			packageImage, controllerImage := providerTest.GetImagesFromJsonOrPanic(os.Getenv(UUT_IMAGES_KEY))
			if err := runtime.Restart(ctx, "btp-account", packageImage, controllerImage); err != nil {
				t.Fatalf("restart provider runtime: %v", err)
			}
			if err := setCredentialRestartPausedE2E(ctx, cfg, name, false); err != nil {
				t.Fatalf("resume credential after restart: %v", err)
			}
			return ctx
		}).
		Assess("cold reconciliation preserves health, identity and complete connection data", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			if !waitForCredentialReadyAndSyncedE2E(ctx, t, cfg, name, timeout) {
				t.FailNow()
			}
			after := getCredentialForAdoptionE2E(ctx, t, cfg, name)
			if after.UID != before.UID || xpmeta.GetExternalName(after) != xpmeta.GetExternalName(before) ||
				valueForAdoptionE2E(after.Status.AtProvider.SubaccountID) != valueForAdoptionE2E(before.Status.AtProvider.SubaccountID) ||
				valueForAdoptionE2E(after.Status.AtProvider.Name) != valueForAdoptionE2E(before.Status.AtProvider.Name) {
				t.Error("provider restart changed MR UID or established compound credential identity")
			}
			assertCredentialIdentityE2E(t, after)
			beforeRef, afterRef := before.GetWriteConnectionSecretToReference(), after.GetWriteConnectionSecretToReference()
			if afterRef == nil || *afterRef != *beforeRef {
				t.Fatal("provider restart changed the connection Secret destination")
			}
			assertCompleteCredentialSecretE2E(t, ctx, cfg, after)
			afterSecret := getCredentialSecretE2E(ctx, t, cfg, after)
			if afterSecret.UID != beforeSecret.UID {
				t.Error("provider restart replaced the original connection Secret")
			}
			// Compare only in memory: neither credential values nor hashes belong
			// in test output. In particular client_secret must remain byte-identical.
			for _, key := range credentialConnectionSecretKeys {
				if !bytes.Equal(beforeSecret.Data[key], afterSecret.Data[key]) {
					t.Errorf("provider restart changed connection Secret key %q", key)
				}
			}
			return ctx
		}).Feature()

	testenv.Test(t, feature)
}

func setCredentialRestartPausedE2E(ctx context.Context, cfg *envconf.Config, name string, paused bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		credential := &securityv1alpha1.SubaccountApiCredential{}
		if err := cfg.Client().Resources().Get(ctx, name, cfg.Namespace(), credential); err != nil {
			return err
		}
		if credential.Annotations == nil {
			credential.Annotations = make(map[string]string)
		}
		credential.Annotations[xpmeta.AnnotationKeyReconciliationPaused] = fmt.Sprintf("%t", paused)
		return cfg.Client().Resources().Update(ctx, credential)
	})
}
