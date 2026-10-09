package main

import (
	"context"
	"maps"
	"regexp"
	"testing"

	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/google/go-cmp/cmp"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/input/v1beta1"
	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btp"
	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btp/fake"
	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

// ---------------------------------------------------------------------------
// TestSmSubaccountID
// ---------------------------------------------------------------------------

func TestSmSubaccountID(t *testing.T) {
	t.Parallel()

	type args struct {
		observed map[resource.Name]resource.ObservedComposed
	}
	type want struct {
		id string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"V1Alpha1ServiceInstanceSubaccountId": {
			reason: "v1alpha1 ServiceInstance with spec.forProvider.subaccountId returns it",
			args: args{
				observed: map[resource.Name]resource.ObservedComposed{
					"si": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withSpec(map[string]any{"forProvider": map[string]any{"subaccountId": "sub-from-si"}})),
				},
			},
			want: want{id: "sub-from-si"},
		},
		"V1Beta1ServiceManagerSubaccountGuid": {
			reason: "v1beta1 ServiceManager with spec.forProvider.subaccountGuid returns it",
			args: args{
				observed: map[resource.Name]resource.ObservedComposed{
					"sm": newObservedResource(accountV1Beta1GroupVersion, kindServiceManager, withSpec(map[string]any{"forProvider": map[string]any{"subaccountGuid": "sub-from-sm"}})),
				},
			},
			want: want{id: "sub-from-sm"},
		},
		"V1Alpha1KymaEnvironmentSubaccountGuid": {
			reason: "v1alpha1 KymaEnvironment with spec.subaccountGuid returns it",
			args: args{
				observed: map[resource.Name]resource.ObservedComposed{
					"kyma": newObservedResource(environmentV1Alpha1GroupVersion, kindKymaEnvironment, withSpec(map[string]any{"subaccountGuid": "sub-from-kyma"})),
				},
			},
			want: want{id: "sub-from-kyma"},
		},
		"NoSMResources": {
			reason: "no SM-related resources in observed returns empty string",
			args: args{
				observed: map[resource.Name]resource.ObservedComposed{
					"unrelated": newObservedResource("other.group/v1", "Widget", withSpec(map[string]any{"forProvider": map[string]any{"name": "foo"}})),
				},
			},
			want: want{id: ""},
		},
		"UnknownGVK": {
			reason: "resource with unknown GVK returns empty string",
			args: args{
				observed: map[resource.Name]resource.ObservedComposed{
					"unknown": newObservedResource("unknown.io/v1", "Unknown", withSpec(map[string]any{"forProvider": map[string]any{"subaccountId": "should-not-match"}})),
				},
			},
			want: want{id: ""},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := smSubaccountID(tc.args.observed)
			if diff := cmp.Diff(tc.want.id, got); diff != "" {
				t.Errorf("%s\nsmSubaccountID(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestEnvBindingName
// ---------------------------------------------------------------------------

func TestEnvBindingName(t *testing.T) {
	t.Parallel()

	type args struct {
		desired  map[resource.Name]*resource.DesiredComposed
		observed map[resource.Name]resource.ObservedComposed
	}
	type want struct {
		name string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"KymaWithCloudManagementBindingName": {
			reason: "KymaEnvironment in desired + CloudManagement with serviceBindingName returns the binding name",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": newDesiredResource(environmentV1Alpha1GroupVersion, kindKymaEnvironment, withName("my-kyma-env")),
					"my-cm":   newDesiredResource(accountV1Alpha1GroupVersion, kindCloudManagement, withCompositeKey("cm-inst", "custom-cm-binding")),
				},
				observed: map[resource.Name]resource.ObservedComposed{},
			},
			want: want{name: "custom-cm-binding"},
		},
		"KymaWithoutCloudManagementReturnsDefault": {
			reason: "KymaEnvironment in desired, no CloudManagement returns defaultCMBindingName",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": newDesiredResource(environmentV1Alpha1GroupVersion, kindKymaEnvironment, withName("my-kyma-env")),
				},
				observed: map[resource.Name]resource.ObservedComposed{},
			},
			want: want{name: defaultCMBindingName},
		},
		"NoEnvironmentResources": {
			reason: "no environment resources in desired returns empty string",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-si": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				observed: map[resource.Name]resource.ObservedComposed{},
			},
			want: want{name: ""},
		},
		"CloudManagementBindingFromObserved": {
			reason: "KymaEnvironment in desired, CloudManagement binding name only in observed returns observed name",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": newDesiredResource(environmentV1Alpha1GroupVersion, kindKymaEnvironment, withName("my-kyma-env")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"my-cm": newObservedResource(accountV1Alpha1GroupVersion, kindCloudManagement, withSpec(map[string]any{"forProvider": map[string]any{"serviceBindingName": "observed-cm-binding"}})),
				},
			},
			want: want{name: "observed-cm-binding"},
		},
		"CloudFoundryEnvironmentAlsoCounts": {
			reason: "CloudFoundryEnvironment in desired also triggers env binding lookup",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-cf": newDesiredResource(environmentV1Alpha1GroupVersion, kindCloudFoundryEnvironment, withName("my-cf-env")),
				},
				observed: map[resource.Name]resource.ObservedComposed{},
			},
			want: want{name: defaultCMBindingName},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := envBindingName(tc.args.desired, tc.args.observed)
			if diff := cmp.Diff(tc.want.name, got); diff != "" {
				t.Errorf("%s\nenvBindingName(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// resource builders
// ---------------------------------------------------------------------------

// resourceOption customizes a composed resource under construction.
type resourceOption func(*composed.Unstructured)

// newDesiredResource builds a desired composed resource of the given GVK with
// empty metadata and spec, then applies opts.
func newDesiredResource(apiVersion, kind string, opts ...resourceOption) *resource.DesiredComposed {
	return &resource.DesiredComposed{Resource: newComposed(apiVersion, kind, opts...)}
}

// newObservedResource builds an observed composed resource of the given GVK
// with empty metadata, then applies opts.
func newObservedResource(apiVersion, kind string, opts ...resourceOption) resource.ObservedComposed {
	return resource.ObservedComposed{Resource: newComposed(apiVersion, kind, opts...)}
}

func newComposed(apiVersion, kind string, opts ...resourceOption) *composed.Unstructured {
	cd := composed.New()
	cd.Object = map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{},
	}
	for _, opt := range opts {
		opt(cd)
	}
	return cd
}

// forProvider returns the resource's spec.forProvider map, creating it.
func forProvider(cd *composed.Unstructured) map[string]any {
	spec, _ := cd.Object["spec"].(map[string]any)
	if spec == nil {
		spec = map[string]any{}
		cd.Object["spec"] = spec
	}
	fp, _ := spec["forProvider"].(map[string]any)
	if fp == nil {
		fp = map[string]any{}
		spec["forProvider"] = fp
	}
	return fp
}

// withName sets spec.forProvider.name.
func withName(name string) resourceOption {
	return func(cd *composed.Unstructured) { forProvider(cd)["name"] = name }
}

// withIdentity declares the offering/plan identity the composition expects;
// empty values are left unset.
func withIdentity(offeringName, planName string) resourceOption {
	return func(cd *composed.Unstructured) {
		fp := forProvider(cd)
		if offeringName != "" {
			fp["offeringName"] = offeringName
		}
		if planName != "" {
			fp["planName"] = planName
		}
	}
}

// withCompositeKey sets the ServiceManager instance/binding name pair; empty
// values are left unset.
func withCompositeKey(instName, bindName string) resourceOption {
	return func(cd *composed.Unstructured) {
		fp := forProvider(cd)
		if instName != "" {
			fp["serviceInstanceName"] = instName
		}
		if bindName != "" {
			fp["serviceBindingName"] = bindName
		}
	}
}

// withSubaccount sets the Subaccount subdomain/region pair.
func withSubaccount(subdomain, region string) resourceOption {
	return func(cd *composed.Unstructured) {
		fp := forProvider(cd)
		fp["subdomain"] = subdomain
		fp["region"] = region
	}
}

// withSubscription sets the Subscription appName/planName pair. Both keys
// are always written, including an empty planName — the provider treats an
// empty plan as the cockpit "default" plan, so presence matters.
func withSubscription(appName, planName string) resourceOption {
	return func(cd *composed.Unstructured) {
		fp := forProvider(cd)
		fp["appName"] = appName
		fp["planName"] = planName
	}
}

// withSpec merges fields into the resource's spec.
func withSpec(fields map[string]any) resourceOption {
	return func(cd *composed.Unstructured) {
		spec, _ := cd.Object["spec"].(map[string]any)
		if spec == nil {
			spec = map[string]any{}
			cd.Object["spec"] = spec
		}
		maps.Copy(spec, fields)
	}
}

// withMetadataName sets metadata.name.
func withMetadataName(name string) resourceOption {
	return func(cd *composed.Unstructured) { cd.SetName(name) }
}

// withExternalName sets the crossplane.io/external-name annotation.
func withExternalName(name string) resourceOption {
	return func(cd *composed.Unstructured) { xpmeta.SetExternalName(cd, name) }
}

// withAnnotation sets one annotation.
func withAnnotation(key, value string) resourceOption {
	return func(cd *composed.Unstructured) {
		annotations := cd.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[key] = value
		cd.SetAnnotations(annotations)
	}
}

// ---------------------------------------------------------------------------
// TestNewResolveCtxNoSubaccountSkipsSM
// ---------------------------------------------------------------------------

// TestNewResolverNoSubaccountSkipsSM is a focused test rather than a table:
// it pins the caller-side policy that no SM admin binding is acquired while no
// observed resource carries a resolved subaccount ID, even with import
// candidates present. The mock is down, so any HTTP request would fail and
// surface as a warning or error; the skip must be silent — no warning, no
// error, just a nil SM client.
func TestNewResolverNoSubaccountSkipsSM(t *testing.T) {
	t.Parallel()

	cfg := btptest.Default()
	cfg.Down = true
	trap := btptest.Serve(t, cfg)

	creds := &btp.CISCredentials{}
	creds.UAA.ClientID = "cid"
	creds.UAA.ClientSecret = "csecret"
	creds.UAA.URL = trap.URL
	creds.Endpoints.AccountsServiceURL = trap.URL

	var warnings []error
	r, err := newResolver(context.Background(), resolveConfig{
		Creds: creds,
		Desired: map[resource.Name]*resource.DesiredComposed{
			"si": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
		},
		Input: &v1beta1.Input{Mode: v1beta1.ModeAuto},
		Log:   logging.NewNopLogger(),
		Warn:  func(e error) { warnings = append(warnings, e) },
	})
	if err != nil {
		t.Fatalf("newResolver(): unexpected error: %v", err)
	}
	if r.sm != nil {
		t.Errorf("newResolver(): want nil SM client when no subaccount ID is observed, got non-nil")
	}
	if len(warnings) != 0 {
		t.Errorf("newResolver(): want no warnings, got %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// TestResolveExternalNames
// ---------------------------------------------------------------------------

func TestResolveExternalNames(t *testing.T) {
	t.Parallel()

	type args struct {
		desired      map[resource.Name]*resource.DesiredComposed
		observed     map[resource.Name]resource.ObservedComposed
		sm           smClient
		accounts     accountsClient
		provisioning provisioningClient
		mode         v1beta1.Mode
		include      []*regexp.Regexp
		exclude      []*regexp.Regexp
	}
	// result is everything observable from one resolveAll pass: the
	// external-name of every desired resource afterwards, the warnings
	// emitted, the plan-identity calls made against the SM fake, and the
	// summary counters.
	type result struct {
		externalNames map[resource.Name]string
		warningCount  int
		planGets      int
		summary       resolveSummary
	}
	type want struct {
		result result
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SubaccountImported": {
			reason: "Subaccount with matching subdomain+region → external-name set to guid",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subaccount": newDesiredResource(accountV1Alpha1GroupVersion, kindSubaccount, withSubaccount("my-subdomain", "eu10")),
				},
				sm:       &fake.SMClient{},
				accounts: &fake.AccountsClient{Subaccounts: map[string]string{"my-subdomain/eu10": "sa-guid-1"}},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subaccount": "sa-guid-1"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"SubaccountNoMatch": {
			reason: "Subaccount with no matching subdomain+region → external-name left unset, no warning",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subaccount": newDesiredResource(accountV1Alpha1GroupVersion, kindSubaccount, withSubaccount("my-subdomain", "eu10")),
				},
				sm:       &fake.SMClient{},
				accounts: &fake.AccountsClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subaccount": ""},
				summary:       resolveSummary{total: 1, noMatches: 1},
			}},
		},
		"SubaccountLookupError": {
			reason: "Accounts API returns error → warning emitted, external-name left unset",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subaccount": newDesiredResource(accountV1Alpha1GroupVersion, kindSubaccount, withSubaccount("my-subdomain", "eu10")),
				},
				sm:       &fake.SMClient{},
				accounts: &fake.AccountsClient{Err: errors.New("accounts api unavailable")},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subaccount": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"UnknownGVKSkipped": {
			reason: "a resource with an unknown GVK passes through unchanged",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"unknown-resource": newDesiredResource("unknown.group/v1", "Unknown", withName("my-thing")),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"unknown-resource": ""},
				summary:       resolveSummary{},
			}},
		},
		"ExternalNameAlreadySet": {
			reason: "a resource with external-name already set is not touched",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withExternalName("existing-uuid")),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "existing-uuid"},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"ObservedExternalNameSkipsLookupAndPropagates": {
			reason: "a resource whose OBSERVED counterpart carries a real external-name is already bound — no lookup runs, and the bound value is propagated into desired: the composition's field manager owns the annotation on adopted resources, so omitting it from the render would strip it via server-side apply and restart the import loop",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("bound-uuid")),
				},
				// no sm client on purpose: a lookup attempt would warn "SM client
				// not available"; want.warningCount 0 proves no lookup ran.
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "bound-uuid"},
				summary:       resolveSummary{total: 1, skipped: 1, propagated: 1},
			}},
		},
		"ObservedExternalNamePropagationIsIdempotent": {
			reason: "second render: desired already carries the propagated external-name — the value is stable, nothing is re-propagated, no lookup runs",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withExternalName("bound-uuid")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("bound-uuid")),
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "bound-uuid"},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"ObservedExternalNamePropagatedInExplicitModeWithAnnotation": {
			reason: "mode=explicit with the lookup annotation: an adopted resource keeps its external-name across renders",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withAnnotation("import.btp.sap.crossplane.io/lookup", "true")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("bound-uuid")),
				},
				mode: v1beta1.ModeExplicit,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "bound-uuid"},
				summary:       resolveSummary{total: 1, skipped: 1, propagated: 1},
			}},
		},
		"ObservedExternalNameNotPropagatedInExplicitModeWithoutAnnotation": {
			reason: "mode=explicit without the lookup annotation: the importer never stamped this resource, so the provider-owned observed external-name must not be adopted into the render",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("bound-uuid")),
				},
				mode: v1beta1.ModeExplicit,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"ObservedExternalNameNotPropagatedWhenExcluded": {
			reason: "a resource matched by the exclude filter is outside the importer's population — its observed external-name stays provider-owned",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("bound-uuid")),
				},
				exclude: []*regexp.Regexp{regexp.MustCompile("^service-instance$")},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"DesiredExternalNameWinsOverObserved": {
			reason: "a composition-templated external-name is explicit intent — propagation must not overwrite it even when observed differs",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withExternalName("template-uuid")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("other-uuid")),
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "template-uuid"},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"ModeExplicitSkipsUnannotated": {
			reason: "mode=explicit skips resources without the lookup annotation",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm:   &fake.SMClient{},
				mode: v1beta1.ModeExplicit,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"ModeExplicitProcessesAnnotated": {
			reason: "mode=explicit processes resources with the lookup annotation",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withAnnotation("import.btp.sap.crossplane.io/lookup", "true")),
				},
				sm:   &fake.SMClient{Instances: map[string]*btp.SMResource{"my-instance": {ID: "found-uuid"}}},
				mode: v1beta1.ModeExplicit,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "found-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"FilterIncludeSkips": {
			reason: "resource name not matching include pattern is skipped",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"other-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm:      &fake.SMClient{},
				include: []*regexp.Regexp{regexp.MustCompile(`^prod-`)},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"other-instance": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"FilterExcludeSkips": {
			reason: "resource name matching exclude pattern is skipped",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"test-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm:      &fake.SMClient{},
				exclude: []*regexp.Regexp{regexp.MustCompile(`^test-`)},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"test-instance": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"FilterIncludeMatchProceedsToLookup": {
			reason: "resource name matching include pattern proceeds to lookup",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"prod-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm:      &fake.SMClient{Instances: map[string]*btp.SMResource{"my-instance": {ID: "prod-uuid"}}},
				include: []*regexp.Regexp{regexp.MustCompile(`^prod-`)},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"prod-instance": "prod-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"ServiceInstanceLookupSuccess": {
			reason: "SM returns a match → external-name set",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm: &fake.SMClient{Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid"}}},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "instance-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"BindingDeferredWhenNoInstanceKnown": {
			reason: "a name-matched binding cannot be verified while no composition-managed instance UUID is known — skipped without a warning this pass, converges once an instance resolves",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-binding": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceBinding, withName("my-binding")),
				},
				sm: &fake.SMClient{Bindings: map[string]*btp.SMResource{"my-binding": {ID: "binding-uuid", ServiceInstanceID: "inst-uuid"}}},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-binding": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"ZeroResultsLeaveUnset": {
			reason: "SM returns no match → external-name left unset",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				summary:       resolveSummary{total: 1, noMatches: 1},
			}},
		},
		"SMClientNilEmitsWarning": {
			reason: "SM client nil → warning emitted, resource skipped",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm: nil,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"SMClientReturnsError": {
			reason: "SM API error → warning emitted, external-name left unset",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm: &fake.SMClient{Err: errors.New("connection refused")},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"EnvironmentLookupNilProvisioningClient": {
			reason: "provisioning client nil → warning emitted regardless of resource fields",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": newDesiredResource(environmentV1Alpha1GroupVersion, kindKymaEnvironment, withName("my-kyma-env")),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-kyma": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"EnvironmentLookupKymaSuccess": {
			reason: "KymaEnvironment with provisioning client → external-name set using name+plan disambiguation",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": {Resource: func() *composed.Unstructured {
						cd := composed.New()
						cd.Object = map[string]any{
							"apiVersion": environmentV1Alpha1GroupVersion,
							"kind":       kindKymaEnvironment,
							"metadata":   map[string]any{},
							"spec":       map[string]any{"forProvider": map[string]any{"name": "my-kyma-env", "planName": "aws"}},
						}
						return cd
					}()},
				},
				sm: &fake.SMClient{},
				provisioning: &fake.ProvisioningClient{
					Kyma: map[string]string{"my-kyma-env/aws": "kyma-env-uuid"},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-kyma": "kyma-env-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"EnvironmentLookupCloudFoundrySuccess": {
			reason: "CloudFoundryEnvironment uses spec.forProvider.environmentName for lookup",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-cf": {Resource: func() *composed.Unstructured {
						cd := composed.New()
						cd.Object = map[string]any{
							"apiVersion": environmentV1Alpha1GroupVersion,
							"kind":       kindCloudFoundryEnvironment,
							"metadata":   map[string]any{},
							"spec":       map[string]any{"forProvider": map[string]any{"environmentName": "my-cf-env"}},
						}
						return cd
					}()},
				},
				sm: &fake.SMClient{},
				provisioning: &fake.ProvisioningClient{
					CloudFoundry: map[string]string{"my-cf-env": "cf-env-uuid"},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-cf": "cf-env-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"EnvironmentLookupFindError": {
			reason: "FindKymaEnvironment returns error → warning emitted, external-name left unset",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": {Resource: func() *composed.Unstructured {
						cd := composed.New()
						cd.Object = map[string]any{
							"apiVersion": environmentV1Alpha1GroupVersion,
							"kind":       kindKymaEnvironment,
							"metadata":   map[string]any{},
							"spec":       map[string]any{"forProvider": map[string]any{"name": "my-kyma-env", "planName": "aws"}},
						}
						return cd
					}()},
				},
				sm:           &fake.SMClient{},
				provisioning: &fake.ProvisioningClient{Err: errors.New("api unavailable")},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-kyma": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"EnvironmentLookupMissingName": {
			reason: "KymaEnvironment missing spec.forProvider.name → warning emitted",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": {Resource: func() *composed.Unstructured {
						cd := composed.New()
						cd.Object = map[string]any{
							"apiVersion": environmentV1Alpha1GroupVersion,
							"kind":       kindKymaEnvironment,
							"metadata":   map[string]any{},
							"spec":       map[string]any{"forProvider": map[string]any{"planName": "aws"}},
						}
						return cd
					}()},
				},
				sm:           &fake.SMClient{},
				provisioning: &fake.ProvisioningClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-kyma": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"EnvironmentLookupMissingPlanName": {
			reason: "KymaEnvironment missing spec.forProvider.planName → warning emitted",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-kyma": {Resource: func() *composed.Unstructured {
						cd := composed.New()
						cd.Object = map[string]any{
							"apiVersion": environmentV1Alpha1GroupVersion,
							"kind":       kindKymaEnvironment,
							"metadata":   map[string]any{},
							"spec":       map[string]any{"forProvider": map[string]any{"name": "my-kyma-env"}},
						}
						return cd
					}()},
				},
				sm:           &fake.SMClient{},
				provisioning: &fake.ProvisioningClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-kyma": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"EnvironmentLookupCFMissingEnvironmentName": {
			reason: "CloudFoundryEnvironment missing spec.forProvider.environmentName → warning emitted",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-cf": {Resource: func() *composed.Unstructured {
						cd := composed.New()
						cd.Object = map[string]any{
							"apiVersion": environmentV1Alpha1GroupVersion,
							"kind":       kindCloudFoundryEnvironment,
							"metadata":   map[string]any{},
							"spec":       map[string]any{"forProvider": map[string]any{}},
						}
						return cd
					}()},
				},
				sm:           &fake.SMClient{},
				provisioning: &fake.ProvisioningClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-cf": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"CompositeKeySuccess": {
			reason: "ServiceManager lookup finds instance + binding → composite external-name set",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-sm": newDesiredResource(accountV1Beta1GroupVersion, kindServiceManager, withCompositeKey("my-inst", "my-bind")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-inst": {ID: "inst-uuid"}},
					Bindings:  map[string]*btp.SMResource{"my-bind": {ID: "bind-uuid", ServiceInstanceID: "inst-uuid"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-sm": "inst-uuid/bind-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"CompositeKeyUsesDefaults": {
			reason: "ServiceManager without explicit instance/binding names falls back to the managed-service-manager defaults",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-sm": newDesiredResource(accountV1Beta1GroupVersion, kindServiceManager, withName("ignored")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"managed-service-manager": {ID: "inst-uuid"}},
					Bindings:  map[string]*btp.SMResource{"managed-service-manager-binding": {ID: "bind-uuid", ServiceInstanceID: "inst-uuid"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-sm": "inst-uuid/bind-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"CompositeKeyInstanceNotFound": {
			reason: "ServiceManager instance not found → external-name left unset",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-sm": newDesiredResource(accountV1Beta1GroupVersion, kindServiceManager, withCompositeKey("missing", "my-bind")),
				},
				sm: &fake.SMClient{Bindings: map[string]*btp.SMResource{"my-bind": {ID: "bind-uuid"}}},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-sm": ""},
				summary:       resolveSummary{total: 1, noMatches: 1},
			}},
		},
		"CompositeKeyBindingNotFound": {
			reason: "ServiceManager instance found but binding not → external-name left unset",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-sm": newDesiredResource(accountV1Beta1GroupVersion, kindServiceManager, withCompositeKey("my-inst", "missing")),
				},
				sm: &fake.SMClient{Instances: map[string]*btp.SMResource{"my-inst": {ID: "inst-uuid"}}},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-sm": ""},
				summary:       resolveSummary{total: 1, noMatches: 1},
			}},
		},
		"InstanceIdentityMatchStamped": {
			reason: "a name match whose plan resolves to the offering/plan the spec declares is adopted",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "instance-uuid"},
				planGets:      1,
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"InstancePlanMismatchBlocked": {
			reason: "a name match on a different plan is refused with a warning naming expected vs actual — adopting it would let the provider mutate or delete a foreign service",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana-large"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				warningCount:  1,
				planGets:      1,
				summary: resolveSummary{total: 1, identityMismatch: 1, blocked: []string{
					`instance "my-instance" exists but is hana-cloud/hana-large, expected hana-cloud/hana; skipping import`,
				}},
			}},
		},
		"InstanceOfferingMismatchBlocked": {
			reason: "a name match on a different offering is refused — the reviewer scenario: same instance name, entirely different service",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "cloud-logging", Plan: "standard"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				warningCount:  1,
				planGets:      1,
				summary: resolveSummary{total: 1, identityMismatch: 1, blocked: []string{
					`instance "my-instance" exists but is cloud-logging/standard, expected hana-cloud/hana; skipping import`,
				}},
			}},
		},
		"InstanceWithoutIdentitySpecAdoptedLeniently": {
			reason: "a spec that declares no offering/plan names cannot be verified — the name match is adopted as before, without any plan lookup",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "instance-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"InstanceWithOnlyOfferingNameBlocked": {
			reason: "partial identity is verified in full, not half-checked — a declared offering with an undeclared plan mismatches rather than silently passing",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				warningCount:  1,
				planGets:      1,
				summary: resolveSummary{total: 1, identityMismatch: 1, blocked: []string{
					`instance "my-instance" exists but is hana-cloud/hana, expected hana-cloud/; skipping import`,
				}},
			}},
		},
		"InstancePlanDetailFailureBlocked": {
			reason: "a plan that cannot be resolved (removed from the catalog) is a verification failure — refused, operator can stamp manually",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-gone"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				warningCount:  1,
				planGets:      1,
				summary: resolveSummary{total: 1, identityMismatch: 1, blocked: []string{
					`instance "my-instance" exists but its plan identity could not be verified: service plan "plan-gone" not found; skipping import`,
				}},
			}},
		},
		"InstanceMismatchBlockedInExplicitModeToo": {
			reason: "the lookup annotation opts a resource into the import population, not out of verification — explicit mode blocks a mismatch the same way",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana"), withAnnotation("import.btp.sap.crossplane.io/lookup", "true")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "cloud-logging", Plan: "standard"}},
				},
				mode: v1beta1.ModeExplicit,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": ""},
				warningCount:  1,
				planGets:      1,
				summary: resolveSummary{total: 1, identityMismatch: 1, blocked: []string{
					`instance "my-instance" exists but is cloud-logging/standard, expected hana-cloud/hana; skipping import`,
				}},
			}},
		},
		"IdentityVerificationSkippedForExcluded": {
			reason: "a resource outside the import population is never looked up, so verification costs nothing for it",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"test-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana"}},
				},
				exclude: []*regexp.Regexp{regexp.MustCompile(`^test-`)},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"test-instance": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"BindingLinkedToInstanceStampedSamePass": {
			reason: "a binding whose service instance ID matches an instance stamped earlier in the same pass is adopted — DR re-import of instance and binding converges in one reconcile",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana")),
					"service-binding":  newDesiredResource(accountV1Alpha1GroupVersion, kindServiceBinding, withName("my-binding")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Bindings:  map[string]*btp.SMResource{"my-binding": {ID: "binding-uuid", ServiceInstanceID: "instance-uuid"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "instance-uuid", "service-binding": "binding-uuid"},
				planGets:      1,
				summary:       resolveSummary{total: 2, imported: 2},
			}},
		},
		"BindingLinkedToObservedInstanceStamped": {
			reason: "a binding whose service instance ID matches an already-bound observed instance is adopted",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-binding": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceBinding, withName("my-binding")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("instance-uuid")),
				},
				sm: &fake.SMClient{
					Bindings: map[string]*btp.SMResource{"my-binding": {ID: "binding-uuid", ServiceInstanceID: "instance-uuid"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-binding": "binding-uuid"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"BindingForeignInstanceBlocked": {
			reason: "a name-matched binding that belongs to an instance outside this composition is refused — adopting it would hand its credentials and lifecycle to the wrong owner",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-binding": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceBinding, withName("my-binding")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("instance-uuid")),
				},
				sm: &fake.SMClient{
					Bindings: map[string]*btp.SMResource{"my-binding": {ID: "binding-uuid", ServiceInstanceID: "other-uuid"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-binding": ""},
				warningCount:  1,
				summary: resolveSummary{total: 1, identityMismatch: 1, blocked: []string{
					`binding "my-binding" belongs to instance other-uuid, not managed by this composition; skipping import`,
				}},
			}},
		},
		"TwoPhaseResolvesBindingDeclaredFirst": {
			reason: "resolution order is instances before bindings regardless of resource-name order — a binding sorting ahead of its instance still finds the UUID set populated",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"a-binding":  newDesiredResource(accountV1Alpha1GroupVersion, kindServiceBinding, withName("my-binding")),
					"z-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Bindings:  map[string]*btp.SMResource{"my-binding": {ID: "binding-uuid", ServiceInstanceID: "instance-uuid"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"z-instance": "instance-uuid", "a-binding": "binding-uuid"},
				summary:       resolveSummary{total: 2, imported: 2},
			}},
		},
		"CompositeKeyUnlinkedBindingBlocked": {
			reason: "a composite key must be a real pair — a name-matched binding that belongs to a different instance than the name-matched instance is refused",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-sm": newDesiredResource(accountV1Beta1GroupVersion, kindServiceManager, withCompositeKey("my-inst", "my-bind")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-inst": {ID: "inst-uuid"}},
					Bindings:  map[string]*btp.SMResource{"my-bind": {ID: "bind-uuid", ServiceInstanceID: "other-uuid"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-sm": ""},
				warningCount:  1,
				summary: resolveSummary{total: 1, identityMismatch: 1, blocked: []string{
					`binding "my-bind" belongs to instance other-uuid, not inst-uuid; skipping import`,
				}},
			}},
		},
		"PlanCacheCollapsesRepeatedGets": {
			reason: "two instances sharing a plan resolve its identity once per pass — the cache bounds the plan GETs the import window repeats",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"instance-a": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("instance-a"), withIdentity("hana-cloud", "hana")),
					"instance-b": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("instance-b"), withIdentity("hana-cloud", "hana")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{
						"instance-a": {ID: "uuid-a", ServicePlanID: "plan-1"},
						"instance-b": {ID: "uuid-b", ServicePlanID: "plan-1"},
					},
					Plans: map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"instance-a": "uuid-a", "instance-b": "uuid-b"},
				planGets:      1,
				summary:       resolveSummary{total: 2, imported: 2},
			}},
		},
		"SecondPassOverVerifiedStateIsNoOp": {
			reason: "once an instance is adopted and observed bound, later renders neither look it up nor re-verify — steady state makes zero SM calls",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance"), withIdentity("hana-cloud", "hana"), withExternalName("instance-uuid")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("instance-uuid")),
				},
				sm: &fake.SMClient{
					Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid", ServicePlanID: "plan-1"}},
					Plans:     map[string]fake.PlanIdentity{"plan-1": {Offering: "hana-cloud", Plan: "hana"}},
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "instance-uuid"},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"SubscriptionStampedFromSpec": {
			reason: "a Subscription's external-name is appName/planName straight from the desired spec — no client involved, counted as imported",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan")),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": "my-app/my-plan"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"SubscriptionEmptyPlanNameStampsTrailingSlash": {
			reason: "planName present but empty is the provider's cockpit-default plan — the stamp is appName followed by a bare slash, exactly what the provider writes",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "")),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": "my-app/"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"SubscriptionMissingAppNameWarns": {
			reason: "appName key absent → nothing stamped, one warning naming the resource",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSpec(map[string]any{"forProvider": map[string]any{"planName": "my-plan"}})),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"SubscriptionEmptyAppNameWarns": {
			reason: "appName present but empty → nothing stamped, one warning; a bare '/plan' is never invented",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("", "my-plan")),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"SubscriptionNonStringAppNameWarns": {
			reason: "appName that is not a string (a number) → nothing stamped, one warning",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSpec(map[string]any{"forProvider": map[string]any{"appName": 42, "planName": "my-plan"}})),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"SubscriptionMissingPlanNameWarns": {
			reason: "planName key absent (CRD-invalid input) → nothing stamped, one warning; missing is not the same as empty, so 'app/' is never invented",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSpec(map[string]any{"forProvider": map[string]any{"appName": "my-app"}})),
				},
				sm: &fake.SMClient{},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": ""},
				warningCount:  1,
				summary:       resolveSummary{total: 1, warnings: 1},
			}},
		},
		"SubscriptionObservedExternalNamePropagatedAndSkipped": {
			reason: "an adopted Subscription observed with its external-name is skipped and the value is carried into the desired render, same as every other kind",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"my-subscription": newObservedResource(accountV1Alpha1GroupVersion, kindSubscription, withMetadataName("my-subscription-x7k2p"), withExternalName("my-app/my-plan")),
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": "my-app/my-plan"},
				summary:       resolveSummary{total: 1, propagated: 1, skipped: 1},
			}},
		},
		"SubscriptionSecondPassIsNoOp": {
			reason: "second render over a stamped, observed-bound Subscription: the value is stable and nothing is re-imported or re-propagated",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan"), withExternalName("my-app/my-plan")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"my-subscription": newObservedResource(accountV1Alpha1GroupVersion, kindSubscription, withMetadataName("my-subscription-x7k2p"), withExternalName("my-app/my-plan")),
				},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": "my-app/my-plan"},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"SubscriptionResolvesWithoutSMClient": {
			reason: "no subaccount observed, so no SM client exists — a Subscription still stamps because its lookup never needs one",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan")),
				},
				sm: nil,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": "my-app/my-plan"},
				summary:       resolveSummary{total: 1, imported: 1},
			}},
		},
		"SubscriptionExplicitModeHonoursAnnotation": {
			reason: "mode=explicit: the annotated Subscription is stamped, the unannotated one is untouched",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"annotated-subscription":   newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan"), withAnnotation(lookupAnnotation, "true")),
					"unannotated-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("other-app", "other-plan")),
				},
				sm:   &fake.SMClient{},
				mode: v1beta1.ModeExplicit,
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"annotated-subscription": "my-app/my-plan", "unannotated-subscription": ""},
				summary:       resolveSummary{total: 2, imported: 1, skipped: 1},
			}},
		},
		"SubscriptionIncludeFilterScopesStamp": {
			reason: "include pattern: the matching Subscription is stamped, the non-matching one is skipped",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"included-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan")),
					"other-subscription":    newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("other-app", "other-plan")),
				},
				sm:      &fake.SMClient{},
				include: []*regexp.Regexp{regexp.MustCompile("^included-")},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"included-subscription": "my-app/my-plan", "other-subscription": ""},
				summary:       resolveSummary{total: 2, imported: 1, skipped: 1},
			}},
		},
		"SubscriptionExcludeFilterSkips": {
			reason: "exclude pattern matching the Subscription → untouched and counted as skipped (filters have no counter of their own)",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan")),
				},
				sm:      &fake.SMClient{},
				exclude: []*regexp.Regexp{regexp.MustCompile("subscription")},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"my-subscription": ""},
				summary:       resolveSummary{total: 1, skipped: 1},
			}},
		},
		"SubscriptionAndServiceInstanceResolveInOnePass": {
			reason: "a Subscription beside a ServiceInstance: the instance resolves through SM, the Subscription from its spec, both stamped in the same pass",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
					"my-subscription":  newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("other-app", "other-plan")),
				},
				sm: &fake.SMClient{Instances: map[string]*btp.SMResource{"my-instance": {ID: "instance-uuid"}}},
			},
			want: want{result: result{
				externalNames: map[resource.Name]string{"service-instance": "instance-uuid", "my-subscription": "other-app/other-plan"},
				summary:       resolveSummary{total: 2, imported: 2},
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mode := tc.args.mode
			if mode == "" {
				mode = v1beta1.ModeAuto
			}

			warnings := 0
			r := &resolver{
				sm:           tc.args.sm,
				accounts:     tc.args.accounts,
				provisioning: tc.args.provisioning,
				observed:     tc.args.observed,
				mode:         mode,
				include:      tc.args.include,
				exclude:      tc.args.exclude,
				log:          logging.NewNopLogger(),
				warn:         func(_ error) { warnings++ },
			}

			summary := r.resolveAll(context.Background(), tc.args.desired)

			got := result{externalNames: map[resource.Name]string{}, warningCount: warnings, summary: summary}
			for resName, dcd := range tc.args.desired {
				got.externalNames[resName] = xpmeta.GetExternalName(dcd.Resource)
			}
			if sm, ok := tc.args.sm.(*fake.SMClient); ok {
				got.planGets = sm.PlanGets
			}

			if diff := cmp.Diff(tc.want.result, got, cmp.AllowUnexported(result{}, resolveSummary{})); diff != "" {
				t.Errorf("%s\nresolveAll(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestAnyLookupCandidates
// ---------------------------------------------------------------------------

func TestAnyLookupCandidates(t *testing.T) {
	t.Parallel()

	type args struct {
		desired  map[resource.Name]*resource.DesiredComposed
		observed map[resource.Name]resource.ObservedComposed
	}
	type want struct {
		candidates bool
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"UnboundResourceIsCandidate": {
			reason: "an unbound supported resource is a lookup candidate",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
			},
			want: want{candidates: true},
		},
		"ObservedBoundResourceIsNoCandidate": {
			reason: "an observed-bound resource must not be a candidate — the SM binding would be acquired for nothing",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"service-instance": newDesiredResource(accountV1Alpha1GroupVersion, kindServiceInstance, withName("my-instance")),
				},
				observed: map[resource.Name]resource.ObservedComposed{
					"service-instance": newObservedResource(accountV1Alpha1GroupVersion, kindServiceInstance, withMetadataName("my-instance-x7k2p"), withExternalName("bound-uuid")),
				},
			},
			want: want{candidates: false},
		},
		"UnsupportedGVKIsNoCandidate": {
			reason: "an unsupported GVK must not be a candidate",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"thing": newDesiredResource("unknown.group/v1", "Unknown", withName("my-thing")),
				},
			},
			want: want{candidates: false},
		},
		"UnboundSubscriptionIsCandidate": {
			reason: "a Subscription still needing its stamp is a lookup candidate like any other supported kind, even though its lookup never uses SM — the documented trade-off is that this alone can trigger one SM binding acquisition",
			args: args{
				desired: map[resource.Name]*resource.DesiredComposed{
					"my-subscription": newDesiredResource(accountV1Alpha1GroupVersion, kindSubscription, withSubscription("my-app", "my-plan")),
				},
			},
			want: want{candidates: true},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := anyLookupCandidates(tc.args.desired, tc.args.observed, v1beta1.ModeAuto, nil, nil, logging.NewNopLogger())
			if diff := cmp.Diff(tc.want.candidates, got); diff != "" {
				t.Errorf("%s\nanyLookupCandidates(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestLookupTableConsistency(t *testing.T) {
	t.Parallel()

	// Every dispatch entry must be reachable through the support filter, and
	// every entry must carry a callable lookup — a nil entry would pass the
	// filter and panic at dispatch.
	for key, fn := range lookups {
		if !isSupportedGVK(key.apiVersion, key.kind) {
			t.Errorf("isSupportedGVK(%q, %q) = false for a dispatch table key; supported and dispatched must be the same fact", key.apiVersion, key.kind)
		}
		if fn == nil {
			t.Errorf("lookups[%+v] is nil; a nil entry passes the support filter and panics at dispatch", key)
		}
	}

	type args struct {
		apiVersion string
		kind       string
	}
	type want struct {
		supported bool
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"UnknownGroupRejected": {
			reason: "A GVK with no dispatch table entry must be reported unsupported.",
			args:   args{apiVersion: "unknown.group/v1", kind: "Unknown"},
			want:   want{supported: false},
		},
		"KnownKindInWrongGroupVersionRejected": {
			reason: "Support is keyed on the full apiVersion/kind pair, not the kind alone.",
			args:   args{apiVersion: environmentV1Alpha1GroupVersion, kind: kindServiceInstance},
			want:   want{supported: false},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := isSupportedGVK(tc.args.apiVersion, tc.args.kind)
			if diff := cmp.Diff(tc.want.supported, got); diff != "" {
				t.Errorf("%s\nisSupportedGVK(%q, %q): -want, +got:\n%s", tc.reason, tc.args.apiVersion, tc.args.kind, diff)
			}
		})
	}
}
