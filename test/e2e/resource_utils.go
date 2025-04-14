//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	crClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	wairres "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type mockList struct {
	crClient.ObjectList

	Items []k8s.Object
}

func waitForResource(res k8s.Object, cfg *envconf.Config, t *testing.T, opts ...wait.Option) {
	client := cfg.Client()

	c := conditions.New(client.Resources())

	match := c.ResourceMatch(res, func(object k8s.Object) bool {
		d := object.(resource.Conditioned)
		condition := d.GetCondition(xpv1.Available().Type)
		result := condition.Status == v1.ConditionTrue
		klog.V(4).Infof(
			"Checking %s on %v. result=%v",
			resources.Identifier(res),
			condition,
			condition.Status == v1.ConditionTrue,
		)
		return result
	})

	err := wait.For(match, opts...)

	if err != nil {
		t.Error(err)
	}
}

// MustGetResource generic loading of resources, potential errors are passed to the testing context
func MustGetResource[T k8s.Object](t *testing.T, cfg *envconf.Config, name string, ns *string, ct T) T {
	res, err := GetResource(cfg, name, ns, ct)
	if err != nil {
		t.Error("Failed to get resource. error: ", err)
	}
	return res
}

// GetResource generic loading of resources from config, returns potential err
func GetResource[T k8s.Object](cfg *envconf.Config, name string, ns *string, ct T) (T, error) {
	var namespace string
	if ns != nil {
		namespace = *ns
	} else {
		namespace = cfg.Namespace()
	}
	r := cfg.Client().Resources()

	err := r.Get(context.TODO(), name, namespace, ct)
	return ct, err
}

// DeleteResourcesIgnoreMissing deletes resources defined in a certain directory relative to testdata/crs/
func DeleteResourcesIgnoreMissing(ctx context.Context, t *testing.T, cfg *envconf.Config, manifestDir string, timeout wait.Option) context.Context {
	klog.V(4).Info("Attempt to delete previously imported resources")
	r, _ := GetResourcesWithRESTConfig(cfg)
	objects, err := getObjectsToImport(ctx, cfg, manifestDir)
	if err != nil {
		t.Fatal(objects)
	}
	for _, obj := range objects {
		delErr := r.Delete(ctx, obj)
		if delErr != nil && !errors.IsNotFound(delErr) {
			t.Fatal(delErr)
		}
	}

	if err = wait.For(
		conditions.New(r).ResourcesDeleted(&mockList{Items: objects}),
		timeout,
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// AwaitResourceDeletionOrFail deletes a given k8s object with a timeout of configurable duration
// this should be moved into xp-testing library
func AwaitResourceDeletionOrFail(ctx context.Context, t *testing.T, cfg *envconf.Config, object k8s.Object, opts ...wait.Option) {
	res := cfg.Client().Resources()

	err := res.Delete(ctx, object)
	if err != nil {
		t.Fatalf("Failed to delete object %s.", identifier(object))
	}

	err = wait.For(conditions.New(res).ResourceDeleted(object), opts...)
	if err != nil {
		t.Fatalf(
			"Failed to delete object in time %s.",
			identifier(object),
		)
	}
}

// GetResourcesWithRESTConfig returns new resource from REST config
func GetResourcesWithRESTConfig(cfg *envconf.Config) (*wairres.Resources, error) {
	r, err := wairres.New(cfg.Client().RESTConfig())
	return r, err
}

func getObjectsToImport(ctx context.Context, cfg *envconf.Config, dir string) ([]k8s.Object, error) {
	r := resClient(cfg)

	r.WithNamespace(cfg.Namespace())

	objects := make([]k8s.Object, 0)
	err := decoder.DecodeEachFile(
		ctx, os.DirFS(filepath.Join("./testdata/crs", dir)), "*",
		func(ctx context.Context, obj k8s.Object) error {
			objects = append(objects, obj)
			return nil
		},
	)
	return objects, err
}

func resClient(cfg *envconf.Config) *wairres.Resources {
	r, _ := GetResourcesWithRESTConfig(cfg)
	return r
}

// Identifier returns k8s object name
func identifier(object k8s.Object) string {
	return fmt.Sprintf("%s/%s", object.GetObjectKind().GroupVersionKind().String(), object.GetName())
}
