//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal"
	"sigs.k8s.io/e2e-framework/klient/wait"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
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
	cmImportName  = "cm-import-test"
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

func TestCloudManagementImport(t *testing.T) {
	importFeatureSuite := features.New("CloudManagement Import Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/cloudmanagement/env")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				// Wait for the ServiceManager to be ready before creating CloudManagement
				waitForResource(&v1beta1.ServiceManager{
					ObjectMeta: metav1.ObjectMeta{Name: "e2e-sm-cis", Namespace: cfg.Namespace()},
				}, cfg, t, wait.WithTimeout(15*time.Minute))

				cm := &v1beta1.CloudManagement{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmImportName + "-create",
						Namespace: cfg.Namespace(),
					},
					Spec: v1beta1.CloudManagementSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ManagementPolicies: []xpv1.ManagementAction{
								xpv1.ManagementActionObserve,
								xpv1.ManagementActionCreate,
								xpv1.ManagementActionUpdate,
								xpv1.ManagementActionLateInitialize,
							},
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      cmImportName + "-create",
								Namespace: cfg.Namespace(),
							},
						},
						ForProvider: v1beta1.CloudManagementParameters{
							ServiceManagerRef: &xpv1.Reference{Name: "e2e-sm-cis"},
							SubaccountRef:     &xpv1.Reference{Name: "cis-sa-test"},
						},
					},
				}

				err := cfg.Client().Resources().Create(ctx, cm)
				if err != nil {
					t.Errorf("Failed to create CloudManagement for import preparation: %v", err)
				}

				waitForResource(cm, cfg, t, wait.WithTimeout(10*time.Minute))

				createdCM := &v1beta1.CloudManagement{}
				MustGetResource(t, cfg, cmImportName+"-create", nil, createdCM)

				actualExternalName := createdCM.GetAnnotations()["crossplane.io/external-name"]
				t.Logf("Using external name for import: %s", actualExternalName)

				AwaitResourceDeletionOrFail(ctx, t, cfg, createdCM, wait.WithTimeout(time.Minute*2))

				ctx = context.WithValue(ctx, "actualExternalName", actualExternalName)

				return ctx
			},
		).
		Assess(
			"Check Imported CloudManagement gets healthy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				actualExternalName := ctx.Value("actualExternalName").(string)

				cm := &v1beta1.CloudManagement{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmImportName,
						Namespace: cfg.Namespace(),
						Annotations: map[string]string{
							"crossplane.io/external-name": actualExternalName,
						},
					},
					Spec: v1beta1.CloudManagementSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionObserve},
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      cmImportName,
								Namespace: cfg.Namespace(),
							},
						},
						ForProvider: v1beta1.CloudManagementParameters{
							ServiceManagerRef: &xpv1.Reference{Name: "e2e-sm-cis"},
							SubaccountRef:     &xpv1.Reference{Name: "cis-sa-test"},
						},
					},
				}

				err := cfg.Client().Resources().Create(ctx, cm)
				if err != nil {
					t.Errorf("Failed to create import CloudManagement: %v", err)
				}

				waitForResource(cm, cfg, t)

				importedCM := &v1beta1.CloudManagement{}
				MustGetResource(t, cfg, cmImportName, nil, importedCM)

				if importedCM.Status.AtProvider.Status != v1beta1.CisStatusBound {
					t.Error("CloudManagement Status not as expected")
				}

				assertProperSecretWritten(t, ctx, cfg, importedCM)

				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cm := &v1beta1.CloudManagement{}
			MustGetResource(t, cfg, cmImportName, nil, cm)

			// Allow resource to be deleted for teardown
			cm.Spec.ResourceSpec.ManagementPolicies = []xpv1.ManagementAction{xpv1.ManagementActionDelete, xpv1.ManagementActionObserve}
			if err := cfg.Client().Resources().Update(ctx, cm); err != nil {
				t.Errorf("Failed to update CloudManagement deletion policy: %v", err)
			}

			AwaitResourceDeletionOrFail(ctx, t, cfg, cm, wait.WithTimeout(time.Minute*10))

			DeleteResourcesIgnoreMissing(ctx, t, cfg, "cloudmanagement/env", wait.WithTimeout(time.Minute*15))
			return ctx
		},
	).Feature()

	testenv.Test(t, importFeatureSuite)
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
