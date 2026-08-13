//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
)

var (
	smCreateName = "e2e-sm-servicemanager"
	smImportName = "sm-import-test"
)

func TestServiceManagerCreationFlow(t *testing.T) {
	crudFeatureSuite := features.New("ServiceManager Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("servicemanager/create_flow"))
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				sm := v1beta1.ServiceManager{
					ObjectMeta: metav1.ObjectMeta{Name: smCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sm, cfg, t, wait.WithTimeout(7*time.Minute))
				return ctx
			},
		).
		Assess(
			"Check ServiceManager Resources are fully created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sm := &v1beta1.ServiceManager{}
				MustGetResource(t, cfg, smCreateName, nil, sm)
				// Status bound?
				if sm.Status.AtProvider.Status != v1alpha1.ServiceManagerBound {
					t.Error("Binding status not set as expected")
				}

				assertServiceManagerSecret(t, ctx, cfg, sm)

				return ctx
			},
		).Assess(
		"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// k8s resource cleaned up?
			sm := &v1beta1.ServiceManager{}
			MustGetResource(t, cfg, smCreateName, nil, sm)

			AwaitResourceDeletionOrFail(ctx, t, cfg, sm, wait.WithTimeout(time.Minute*5))

			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, crsPath("servicemanager/create_flow"), wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}

// TestServiceManagerImportFlow drives the ADR import contract through the shared
// ImportTester harness: create the external resource, drop the CR without deleting
// it in BTP, then re-adopt it from crossplane.io/external-name alone. Beyond "it
// went Ready" it asserts the compound key's shape, that status mirrors both
// halves, and that adoption did not fall through to a second Create.
//
// No writeConnectionSecretToRef here: the harness fixes the CR name at
// construction time, while the target namespace only exists inside a running
// feature. TestServiceManagerCreationFlow covers the credentials secret.
func TestServiceManagerImportFlow(t *testing.T) {
	importTester := NewImportTester(
		&v1beta1.ServiceManager{
			Spec: v1beta1.ServiceManagerSpec{
				ForProvider: v1beta1.ServiceManagerParameters{
					SubaccountRef: &xpv1.Reference{Name: "sm-import-sa-test"},
				},
			},
		},
		smImportName,
		WithWaitDependentResourceTimeout[*v1beta1.ServiceManager](wait.WithTimeout(15*time.Minute)),
		WithWaitCreateTimeout[*v1beta1.ServiceManager](wait.WithTimeout(10*time.Minute)),
		// Also bounds the dependent Subaccount teardown, which is the slowest of
		// the three waits this option feeds. Matches TestServiceInstanceImportFlow.
		WithWaitDeletionTimeout[*v1beta1.ServiceManager](wait.WithTimeout(20*time.Minute)),
		WithDependentResourceDirectory[*v1beta1.ServiceManager](crsPath("servicemanager/import/environment")),
	)

	testenv.Test(t, importTester.BuildTestFeature("ServiceManager Import Flow").
		Assess(
			"Imported ServiceManager adopts the compound external-name",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				imported := &v1beta1.ServiceManager{}
				MustGetResource(t, cfg, importTester.GetPrefixedName(), nil, imported)

				externalName := xpmeta.GetExternalName(imported)
				// Shape alone would pass on a CR that adopted some other
				// instance/binding pair. This is the key captured before the CR was
				// dropped, so it pins adoption of the surviving resource itself.
				if want := ctx.Value(importFeatureContextKey).(string); externalName != want {
					t.Fatalf("imported external-name = %q, want the key captured before the CR was dropped (%q)", externalName, want)
				}
				// The provider's own predicate, not a looser hand-rolled one:
				// internal.IsValidUUID alone admits the braced, urn:uuid: and
				// unhyphenated spellings that Observe() rejects.
				if err := sm.ValidateExternalName(imported.GetName(), externalName); err != nil {
					t.Fatalf("imported external-name %q violates the ADR key format: %v", externalName, err)
				}
				// ValidateExternalName also accepts the phase-1 one-segment
				// transient; an imported resource must be past that.
				parts := strings.Split(externalName, "/")
				if len(parts) != 2 {
					t.Fatalf(
						"imported external-name %q is not the compound key \"<serviceInstanceID>/<serviceBindingID>\" (%d segments)",
						externalName, len(parts),
					)
				}

				if imported.Status.AtProvider.Status != v1beta1.ServiceManagerBound {
					t.Errorf("imported ServiceManager status = %q, want %q", imported.Status.AtProvider.Status, v1beta1.ServiceManagerBound)
				}
				if imported.Status.AtProvider.ServiceInstanceID != parts[0] {
					t.Errorf(
						"status.atProvider.serviceInstanceID = %q, want the first key segment %q",
						imported.Status.AtProvider.ServiceInstanceID, parts[0],
					)
				}
				if imported.Status.AtProvider.ServiceBindingID != parts[1] {
					t.Errorf(
						"status.atProvider.serviceBindingID = %q, want the second key segment %q",
						imported.Status.AtProvider.ServiceBindingID, parts[1],
					)
				}

				// crossplane-runtime writes these three only around its own Create.
				// Any of them on an imported CR means Observe failed to match the
				// surviving BTP resource and provisioned a duplicate pair instead.
				annotations := imported.GetAnnotations()
				pending := annotations[xpmeta.AnnotationKeyExternalCreatePending]
				succeeded := annotations[xpmeta.AnnotationKeyExternalCreateSucceeded]
				failed := annotations[xpmeta.AnnotationKeyExternalCreateFailed]
				if pending != "" || succeeded != "" || failed != "" {
					t.Fatalf(
						"imported ServiceManager carries an external-create annotation (pending=%q succeeded=%q failed=%q): "+
							"the provider created a new service manager instead of adopting the existing one via its external-name",
						pending, succeeded, failed,
					)
				}
				return ctx
			},
		).
		Feature())
}

func assertServiceManagerSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, cm *v1beta1.ServiceManager) {
	secretName := cm.GetWriteConnectionSecretToReference().Name
	secretNS := cm.GetWriteConnectionSecretToReference().Namespace
	secret := &corev1.Secret{}
	err := cfg.Client().Resources().Get(ctx, secretName, secretNS, secret)
	if err != nil {
		t.Error("Error while loading expected secret from Ref")
	}
	// secret contains correct structure
	if _, ok := secret.Data["tokenurl"]; !ok {
		t.Error("Secret not in proper format")
	}
	// raw credentials blob preserved under __raw
	rawBlob, ok := secret.Data[providerv1alpha1.RawBindingKey]
	if !ok || len(rawBlob) == 0 {
		t.Errorf("Secret missing %q key with raw credentials blob", providerv1alpha1.RawBindingKey)
		return
	}
	var rawCreds map[string]any
	if err := json.Unmarshal(rawBlob, &rawCreds); err != nil {
		t.Errorf("Secret %q value is not valid JSON: %v", providerv1alpha1.RawBindingKey, err)
		return
	}
	for _, key := range []string{"clientid", "clientsecret", "sm_url", "url", "xsappname"} {
		if _, ok := rawCreds[key]; !ok {
			t.Errorf("Secret %q blob missing field %q", providerv1alpha1.RawBindingKey, key)
		}
	}
}

//
//func createAPIInstance(t *testing.T, apiClient *servicemanager.APIClient, externalName string) *string {
//	request := apiClient.ServiceInstancesAPI.CreateServiceInstance(context.TODO())
//	parameters := map[string]string{"grantType": "clientCredentials"}
//
//	createCisLocalInstanceRequest := servicemanager.CreateServiceInstanceRequestPayload{
//		CreateByOfferingAndPlanName: &servicemanager.CreateByOfferingAndPlanName{
//			Name:                externalName,
//			ServiceOfferingName: "cis",
//			ServicePlanName:     "local",
//			Parameters:          &parameters,
//		},
//		CreateByPlanID: nil,
//	}
//
//	request = request.CreateServiceInstanceRequestPayload(createCisLocalInstanceRequest)
//	request = request.Async(false)
//	response, _, err := request.Execute()
//	if err != nil {
//		t.Errorf("Cannot create cis instance over API")
//		return nil
//	}
//	return response.Id
//}
//
//func createAPIBinding(t *testing.T, apiClient *servicemanager.APIClient, externalName string, serviceInstanceId *string) *string {
//	request := apiClient.ServiceBindingsAPI.CreateServiceBinding(context.TODO())
//	createCisLocalBindingRequest := servicemanager.CreateServiceBindingRequestPayload{
//		Name:              externalName,
//		ServiceInstanceId: *serviceInstanceId,
//		Parameters:        nil,
//		BindResource:      nil,
//	}
//	request = request.CreateServiceBindingRequestPayload(createCisLocalBindingRequest)
//	request = request.Async(false)
//	res, _, err := request.Execute()
//
//	if err != nil {
//		t.Errorf("Cannot create cis binding over API")
//	}
//	return res.Id
//}
