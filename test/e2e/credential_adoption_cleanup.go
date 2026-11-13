//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerTest "github.com/sap/crossplane-provider-btp/test"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func credentialAdoptionCleanupActionsE2E(cfg *envconf.Config, manifestDir string) []providerTest.CredentialAdoptionCleanupAction {
	adopters := []string{
		"sac-adopt-init-complete-" + BUILD_ID,
		"sac-adopt-incomplete-" + BUILD_ID,
		"sac-adopt-certificate-input-" + BUILD_ID,
		"sac-adopt-complete-" + BUILD_ID,
	}
	sources := []string{"sac-adopt-init-source-" + BUILD_ID, "sac-adopt-source-" + BUILD_ID}
	actions := make([]providerTest.CredentialAdoptionCleanupAction, 0, 16)
	for _, name := range adopters {
		name := name
		actions = append(actions, providerTest.CredentialAdoptionCleanupAction{
			Name: "adopter " + name, Timeout: 2 * time.Minute,
			Run: func(ctx context.Context) error { return deleteCredentialForAdoptionE2E(ctx, cfg, name) },
		})
	}
	for _, name := range sources {
		name := name
		actions = append(actions, providerTest.CredentialAdoptionCleanupAction{
			Name: "source " + name, Timeout: 3 * time.Minute,
			Run: func(ctx context.Context) error {
				if err := ensureCredentialsAbsentForAdoptionE2E(ctx, cfg, adopters); err != nil {
					return fmt.Errorf("dependent adopter remains: %w", err)
				}
				return deleteCredentialForAdoptionE2E(ctx, cfg, name)
			},
		})
	}
	secretOwners := []struct{ name, owner string }{
		{"sac-adopt-source-secret-" + BUILD_ID, sources[1]},
		{"sac-adopt-init-source-" + BUILD_ID + "-secret", sources[0]},
		{"sac-adopt-init-complete-" + BUILD_ID + "-secret", adopters[0]},
		{"sac-adopt-incomplete-" + BUILD_ID + "-secret", adopters[1]},
		{"sac-adopt-certificate-input-" + BUILD_ID + "-secret", adopters[2]},
		{"sac-adopt-complete-" + BUILD_ID + "-secret", adopters[3]},
	}
	for _, entry := range secretOwners {
		entry := entry
		actions = append(actions, providerTest.CredentialAdoptionCleanupAction{
			Name: "connection Secret " + entry.name, Timeout: 30 * time.Second,
			Run: func(ctx context.Context) error {
				if err := ensureCredentialAbsentForAdoptionE2E(ctx, cfg, entry.owner); err != nil {
					return fmt.Errorf("owner %q remains; preserving Secret: %w", entry.owner, err)
				}
				secret := &corev1.Secret{}
				if err := cfg.Client().Resources().Get(ctx, entry.name, cfg.Namespace(), secret); apierrors.IsNotFound(err) {
					return nil
				} else if err != nil {
					return fmt.Errorf("read Secret %s/%s: %w", cfg.Namespace(), entry.name, err)
				}
				if err := cfg.Client().Resources().Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
					return fmt.Errorf("delete Secret %s/%s: %w", cfg.Namespace(), entry.name, err)
				}
				return nil
			},
		})
	}
	actions = append(actions, providerTest.CredentialAdoptionCleanupAction{
		Name: "imported adoption fixtures", Timeout: 3 * time.Minute,
		Run: func(ctx context.Context) error {
			if err := ensureCredentialsAbsentForAdoptionE2E(ctx, cfg, append(adopters, sources...)); err != nil {
				return fmt.Errorf("credential resources remain; preserving parent fixture: %w", err)
			}
			objects, err := providerTest.GetObjectsToImport(ctx, cfg, []string{crsPath(manifestDir)})
			if err != nil {
				return fmt.Errorf("decode imported fixture objects: %w", err)
			}
			var failures []string
			for _, object := range objects {
				if err := cfg.Client().Resources().Delete(ctx, object); err != nil && !apierrors.IsNotFound(err) {
					failures = append(failures, fmt.Sprintf("%s/%s delete: %v", object.GetObjectKind().GroupVersionKind(), object.GetName(), err))
					continue
				}
				if err := wait.For(conditions.New(cfg.Client().Resources()).ResourceDeleted(object), wait.WithContext(ctx)); err != nil {
					failures = append(failures, fmt.Sprintf("%s/%s deletion wait: %v", object.GetObjectKind().GroupVersionKind(), object.GetName(), err))
				}
			}
			if len(failures) != 0 {
				return fmt.Errorf("unresolved fixture resources: %s", strings.Join(failures, "; "))
			}
			return nil
		},
	})
	return actions
}

func ensureCredentialsAbsentForAdoptionE2E(ctx context.Context, cfg *envconf.Config, names []string) error {
	var remaining []string
	for _, name := range names {
		if err := ensureCredentialAbsentForAdoptionE2E(ctx, cfg, name); err != nil {
			remaining = append(remaining, name)
		}
	}
	if len(remaining) > 0 {
		return fmt.Errorf("unresolved SubaccountApiCredential MRs: %s", strings.Join(remaining, ", "))
	}
	return nil
}

func ensureCredentialAbsentForAdoptionE2E(ctx context.Context, cfg *envconf.Config, name string) error {
	credential := &v1alpha1.SubaccountApiCredential{}
	if err := cfg.Client().Resources().Get(ctx, name, cfg.Namespace(), credential); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read MR %q: %w", name, err)
	}
	return fmt.Errorf("MR %q still exists", name)
}

func deleteCredentialForAdoptionE2E(ctx context.Context, cfg *envconf.Config, name string) error {
	client := cfg.Client().Resources()
	credential := &v1alpha1.SubaccountApiCredential{}
	if err := client.Get(ctx, name, cfg.Namespace(), credential); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read MR %q: %w", name, err)
	}
	if err := client.Delete(ctx, credential); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete MR %q: %w", name, err)
	}
	if err := wait.For(conditions.New(client).ResourceDeleted(credential), wait.WithContext(ctx)); err != nil {
		return fmt.Errorf("wait for MR %q deletion: %w", name, err)
	}
	return nil
}
