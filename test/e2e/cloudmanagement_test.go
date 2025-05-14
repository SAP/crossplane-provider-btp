//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal"
	"sigs.k8s.io/e2e-framework/klient/wait"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	cisCreateName = "e2e-cis-created"
	siName        = "e2e-si-created"
	sbName        = "e2e-sb-created"
)

func TestCloudManagemen(t *testing.T) {
	crudFeatureSuite := features.New("CloudManagement Controller Test").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/cloudmanagement/env")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())
				return ctx
			},
		).
		Assess(
			"Check CloudManagement Resource is fully created and updated", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cm := createAndReturnCloudmanagement(ctx, t, cfg, "testdata/crs/cloudmanagement/creation")

				// Status bound?
				if cm.Status.AtProvider.Status != v1beta1.CisStatusBound {
					t.Error("Binding status not set as expected")
				}

				if internal.Val(cm.Status.AtProvider.Instance.Name) != siName {
					t.Errorf("Instance name not as expected")
				}

				if internal.Val(cm.Status.AtProvider.Binding.Name) != sbName {
					t.Errorf("Binding name not as expected")
				}

				assertProperSecretWritten(t, ctx, cfg, cm)

				// all external resources exist?
				sm := &v1beta1.ServiceManager{}
				MustGetResource(t, cfg, cm.Spec.ForProvider.ServiceManagerRef.Name, nil, sm)

				mustDeleteCloudManagement(ctx, t, cfg, cm)

				return ctx
			},
		).Assess(
		"Check CloudManagement Resource is fully created with default values", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cm := createAndReturnCloudmanagement(ctx, t, cfg, "testdata/crs/cloudmanagement/creationDefaultName")

			// Status bound?
			if cm.Status.AtProvider.Status != v1beta1.CisStatusBound {
				t.Error("Binding status not set as expected")
			}

			if internal.Val(cm.Status.AtProvider.Instance.Name) != v1beta1.DefaultCloudManagementInstanceName {
				t.Errorf("Instance name not as expected")
			}

			if internal.Val(cm.Status.AtProvider.Binding.Name) != v1beta1.DefaultCloudManagementBindingName {
				t.Errorf("Binding name not as expected")
			}

			assertProperSecretWritten(t, ctx, cfg, cm)

			// all external resources exist?
			sm := &v1beta1.ServiceManager{}
			MustGetResource(t, cfg, cm.Spec.ForProvider.ServiceManagerRef.Name, nil, sm)

			mustDeleteCloudManagement(ctx, t, cfg, cm)

			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, "cloudmanagement/env", wait.WithTimeout(time.Minute*10))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}

func assertProperSecretWritten(t *testing.T, ctx context.Context, cfg *envconf.Config, cm *v1beta1.CloudManagement) {
	// binding secret written?
	secretName := cm.GetWriteConnectionSecretToReference().Name
	secretNS := cm.GetWriteConnectionSecretToReference().Namespace
	secret := &corev1.Secret{}
	err := cfg.Client().Resources().Get(ctx, secretName, secretNS, secret)
	if err != nil {
		t.Error("Error while loading expected secret from Ref")
	}
	// secret contains correct structure
	if _, ok := secret.Data["uaa.url"]; !ok {
		t.Error("Secret not in proper format")
	}
}

func createAndReturnCloudmanagement(ctx context.Context, t *testing.T, cfg *envconf.Config, dir string) *v1beta1.CloudManagement {
	resources.ImportResources(ctx, t, cfg, dir)

	cm := v1beta1.CloudManagement{
		ObjectMeta: metav1.ObjectMeta{Name: cisCreateName, Namespace: cfg.Namespace()},
	}
	waitForResource(&cm, cfg, t, wait.WithTimeout(10*time.Minute))
	return MustGetResource(t, cfg, cisCreateName, nil, &cm)
}

func mustDeleteCloudManagement(ctx context.Context, t *testing.T, cfg *envconf.Config, cm *v1beta1.CloudManagement) {
	MustGetResource(t, cfg, cisCreateName, nil, cm)
	AwaitResourceDeletionOrFail(ctx, t, cfg, cm, wait.WithTimeout(time.Minute*10))
}
