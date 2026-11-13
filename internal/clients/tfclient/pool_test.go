package tfclient

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
)

// barrierProvider blocks every Configure until `target` of them are in flight at
// once, recording the peak concurrency reached.
type barrierProvider struct {
	provider.Provider
	target   int64
	inflight atomic.Int64
	peak     atomic.Int64
	gate     chan struct{}
	once     sync.Once
}

func (b *barrierProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	n := b.inflight.Add(1)
	for {
		p := b.peak.Load()
		if n <= p || b.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if n >= b.target {
		b.once.Do(func() { close(b.gate) })
	}
	<-b.gate
	b.inflight.Add(-1)
	resp.ResourceData = &struct{}{}
}

// A size-N pool lets N reconciles for one key configure concurrently; a serialized
// (size 1) pool could never reach a peak of N, so this would deadlock on the gate.
func TestPoolParallelism(t *testing.T) {
	const (
		N = 4
		M = 8
	)
	b := &barrierProvider{target: N, gate: make(chan struct{})}
	p := newTerraformProvider(b, poolConfig{size: N})
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < M; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var r provider.ConfigureResponse
			p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
		}()
	}
	wg.Wait()

	if got := b.peak.Load(); got != N {
		t.Fatalf("peak concurrency = %d, want %d (min(M,N))", got, N)
	}
}

// Wrong credentials through a pool still lock the technical user at the threshold:
// the pool does not amplify (or mask) lockout.
func TestPool_WrongPassword_Locks(t *testing.T) {
	t.Parallel()
	fake := &fakeBTPCLI{password: "correct-pw", lockThreshold: 5}
	srv := fake.start(t)
	p := newTerraformProvider(tfprovider.New(), poolConfig{size: 3})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = noForkConfigureOnce(ctx, p, srv.URL, "WRONG")
	}

	fake.mu.Lock()
	locked := fake.locked
	fake.mu.Unlock()
	if !locked {
		t.Error("wrong password through the pool must still lock the user at the threshold")
	}
}
