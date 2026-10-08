package diagnostics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
)

func TestCopyRetainsDestinationCancellation(t *testing.T) {
	src := Begin(context.Background(), "ServiceManager/sm-parent", nil)
	src, _ = Stage(src, "create")
	dst, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	copied := Copy(dst, src)
	if Resource(copied) != "ServiceManager/sm-parent" || ReconcileID(copied) == "" || StageName(copied) != "create" {
		t.Fatal("diagnostic values were lost")
	}
	cancel()
	if copied.Err() != context.Canceled {
		t.Fatal("destination cancellation was lost")
	}
	// This is also how async hooks preserve trace values in a fresh background context.
	async := Copy(context.Background(), copied)
	if async.Err() != nil || ReconcileID(async) != ReconcileID(copied) {
		t.Fatal("async copy inherited cancellation or lost identity")
	}
}

func TestAttemptCountersArePerLogicalCall(t *testing.T) {
	ctx := BeginRequest(context.Background())
	const n = 100
	results := make(chan int, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() { results <- NextAttempt(ctx) })
	}
	wg.Wait()
	close(results)
	seen := map[int]bool{}
	for i := range results {
		if i < 1 || i > n || seen[i] {
			t.Fatal("duplicate attempt", i)
		}
		seen[i] = true
	}
	if NextAttempt(BeginRequest(ctx)) != 1 || NextAttempt(context.Background()) != 1 {
		t.Fatal("counter leaked into another logical call")
	}
}

type externalRecorder struct {
	managed.ExternalClient
	contexts []context.Context
}

func (e *externalRecorder) Observe(ctx context.Context, _ resource.Managed) (managed.ExternalObservation, error) {
	e.contexts = append(e.contexts, ctx)
	return managed.ExternalObservation{}, nil
}
func (e *externalRecorder) Delete(ctx context.Context, _ resource.Managed) (managed.ExternalDelete, error) {
	e.contexts = append(e.contexts, ctx)
	return managed.ExternalDelete{}, nil
}

func TestExternalRestoresParentTraceAndStage(t *testing.T) {
	parent := Begin(context.Background(), "ServiceManager/sm-parent", nil)
	inner := &externalRecorder{}
	e := &external{ExternalClient: inner, trace: parent}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, _ = Stage(ctx, "instance-observe-repeat")
	if _, err := e.Observe(ctx, &fake.Managed{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Delete(context.Background(), &fake.Managed{}); err != nil {
		t.Fatal(err)
	}
	for _, received := range inner.contexts {
		if Resource(received) != "ServiceManager/sm-parent" || ReconcileID(received) != ReconcileID(parent) {
			t.Fatal("parent trace lost")
		}
	}
	if StageName(inner.contexts[0]) != "instance-observe-repeat" || StageName(inner.contexts[1]) != "delete" {
		t.Fatal("stage lost")
	}
	cancel()
	if inner.contexts[0].Err() != context.Canceled {
		t.Fatal("Observe cancellation lost")
	}
}
