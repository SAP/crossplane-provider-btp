package tfclient

import (
	"context"
	"time"

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
	// defaultMaxConditionMessageBytes bounds how much terraform output is
	// copied into a condition message. The error of a failed terraform
	// apply/destroy is the full CLI output and can be tens of kilobytes;
	// unbounded it would bloat the status of every failing managed resource.
	defaultMaxConditionMessageBytes = 16 * 1024

	// eventReasonAsyncCallbackFailed is raised when an async result cannot be
	// persisted on the managed resource it belongs to.
	eventReasonAsyncCallbackFailed = "AsyncCallbackFailed"

	// saveResultTimeout is the write budget of one async result. Independent
	// of the async operation's own deadline: a result produced BY that
	// deadline expiring must still be persisted — see callback().
	saveResultTimeout = time.Minute
)

var errUpdateStatusFmt = "cannot update status of the resource %s after an async %s"

// APICallbacksOption configures an APICallbacks.
type APICallbacksOption func(*APICallbacks)

// WithCallbackLogger overrides the logger used for the (loud) failure path.
func WithCallbackLogger(l logr.Logger) APICallbacksOption {
	return func(ac *APICallbacks) {
		ac.log = l
	}
}

// WithCallbackEventRecorder makes the callbacks emit a Warning event on the
// managed resource when an async result cannot be persisted. objFn builds the
// event target from the terraform resource name and may return nil to skip the
// event.
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

	log logr.Logger

	// record and newEventTarget are optional; both must be set for an event
	// to be emitted.
	record         event.Recorder
	newEventTarget func(types.NamespacedName) resource.Managed

	maxMsgBytes int
}

// Create makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Create(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("create", name)
}

// Update makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Update(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("update", name)
}

// Destroy makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Destroy(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("destroy", name)
}

// callback resolves the terraform resource name to the native managed
// resource via SaveConditionsFn.
func (ac *APICallbacks) callback(op string, name types.NamespacedName) terraform.CallbackFn {
	return func(err error, ctx context.Context) error {
		// The context upjet hands the callback carries the async operation's
		// own deadline. When the operation is killed BY that deadline the
		// callback runs with the context already expired and every write
		// would fail with DeadlineExceeded, so the write gets a budget of its
		// own.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveResultTimeout)
		defer cancel()
		uErr := ac.saveCallbackFn(ctx, ac.kube, name, ac.bound(ujresource.LastAsyncOperationCondition(err)), ujresource.AsyncOperationFinishedCondition())
		if uErr == nil {
			return nil
		}
		// upjet logs a failing callback at info level only (#968).
		ac.log.Error(uErr, "async terraform callback could not persist its result",
			"operation", op, "terraformName", name.String())
		ac.emitWarning(name, op, uErr)
		return errors.Wrapf(uErr, errUpdateStatusFmt, name, op)
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
func (ac *APICallbacks) emitWarning(name types.NamespacedName, op string, err error) {
	if ac.record == nil || ac.newEventTarget == nil {
		return
	}
	obj := ac.newEventTarget(name)
	if obj == nil {
		return
	}
	ac.record.Event(obj, event.Warning(event.Reason(eventReasonAsyncCallbackFailed),
		errors.Wrapf(err, "cannot persist the result of the async %s operation", op)))
}
