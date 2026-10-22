//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	credentialconfig "github.com/sap/crossplane-provider-btp/config/btp_subaccount_api_credential"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type credentialUpgradeIdentity struct {
	uid          types.UID
	subaccountID string
	name         string
}

func (i credentialUpgradeIdentity) externalName() string { return i.subaccountID + "/" + i.name }

func captureCredentialUpgradeIdentity(cr *securityv1alpha1.SubaccountApiCredential) (credentialUpgradeIdentity, error) {
	identity := credentialUpgradeIdentity{uid: cr.UID, name: meta.GetExternalName(cr), subaccountID: credentialUpgradeString(cr.Spec.ForProvider.SubaccountID)}
	observedID := credentialUpgradeString(cr.Status.AtProvider.SubaccountID)
	if identity.subaccountID == "" {
		identity.subaccountID = observedID
	} else if observedID != "" && identity.subaccountID != observedID {
		return identity, fmt.Errorf("resolved and observed subaccount IDs disagree")
	}
	if identity.uid == "" || identity.name == "" || identity.subaccountID == "" || strings.ContainsAny(identity.name+identity.subaccountID, "/ \t\r\n") {
		return identity, fmt.Errorf("expected an MR UID, legacy name-only external-name, and resolved or observed subaccount ID")
	}
	return identity, nil
}

func credentialAwaitingLegacyApproval(cr *securityv1alpha1.SubaccountApiCredential, identity credentialUpgradeIdentity) bool {
	synced := cr.GetCondition(xpv1.TypeSynced)
	subaccountID, name, established := credentialconfig.EstablishedCredentialIdentity(cr)
	return established && subaccountID == identity.subaccountID && name == identity.name &&
		cr.UID == identity.uid && meta.GetExternalName(cr) == identity.externalName() && !meta.IsPaused(cr) && !meta.WasDeleted(cr) &&
		credentialconfig.IsLegacyManagementUnresolved(cr) && !credentialconfig.LegacyManagementApproved(cr) &&
		synced.Status == "False" && strings.Contains(synced.Message, credentialconfig.LegacyManagementApprovalDiagnostic())
}

func credentialUpgradeReady(cr *securityv1alpha1.SubaccountApiCredential, identity credentialUpgradeIdentity, approval string) bool {
	synced, ready := cr.GetCondition(xpv1.TypeSynced), cr.GetCondition(xpv1.TypeReady)
	return cr.UID == identity.uid && meta.GetExternalName(cr) == identity.externalName() && !meta.IsPaused(cr) && !meta.WasDeleted(cr) &&
		cr.Annotations[credentialconfig.LegacyManagementApprovalAnnotation] == approval && credentialconfig.LegacyManagementApproved(cr) &&
		synced.Status == "True" && ready.Status == "True" && synced.ObservedGeneration >= cr.Generation
}

// waitForCredentialUpgrade always reads the current cluster-scoped MR. A
// replacement MR, deletion, or API error is not a migration result to approve.
func waitForCredentialUpgrade(ctx context.Context, kube client.Reader, key client.ObjectKey, uid types.UID, timeout time.Duration, match func(*securityv1alpha1.SubaccountApiCredential) bool) (*securityv1alpha1.SubaccountApiCredential, error) {
	var last *securityv1alpha1.SubaccountApiCredential
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		current := &securityv1alpha1.SubaccountApiCredential{}
		if err := kube.Get(ctx, key, current); err != nil {
			return false, fmt.Errorf("read credential MR: %w", err)
		}
		last = current
		if current.UID != uid || meta.WasDeleted(current) {
			return false, fmt.Errorf("credential MR was replaced or deletion was requested")
		}
		return match(current), nil
	})
	if err != nil {
		return last, fmt.Errorf("credential upgrade wait failed for %q: %w; last observed state: %s", key.Name, err, credentialUpgradeDiagnostics(last))
	}
	return last, nil
}

// Patch only approval metadata, using resourceVersion as an optimistic lock and
// rechecking the original UID, exact identity, and gate on every conflict retry.
func approveLegacyUpgradeCredential(ctx context.Context, kube client.Client, key client.ObjectKey, identity credentialUpgradeIdentity) (string, error) {
	approval := string(identity.uid) + ":" + identity.externalName()
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &securityv1alpha1.SubaccountApiCredential{}
		if err := kube.Get(ctx, key, current); err != nil {
			return err
		}
		if !credentialAwaitingLegacyApproval(current, identity) {
			return fmt.Errorf("refusing legacy approval: original MR identity or approval gate changed; %s", credentialUpgradeDiagnostics(current))
		}
		before := current.DeepCopy()
		meta.AddAnnotations(current, map[string]string{credentialconfig.LegacyManagementApprovalAnnotation: approval})
		return kube.Patch(ctx, current, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{}))
	})
	return approval, err
}

// Never dump the MR, all annotations, raw error messages, or Secret contents.
// Only identity and reconciliation metadata are needed to diagnose the wait.
func credentialUpgradeDiagnostics(cr *securityv1alpha1.SubaccountApiCredential) string {
	if cr == nil {
		return "no MR read successfully"
	}
	synced, ready, legacy := cr.GetCondition(xpv1.TypeSynced), cr.GetCondition(xpv1.TypeReady), cr.GetCondition("LegacyManagementUnresolved")
	return fmt.Sprintf("uid=%q generation=%d externalName=%q resolvedSubaccountID=%q observedSubaccountID=%q paused=%q deleting=%t Synced=%s/%s SyncedObservedGeneration=%d Ready=%s/%s LegacyManagementUnresolved=%s/%s legacyUIDBound=%t approvalRequiredDiagnostic=%t",
		cr.UID, cr.Generation, meta.GetExternalName(cr), credentialUpgradeString(cr.Spec.ForProvider.SubaccountID), credentialUpgradeString(cr.Status.AtProvider.SubaccountID),
		cr.Annotations[meta.AnnotationKeyReconciliationPaused], meta.WasDeleted(cr), synced.Status, synced.Reason, synced.ObservedGeneration, ready.Status, ready.Reason, legacy.Status, legacy.Reason,
		credentialconfig.IsLegacyManagementUnresolved(cr), strings.Contains(synced.Message, credentialconfig.LegacyManagementApprovalDiagnostic()))
}

func credentialUpgradeString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
