//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	sbSecretFormatName = "e2e-destination-binding-fmt"
	sbSecretFormatKey  = "credentials"
)

// TestServiceBinding_SecretFormatSAPKubernetes verifies the sap-kubernetes
// secret format: the connection secret carries SAP Kubernetes Service Binding
// metadata fields (type, label, plan, instance_name, instance_guid, tags) plus
// a .metadata descriptor, and credentials are bundled under the configured
// secretKey instead of flattened.
func TestServiceBinding_SecretFormatSAPKubernetes(t *testing.T) {
	feature := features.New("ServiceBinding sap-kubernetes secret format").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("servicebinding/secretformat-env"))
				resources.ImportResources(ctx, t, cfg, crsPath("servicebinding/secretformat"))
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				sb := v1alpha1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{Name: sbSecretFormatName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sb, cfg, t, wait.WithTimeout(7*time.Minute))
				return ctx
			},
		).Assess(
		"ServiceBinding spec carries secretFormat and secretKey", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sb := &v1alpha1.ServiceBinding{}
			MustGetResource(t, cfg, sbSecretFormatName, nil, sb)
			if sb.Spec.SecretFormat != "sap-kubernetes" {
				t.Errorf("expected secretFormat=sap-kubernetes, got %q", sb.Spec.SecretFormat)
			}
			if sb.Spec.SecretKey == nil || *sb.Spec.SecretKey != sbSecretFormatKey {
				t.Errorf("expected secretKey=%q, got %v", sbSecretFormatKey, sb.Spec.SecretKey)
			}
			if sb.Status.AtProvider.ID == "" {
				t.Error("ServiceBinding not fully initialized")
			}
			return ctx
		},
	).Assess(
		"Secret carries sap-kubernetes metadata fields, .metadata descriptor and bundled credentials", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sb := &v1alpha1.ServiceBinding{}
			MustGetResource(t, cfg, sbSecretFormatName, nil, sb)
			secretRef := sb.GetWriteConnectionSecretToReference()
			secret := &corev1.Secret{}
			MustGetResource(t, cfg, secretRef.Name, &secretRef.Namespace, secret)

			// Required SAP Kubernetes Service Binding metadata keys.
			for _, key := range []string{"type", "label", "plan", "instance_name", "instance_guid", "tags"} {
				if _, ok := secret.Data[key]; !ok {
					t.Errorf("Secret missing sap-kubernetes metadata key %q", key)
				}
			}

			// Bundled credentials under the configured secretKey.
			bundled, ok := secret.Data[sbSecretFormatKey]
			if !ok || len(bundled) == 0 {
				t.Errorf("Secret missing bundled credentials under key %q", sbSecretFormatKey)
				return ctx
			}
			var bundleObj map[string]any
			if err := json.Unmarshal(bundled, &bundleObj); err != nil {
				t.Errorf("bundled credentials under %q are not valid JSON: %v", sbSecretFormatKey, err)
				return ctx
			}
			if len(bundleObj) == 0 {
				t.Errorf("bundled credentials under %q are an empty object", sbSecretFormatKey)
			}

			// .metadata descriptor JSON.
			meta, ok := secret.Data[".metadata"]
			if !ok || len(meta) == 0 {
				t.Errorf("Secret missing .metadata descriptor")
				return ctx
			}
			var metaObj struct {
				MetaDataProperties []struct {
					Name      string `json:"name"`
					Format    string `json:"format"`
					Container bool   `json:"container,omitempty"`
				} `json:"metaDataProperties"`
				CredentialProperties []struct {
					Name      string `json:"name"`
					Format    string `json:"format"`
					Container bool   `json:"container,omitempty"`
				} `json:"credentialProperties"`
			}
			if err := json.Unmarshal(meta, &metaObj); err != nil {
				t.Errorf(".metadata is not valid JSON: %v", err)
				return ctx
			}
			if len(metaObj.MetaDataProperties) == 0 {
				t.Error(".metadata.metaDataProperties is empty")
			}
			// credentialProperties must mark the bundle key with container: true.
			var foundContainer bool
			for _, p := range metaObj.CredentialProperties {
				if p.Name == sbSecretFormatKey && p.Container {
					foundContainer = true
					break
				}
			}
			if !foundContainer {
				t.Errorf(".metadata.credentialProperties does not mark %q as container:true", sbSecretFormatKey)
			}
			return ctx
		},
	).Assess(
		"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sb := &v1alpha1.ServiceBinding{}
			MustGetResource(t, cfg, sbSecretFormatName, nil, sb)
			AwaitResourceDeletionOrFail(ctx, t, cfg, sb, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, "servicebinding/secretformat-env", wait.WithTimeout(time.Minute*15))
			return ctx
		},
	).Feature()

	testenv.Test(t, feature)
}
