//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/e2e-framework/klient/wait"
)

func TestCredentialExistenceProof(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
		want    bool
	}{
		{name: "guarded observation found existing target", message: "observe failed: " + unsupportedCredentialAdoptionMessage, want: true},
		{name: "provider duplicate collision", message: "credential already exists", want: true},
		{name: "arbitrary reconcile error", message: "cannot connect to provider: credentials unavailable", want: false},
		{name: "unrelated adoption initialization error", message: "adoption initialization failed", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := credentialExistenceProof(tc.message); got != tc.want {
				t.Fatalf("credentialExistenceProof(%q) = %t, want %t", tc.message, got, tc.want)
			}
		})
	}
}

func TestCredentialRejectionError(t *testing.T) {
	const identity = "subaccount/credential"
	for _, tt := range []struct {
		name            string
		message         string
		requireAdoption bool
		mutate          func(*v1alpha1.SubaccountApiCredential)
		wantError       string
	}{
		{name: "explicit rejection", message: unsupportedCredentialAdoptionMessage, requireAdoption: true},
		{name: "collision is not adoption rejection", message: "credential already exists", requireAdoption: true, wantError: "explicit adoption rejection"},
		{name: "missing secret is not adoption rejection", message: "cannot read client_secret from source", requireAdoption: true, wantError: "explicit adoption rejection"},
		{name: "arbitrary adoption text is insufficient", message: "adoption initialization failed", requireAdoption: true, wantError: "explicit adoption rejection"},
		{name: "collision probe uses separate assertion", message: "credential already exists"},
		{name: "cleared annotation is allowed", message: unsupportedCredentialAdoptionMessage, requireAdoption: true, mutate: func(cr *v1alpha1.SubaccountApiCredential) { xpmeta.SetExternalName(cr, "") }},
		{name: "retargeted annotation", message: unsupportedCredentialAdoptionMessage, requireAdoption: true, mutate: func(cr *v1alpha1.SubaccountApiCredential) { xpmeta.SetExternalName(cr, "subaccount/other") }, wantError: "retargeted"},
		{name: "ready credential", message: unsupportedCredentialAdoptionMessage, requireAdoption: true, mutate: func(cr *v1alpha1.SubaccountApiCredential) { cr.SetConditions(xpv1.Available()) }, wantError: "became Ready"},
		{name: "stale reconcile error", message: unsupportedCredentialAdoptionMessage, requireAdoption: true, mutate: func(cr *v1alpha1.SubaccountApiCredential) { cr.Generation++ }, wantError: "current-generation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: "rejected", Generation: 1}}
			xpmeta.SetExternalName(cr, identity)
			cr.SetConditions(xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionFalse, ObservedGeneration: 1, Message: tt.message})
			if tt.mutate != nil {
				tt.mutate(cr)
			}
			err := credentialRejectionError(cr, identity, tt.requireAdoption)
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("wanted error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}

type adoptionCleanupClient struct {
	client.Client
	deleteError error
	deletions   []string
}

func (c *adoptionCleanupClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.deletions = append(c.deletions, obj.GetName())
	if obj.GetName() == "delete-error" {
		return c.deleteError
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestDeleteCredentialAdoptionTestResourcesContinuesAfterFailures(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := accountv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	credential := func(name string) *v1alpha1.SubaccountApiCredential {
		cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name)}}
		xpmeta.SetExternalName(cr, "subaccount/"+name)
		return cr
	}
	failed, stuck, healthy := credential("delete-error"), credential("stuck"), credential("healthy")
	stuck.Finalizers = []string{"finalizer.managedresource.crossplane.io"}
	subaccount := &accountv1alpha1.Subaccount{ObjectMeta: metav1.ObjectMeta{Name: "subaccount", UID: "subaccount-uid"}}
	xpmeta.SetExternalName(subaccount, "btp-subaccount-id")
	deleteError := errors.New("delete forbidden")
	kube := &adoptionCleanupClient{
		Client:      fake.NewClientBuilder().WithScheme(scheme).WithObjects(failed, stuck, healthy, subaccount).Build(),
		deleteError: deleteError,
	}
	var logs []string
	logf := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	err := deleteCredentialAdoptionTestResources(context.Background(), kube,
		[]client.Object{failed.DeepCopy(), stuck.DeepCopy(), healthy.DeepCopy(), subaccount.DeepCopy()}, logf,
		wait.WithTimeout(25*time.Millisecond), wait.WithInterval(time.Millisecond))
	if !errors.Is(err, deleteError) || !strings.Contains(err.Error(), `"stuck"`) {
		t.Fatalf("expected aggregated deletion and finalization failures, got %v", err)
	}
	if got := strings.Join(kube.deletions, ","); got != "delete-error,stuck,healthy,subaccount" {
		t.Fatalf("cleanup skipped subsequent resources: %s", got)
	}
	for _, object := range []client.Object{healthy, subaccount} {
		if err := kube.Get(context.Background(), client.ObjectKeyFromObject(object), object); !apierrors.IsNotFound(err) {
			t.Errorf("resource %q should be deleted, got %v", object.GetName(), err)
		}
	}
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(stuck), stuck); err != nil {
		t.Fatal(err)
	}
	if len(stuck.Finalizers) != 1 || stuck.DeletionTimestamp.IsZero() {
		t.Fatalf("cleanup bypassed finalization safeguards: %#v", stuck.ObjectMeta)
	}
	if !strings.Contains(err.Error(), "subaccount/stuck") || !strings.Contains(strings.Join(logs, "\n"), "btp-subaccount-id") {
		t.Fatalf("cleanup diagnostics omitted external identities: err=%v logs=%v", err, logs)
	}
	// Already absent objects are harmless, including on a repeated cleanup.
	if err := deleteCredentialAdoptionTestResources(context.Background(), kube, []client.Object{healthy, subaccount}, logf); err != nil {
		t.Fatal(err)
	}
}
