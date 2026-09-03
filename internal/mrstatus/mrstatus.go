// Package mrstatus holds the shared vocabulary this provider uses to report
// the health of an external (BTP-side) resource on a managed resource.
//
// The conditions built here exist because a managed resource used to report
// Ready=True while the platform reported the backing resource as failed: the
// controllers only ever asserted "the provider applied the spec", never "the
// external system is actually healthy". Keeping the reasons and the message
// bounds in one package keeps that reporting consistent across kinds.
package mrstatus

import (
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ReasonExternalResourceFailed marks a managed resource whose external
	// counterpart is reported as failed or otherwise unhealthy by BTP. The
	// provider applied the spec successfully; the platform then rejected or
	// failed the operation, so readiness must not be asserted.
	ReasonExternalResourceFailed xpv1.ConditionReason = "ExternalResourceFailed"

	// ReasonDeleteFailed marks a managed resource whose external deprovision
	// was refused or failed on the platform side.
	ReasonDeleteFailed xpv1.ConditionReason = "DeleteFailed"

	// MaxMessageBytes bounds how much platform-supplied detail is copied into
	// a condition message. Terraform and BTP error payloads can be tens of
	// kilobytes; unbounded they would bloat the status of every failing
	// managed resource in etcd.
	MaxMessageBytes = 2048
)

// TypeExternalDeletion carries the state of the external deprovision.
//
// It is deliberately NOT the Ready condition. The managed reconciler stamps
// xpv1.Deleting() — a Ready-type condition — together with ReconcileSuccess()
// after Observe and Delete on every deletion pass, and SetConditions replaces
// conditions of the same type. A Ready=False/DeleteFailed condition written
// during Observe would therefore be overwritten in the very same pass and
// never persisted. A dedicated type survives that overwrite and can be
// inspected by operators and by kubectl printers alike.
const TypeExternalDeletion xpv1.ConditionType = "ExternalDeletion"

// EventReasonDeleteFailed is the Kubernetes event reason used when a newly
// observed external deprovision failure is reported on a managed resource.
const EventReasonDeleteFailed event.Reason = "ExternalDeleteFailed"

// ExternalResourceFailed builds the Ready=False condition used to report that
// the external resource behind a managed resource is unhealthy.
//
// generation is stamped as ObservedGeneration so a later spec change can be
// distinguished from a stale failure report.
func ExternalResourceFailed(msg string, generation int64) xpv1.Condition {
	return xpv1.Condition{
		Type:               xpv1.TypeReady,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonExternalResourceFailed,
		Message:            msg,
		ObservedGeneration: generation,
	}
}

// DeleteFailed builds the ExternalDeletion condition for an external
// deprovision the platform refused or failed to carry out.
//
// The message is deliberately keyed on the deletion timestamp rather than on
// wall-clock time or on an attempt counter, so it stays byte-stable across
// reconciles for as long as the failure itself does not change. That is what
// makes RecordOnChange fire exactly once per DISTINCT failure instead of once
// per reconcile pass. An attempt counter would need a new status field — an
// API change, and therefore code generation — which this fix deliberately
// avoids.
func DeleteFailed(since metav1.Time, lastErr string) xpv1.Condition {
	return xpv1.Condition{
		Type:               TypeExternalDeletion,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonDeleteFailed,
		Message: fmt.Sprintf("external deprovision has been failing since %s: %s",
			since.UTC().Format(time.RFC3339), Truncate(lastErr, MaxMessageBytes)),
	}
}

// RecordOnChange sets cond on mg and reports whether that was a change with
// respect to the condition of the same type already present on mg. When it is
// a change — and only then — a Warning event carrying the condition message is
// emitted, which makes the reporting edge-triggered rather than per-pass.
//
// Change detection uses xpv1.Condition.Equal, which ignores
// LastTransitionTime, so a condition rebuilt from an unchanged failure is a
// no-op. rec may be nil, in which case the condition is still set and no event
// is emitted.
//
// The return value reports whether an event was emitted.
func RecordOnChange(rec event.Recorder, mg resource.Managed, cond xpv1.Condition, reason event.Reason) bool {
	changed := !mg.GetCondition(cond.Type).Equal(cond)
	mg.SetConditions(cond)

	if !changed || rec == nil {
		return false
	}
	rec.Event(mg, event.Warning(reason, errors.New(cond.Message)))
	return true
}

// Truncate shortens s to at most max bytes, appending a marker that states how
// much was dropped. The head is preserved because that is where the error
// class of a platform or terraform error appears, and callers match on it.
//
// A non-positive max disables truncation.
func Truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf(" ... [truncated %d bytes]", len(s)-max)
}
