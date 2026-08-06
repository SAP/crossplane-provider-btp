package tfclient

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/internal/mrstatus"
)

const (
	// ShadowNamePrefix is the prefix native controllers put in front of the
	// native managed resource name when they drive BTP through an internal
	// upjet shadow resource, because terraform resource names may not start
	// with a digit. See
	// internal/clients/account/serviceinstance.buildBaseTfResource.
	ShadowNamePrefix = "TF-"

	// defaultMaxConditionMessageBytes bounds how much terraform output is
	// copied into a condition message. The error of a failed terraform
	// apply/destroy is the full CLI output and can be tens of kilobytes;
	// unbounded it would bloat the status of every failing managed resource.
	defaultMaxConditionMessageBytes = 16 * 1024

	// eventReasonAsyncCallbackFailed is raised when an async result cannot be
	// persisted on the managed resource it belongs to.
	eventReasonAsyncCallbackFailed = "AsyncCallbackFailed"
)

var errUpdateStatusFmt = "cannot update status of the resource %s after an async %s"

// NameResolverFn maps the identity upjet hands to the async callbacks — the
// identity of the terraform SHADOW resource — back to the identity of the
// native managed resource whose status must carry the result.
type NameResolverFn func(types.NamespacedName) types.NamespacedName

// StripShadowPrefix is the default NameResolverFn. It removes the shadow
// prefix when present and leaves every other name byte-identical, so consumers
// that do not use shadow naming keep working unchanged.
func StripShadowPrefix(nn types.NamespacedName) types.NamespacedName {
	nn.Name = strings.TrimPrefix(nn.Name, ShadowNamePrefix)
	return nn
}

// APICallbacksOption configures an APICallbacks.
type APICallbacksOption func(*APICallbacks)

// WithNameResolver overrides how the terraform shadow identity is mapped back
// to the native managed resource.
func WithNameResolver(fn NameResolverFn) APICallbacksOption {
	return func(ac *APICallbacks) {
		if fn != nil {
			ac.resolveName = fn
		}
	}
}

// WithCallbackLogger overrides the logger used for the (loud) failure path.
func WithCallbackLogger(l logr.Logger) APICallbacksOption {
	return func(ac *APICallbacks) {
		ac.log = l
	}
}

// WithCallbackEventRecorder makes the callbacks emit a Warning event on the
// managed resource when an async result cannot be persisted. objFn builds the
// event target from the resolved name and may return nil to skip the event.
func WithCallbackEventRecorder(rec event.Recorder, objFn func(types.NamespacedName) resource.Managed) APICallbacksOption {
	return func(ac *APICallbacks) {
		ac.record = rec
		ac.newEventTarget = objFn
	}
}

// WithMaxConditionMessageBytes bounds the size of condition messages produced
// by the callbacks. A non-positive value disables the bound.
func WithMaxConditionMessageBytes(n int) APICallbacksOption {
	return func(ac *APICallbacks) {
		ac.maxMsgBytes = n
	}
}

// NewAPICallbacks returns the terraform async callbacks used by the native
// controllers that drive BTP through an internal upjet shadow resource.
func NewAPICallbacks(kube client.Client, saveConditionsFn SaveConditionsFn, opts ...APICallbacksOption) *APICallbacks {
	ac := &APICallbacks{
		kube:           kube,
		saveCallbackFn: saveConditionsFn,
		resolveName:    StripShadowPrefix,
		log:            ctrl.Log.WithName("tf-async-callback"),
		maxMsgBytes:    defaultMaxConditionMessageBytes,
	}
	for _, o := range opts {
		o(ac)
	}
	return ac
}

// APICallbacks persists the result of an asynchronous terraform operation on
// the native managed resource that triggered it.
type APICallbacks struct {
	kube           client.Client
	saveCallbackFn SaveConditionsFn

	// resolveName maps the shadow identity back to the native managed
	// resource. Never nil once built through NewAPICallbacks.
	resolveName NameResolverFn

	log logr.Logger

	// record and newEventTarget are optional; both must be set for an event
	// to be emitted.
	record         event.Recorder
	newEventTarget func(types.NamespacedName) resource.Managed

	maxMsgBytes int
}

// Create makes sure the error is saved in the async operation condition.
func (ac *APICallbacks) Create(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("create", name)
}

// Update makes sure the error is saved in the async operation condition.
func (ac *APICallbacks) Update(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("update", name)
}

// Destroy makes sure the error is saved in the async operation condition.
func (ac *APICallbacks) Destroy(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("destroy", name)
}

// callback is the single implementation behind all three verbs.
//
// name is the identity of the terraform shadow resource upjet built for this
// operation, which for shadow-driven kinds is not the identity of any object
// that exists in the API server. It is resolved back to the native managed
// resource before the result is persisted; resolution happens once here, on
// the reconcile goroutine, so both names remain available for logging from the
// detached callback goroutine.
func (ac *APICallbacks) callback(op string, name types.NamespacedName) terraform.CallbackFn {
	target := ac.resolveName(name)
	return func(err error, ctx context.Context) error {
		conds := []xpv1.Condition{
			ac.bound(ujresource.LastAsyncOperationCondition(err)),
			ujresource.AsyncOperationFinishedCondition(),
		}
		uErr := ac.saveCallbackFn(ctx, ac.kube, target, conds...)
		if uErr == nil {
			return nil
		}
		// Loud on purpose. Upjet logs a failing callback only at info level
		// (pkg/terraform/workspace.go), and a dropped async result is not
		// cosmetic: it masks failed creates — nothing records the failure, so
		// the resource still goes Available — and it wedges failed destroys
		// into an unbounded blind-retry loop with no error condition and no
		// backoff.
		ac.log.Error(uErr, "async terraform callback could not persist its result on the managed resource",
			"operation", op,
			"terraformName", name.String(),
			"managedResource", target.String(),
			"asyncError", errString(err))
		ac.emitWarning(target, op, uErr)
		return errors.Wrapf(uErr, errUpdateStatusFmt, target, op)
	}
}

// bound truncates the tail of an over-long condition message. The head is kept
// intact because that is where the terraform error class appears (for example
// "Conflict"), and the ServiceInstance controller matches on it.
func (ac *APICallbacks) bound(c xpv1.Condition) xpv1.Condition {
	c.Message = mrstatus.Truncate(c.Message, ac.maxMsgBytes)
	return c
}

// emitWarning raises a Warning event on the managed resource whose async
// result could not be persisted. It is best effort: by construction the object
// could not be fetched, so the event target carries only TypeMeta and a name.
// The error log above is the load-bearing signal.
func (ac *APICallbacks) emitWarning(target types.NamespacedName, op string, err error) {
	if ac.record == nil || ac.newEventTarget == nil {
		return
	}
	obj := ac.newEventTarget(target)
	if obj == nil {
		return
	}
	ac.record.Event(obj, event.Warning(event.Reason(eventReasonAsyncCallbackFailed),
		errors.Wrapf(err, "cannot persist the result of the async %s operation", op)))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
