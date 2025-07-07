//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"

	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	saK8sResName               = "e2e-test-sa"
	dirk8sResName              = "e2e-test-directory-sa"
	subaccountNameE2e          string
	subaccountDirectoryNameE2e string
)

func TestAccount(t *testing.T) {
	subaccountNameE2e = NewID(saK8sResName, BUILD_ID)
	subaccountDirectoryNameE2e = NewID("e2e-test-directory-sa", BUILD_ID)

	crudFeature := features.New("BTP Subaccount Controller").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				mutateResource := mutateSubAccResource()
				createK8sResources(ctx, t, cfg, r, "subaccount", "*", mutateResource)

				waitForResource(newSubaccountResource(cfg, saK8sResName), cfg, t)
				return ctx
			},
		).
		Assess(
			"Check Subaccount Created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subaccountObserved := GetSubaccountOrError(t, cfg, saK8sResName)
				klog.InfoS("Subaccount Details", "cr", subaccountObserved)
				return ctx
			},
		).
		Assess(
			"Check Subaccount Updated", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				observed := GetSubaccountOrError(t, cfg, saK8sResName)

				// Updated Subaccount
				subaccount := observed.DeepCopy()
				want := NewID("Updated e2e Name", BUILD_ID)
				subaccount.Spec.ForProvider.DisplayName = want

				resources.AwaitResourceUpdateOrError(ctx, t, cfg, subaccount)

				resources.AwaitResourceUpdateFor(
					ctx, t, cfg, subaccount,
					func(object k8s.Object) bool {
						sa := object.(*v1alpha1.Subaccount)
						got := sa.Status.AtProvider.DisplayName
						if diff := cmp.Diff(want, *got, test.EquateErrors()); diff != "" {
							return false
						}
						return true
					},
					wait.WithTimeout(time.Second*90),
				)
				return ctx
			},
		).
		Assess(
			"Check rejected Updates", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				observed := GetSubaccountOrError(t, cfg, saK8sResName)

				// Updated Subaccount
				subaccount := observed.DeepCopy()
				// change admins should be rejected by K8s validation rules annotated in _types
				subaccount.Spec.ForProvider.SubaccountAdmins = append(subaccount.Spec.ForProvider.SubaccountAdmins, "changedemail")

				err := cfg.Client().Resources().Update(ctx, subaccount)
				if err == nil {
					t.Fatal("Expected validation error")
				}
				return ctx
			},
		).
		Assess(
			"Check Subaccount moved to directory", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				observed := GetSubaccountOrError(t, cfg, saK8sResName)

				// Updated Subaccount
				subaccount := observed.DeepCopy()
				want := &xpv1.Reference{
					Name: dirk8sResName,
					Policy: &xpv1.Policy{
						Resolve: internal.Ptr(xpv1.ResolvePolicyAlways),
					},
				}
				subaccount.Spec.ForProvider.DirectoryRef = want

				resources.AwaitResourceUpdateOrError(ctx, t, cfg, subaccount)

				resources.AwaitResourceUpdateFor(
					ctx, t, cfg, subaccount,
					func(object k8s.Object) bool {
						sa := object.(*v1alpha1.Subaccount)
						return sa.Status.AtProvider.ParentGuid != nil
					},
					wait.WithTimeout(time.Second*90),
				)
				return ctx
			},
		).
		Assess(
			"Check Subaccount Deleted", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subaccountObserved := GetSubaccountOrError(t, cfg, saK8sResName)
				resources.AwaitResourceDeletionOrFail(ctx, t, cfg, subaccountObserved)
				directoryObserved := GetDirectoryOrError(t, cfg, dirk8sResName)
				resources.AwaitResourceDeletionOrFail(ctx, t, cfg, directoryObserved)
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			resources.DumpManagedResources(ctx, t, cfg)
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeature)
}

func mutateSubAccResource() func(obj k8s.Object) error {
	mutateResource := func(obj k8s.Object) error {

		if mg, ok := any(obj).(*v1alpha1.Subaccount); ok {
			newId := subaccountNameE2e
			mg.SetExternalID(newId)
		}

		if mg, ok := any(obj).(*v1alpha1.Directory); ok {
			newId := subaccountDirectoryNameE2e
			mg.SetExternalID(newId)
		}

		return nil
	}
	return mutateResource
}

func GetSubaccountOrError(t *testing.T, cfg *envconf.Config, subaccount string) *v1alpha1.Subaccount {
	ct := &v1alpha1.Subaccount{}
	namespace := cfg.Namespace()
	res := cfg.Client().Resources()

	err := res.Get(context.TODO(), subaccount, namespace, ct)
	if err != nil {
		t.Error("Failed to get Subaccount. error : ", err)
	}
	return ct
}

func newSubaccountResource(cfg *envconf.Config, subaccountNameE2e string) *v1alpha1.Subaccount {
	return &v1alpha1.Subaccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: subaccountNameE2e, Namespace: cfg.Namespace(),
		},
	}
}
