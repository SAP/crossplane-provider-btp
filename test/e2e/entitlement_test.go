//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	entApi "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	entitlementSubaccountName = "entitlement-sa-test"
	entitlements              = &v1alpha1.EntitlementList{}
)

const (
	entitlementIdentityLabelKey   = "external-name-resolver-test"
	entitlementIdentityLabelValue = "true"

	entitlementSelectorResolvedName = "postgres-development-selector"
)

func TestEntitlements(t *testing.T) {
	crudFeatureSuite := features.New("BTP Entitlement Controller").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("entitlement"))
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
			"Check Entitlement identity is immutable", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subaccount := GetSubaccountOrError(t, cfg, entitlementSubaccountName)
				resolvedSubaccountGuid := internal.Val(subaccount.Status.AtProvider.SubaccountGuid)
				if resolvedSubaccountGuid == "" {
					t.Fatalf("subaccount %s has no resolved GUID yet", entitlementSubaccountName)
				}

				// 1. Patch entitlement-sa-test so the selector-only Entitlement
				// below can resolve it without depending on any other fixture's
				// labels.
				subaccountLabelPatch := k8s.Patch{
					PatchType: types.MergePatchType,
					Data: []byte(fmt.Sprintf(
						`{"metadata":{"labels":{"%s":"%s"}}}`,
						entitlementIdentityLabelKey,
						entitlementIdentityLabelValue,
					)),
				}
				if err := cfg.Client().Resources().Patch(ctx, subaccount, subaccountLabelPatch); err != nil {
					t.Fatalf("failed to label %s: %v", entitlementSubaccountName, err)
				}
				t.Cleanup(func() {
					if err := cfg.Client().Resources().Patch(ctx, subaccount, k8s.Patch{
						PatchType: types.MergePatchType,
						Data: []byte(fmt.Sprintf(
							`{"metadata":{"labels":{"%s":null}}}`,
							entitlementIdentityLabelKey,
						)),
					}); err != nil {
						t.Fatalf("failed to remove label from %s: %v", entitlementSubaccountName, err)
					}
				})

				// 2. A selector-only Entitlement: the real generated resolver must
				// resolve both subaccountGuid and subaccountRef together in one
				// patch against the already-existing object.
				selectorResolved := &v1alpha1.Entitlement{
					ObjectMeta: metav1.ObjectMeta{
						Name:      entitlementSelectorResolvedName,
						Namespace: cfg.Namespace(),
					},
					Spec: v1alpha1.EntitlementSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &xpv1.Reference{Name: "default"},
						},
						ForProvider: v1alpha1.EntitlementParameters{
							ServiceName:     "postgresql-db",
							ServicePlanName: "development",
							Amount:          internal.Ptr(0),
							SubaccountSelector: &xpv1.Selector{
								MatchLabels: map[string]string{
									entitlementIdentityLabelKey: entitlementIdentityLabelValue,
								},
							},
						},
					},
				}
				if err := cfg.Client().Resources().Create(ctx, selectorResolved); err != nil {
					t.Fatalf("failed to create %s: %v", entitlementSelectorResolvedName, err)
				}
				t.Cleanup(func() {
					resources.AwaitResourceDeletionOrFail(ctx, t, cfg, GetEntitlementOrError(t, cfg, entitlementSelectorResolvedName))
				})

				// 3. Wait until the resolver's absent->present patch is admitted
				// and reaches Observe.
				waitForEntitlementIdentityResolved(cfg, t, entitlementSelectorResolvedName, wait.WithTimeout(5*time.Minute))

				// 4-5. Fetch a fresh copy before each rejected mutation: status
				// keeps changing resourceVersion while the controller reconciles,
				// so reusing a stale copy would race a conflict error instead of
				// exercising the validation rule.
				requireEntitlementUpdateRejected(ctx, t, cfg,
					GetEntitlementOrError(t, cfg, entitlementSelectorResolvedName),
					"serviceName cannot be changed",
					func(e *v1alpha1.Entitlement) { e.Spec.ForProvider.ServiceName = "hana-cloud" },
				)
				requireEntitlementUpdateRejected(ctx, t, cfg,
					GetEntitlementOrError(t, cfg, entitlementSelectorResolvedName),
					"servicePlanUniqueIdentifier cannot be changed",
					func(e *v1alpha1.Entitlement) {
						e.Spec.ForProvider.ServicePlanUniqueIdentifier = internal.Ptr("qualifier-a")
					},
				)
				requireEntitlementUpdateRejected(ctx, t, cfg,
					GetEntitlementOrError(t, cfg, entitlementSelectorResolvedName),
					"subaccountGuid cannot be changed after resolution",
					func(e *v1alpha1.Entitlement) {
						e.Spec.ForProvider.SubaccountGuid = "00000000-0000-0000-0000-000000000000"
					},
				)
				requireEntitlementUpdateRejected(ctx, t, cfg,
					GetEntitlementOrError(t, cfg, entitlementSelectorResolvedName),
					"subaccountRef cannot be changed after subaccountGuid is resolved",
					func(e *v1alpha1.Entitlement) {
						e.Spec.ForProvider.SubaccountRef = &xpv1.Reference{Name: "some-other-subaccount"}
					},
				)

				// The immutability rules above pin a field to its created value,
				// so an empty identity segment admitted at creation could never
				// be repaired. MinLength=1 rejects it up front instead.
				requireEntitlementCreateRejected(ctx, t, cfg, "serviceName",
					func(e *v1alpha1.Entitlement) { e.Spec.ForProvider.ServiceName = "" },
				)

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

											logAwaitedDiff(expectedAmount, entlmt.Status.AtProvider.Assigned)

											if entlmt.Status.AtProvider.Assigned == nil {
												return false
											}
											got := entlmt.Status.AtProvider.Assigned.Amount
											if diff := cmp.Diff(&expectedAmount, got, test.EquateErrors()); diff != "" {
												return false
											}
											return true
										},
										wait.WithTimeout(time.Minute*3),
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

