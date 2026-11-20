//go:build upgrade

package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	credentialconfig "github.com/sap/crossplane-provider-btp/config/btp_subaccount_api_credential"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func legacyUpgradeFixture() (*securityv1alpha1.SubaccountApiCredential, credentialUpgradeIdentity) {
	identity := credentialUpgradeIdentity{uid: "original-uid", subaccountID: "original-subaccount", name: "original-name"}
	cr := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: "credential", UID: identity.uid, Generation: 2}}
	cr.Spec.ForProvider.SubaccountID = &identity.subaccountID
	meta.SetExternalName(cr, identity.externalName())
	cr.SetConditions(
		xpv1.Condition{Type: "LegacyManagementUnresolved", Status: "True", Message: string(identity.uid) + ":" + credentialconfig.LegacyManagementApprovalDiagnostic()},
		xpv1.ReconcileError(errors.New(credentialconfig.LegacyManagementApprovalDiagnostic())),
		// A stale source-provider Ready=True must not satisfy post-approval health.
		xpv1.Available(),
	)
	return cr, identity
}

func credentialUpgradeTestClient(t *testing.T, cr *securityv1alpha1.SubaccountApiCredential, hooks interceptor.Funcs) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := securityv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).WithInterceptorFuncs(hooks).Build()
}

func TestCaptureCredentialUpgradeIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*securityv1alpha1.SubaccountApiCredential)
		wantErr bool
	}{
		{name: "resolved ID"},
		{name: "observed ID fallback", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			cr.Status.AtProvider.SubaccountID = cr.Spec.ForProvider.SubaccountID
			cr.Spec.ForProvider.SubaccountID = nil
		}},
		{name: "conflicting observed ID", wantErr: true, mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			other := "other-subaccount"
			cr.Status.AtProvider.SubaccountID = &other
		}},
		{name: "no ID", wantErr: true, mutate: func(cr *securityv1alpha1.SubaccountApiCredential) { cr.Spec.ForProvider.SubaccountID = nil }},
		{name: "no UID", wantErr: true, mutate: func(cr *securityv1alpha1.SubaccountApiCredential) { cr.UID = "" }},
		{name: "not a legacy annotation", wantErr: true, mutate: func(cr *securityv1alpha1.SubaccountApiCredential) { meta.SetExternalName(cr, "unexpected/name") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cr, original := legacyUpgradeFixture()
			meta.SetExternalName(cr, original.name)
			if tc.mutate != nil {
				tc.mutate(cr)
			}
			got, err := captureCredentialUpgradeIdentity(cr)
			if (err != nil) != tc.wantErr || err == nil && got != original {
				t.Fatalf("capture identity = %#v, %v; want %#v, error=%t", got, err, original, tc.wantErr)
			}
		})
	}
}

func TestCredentialAwaitingLegacyApproval(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*securityv1alpha1.SubaccountApiCredential)
	}{
		{name: "legacy name only", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) { meta.SetExternalName(cr, "original-name") }},
		{name: "different subaccount", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			meta.SetExternalName(cr, "other-subaccount/original-name")
		}},
		{name: "different name", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			meta.SetExternalName(cr, "original-subaccount/other-name")
		}},
		{name: "replacement MR", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) { cr.UID = "replacement-uid" }},
		{name: "conflicting configuration", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			other := "other-subaccount"
			cr.Spec.ForProvider.SubaccountID = &other
		}},
		{name: "copied legacy condition", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			cr.SetConditions(xpv1.Condition{Type: "LegacyManagementUnresolved", Status: "True", Message: "copied-uid:unresolved"})
		}},
		{name: "generic failure", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			cr.SetConditions(xpv1.ReconcileError(errors.New("unrelated error")))
		}},
		{name: "paused", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) {
			meta.AddAnnotations(cr, map[string]string{meta.AnnotationKeyReconciliationPaused: "true"})
		}},
		{name: "deleting", mutate: func(cr *securityv1alpha1.SubaccountApiCredential) { now := metav1.Now(); cr.DeletionTimestamp = &now }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cr, identity := legacyUpgradeFixture()
			if !credentialAwaitingLegacyApproval(cr, identity) {
				t.Fatal("fixture should have reached the approval gate")
			}
			tc.mutate(cr)
			if credentialAwaitingLegacyApproval(cr, identity) {
				t.Fatal("incorrect identity or gate must not authorize approval")
			}
		})
	}
}

