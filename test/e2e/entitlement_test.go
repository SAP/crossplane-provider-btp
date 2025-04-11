//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	entitlementSubaccountName = "entitlement-sa-test"
	entitlements              = &v1alpha1.EntitlementList{}
)

func TestEntitlements(t *testing.T) {
	crudFeatureSuite := features.New("BTP Entitlement Controller").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/entitlement")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())
				unfilteredEntitlements := &v1alpha1.EntitlementList{}
				r.List(ctx, unfilteredEntitlements)

				for _, entitlement := range unfilteredEntitlements.Items {
					if entitlement.Spec.ForProvider.ServiceName != "cis" {
						entitlements.Items = append(entitlements.Items, entitlement)
					}
				}

				for _, entitlement := range entitlements.Items {
					waitForEntitlementResource(cfg, t, entitlement.Name)
				}
				return ctx
			},
		).
		Assess(
			"Check Entitlements are managed", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				crudFeatures := []features.Feature{}
				for _, entitlement := range entitlements.Items {
					entitlementName := strings.Clone(entitlement.Name)
					crudFeature := features.New(fmt.Sprintf("Entitlement %s", entitlementName)).
						Assess(
							"Check Entitlement is created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
								entitlementObserved := GetEntitlementOrError(t, cfg, entitlementName)
								klog.InfoS("Entitlement Details", "cr", entitlementObserved)
								return ctx
							},
						).
						Assess(
							"Check Entitlements are updated", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
								entitlementObserved := GetEntitlementOrError(t, cfg, entitlementName)
								if entitlementObserved.Spec.ForProvider.Amount != nil {
									want := 2
									entitlement := entitlementObserved.DeepCopy()
									entitlement.Spec.ForProvider.Amount = &want

									resources.AwaitResourceUpdateOrError(ctx, t, cfg, entitlement)

									resources.AwaitResourceUpdateFor(ctx, t, cfg, entitlement,
										func(object k8s.Object) bool {
											entlmt := object.(*v1alpha1.Entitlement)
											expectedAmount := expectedAssignAmount(ctx, cfg, entlmt.Spec.ForProvider.ServiceName)
											if entlmt.Status.AtProvider.Assigned == nil {
												return false
											}
											got := entlmt.Status.AtProvider.Assigned.Amount
											if diff := cmp.Diff(&expectedAmount, got, test.EquateErrors()); diff != "" {
												return false
											}
											return true
										},
										wait.WithTimeout(time.Second*90),
									)
								}
								return ctx
							},
						).
						Assess(
							"Check Entitlements are deleted", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
								entitlementObserved := GetEntitlementOrError(t, cfg, entitlementName)
								resources.AwaitResourceDeletionOrFail(ctx, t, cfg, entitlementObserved)
								return ctx
							},
						).Feature()
					crudFeatures = append(crudFeatures, crudFeature)
				}
				testenv.Test(t, crudFeatures...)
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// have to delete the SA since it is a dependency before we can delete the GA
			subaccountObserved := GetSubaccountOrError(t, cfg, entitlementSubaccountName)
			resources.AwaitResourceDeletionOrFail(ctx, t, cfg, subaccountObserved)

			resources.DumpManagedResources(ctx, t, cfg)
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}

func GetEntitlementOrError(t *testing.T, cfg *envconf.Config, entitlement string) *v1alpha1.Entitlement {
	ct := &v1alpha1.Entitlement{}
	namespace := cfg.Namespace()
	res := cfg.Client().Resources()

	err := res.Get(context.TODO(), entitlement, namespace, ct)
	if err != nil {
		t.Error("Failed to get Entitlement. error : ", err)
	}
	return ct
}

func waitForEntitlementResource(cfg *envconf.Config, t *testing.T, entitlementName string) {
	client := cfg.Client()

	// Fetch the Entitlement resource via the client
	res := newEntitlementResource(cfg, entitlementName)
	err := wait.For(
		conditions.New(client.Resources()).ResourceMatch(
			res, func(object k8s.Object) bool {
				d := object.(*v1alpha1.Entitlement)
				condition := d.GetCondition(xpv1.Available().Type)
				result := condition.Status == v1.ConditionTrue
				klog.V(4).Infof(
					"Checking %s on %v. result=%v",
					resources.Identifier(d),
					condition,
					condition.Status == v1.ConditionTrue,
				)
				return result
			},
		),
	)

	if err != nil {
		t.Error(err)
	}
}

func expectedAssignAmount(ctx context.Context, cfg *envconf.Config, service string) int {
	client := cfg.Client()
	unfilteredEntitlements := &v1alpha1.EntitlementList{}
	client.Resources().List(ctx, unfilteredEntitlements)
	sum := 0

	for _, v := range unfilteredEntitlements.Items {
		if v.Spec.ForProvider.ServiceName == service && v.Spec.ForProvider.Amount != nil {
			sum = sum + *v.Spec.ForProvider.Amount
		}
	}
	return sum
}

func newEntitlementResource(cfg *envconf.Config, entitlementName string) *v1alpha1.Entitlement {
	return &v1alpha1.Entitlement{
		ObjectMeta: metav1.ObjectMeta{
			Name: entitlementName, Namespace: cfg.Namespace(),
		},
	}
}
