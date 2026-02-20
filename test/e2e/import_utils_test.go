//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	xpmeta "github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	meta "github.com/sap/crossplane-provider-btp/apis"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var importManagementPolicies = []xpv1.ManagementAction{
	xpv1.ManagementActionObserve,
	xpv1.ManagementActionCreate,
	xpv1.ManagementActionUpdate,
	xpv1.ManagementActionLateInitialize,
}

const (
	importFeatureContextKey = "importExternalName"
)

// ImportTester helps to build e2e test feature for import flow of a managed resource.
// T is the type of the managed resource to be imported.
// Use NewImportTester to create an instance, then use BuildTestFeature to build the test feature.
// Use ImportTesterOption to customize timeouts.
type ImportTester[T resource.Managed] struct {
	//will be used as importing resource. The ObjectMeta.Name will be set automatically.
	BaseResource T

	// will be prefixed with BUILD_ID to ensure uniqueness
	BaseName string

	// the path to the dependent resource yaml files, if any
	DependentResourceDirectory string

	// the timeout for waiting till resource get healthy after creating (in setup and assess)
	WaitCreateTimeout wait.Option

	// the timeout for waiting till resource get deleted (in setup and teardown)
	WaitDeletionTimeout wait.Option
}

type ImportTesterOption[T resource.Managed] func(*ImportTester[T])

func WithWaitCreateTimeout[T resource.Managed](timeout wait.Option) ImportTesterOption[T] {
	return func(it *ImportTester[T]) {
		it.WaitCreateTimeout = timeout
	}
}

func WithWaitDeletionTimeout[T resource.Managed](timeout wait.Option) ImportTesterOption[T] {
	return func(it *ImportTester[T]) {
		it.WaitDeletionTimeout = timeout
	}
}

func WithDependentResourceDirectory[T resource.Managed](path string) ImportTesterOption[T] {
	return func(it *ImportTester[T]) {
		it.DependentResourceDirectory = path
	}
}

// NewImportTester creates an ImportTester for the given managed resource and base name.
// The base name will be prefixed with BUILD_ID to ensure uniqueness.
// Additional options can be provided to customize timeouts using ImportTesterOption.
func NewImportTester[T resource.Managed](baseResource T, baseName string, o ...ImportTesterOption[T]) *ImportTester[T] {
	it := &ImportTester[T]{
		BaseResource:        baseResource,
		BaseName:            baseName,
		WaitCreateTimeout:   wait.WithInterval(3 * time.Minute),
		WaitDeletionTimeout: wait.WithInterval(3 * time.Minute),
	}
	it.BaseResource.SetName(it.GetPrefixedName())

	for _, opt := range o {
		opt(it)
	}

	return it
}

func (it *ImportTester[T]) GetPrefixedName() string {
	return NewID(it.BaseName, envvar.GetOrDefault(UUT_BUILD_ID_KEY, "0000"))
}

func (it *ImportTester[T]) BuildTestFeature(name string) *features.FeatureBuilder {
	return features.New(name).
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				if it.DependentResourceDirectory != "" {
					log("Applying dependent resources from "+it.DependentResourceDirectory, it.BaseResource, func() {
						resources.ImportResources(ctx, t, cfg, it.DependentResourceDirectory)

						if err := resources.WaitForResourcesToBeSynced(ctx, cfg, it.DependentResourceDirectory, nil, wait.WithTimeout(time.Minute*5)); err != nil {
							resources.DumpManagedResources(ctx, t, cfg)
							t.Fatal(err)
						}
					})
				}

				//prepare the resource for creation
				createResource := it.BaseResource.DeepCopyObject().(T)
				createResource.SetManagementPolicies(importManagementPolicies)

				log("Creating resource on external system to be imported later", createResource, func() {
					if err := cfg.Client().Resources().Create(ctx, createResource); err != nil {
						t.Fatalf("Failed to create resource for import test: %v", err)
					}
					waitForResource(createResource, cfg, t, it.WaitCreateTimeout)
				})

				createdResource := it.BaseResource.DeepCopyObject().(T)
				log("Getting created resource to obtain external name", createResource, func() {
					MustGetResource(t, cfg, it.GetPrefixedName(), nil, createdResource)
					externalName := xpmeta.GetExternalName(createdResource)
					ctx = context.WithValue(ctx, importFeatureContextKey, externalName)
				})

				// delete the created resource to prepare for import. With managment policies missing Delete, it will not be deleted in the external system
				log("Deleting resource", createdResource, func() {
					AwaitResourceDeletionOrFail(ctx, t, cfg, createdResource, it.WaitDeletionTimeout)
				})

				return ctx
			},
		).Assess(
		"Check Imported Resource gets healthy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			externalName := ctx.Value(importFeatureContextKey).(string)

			//preare the resource for import
			resource := it.BaseResource.DeepCopyObject().(T)
			xpmeta.SetExternalName(resource, externalName)
			resource.SetManagementPolicies(xpv1.ManagementPolicies{xpv1.ManagementActionAll})

			//create the resource again for importing, should match the external resource
			log("Create MR for importing", resource, func() {
				if err := cfg.Client().Resources().Create(ctx, resource); err != nil {
					t.Fatalf("Failed to create cr when importing: %v", err)
				}
			})

			log("Waiting for imported resource to become healthy", resource, func() {
				waitForResource(resource, cfg, t, it.WaitCreateTimeout)
			})
			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			resource := it.BaseResource.DeepCopyObject().(T)
			MustGetResource(t, cfg, it.GetPrefixedName(), nil, resource)

			log("Deleting imported resource", resource, func() {
				AwaitResourceDeletionOrFail(ctx, t, cfg, resource, it.WaitDeletionTimeout)
			})

			log("Deleting dependent resources", resource, func() {
				if it.DependentResourceDirectory != "" {
					DeleteResourcesIgnoreMissing(ctx, t, cfg, it.DependentResourceDirectory, it.WaitDeletionTimeout)
				}
			})
			return ctx
		},
	)
}

// log is a helper function to log the start and end of an operation on a managed resource with name and external name.
func log(msg string, mr resource.Managed, f func(), keysAndValues ...any) {
	kAndV := []interface{}{"name", mr.GetName(), "external-name", xpmeta.GetExternalName(mr)}
	kAndV = append(kAndV, keysAndValues...)

	klog.InfoS("STARTING: "+msg, kAndV...)
	f()
	klog.InfoS("DONE: "+msg, kAndV...)
}