func TestCredentialUpgradeReadyRequiresCurrentReconciliation(t *testing.T) {
	cr, identity := legacyUpgradeFixture()
	approval := string(identity.uid) + ":" + identity.externalName()
	meta.AddAnnotations(cr, map[string]string{credentialconfig.LegacyManagementApprovalAnnotation: approval})
	if credentialUpgradeReady(cr, identity, approval) {
		t.Fatal("stale Ready=True from source provider is not successful reconciliation")
	}
	cr.SetConditions(xpv1.ReconcileSuccess().WithObservedGeneration(cr.Generation - 1))
	if credentialUpgradeReady(cr, identity, approval) {
		t.Fatal("old observed generation must not satisfy current readiness")
	}
	synced := cr.GetCondition(xpv1.TypeSynced)
	synced.ObservedGeneration = cr.Generation
	cr.SetConditions(synced)
	if !credentialUpgradeReady(cr, identity, approval) {
		t.Fatal("current Ready and Synced with unchanged identity and approval should succeed")
	}
	meta.SetExternalName(cr, "other-subaccount/original-name")
	if credentialUpgradeReady(cr, identity, approval) {
		t.Fatal("readiness must not hide identity drift")
	}
}

func TestWaitForCredentialUpgradePollsAfterResume(t *testing.T) {
	cr, identity := legacyUpgradeFixture()
	reads := 0
	kube := credentialUpgradeTestClient(t, cr, interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		if err := c.Get(ctx, key, obj, opts...); err != nil {
			return err
		}
		reads++
		if reads == 1 {
			// First read after resume precedes migration and gate reporting.
			meta.SetExternalName(obj, identity.name)
			obj.(*securityv1alpha1.SubaccountApiCredential).Status.Conditions = nil
		}
		return nil
	}})
	got, err := waitForCredentialUpgrade(context.Background(), kube, client.ObjectKeyFromObject(cr), identity.uid, 5*time.Second,
		func(cr *securityv1alpha1.SubaccountApiCredential) bool {
			return credentialAwaitingLegacyApproval(cr, identity)
		})
	if err != nil || reads < 2 || got == nil || meta.GetExternalName(got) != identity.externalName() {
		t.Fatalf("wait must poll past initial legacy read: reads=%d err=%v", reads, err)
	}
}

func TestWaitForCredentialUpgradeReportsSafeTimeoutDiagnostics(t *testing.T) {
	cr, identity := legacyUpgradeFixture()
	meta.SetExternalName(cr, identity.name)
	cr.SetConditions(xpv1.ReconcileError(errors.New("private-error-canary")))
	meta.AddAnnotations(cr, map[string]string{"unrelated": "private-annotation-canary"})
	kube := credentialUpgradeTestClient(t, cr, interceptor.Funcs{})
	_, err := waitForCredentialUpgrade(context.Background(), kube, client.ObjectKeyFromObject(cr), identity.uid, 10*time.Millisecond,
		func(cr *securityv1alpha1.SubaccountApiCredential) bool {
			return credentialAwaitingLegacyApproval(cr, identity)
		})
	if err == nil {
		t.Fatal("unmigrated MR must time out")
	}
	for _, want := range []string{"last observed state", "original-name", "original-subaccount", "Synced=False", "LegacyManagementUnresolved=True", "paused="} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("diagnostics missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "private-") {
		t.Fatalf("diagnostics exposed raw messages or annotations: %v", err)
	}
}

