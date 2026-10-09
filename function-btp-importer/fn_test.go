package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	xplogging "github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/input/v1beta1"
	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

func TestRunFunction(t *testing.T) {
	t.Parallel()

	type args struct {
		setup    func(btptest.Config) btptest.Config           // mutates the default mock; nil = default
		buildReq func(mockURL string) *fnv1.RunFunctionRequest // the request, with credentials pointed at the mock
	}
	type want struct {
		rsp *fnv1.RunFunctionResponse
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Pass1DeclaresRequirement": {
			reason: "pass 1: no required_resources key → declare cis-secret requirement with correct namespace",
			args: args{
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					return buildPass1Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"service-instance": {
								Resource: desiredResourceStruct(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance"),
							},
						},
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"service-instance": {
							Resource: desiredResourceStruct(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass1MissingCISConfig": {
			reason: "pass 1: secretRef.name has no value source → Fatal",
			args: args{
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					input, _ := structpb.NewStruct(map[string]any{
						"apiVersion": "import.btp.sap.crossplane.io/v1beta1",
						"kind":       "Input",
						"mode":       "auto",
						"secretRef": map[string]any{
							"name":      map[string]any{},
							"namespace": map[string]any{"value": "test-ns"},
							"key":       "cisCredentials",
						},
					})
					return buildPass1Req(xrStruct("test-ns"), input, nil)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{{Severity: fnv1.Severity_SEVERITY_FATAL, Target: fnv1.Target_TARGET_COMPOSITE.Enum()}},
				Desired: &fnv1.State{},
			}},
		},
		"Pass1AmbiguousSecretRefFatal": {
			reason: "pass 1: secretRef.name sets both value and fromFieldPath — the ambiguous input is refused with a Fatal instead of silently resolving one and ignoring the other",
			args: args{
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					input, _ := structpb.NewStruct(map[string]any{
						"apiVersion": "import.btp.sap.crossplane.io/v1beta1",
						"kind":       "Input",
						"mode":       "auto",
						"secretRef": map[string]any{
							"name": map[string]any{
								"value":         "explicit-name",
								"fromFieldPath": "metadata.labels[crossplane.io/claim-namespace]",
							},
							"namespace": map[string]any{"value": "test-ns"},
							"key":       "cisCredentials",
						},
					})
					return buildPass1Req(xrStruct("test-ns"), input, nil)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{{Severity: fnv1.Severity_SEVERITY_FATAL, Target: fnv1.Target_TARGET_COMPOSITE.Enum()}},
				Desired: &fnv1.State{},
			}},
		},
		"Pass2SecretNotFound": {
			reason: "pass 2: required_resources key present with empty list → Fatal (secret not found)",
			args: args{
				setup: down,
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					return buildPass2ReqSecretNotFound(xrStruct("test-ns"), inputStruct("auto", "", ""))
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{{Severity: fnv1.Severity_SEVERITY_FATAL, Target: fnv1.Target_TARGET_COMPOSITE.Enum()}},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2SecretMalformed": {
			reason: "pass 2: secret credentials JSON missing required field → Fatal",
			args: args{
				setup: down,
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					encoded := base64.StdEncoding.EncodeToString([]byte(`{"uaa":{"clientid":"x","url":"y","subaccountid":"s"},"endpoints":{"accounts_service_url":"z"}}`))
					secretStruct, _ := structpb.NewStruct(map[string]any{
						"apiVersion": "v1",
						"kind":       "Secret",
						"data":       map[string]any{"cisCredentials": encoded},
					})
					return buildPass2Req(xrStruct("test-ns"), inputStruct("auto", "", ""), nil, secretStruct, nil)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{{Severity: fnv1.Severity_SEVERITY_FATAL, Target: fnv1.Target_TARGET_COMPOSITE.Enum()}},
				Desired: &fnv1.State{},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2InvalidRegexFatals": {
			reason: "pass 2: invalid RE2 in exclude patterns → Fatal",
			args: args{
				setup: down,
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(xrStruct("test-ns"), inputStruct("auto", "", "[invalid"), nil, secretStruct, nil)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{{Severity: fnv1.Severity_SEVERITY_FATAL, Target: fnv1.Target_TARGET_COMPOSITE.Enum()}},
				Desired: &fnv1.State{},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2SMContention": {
			reason: "pass 2: SM binding POST returns 409 — Warning emitted, desired resources passed through unchanged",
			args: args{
				// No existing binding, and the create contends with provider-btp.
				setup: withFaults(
					btptest.Fault{Method: http.MethodGet, Path: smBindingPath, Status: http.StatusNotFound},
					btptest.Fault{Method: http.MethodPost, Path: smBindingPath, Status: http.StatusConflict},
				),
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"service-instance": {
								Resource: desiredResourceStruct(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance"),
							},
						},
						secretStruct,
						map[string]*fnv1.Resource{
							"service-instance": {Resource: observedServiceInstanceWithSubaccount("sub-1")},
						},
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta: &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{
					{Severity: fnv1.Severity_SEVERITY_WARNING, Target: fnv1.Target_TARGET_COMPOSITE.Enum()},
					{Severity: fnv1.Severity_SEVERITY_WARNING, Target: fnv1.Target_TARGET_COMPOSITE.Enum()},
				},
				Conditions: []*fnv1.Condition{
					{
						Type:    "ImportsVerified",
						Status:  fnv1.Status_STATUS_CONDITION_UNKNOWN,
						Reason:  "LookupErrors",
						Message: new("1 lookup(s) failed; identity verification incomplete this pass"),
						Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"service-instance": {
							Resource: desiredResourceStruct(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2ObservedExternalNamePropagated": {
			reason: "pass 2: observed resource already bound (adopted) — no lookup runs, and the observed external-name is propagated into the desired render so server-side apply keeps the composition-owned annotation",
			args: args{
				setup: down,
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"service-instance": {
								Resource: desiredResourceStruct(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance"),
							},
						},
						secretStruct,
						map[string]*fnv1.Resource{
							"service-instance": {Resource: observedResourceWithExternalName(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance-x7k2p", "bound-uuid")},
						},
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Conditions: []*fnv1.Condition{
					{
						Type:   "ImportsVerified",
						Status: fnv1.Status_STATUS_CONDITION_TRUE,
						Reason: "NothingToImport",
						Target: fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"service-instance": {
							Resource: desiredResourceStructWithExternalName(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance", "bound-uuid"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2UnknownGVKSkipped": {
			reason: "pass 2: unknown GVK in desired → passes through unchanged, no error",
			args: args{
				setup: down,
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"unknown-res": {
								Resource: desiredResourceStruct("unknown.group/v1", "Unknown", "my-thing"),
							},
						},
						secretStruct,
						nil,
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Conditions: []*fnv1.Condition{
					{
						Type:   "ImportsVerified",
						Status: fnv1.Status_STATUS_CONDITION_TRUE,
						Reason: "NothingToImport",
						Target: fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"unknown-res": {
							Resource: desiredResourceStruct("unknown.group/v1", "Unknown", "my-thing"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2IdentityMismatchBlocksImport": {
			reason: "pass 2, reviewer scenario end-to-end: the desired instance's name matches an existing SM instance that belongs to a different offering/plan — the import is refused with a warning, the claim-visible condition explains why, and no external-name is stamped",
			args: args{
				// An SM instance squats on the desired name under a different offering/plan.
				setup: func(c btptest.Config) btptest.Config {
					c.ServiceInstances = []btptest.SMResource{{Name: "my-db", ID: "squatter-uuid", ServicePlanID: "plan-logging"}}
					c.ServicePlans = []btptest.CatalogResource{{ID: "plan-logging", Name: "standard", ServiceOfferingID: "off-logging"}}
					c.ServiceOfferings = []btptest.CatalogResource{{ID: "off-logging", Name: "cloud-logging"}}
					return c
				},
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"service-instance": {
								Resource: desiredInstanceStructWithIdentity("my-db", "hana-cloud", "hana"),
							},
						},
						secretStruct,
						map[string]*fnv1.Resource{
							"service-instance": {Resource: observedServiceInstanceWithSubaccount("sub-1")},
						},
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta: &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{
					{Severity: fnv1.Severity_SEVERITY_WARNING, Target: fnv1.Target_TARGET_COMPOSITE.Enum()},
				},
				Conditions: []*fnv1.Condition{
					{
						Type:    "ImportsVerified",
						Status:  fnv1.Status_STATUS_CONDITION_FALSE,
						Reason:  "IdentityMismatch",
						Message: new(`instance "my-db" exists but is cloud-logging/standard, expected hana-cloud/hana; skipping import`),
						Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"service-instance": {
							Resource: desiredInstanceStructWithIdentity("my-db", "hana-cloud", "hana"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2ServiceInstanceImported": {
			reason: "pass 2 happy path end-to-end: credential parsing, OAuth, SM admin binding acquisition, and the fieldQuery lookup all succeed - the matched instance UUID is stamped as crossplane.io/external-name on the desired resource and the claim-visible condition reports the adoption",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.ServiceInstances = []btptest.SMResource{{Name: "my-instance", ID: "instance-uuid"}}
					return c
				},
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"service-instance": {
								Resource: desiredResourceStruct(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance"),
							},
						},
						secretStruct,
						map[string]*fnv1.Resource{
							"service-instance": {Resource: observedServiceInstanceWithSubaccount("sub-1")},
						},
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Conditions: []*fnv1.Condition{
					{
						Type:    "ImportsVerified",
						Status:  fnv1.Status_STATUS_CONDITION_TRUE,
						Reason:  "ResourcesImported",
						Message: new("1 existing BTP resource(s) adopted"),
						Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"service-instance": {
							Resource: desiredResourceStructWithExternalName(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance", "instance-uuid"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2ServiceManagerCompositeKeyImported": {
			reason: "pass 2, composite-key path end-to-end: a ServiceManager resolves through two SM fieldQuery lookups (instance by serviceInstanceName, binding by serviceBindingName), the binding's service_instance_id linkage to the matched instance is verified, and the instanceID/bindingID composite external-name is stamped",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.ServiceInstances = []btptest.SMResource{{Name: "managed-sm", ID: "sm-inst-uuid"}}
					c.ServiceBindings = []btptest.SMResource{{Name: "managed-sm-binding", ID: "sm-bind-uuid", ServiceInstanceID: "sm-inst-uuid"}}
					return c
				},
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"service-manager": {
								Resource: desiredComposedStruct(accountV1Alpha1GroupVersion, kindServiceManager, map[string]any{"serviceInstanceName": "managed-sm", "serviceBindingName": "managed-sm-binding"}, ""),
							},
						},
						secretStruct,
						map[string]*fnv1.Resource{
							"service-instance": {Resource: observedServiceInstanceWithSubaccount("sub-1")},
						},
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Conditions: []*fnv1.Condition{
					{
						Type:    "ImportsVerified",
						Status:  fnv1.Status_STATUS_CONDITION_TRUE,
						Reason:  "ResourcesImported",
						Message: new("1 existing BTP resource(s) adopted"),
						Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"service-manager": {
							Resource: desiredComposedStruct(accountV1Alpha1GroupVersion, kindServiceManager, map[string]any{"serviceInstanceName": "managed-sm", "serviceBindingName": "managed-sm-binding"}, "sm-inst-uuid/sm-bind-uuid"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2KymaEnvironmentChainImported": {
			reason: "pass 2, Kyma chain end-to-end: the SM admin binding resolves the default cloud-management binding via fieldQuery plus binding-detail GET, the provisioning client authenticates with the credentials from that binding (not the CIS secret), and the environments-list match on name and planName stamps the environment ID as external-name",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.ServiceBindings = []btptest.SMResource{{Name: "managed-cloud-management-binding", ID: "cm-bind-id"}}
					c.Environments = []btptest.Environment{{ID: "kyma-env-id", Name: "my-kyma", EnvironmentType: "kyma", PlanName: "aws"}}
					return c
				},
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"kyma-environment": {
								Resource: desiredComposedStruct(environmentV1Alpha1GroupVersion, kindKymaEnvironment, map[string]any{"name": "my-kyma", "planName": "aws"}, ""),
							},
						},
						secretStruct,
						map[string]*fnv1.Resource{
							"service-instance": {Resource: observedServiceInstanceWithSubaccount("sub-1")},
						},
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Conditions: []*fnv1.Condition{
					{
						Type:    "ImportsVerified",
						Status:  fnv1.Status_STATUS_CONDITION_TRUE,
						Reason:  "ResourcesImported",
						Message: new("1 existing BTP resource(s) adopted"),
						Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"kyma-environment": {
							Resource: desiredComposedStruct(environmentV1Alpha1GroupVersion, kindKymaEnvironment, map[string]any{"name": "my-kyma", "planName": "aws"}, "kyma-env-id"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2SubscriptionImported": {
			reason: "pass 2 end-to-end: a Subscription is stamped with appName/planName straight from its spec while the mock BTP is unreachable — proving the import needs no BTP call — and the claim-visible condition counts it as adopted (the stamp is the provider's identity key, existence is the provider's Observe to decide)",
			args: args{
				setup: down,
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"my-subscription": {
								Resource: desiredComposedStruct(accountV1Alpha1GroupVersion, kindSubscription, map[string]any{"appName": "my-app", "planName": "my-plan"}, ""),
							},
						},
						secretStruct,
						nil,
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Conditions: []*fnv1.Condition{
					{
						Type:    "ImportsVerified",
						Status:  fnv1.Status_STATUS_CONDITION_TRUE,
						Reason:  "ResourcesImported",
						Message: new("1 existing BTP resource(s) adopted"),
						Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"my-subscription": {
							Resource: desiredComposedStruct(accountV1Alpha1GroupVersion, kindSubscription, map[string]any{"appName": "my-app", "planName": "my-plan"}, "my-app/my-plan"),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass2SubscriptionMissingAppNameWarns": {
			reason: "pass 2 end-to-end: a Subscription whose spec has no appName is not stamped and surfaces one Warning naming it while the mock BTP is unreachable — the malformed spec is reported, never silently skipped or stamped as a bare plan",
			args: args{
				setup: down,
				buildReq: func(mockURL string) *fnv1.RunFunctionRequest {
					secretStruct := cisSecretStruct(mockURL, "sub-1")
					return buildPass2Req(
						xrStruct("test-ns"),
						inputStruct("auto", "", ""),
						map[string]*fnv1.Resource{
							"my-subscription": {
								Resource: desiredComposedStruct(accountV1Alpha1GroupVersion, kindSubscription, map[string]any{"planName": "my-plan"}, ""),
							},
						},
						secretStruct,
						nil,
					)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta: &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{
					{Severity: fnv1.Severity_SEVERITY_WARNING, Target: fnv1.Target_TARGET_COMPOSITE.Enum()},
				},
				Conditions: []*fnv1.Condition{
					{
						Type:    "ImportsVerified",
						Status:  fnv1.Status_STATUS_CONDITION_UNKNOWN,
						Reason:  "LookupErrors",
						Message: new("1 lookup(s) failed; identity verification incomplete this pass"),
						Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
					},
				},
				Desired: &fnv1.State{
					Resources: map[string]*fnv1.Resource{
						"my-subscription": {
							Resource: desiredComposedStruct(accountV1Alpha1GroupVersion, kindSubscription, map[string]any{"planName": "my-plan"}, ""),
						},
					},
				},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
					},
				},
			}},
		},
		"Pass1FromFieldPathResolution": {
			reason: "pass 1: secretRef.name resolved from XR field path → requirement declared with correct name",
			args: args{
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					input, _ := structpb.NewStruct(map[string]any{
						"apiVersion": "import.btp.sap.crossplane.io/v1beta1",
						"kind":       "Input",
						"mode":       "auto",
						"secretRef": map[string]any{
							"name":      map[string]any{"fromFieldPath": "metadata.labels[crossplane.io/claim-namespace]"},
							"namespace": map[string]any{"fromFieldPath": "metadata.labels[crossplane.io/claim-namespace]"},
							"key":       "cisCredentials",
						},
					})
					return buildPass1Req(xrStruct("test-ns"), input, nil)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Desired: &fnv1.State{},
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns"}},
					},
				},
			}},
		},
		"Pass1FromFieldPathMissing": {
			reason: "pass 1: secretRef.name references a missing XR field → Fatal",
			args: args{
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					input, _ := structpb.NewStruct(map[string]any{
						"apiVersion": "import.btp.sap.crossplane.io/v1beta1",
						"kind":       "Input",
						"mode":       "auto",
						"secretRef": map[string]any{
							"name":      map[string]any{"fromFieldPath": "spec.nonExistentField"},
							"namespace": map[string]any{"value": "test-ns"},
							"key":       "cisCredentials",
						},
					})
					return buildPass1Req(xrStruct("test-ns"), input, nil)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{{Severity: fnv1.Severity_SEVERITY_FATAL, Target: fnv1.Target_TARGET_COMPOSITE.Enum()}},
				Desired: &fnv1.State{},
			}},
		},
		"Pass1FromContextKeyResolution": {
			reason: "pass 1: secretRef resolved from pipeline context → requirement declared",
			args: args{
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					input, _ := structpb.NewStruct(map[string]any{
						"apiVersion": "import.btp.sap.crossplane.io/v1beta1",
						"kind":       "Input",
						"mode":       "auto",
						"secretRef": map[string]any{
							"name":      map[string]any{"fromContextKey": "btp.sap/cis-secret-name"},
							"namespace": map[string]any{"fromContextKey": "btp.sap/cis-secret-namespace"},
							"key":       "cisCredentials",
						},
					})
					req := buildPass1Req(xrStruct("test-ns"), input, nil)
					ctx, _ := structpb.NewStruct(map[string]any{
						"btp.sap/cis-secret-name":      "my-cis-secret",
						"btp.sap/cis-secret-namespace": "platform-ns",
					})
					req.Context = ctx
					return req
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: nil,
				Desired: &fnv1.State{},
				Context: contextStruct("btp.sap/cis-secret-name", "my-cis-secret", "btp.sap/cis-secret-namespace", "platform-ns"),
				Requirements: &fnv1.Requirements{
					Resources: map[string]*fnv1.ResourceSelector{
						cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("platform-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "my-cis-secret"}},
					},
				},
			}},
		},
		"Pass1FromContextKeyMissing": {
			reason: "pass 1: secretRef references a missing context key → Fatal",
			args: args{
				buildReq: func(_ string) *fnv1.RunFunctionRequest {
					input, _ := structpb.NewStruct(map[string]any{
						"apiVersion": "import.btp.sap.crossplane.io/v1beta1",
						"kind":       "Input",
						"mode":       "auto",
						"secretRef": map[string]any{
							"name":      map[string]any{"fromContextKey": "missing-key"},
							"namespace": map[string]any{"value": "test-ns"},
							"key":       "cisCredentials",
						},
					})
					return buildPass1Req(xrStruct("test-ns"), input, nil)
				},
			},
			want: want{rsp: &fnv1.RunFunctionResponse{
				Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
				Results: []*fnv1.Result{{Severity: fnv1.Severity_SEVERITY_FATAL, Target: fnv1.Target_TARGET_COMPOSITE.Enum()}},
				Desired: &fnv1.State{},
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			req := tc.args.buildReq(srv.URL)
			rsp, err := (&Function{log: logging.NewNopLogger()}).RunFunction(context.Background(), req)
			if diff := cmp.Diff(tc.want.rsp, rsp, protocmp.Transform(), protocmp.IgnoreFields(&fnv1.Result{}, "message")); diff != "" {
				t.Errorf("%s\nRunFunction(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nRunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}

// TestResolveValue pins the ValueSource contract: exactly one of value,
// fromFieldPath, or fromContextKey resolves; zero or multiple set is an error.
func TestResolveValue(t *testing.T) {
	t.Parallel()

	const (
		claimNSPath = "metadata.labels[crossplane.io/claim-namespace]"
		ctxKey      = "ck"
	)

	type args struct {
		src v1beta1.ValueSource
	}
	type want struct {
		value  string
		errMsg string // exact error message expected; "" skips the check
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValueAlone": {
			reason: "a static value resolves to itself",
			args:   args{src: v1beta1.ValueSource{Value: new("static")}},
			want:   want{value: "static"},
		},
		"FromFieldPathAlone": {
			reason: "a field path resolves against the observed composite",
			args:   args{src: v1beta1.ValueSource{FromFieldPath: new(claimNSPath)}},
			want:   want{value: "test-ns"},
		},
		"FromContextKeyAlone": {
			reason: "a context key resolves against the pipeline context",
			args:   args{src: v1beta1.ValueSource{FromContextKey: new(ctxKey)}},
			want:   want{value: "ctx-value"},
		},
		"NoneSet": {
			reason: "zero sources is an error — nothing to resolve",
			args:   args{src: v1beta1.ValueSource{}},
			want: want{
				errMsg: "one of value, fromFieldPath, or fromContextKey must be set",
				err:    cmpopts.AnyError,
			},
		},
		"ValueAndFromFieldPath": {
			reason: "two sources is ambiguous — refused instead of silently preferring value",
			args:   args{src: v1beta1.ValueSource{Value: new("static"), FromFieldPath: new(claimNSPath)}},
			want: want{
				errMsg: "exactly one of value, fromFieldPath, or fromContextKey must be set; found multiple",
				err:    cmpopts.AnyError,
			},
		},
		"ValueAndFromContextKey": {
			reason: "two sources is ambiguous — refused instead of silently preferring value",
			args:   args{src: v1beta1.ValueSource{Value: new("static"), FromContextKey: new(ctxKey)}},
			want: want{
				errMsg: "exactly one of value, fromFieldPath, or fromContextKey must be set; found multiple",
				err:    cmpopts.AnyError,
			},
		},
		"FromFieldPathAndFromContextKey": {
			reason: "two sources is ambiguous — refused instead of silently preferring fromFieldPath",
			args:   args{src: v1beta1.ValueSource{FromFieldPath: new(claimNSPath), FromContextKey: new(ctxKey)}},
			want: want{
				errMsg: "exactly one of value, fromFieldPath, or fromContextKey must be set; found multiple",
				err:    cmpopts.AnyError,
			},
		},
		"AllThreeSet": {
			reason: "all three sources is ambiguous — refused",
			args:   args{src: v1beta1.ValueSource{Value: new("static"), FromFieldPath: new(claimNSPath), FromContextKey: new(ctxKey)}},
			want: want{
				errMsg: "exactly one of value, fromFieldPath, or fromContextKey must be set; found multiple",
				err:    cmpopts.AnyError,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := buildPass1Req(xrStruct("test-ns"), inputStruct("auto", "", ""), nil)
			req.Context = contextStruct(ctxKey, "ctx-value")
			oxr, err := request.GetObservedCompositeResource(req)
			if err != nil {
				t.Fatalf("GetObservedCompositeResource(): unexpected error: %v", err)
			}

			got, err := resolveValue(tc.args.src, oxr, req)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nresolveValue(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if tc.want.errMsg != "" {
				gotMsg := ""
				if err != nil {
					gotMsg = err.Error()
				}
				if diff := cmp.Diff(tc.want.errMsg, gotMsg); diff != "" {
					t.Errorf("%s\nresolveValue(): -want error message, +got:\n%s", tc.reason, diff)
				}
			}
			if diff := cmp.Diff(tc.want.value, got); diff != "" {
				t.Errorf("%s\nresolveValue(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// captureLogger records log messages for assertion; keys and values are
// discarded. Not safe for concurrent use — RunFunction logs from a single
// goroutine.
type captureLogger struct {
	messages []string
}

func (l *captureLogger) Info(msg string, _ ...any)            { l.messages = append(l.messages, msg) }
func (l *captureLogger) Debug(msg string, _ ...any)           { l.messages = append(l.messages, msg) }
func (l *captureLogger) WithValues(_ ...any) xplogging.Logger { return l }

// TestRunFunctionLogsCleanupFailure is a focused test rather than a table: it
// needs a capturing logger where the shared runner uses a nop one. When the SM
// admin binding was created by this invocation and its cleanup DELETE fails,
// the failure is logged for operators — but cleanup stays best-effort, so the
// response must be exactly the one the same import produces when cleanup
// succeeds.
func TestRunFunctionLogsCleanupFailure(t *testing.T) {
	t.Parallel()
	cfg := btptest.Default()
	cfg.ServiceInstances = []btptest.SMResource{{Name: "my-instance", ID: "instance-uuid"}}
	// No existing binding forces the POST path, so cleanup owns the binding;
	// its DELETE then fails.
	cfg = withFaults(
		btptest.Fault{Method: http.MethodGet, Path: smBindingPath, Status: http.StatusNotFound},
		btptest.Fault{Method: http.MethodDelete, Path: smBindingPath, Status: http.StatusInternalServerError},
	)(cfg)
	srv := btptest.Serve(t, cfg)
	secretStruct := cisSecretStruct(srv.URL, "sub-1")
	req := buildPass2Req(
		xrStruct("test-ns"),
		inputStruct("auto", "", ""),
		map[string]*fnv1.Resource{
			"service-instance": {
				Resource: desiredResourceStruct(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance"),
			},
		},
		secretStruct,
		map[string]*fnv1.Resource{
			"service-instance": {Resource: observedServiceInstanceWithSubaccount("sub-1")},
		},
	)

	wantRsp := &fnv1.RunFunctionResponse{
		Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: durationpb.New(response.DefaultTTL)},
		Results: nil,
		Conditions: []*fnv1.Condition{
			{
				Type:    "ImportsVerified",
				Status:  fnv1.Status_STATUS_CONDITION_TRUE,
				Reason:  "ResourcesImported",
				Message: new("1 existing BTP resource(s) adopted"),
				Target:  fnv1.Target_TARGET_COMPOSITE_AND_CLAIM.Enum(),
			},
		},
		Desired: &fnv1.State{
			Resources: map[string]*fnv1.Resource{
				"service-instance": {
					Resource: desiredResourceStructWithExternalName(accountV1Alpha1GroupVersion, kindServiceInstance, "my-instance", "instance-uuid"),
				},
			},
		},
		Requirements: &fnv1.Requirements{
			Resources: map[string]*fnv1.ResourceSelector{
				cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
			},
		},
	}

	log := &captureLogger{}
	rsp, err := (&Function{log: log}).RunFunction(context.Background(), req)

	if diff := cmp.Diff(wantRsp, rsp, protocmp.Transform()); diff != "" {
		t.Errorf("cleanup failure must not change the response\nRunFunction(...): -want, +got:\n%s", diff)
	}
	if err != nil {
		t.Errorf("cleanup failure must not surface as a Go error\nRunFunction(...): got err %v", err)
	}
	wantMsg := "SM admin binding cleanup failed; existing binding will be reused (not deleted) on future reconciles"
	if !slices.Contains(log.messages, wantMsg) {
		t.Errorf("cleanup failure must be logged for operators\nRunFunction(...): message %q not found in logged messages %q", wantMsg, log.messages)
	}
}

// TestRunFunctionTTL verifies the response TTL matches the SDK default.
func TestRunFunctionTTL(t *testing.T) {
	t.Parallel()

	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1.RunFunctionResponse
		err error
	}

	req := buildPass1Req(xrStruct("test-ns"), inputStruct("auto", "", ""), nil)
	req.Meta = &fnv1.RequestMeta{Tag: "hello"}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ResponseMetaMatchesRequestTag": {
			reason: "Response meta tag and TTL should be copied from request and set to default",
			args: args{
				ctx: context.Background(),
				req: req,
			},
			want: want{
				rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "hello", Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							cisSecretKey: {ApiVersion: "v1", Kind: "Secret", Namespace: new("test-ns"), Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-ns-btp-provider-config"}},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)
			if diff := cmp.Diff(tc.want.rsp, rsp, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nRunFunction(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nRunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

// smBindingPath is the SM admin binding endpoint for the subaccount the
// pass-2 cases observe.
const smBindingPath = "/accounts/v1/subaccounts/sub-1/serviceManagementBinding"

// down closes the mock before the call: rows that must not reach BTP use it,
// so any request would surface as a warning or fatal in the response.
func down(c btptest.Config) btptest.Config {
	c.Down = true
	return c
}

// withFaults answers matching requests with the given faults instead of the
// mock's normal routing.
func withFaults(f ...btptest.Fault) func(btptest.Config) btptest.Config {
	return func(c btptest.Config) btptest.Config {
		c.Faults = append(c.Faults, f...)
		return c
	}
}

// cisCredentialsJSON returns the JSON blob stored in the CIS secret's
// data.credentials field, pointing token exchange and every BTP API call at
// the mock.
func cisCredentialsJSON(mockURL, subaccountID string) string {
	b, err := json.Marshal(map[string]any{
		"uaa": map[string]any{
			"clientid":     "cis-cid",
			"clientsecret": "cis-csecret",
			"url":          mockURL,
			"subaccountid": subaccountID,
		},
		"endpoints": map[string]any{
			"accounts_service_url":     mockURL,
			"provisioning_service_url": mockURL,
		},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// cisSecretStruct returns a *structpb.Struct representing the Kubernetes Secret
// used as the CIS credentials secret in tests.
func cisSecretStruct(mockURL, subaccountID string) *structpb.Struct {
	encoded := base64.StdEncoding.EncodeToString([]byte(cisCredentialsJSON(mockURL, subaccountID)))
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": "test-ns-btp-provider-config", "namespace": "test-ns"},
		"data": map[string]any{
			"cisCredentials": encoded,
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

func xrStruct(claimNamespace string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": "example.crossplane.io/v1alpha1",
		"kind":       "XExampleComposite",
		"metadata": map[string]any{
			"name": "test-xr",
			"labels": map[string]any{
				"crossplane.io/claim-namespace": claimNamespace,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

// desiredResourceStruct builds a desired composed resource struct.
func desiredResourceStruct(apiVersion, kind, name string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{},
		"spec": map[string]any{
			"forProvider": map[string]any{
				"name": name,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

// desiredInstanceStructWithIdentity builds a desired ServiceInstance whose
// spec declares the offering/plan identity the composition expects.
func desiredInstanceStructWithIdentity(name, offeringName, planName string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": accountV1Alpha1GroupVersion,
		"kind":       kindServiceInstance,
		"metadata":   map[string]any{},
		"spec": map[string]any{
			"forProvider": map[string]any{
				"name":         name,
				"offeringName": offeringName,
				"planName":     planName,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

// desiredResourceStructWithExternalName is desiredResourceStruct plus the
// crossplane.io/external-name annotation — the expected shape after the
// importer propagates an observed external-name into the render.
func desiredResourceStructWithExternalName(apiVersion, kind, name, externalName string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"annotations": map[string]any{
				"crossplane.io/external-name": externalName,
			},
		},
		"spec": map[string]any{
			"forProvider": map[string]any{
				"name": name,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

// desiredComposedStruct builds a desired composed resource with the given
// spec.forProvider fields. A non-empty externalName adds the
// crossplane.io/external-name annotation - the shape after the importer
// stamps a match.
func desiredComposedStruct(apiVersion, kind string, forProvider map[string]any, externalName string) *structpb.Struct {
	metadata := map[string]any{}
	if externalName != "" {
		metadata["annotations"] = map[string]any{
			"crossplane.io/external-name": externalName,
		}
	}
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   metadata,
		"spec": map[string]any{
			"forProvider": forProvider,
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

// observedResourceWithExternalName builds an observed composed resource with
// metadata.name and the crossplane.io/external-name annotation set — the
// shape of an adopted MR after a previous import stamped it.
func observedResourceWithExternalName(apiVersion, kind, name, externalName string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name": name,
			"annotations": map[string]any{
				"crossplane.io/external-name": externalName,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

// observedServiceInstanceWithSubaccount builds an observed ServiceInstance
// *structpb.Struct with spec.forProvider.subaccountId set. The provider
// resolves subaccountRef into this field after the first reconcile — it is
// never present in the desired map, only in observed state.
func observedServiceInstanceWithSubaccount(subaccountID string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{
		"apiVersion": accountV1Alpha1GroupVersion,
		"kind":       kindServiceInstance,
		"metadata":   map[string]any{},
		"spec": map[string]any{
			"forProvider": map[string]any{
				"subaccountId": subaccountID,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

func inputStruct(mode, include, exclude string) *structpb.Struct {
	m := map[string]any{
		"apiVersion": "import.btp.sap.crossplane.io/v1beta1",
		"kind":       "Input",
		"mode":       mode,
		"secretRef": map[string]any{
			"name":      map[string]any{"value": "test-ns-btp-provider-config"},
			"namespace": map[string]any{"value": "test-ns"},
			"key":       "cisCredentials",
		},
	}
	if include != "" {
		m["include"] = []any{include}
	}
	if exclude != "" {
		m["exclude"] = []any{exclude}
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		panic(err)
	}
	return s
}

// contextStruct builds a *structpb.Struct for pipeline context with key-value pairs.
func contextStruct(kvs ...string) *structpb.Struct {
	m := map[string]any{}
	for i := 0; i < len(kvs)-1; i += 2 {
		m[kvs[i]] = kvs[i+1]
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		panic(err)
	}
	return s
}

// buildPass1Req constructs a RunFunctionRequest for pass 1 (no required resources yet).
func buildPass1Req(xr *structpb.Struct, input *structpb.Struct, desired map[string]*fnv1.Resource) *fnv1.RunFunctionRequest {
	return &fnv1.RunFunctionRequest{
		Meta:  &fnv1.RequestMeta{Tag: "test"},
		Input: input,
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{Resource: xr},
		},
		Desired: &fnv1.State{
			Resources: desired,
		},
	}
}

// buildPass2Req constructs a RunFunctionRequest for pass 2 (required resources present).
func buildPass2Req(xr *structpb.Struct, input *structpb.Struct, desired map[string]*fnv1.Resource, secretStruct *structpb.Struct, observed map[string]*fnv1.Resource) *fnv1.RunFunctionRequest {
	return &fnv1.RunFunctionRequest{
		Meta:  &fnv1.RequestMeta{Tag: "test"},
		Input: input,
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{Resource: xr},
			Resources: observed,
		},
		Desired: &fnv1.State{
			Resources: desired,
		},
		RequiredResources: map[string]*fnv1.Resources{
			cisSecretKey: {
				Items: []*fnv1.Resource{
					{Resource: secretStruct},
				},
			},
		},
	}
}

// buildPass2ReqSecretNotFound constructs a pass 2 request where the secret
// key is present but the slice is empty (secret not found in cluster).
func buildPass2ReqSecretNotFound(xr *structpb.Struct, input *structpb.Struct) *fnv1.RunFunctionRequest {
	return &fnv1.RunFunctionRequest{
		Meta:  &fnv1.RequestMeta{Tag: "test"},
		Input: input,
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{Resource: xr},
		},
		RequiredResources: map[string]*fnv1.Resources{
			cisSecretKey: {Items: []*fnv1.Resource{}},
		},
	}
}
