//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

// TODO: separate the k8s resource name and the external resource name

func TestDirectory(t *testing.T) {
	dirK8sResName := "e2e-test-directory"
	directoryNameE2e := NewID(dirK8sResName, BUILD_ID)
	crudFeature := features.New("BTP Directory Controller").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				mutateDirResource := newMutateDirFunc(directoryNameE2e)
				createK8sResources(ctx, t, cfg, r, "directory", "*", mutateDirResource)

				waitForResource(newDirectoryResource(cfg, dirK8sResName), cfg, t)
				return ctx
			},
		).
		Assess(
			"Check Directory Created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				directoryObserved := GetDirectoryOrError(t, cfg, dirK8sResName)
				klog.InfoS("Directory Details", "cr", directoryObserved)
				return ctx
			},
		).
		Assess(
			"Check Directory Update With Authorizations", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				dir := GetDirectoryOrError(t, cfg, dirK8sResName)

				featuresWant := []string{"DEFAULT", "ENTITLEMENTS", "AUTHORIZATIONS"}
				dir.Spec.ForProvider.DirectoryFeatures = featuresWant

				resources.AwaitResourceUpdateOrError(ctx, t, cfg, dir)

				resources.AwaitResourceUpdateFor(
					ctx, t, cfg, dir,
					func(object k8s.Object) bool {
						sa := object.(*v1alpha1.Directory)
						got := sa.Status.AtProvider.DirectoryFeatures
						if diff := cmp.Diff(featuresWant, got, test.EquateErrors()); diff != "" {
							return false
						}
						return true
					},
					wait.WithTimeout(time.Second*90),
				)

				klog.InfoS("Directory Details", "cr", dir)
				return ctx
			},
		).
		Assess(
			"Check Directory Deleted", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				directoryObserved := GetDirectoryOrError(t, cfg, dirK8sResName)
				resources.AwaitResourceDeletionOrFail(ctx, t, cfg, directoryObserved)
				return ctx
			},
		).Teardown(resources.DumpManagedResources).Feature()

	testenv.Test(t, crudFeature)
}

func newMutateDirFunc(directoryNameE2e string) func(obj k8s.Object) error {
	mutateDirResource := func(obj k8s.Object) error {
		if mg, ok := any(obj).(meta.ManagedTested); ok {
			newId := directoryNameE2e
			mg.SetExternalID(newId)
		}
		return nil
	}
	return mutateDirResource
}

func GetDirectoryOrError(t *testing.T, cfg *envconf.Config, directory string) *v1alpha1.Directory {
	ct := &v1alpha1.Directory{}
	namespace := cfg.Namespace()
	res := cfg.Client().Resources()

	err := res.Get(context.TODO(), directory, namespace, ct)
	if err != nil {
		t.Error("Failed to get Directory. error : ", err)
	}
	return ct
}

func newDirectoryResource(cfg *envconf.Config, dirK8sResName string) *v1alpha1.Directory {
	return &v1alpha1.Directory{
		ObjectMeta: metav1.ObjectMeta{
			Name: dirK8sResName, Namespace: cfg.Namespace(),
		},
	}
}

func createK8sResources(ctx context.Context, t *testing.T, cfg *envconf.Config, r *res.Resources, directory string, pattern string, mutateFunc decoder.MutateFunc) {

	errdecode := decoder.DecodeEachFile(
		ctx, os.DirFS("./testdata/crs/"+directory), pattern,
		decoder.CreateHandler(r),
		decoder.MutateOption(mutateFunc),
		decoder.MutateNamespace(cfg.Namespace()),
	)

	if errdecode != nil {
		t.Error("Error Details", "errdecode", errdecode)
	}
}

func NewID(oldId string, buildId string) string {
	return buildId + "-" + oldId
}

// TestDirectoryImport tests importing an existing Directory resource using external-name annotation
// Uses ImportTester utility to follow the standard import test pattern
func TestDirectoryImport(t *testing.T) {
	dirImportName := "e2e-test-dir-import"

	importTester := NewImportTester(
		&v1alpha1.Directory{
			Spec: v1alpha1.DirectorySpec{
				ForProvider: v1alpha1.DirectoryParameters{
					DisplayName:     internal.Ptr(dirImportName),
					Description:     internal.Ptr("Directory for import test"),
					DirectoryAdmins: []string{envvar.GetOrPanic(TECHNICAL_USER_EMAIL_ENV_KEY), envvar.GetOrPanic(SECONDARY_DIRC_ADMIN_ENV_KEY)},
				},
			},
		},
		dirImportName,
		WithWaitCreateTimeout[*v1alpha1.Directory](wait.WithTimeout(7*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.Directory](wait.WithTimeout(5*time.Minute)),
	)

	importFeature := importTester.BuildTestFeature("BTP Directory Import Flow").Feature()

	testenv.Test(t, importFeature)
}