func TestWaitForCredentialUpgradeReportsReadFailure(t *testing.T) {
	cr, identity := legacyUpgradeFixture()
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "subaccountapicredentials"}, cr.Name, errors.New("access denied"))
	kube := credentialUpgradeTestClient(t, cr, interceptor.Funcs{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
		return forbidden
	}})
	_, err := waitForCredentialUpgrade(context.Background(), kube, client.ObjectKeyFromObject(cr), identity.uid, time.Second, func(*securityv1alpha1.SubaccountApiCredential) bool { return true })
	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected actionable read failure, got %v", err)
	}
}

func TestApproveLegacyUpgradeCredentialRetriesNarrowPatch(t *testing.T) {
	cr, identity := legacyUpgradeFixture()
	patches := 0
	kube := credentialUpgradeTestClient(t, cr, interceptor.Funcs{Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
		patches++
		data, err := patch.Data(obj)
		if err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		if len(fields) != 1 || fields["metadata"] == nil || !strings.Contains(string(data), "resourceVersion") {
			t.Fatalf("approval must use an optimistic-lock metadata-only patch: %s", data)
		}
		if patches == 1 {
			concurrent := &securityv1alpha1.SubaccountApiCredential{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), concurrent); err != nil {
				return err
			}
			meta.AddAnnotations(concurrent, map[string]string{"concurrent": "preserved"})
			if err := c.Update(ctx, concurrent); err != nil {
				return err
			}
			return apierrors.NewConflict(schema.GroupResource{Resource: "subaccountapicredentials"}, obj.GetName(), errors.New("concurrent update"))
		}
		return c.Patch(ctx, obj, patch, opts...)
	}})
	approval, err := approveLegacyUpgradeCredential(context.Background(), kube, client.ObjectKeyFromObject(cr), identity)
	if err != nil || patches != 2 {
		t.Fatalf("approval should retry conflict: patches=%d err=%v", patches, err)
	}
	got := &securityv1alpha1.SubaccountApiCredential{}
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(cr), got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations["concurrent"] != "preserved" || got.Annotations[credentialconfig.LegacyManagementApprovalAnnotation] != approval || meta.GetExternalName(got) != identity.externalName() {
		t.Fatalf("approval patch lost concurrent metadata or identity: %s", credentialUpgradeDiagnostics(got))
	}
}

func TestApproveLegacyUpgradeCredentialRechecksIdentityOnConflict(t *testing.T) {
	cr, identity := legacyUpgradeFixture()
	patches := 0
	kube := credentialUpgradeTestClient(t, cr, interceptor.Funcs{Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
		patches++
		concurrent := &securityv1alpha1.SubaccountApiCredential{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), concurrent); err != nil {
			return err
		}
		meta.SetExternalName(concurrent, "other-subaccount/original-name")
		if err := c.Update(ctx, concurrent); err != nil {
			return err
		}
		return apierrors.NewConflict(schema.GroupResource{Resource: "subaccountapicredentials"}, obj.GetName(), errors.New("concurrent identity change"))
	}})
	if _, err := approveLegacyUpgradeCredential(context.Background(), kube, client.ObjectKeyFromObject(cr), identity); err == nil || patches != 1 {
		t.Fatalf("identity drift must stop approval retry: patches=%d err=%v", patches, err)
	}
	got := &securityv1alpha1.SubaccountApiCredential{}
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(cr), got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[credentialconfig.LegacyManagementApprovalAnnotation] != "" {
		t.Fatal("must not approve the changed identity")
	}
}

func TestApproveLegacyUpgradeCredentialRefusesReplacement(t *testing.T) {
	cr, identity := legacyUpgradeFixture()
	cr.UID = "replacement-uid"
	kube := credentialUpgradeTestClient(t, cr, interceptor.Funcs{})
	if _, err := approveLegacyUpgradeCredential(context.Background(), kube, client.ObjectKeyFromObject(cr), identity); err == nil {
		t.Fatal("approval must not follow a replaced MR")
	}
}
