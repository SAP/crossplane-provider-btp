package tfclient

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/sap/crossplane-provider-btp/pkg/diagnostics"
)

// HTTP callbacks may run concurrently; timings describe one transport attempt, excluding retry backoff.
type requestTiming struct {
	mu                               sync.Mutex
	start, getConn, tlsStart         time.Time
	connectionMs, tlsMs, firstByteMs int64
	reused                           bool
}

func (t *requestTiming) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(string) { t.mu.Lock(); t.getConn = time.Now(); t.mu.Unlock() },
		GotConn: func(i httptrace.GotConnInfo) {
			t.mu.Lock()
			t.connectionMs = time.Since(t.getConn).Milliseconds()
			t.reused = i.Reused
			t.mu.Unlock()
		},
		TLSHandshakeStart: func() { t.mu.Lock(); t.tlsStart = time.Now(); t.mu.Unlock() },
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.mu.Lock()
			t.tlsMs = time.Since(t.tlsStart).Milliseconds()
			t.mu.Unlock()
		},
		GotFirstResponseByte: func() { t.mu.Lock(); t.firstByteMs = time.Since(t.start).Milliseconds(); t.mu.Unlock() },
	}
}

func (t *requestTiming) fields() []any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return []any{"connectionWaitMs", t.connectionMs, "connectionReused", t.reused, "tlsMs", t.tlsMs, "firstByteMs", t.firstByteMs}
}

// observedBody forwards reads and closes unchanged. It never drains or buffers the body.
type observedBody struct {
	io.ReadCloser
	ctx     context.Context
	start   time.Time
	request *http.Request
	once    sync.Once
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		logErr := err
		if err == io.EOF {
			logErr = nil
		}
		b.record("eof", logErr)
	}
	return n, err
}

func (b *observedBody) Close() error {
	err := b.ReadCloser.Close()
	b.record("close", err)
	return err
}

func (b *observedBody) record(reason string, err error) {
	b.once.Do(func() {
		diagnostics.Record(b.ctx, "btp response body finished", err, "reason", reason, "bodyLifetimeMs", time.Since(b.start).Milliseconds(), "path", b.request.URL.Path, "action", b.request.URL.RawQuery, "correlationID", b.request.Header.Get(headerCorrelationID))
	})
}
