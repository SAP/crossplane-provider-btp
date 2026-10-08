// Package diagnostics records reconciliation timing without changing cancellation or HTTP body consumption.
package diagnostics

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/google/uuid"
)

type traceKey struct{}
type stageKey struct{}
type requestKey struct{}

type trace struct {
	resource string
	id       string
	log      logging.Logger
}

type request struct {
	attempt atomic.Int64
}

func WithResource(ctx context.Context, resource string) context.Context {
	t := contextTrace(ctx)
	t.resource = resource
	return context.WithValue(ctx, traceKey{}, t)
}

func contextTrace(ctx context.Context) trace {
	t, _ := ctx.Value(traceKey{}).(trace)
	return t
}

func Resource(ctx context.Context) string    { return contextTrace(ctx).resource }
func ReconcileID(ctx context.Context) string { return contextTrace(ctx).id }
func StageName(ctx context.Context) string {
	s, _ := ctx.Value(stageKey{}).(string)
	return s
}

func Begin(ctx context.Context, resource string, log logging.Logger) context.Context {
	if contextTrace(ctx).id != "" {
		return ctx
	}
	return context.WithValue(ctx, traceKey{}, trace{resource: resource, id: uuid.NewString(), log: log})
}

// Copy preserves diagnostic values, while retaining the destination's deadline and cancellation.
func Copy(dst, src context.Context) context.Context {
	dst = context.WithValue(dst, traceKey{}, contextTrace(src))
	if StageName(dst) != "" {
		return dst
	}
	return context.WithValue(dst, stageKey{}, StageName(src))
}

func Remaining(ctx context.Context) int64 {
	if d, ok := ctx.Deadline(); ok {
		return time.Until(d).Milliseconds()
	}
	return -1
}

func ContextError(ctx context.Context) string {
	if err := ctx.Err(); err != nil {
		return err.Error()
	}
	return ""
}

func Fields(ctx context.Context) []any {
	return []any{"resource", Resource(ctx), "reconcileID", ReconcileID(ctx), "stage", StageName(ctx), "remainingMs", Remaining(ctx), "contextError", ContextError(ctx)}
}

func Record(ctx context.Context, message string, err error, fields ...any) {
	log := contextTrace(ctx).log
	if log == nil {
		return
	}
	kv := append(Fields(ctx), fields...)
	if err != nil {
		log.Info(message, append(kv, "error", err.Error())...)
		return
	}
	log.Debug(message, kv...)
}

func Stage(ctx context.Context, name string) (context.Context, func(error)) {
	ctx = context.WithValue(ctx, stageKey{}, name)
	start := time.Now()
	Record(ctx, "reconcile stage started", nil)
	return ctx, func(err error) {
		Record(ctx, "reconcile stage finished", err, "durationMs", time.Since(start).Milliseconds())
	}
}

// BeginRequest gives each logical call its own counter, naturally released with its context.
func BeginRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestKey{}, &request{})
}

func NextAttempt(ctx context.Context) int {
	if r, ok := ctx.Value(requestKey{}).(*request); ok {
		return int(r.attempt.Add(1))
	}
	return 1
}
