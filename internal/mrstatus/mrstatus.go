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

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ReasonExternalResourceFailed marks a managed resource whose external
	// counterpart is reported as failed or otherwise unhealthy by BTP. The
	// provider applied the spec successfully; the platform then rejected or
	// failed the operation, so readiness must not be asserted.
	ReasonExternalResourceFailed xpv1.ConditionReason = "ExternalResourceFailed"

	// MaxMessageBytes bounds how much platform-supplied detail is copied into
	// a condition message. Terraform and BTP error payloads can be tens of
	// kilobytes; unbounded they would bloat the status of every failing
	// managed resource in etcd.
	MaxMessageBytes = 2048
)

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
