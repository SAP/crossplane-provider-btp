//go:build diagnostic

package tfclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/sap/crossplane-provider-btp/pkg/diagnostics"
)

type concurrentLogger struct {
	mu      sync.Mutex
	entries []map[string]any
	queued  chan struct{}
}

func (l *concurrentLogger) Info(msg string, kv ...any) { l.Debug(msg, kv...) }
func (l *concurrentLogger) Debug(msg string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := kvToMap(kv)
	e["msg"] = msg
	l.entries = append(l.entries, e)
	if msg == "btp request queued" && e["resource"] == "ServiceManager/waiter" {
		select {
		case l.queued <- struct{}{}:
		default:
		}
	}
}
func (l *concurrentLogger) WithValues(...any) logging.Logger { return l }

// Exercises the actual embedded client's mutex, not a second model of the locking behavior.
func TestSessionTraceRecordsWaitingPastDeadline(t *testing.T) {
	log := &concurrentLogger{queued: make(chan struct{}, 1)}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseHolder := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseHolder)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/login/") {
			w.Header().Set("X-Cpcli-Sessionid", "test-session")
			_ = json.NewEncoder(w).Encode(map[string]string{"mail": "u@example.com", "issuer": "https://example.com"})
			return
		}
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		w.Header().Set("X-Cpcli-Backend-Status", "404")
		_, _ = w.Write([]byte(`{"error":"NotFound","description":"test"}`))
	}))
	t.Cleanup(srv.Close)
	cp := newCachingProvider(tfprovider.NewWithClient(&http.Client{Transport: &cliTransport{base: http.DefaultTransport, log: log}}))
	if err := noForkConfigureOnce(context.Background(), cp, srv.URL, "pw"); err != nil {
		t.Fatal(err)
	}
	var data any
	for _, e := range cp.entries {
		data = e.resp.ResourceData
	}
	// ResourceData's concrete client is internal to the dependency; its exported facade is unchanged.
	get := reflect.ValueOf(data).Elem().FieldByName("Services").FieldByName("Binding").MethodByName("GetById")
	call := func(ctx context.Context) {
		get.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf("subaccount"), reflect.ValueOf("binding")})
	}
	holderDone := make(chan struct{})
	go func() {
		defer close(holderDone)
		call(diagnostics.Begin(context.Background(), "ServiceManager/holder", log))
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("holder did not start")
	}
	ctx, cancel := context.WithTimeout(diagnostics.Begin(context.Background(), "ServiceManager/waiter", log), 20*time.Millisecond)
	defer cancel()
	waiterDone := make(chan struct{})
	go func() { defer close(waiterDone); call(ctx) }()
	select {
	case <-log.queued:
	case <-time.After(5 * time.Second):
		releaseHolder()
		<-holderDone
		<-waiterDone
		t.Fatal("session hooks missing; run make diagnostic-overlay and go test -overlay=.work/diagnostic-overlay/overlay.json")
	}
	<-ctx.Done()
	// The observer must not turn the existing mutex into a cancellation-aware lock.
	select {
	case <-waiterDone:
		t.Fatal("instrumentation changed locking behavior")
	default:
	}
	releaseHolder()
	<-holderDone
	<-waiterDone
	log.mu.Lock()
	defer log.mu.Unlock()
	for _, e := range log.entries {
		if e["msg"] == "btp session lock acquired" && e["resource"] == "ServiceManager/waiter" {
			if e["contextError"] != "context deadline exceeded" || e["waitMs"].(int64) < 10 {
				t.Fatal("deadline or wait missing", e)
			}
			return
		}
	}
	t.Fatal("waiter timing missing")
}