func requireEntitlementUpdateRejected(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	base *v1alpha1.Entitlement,
	message string,
	mutate func(*v1alpha1.Entitlement),
) {
	t.Helper()
	change := base.DeepCopy()
	mutate(change)
	err := cfg.Client().Resources().Update(ctx, change)
	if err == nil {
		t.Fatalf("expected update to be rejected: %s", message)
	}
	if !strings.Contains(err.Error(), message) {
		t.Fatalf("expected %q in validation error, got %v", message, err)
	}
}

// requireEntitlementCreateRejected asserts the API server rejects a
// well-formed Entitlement once mutate blanks out field. The CR is never
// admitted, so there is nothing to clean up; a unique name per call keeps a
// rejection from being mistaken for an AlreadyExists conflict.
func requireEntitlementCreateRejected(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	field string,
	mutate func(*v1alpha1.Entitlement),
) {
	t.Helper()
	candidate := &v1alpha1.Entitlement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-entitlement-blank-" + strings.ToLower(field),
			Namespace: cfg.Namespace(),
		},
		Spec: v1alpha1.EntitlementSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: v1alpha1.EntitlementParameters{
				ServiceName:     "postgresql-db",
				ServicePlanName: "development",
				Amount:          internal.Ptr(0),
				SubaccountSelector: &xpv1.Selector{
					MatchLabels: map[string]string{
						entitlementIdentityLabelKey: entitlementIdentityLabelValue,
					},
				},
			},
		},
	}
	mutate(candidate)
	err := cfg.Client().Resources().Create(ctx, candidate)
	if err == nil {
		t.Cleanup(func() {
			resources.AwaitResourceDeletionOrFail(ctx, t, cfg, candidate)
		})
		t.Fatalf("expected create to be rejected for blank %s", field)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("expected %q in validation error, got %v", field, err)
	}
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

