//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
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

// ImportTester helps to build e2e test feature for import flow of a managed resource.
// T is the type of the managed resource to be imported.
// Use NewImportTester to create an instance, then use BuildTestFeature to build the test feature.
// Use ImportTesterOption to customize timeouts.
type ImportTester[T resource.Managed] struct {
	//will be used as importing resource. The ObjectMeta.Name will be set automatically.
	BaseResource T

	// will be prefixed with BUILD_ID to ensure uniqueness
	BaseName     string
	prefixedName string

	// the timeout for waiting till resource get healthy after creating (in setup and assess)
	WaitCreateTimeout wait.Option

	// the timeout for waiting till resource get deleted (in setup and teardown)
	WaitDeletionTimeout wait.Option
}

const (
	importFeatureContextKey = "importExternalName"
)

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
	it.prefixedName = NewID(it.BaseName, envvar.Get(UUT_BUILD_ID_KEY))
	it.BaseResource.SetName(it.prefixedName)

	for _, opt := range o {
		opt(it)
	}

	return it
}

func (it *ImportTester[T]) BuildTestFeature(name string) *features.FeatureBuilder {
	return features.New(name).
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				//prepare the resource for creation
				createResource := it.BaseResource.DeepCopyObject().(T)
				createResource.SetManagementPolicies(importManagementPolicies)

				if err := cfg.Client().Resources().Create(ctx, createResource); err != nil {
					t.Fatalf("Failed to create Subaccount for import test: %v", err)
				}

				waitForResource(createResource, cfg, t, it.WaitCreateTimeout)

				// load resource to get the external name
				createdResource := it.BaseResource.DeepCopyObject().(T)
				MustGetResource(t, cfg, it.prefixedName, nil, createdResource)
				externalName := xpmeta.GetExternalName(createdResource)

				klog.InfoS("Resource created on external system to be imported later", "type", createdResource.GetObjectKind().GroupVersionKind().String(), "externalName", externalName)
				ctx = context.WithValue(ctx, importFeatureContextKey, externalName)

				// delete the created resource to prepare for import. With managment policies missing Delete, it will not be deleted in the external system
				klog.InfoS("Deleting managed resource without deleting the resource on external system ", "type", createdResource.GetObjectKind().GroupVersionKind().String(), "externalName", externalName)
				AwaitResourceDeletionOrFail(ctx, t, cfg, createdResource, it.WaitDeletionTimeout)

				return ctx
			},
		).Assess(
		"Check Imported Subaccount gets healthy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			externalName := ctx.Value(importFeatureContextKey).(string)

			//preare the resource for import
			resource := it.BaseResource.DeepCopyObject().(T)
			xpmeta.SetExternalName(resource, externalName)
			resource.SetManagementPolicies(xpv1.ManagementPolicies{xpv1.ManagementActionAll})

			//create the resource again for importing, should match the external resource
			if err := cfg.Client().Resources().Create(ctx, resource); err != nil {
				t.Fatalf("Failed to create Subaccount for import test: %v", err)
			}
			klog.InfoS("Waiting for imported resource to become healthy", "type", resource.GetObjectKind().GroupVersionKind().String(), "externalName", externalName)

			waitForResource(resource, cfg, t, it.WaitCreateTimeout)

			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			resource := it.BaseResource.DeepCopyObject().(T)
			MustGetResource(t, cfg, it.prefixedName, nil, resource)

			AwaitResourceDeletionOrFail(ctx, t, cfg, resource, it.WaitDeletionTimeout)
			klog.InfoS("Managed resource deleted (also on external system)", "type", resource.GetObjectKind().GroupVersionKind().String(), "externalName", xpmeta.GetExternalName(resource))

			return ctx
		},
	)
}