// waitForEntitlementIdentityResolved waits until the generated reference
// resolver has written both spec.forProvider.subaccountGuid and
// spec.forProvider.subaccountRef.name in one patch, and the controller has
// produced at least one status.atProvider observation.
func waitForEntitlementIdentityResolved(cfg *envconf.Config, t *testing.T, entitlementName string, opts ...wait.Option) {
	t.Helper()
	res := newEntitlementResource(cfg, entitlementName)
	err := wait.For(
		conditions.New(cfg.Client().Resources()).ResourceMatch(
			res, func(object k8s.Object) bool {
				e := object.(*v1alpha1.Entitlement)
				return e.Spec.ForProvider.SubaccountGuid != "" &&
					e.Spec.ForProvider.SubaccountRef != nil &&
					e.Spec.ForProvider.SubaccountRef.Name != "" &&
					e.Status.AtProvider != nil
			},
		),
		opts...,
	)
	if err != nil {
		t.Fatalf("entitlement %s did not resolve identity: %v", entitlementName, err)
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

func logAwaitedDiff(expected int, assignable *entApi.Assignable) {
	amount := "nil"
	if assignable != nil {
		amount = fmt.Sprintf("%v", internal.Val(assignable.Amount))
	}
	klog.V(4).Infof(
		"Checking Diff on Amount: Expected %v, Got %v",
		expected,
		amount,
	)
}

// TestEntitlementImportFlow tests the import flow for Entitlement.
// ADR(external-name): uses compound key "<subaccountGuid>/<serviceName>/<servicePlanName>"
// (e.g. "abc-123/postgresql-db/development"); the fixture omits servicePlanUniqueIdentifier,
// so the key has exactly three segments.
//
// The dependent Subaccount (testdata/crs/EntitlementImport/subaccount.yaml) is isolated from
// entitlement-sa-test (used by TestEntitlements) to avoid a teardown race between the suites.
func TestEntitlementImportFlow(t *testing.T) {
	const importName = "entitlement-import-test"
	amount := 1
	importTester := NewImportTester(
		&v1alpha1.Entitlement{
			Spec: v1alpha1.EntitlementSpec{
				ForProvider: v1alpha1.EntitlementParameters{
					ServiceName:     "postgresql-db",
					ServicePlanName: "development",
					Amount:          &amount,
					SubaccountRef:   &xpv1.Reference{Name: importName},
				},
			},
		},
		importName,
		WithWaitCreateTimeout[*v1alpha1.Entitlement](wait.WithTimeout(5*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.Entitlement](wait.WithTimeout(5*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.Entitlement](crsPath("EntitlementImport")),
		WithWaitDependentResourceTimeout[*v1alpha1.Entitlement](wait.WithTimeout(15*time.Minute)),
	)
	testenv.Test(t, importTester.BuildTestFeature("Entitlement Import Flow").
		Assess(
			"Imported entitlement resolves compound external-name and status",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subaccount := GetSubaccountOrError(t, cfg, importName)
				resolvedSubaccountGuid := internal.Val(subaccount.Status.AtProvider.SubaccountGuid)
				if resolvedSubaccountGuid == "" {
					t.Fatalf("dependent subaccount %s has no resolved GUID", importName)
				}

				imported := GetEntitlementOrError(t, cfg, importTester.GetPrefixedName())

				externalName := xpmeta.GetExternalName(imported)
				if externalName == "" {
					t.Fatalf(
						"imported entitlement %s has an empty external-name annotation; "+
							"the resource could not be fetched or was never annotated with an external-name",
						importTester.GetPrefixedName(),
					)
				}
				segments := strings.Split(externalName, "/")
				if len(segments) != 3 {
					t.Fatalf(
						"expected a 3-segment compound external-name (no servicePlanUniqueIdentifier is set on the fixture), got %q (%d segments)",
						externalName, len(segments),
					)
				}
				if segments[0] != resolvedSubaccountGuid {
					t.Fatalf(
						"external-name segment 1 (subaccountGuid) = %q, want resolved subaccountGuid %q",
						segments[0], resolvedSubaccountGuid,
					)
				}
				if segments[1] != "postgresql-db" {
					t.Fatalf("external-name segment 2 (serviceName) = %q, want %q", segments[1], "postgresql-db")
				}
				if segments[2] != "development" {
					t.Fatalf("external-name segment 3 (servicePlanName) = %q, want %q", segments[2], "development")
				}

				if imported.Status.AtProvider == nil || imported.Status.AtProvider.Assigned == nil {
					t.Fatalf("expected status.atProvider.assigned to be non-nil on the imported entitlement")
				}

				readyCondition := imported.GetCondition(xpv1.Available().Type)
				if readyCondition.Status != v1.ConditionTrue {
					t.Fatalf(
						"expected imported entitlement to be Ready, got status=%v reason=%v",
						readyCondition.Status, readyCondition.Reason,
					)
				}

				// A correctly imported resource must never have gone through
				// ExternalClient.Create: the reconciler sets the external-create
				// pending/succeeded/failed annotations only around its own Create
				// call, so any of them present here means Observe failed to match
				// the existing BTP assignment and the reconciler re-created it.
				annotations := imported.GetAnnotations()
				pending := annotations[xpmeta.AnnotationKeyExternalCreatePending]
				succeeded := annotations[xpmeta.AnnotationKeyExternalCreateSucceeded]
				failed := annotations[xpmeta.AnnotationKeyExternalCreateFailed]
				if pending != "" || succeeded != "" || failed != "" {
					t.Fatalf(
						"imported entitlement carries an external-create annotation "+
							"(pending=%q succeeded=%q failed=%q): the presence of any of these "+
							"proves the provider CREATED a new assignment instead of adopting the "+
							"surviving one via its external-name, which defeats the import contract",
						pending, succeeded, failed,
					)
				}
				return ctx
			},
		).
		Feature())
}
