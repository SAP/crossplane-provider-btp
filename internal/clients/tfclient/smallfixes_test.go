package tfclient

import (
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestCLIRequestTimeoutClient(t *testing.T) {
	rt := &hierBase{}
	c := newCLIHTTPClient(rt)
	if c.Transport != rt {
		t.Fatalf("Transport = %v, want the given RoundTripper", c.Transport)
	}
	if c.Timeout != cliRequestTimeout {
		t.Fatalf("Timeout = %v, want %v", c.Timeout, cliRequestTimeout)
	}
	if cliRequestTimeout <= 2*hierarchyCallTimeout {
		t.Fatalf("cliRequestTimeout %v must exceed two hierarchy call limits (%v)", cliRequestTimeout, 2*hierarchyCallTimeout)
	}
	if cliRequestTimeout >= time.Minute {
		t.Fatalf("cliRequestTimeout %v must stay below the default reconcile deadline", cliRequestTimeout)
	}
}

func TestCLIRequestTimeoutFrameworkClient(t *testing.T) {
	hc := frameworkHTTPClient(&cachingProvider{entries: map[string]*cacheEntry{}})
	if hc.Timeout != cliRequestTimeout {
		t.Fatalf("Timeout = %v, want %v", hc.Timeout, cliRequestTimeout)
	}
	if _, ok := hc.Transport.(*cliTransport); !ok {
		t.Fatalf("Transport = %T, want *cliTransport", hc.Transport)
	}
}

func TestHierarchyPruneDefault(t *testing.T) {
	if got := newHierarchyLoader().maxIdle; got != hierarchyEntryMaxIdle {
		t.Fatalf("maxIdle = %v, want %v", got, hierarchyEntryMaxIdle)
	}
	if hierarchyEntryMaxIdle <= hierarchyCallTTL {
		t.Fatalf("hierarchyEntryMaxIdle %v must exceed the ttl %v", hierarchyEntryMaxIdle, hierarchyCallTTL)
	}
}

func sfKey(subaccount string) string { return hierHost + "/" + subaccount }

func sfKeys(l *hierarchyLoader) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	keys := make([]string, 0, len(l.entries))
	for k := range l.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sfSend(t *testing.T, tr *cliTransport, subaccount string) {
	t.Helper()
	if status, _ := hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(subaccount))); status != http.StatusOK {
		t.Fatalf("status for %s = %d, want 200", subaccount, status)
	}
}

// sfPruneLoader returns a loader on a fake clock whose first sweep has run.
func sfPruneLoader() (*hierarchyLoader, *hierClock) {
	clk := &hierClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	l := newHierarchyLoader()
	l.now = clk.now
	l.mu.Lock()
	l.entryLocked("first")
	l.mu.Unlock()
	return l, clk
}

func TestHierarchyPruneRemovesIdleEntries(t *testing.T) {
	tr, clk := hierTransport(&hierBase{}, nil)
	sfSend(t, tr, hierSubaccount)
	clk.advance(hierarchyEntryMaxIdle + time.Minute)
	sfSend(t, tr, hierSubaccount2)

	if keys := sfKeys(tr.hierarchy); len(keys) != 1 || keys[0] != sfKey(hierSubaccount2) {
		t.Fatalf("entries = %v, want only %s", keys, sfKey(hierSubaccount2))
	}
}

func TestHierarchyPruneKeepsRecentEntries(t *testing.T) {
	tr, clk := hierTransport(&hierBase{}, nil)
	sfSend(t, tr, hierSubaccount)
	clk.advance(hierarchyEntryMaxIdle - time.Minute)
	sfSend(t, tr, hierSubaccount)
	clk.advance(2 * time.Minute)
	sfSend(t, tr, hierSubaccount2)

	want := []string{sfKey(hierSubaccount), sfKey(hierSubaccount2)}
	sort.Strings(want)
	if keys := sfKeys(tr.hierarchy); strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("entries = %v, want %v", keys, want)
	}
}

func TestHierarchyPruneKeepsEntryInFlight(t *testing.T) {
	clk := &hierClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	l := newHierarchyLoader()
	l.now = clk.now
	l.entries["inflight"] = &hierarchyEntry{flight: &hierarchyFlight{done: make(chan struct{})}}
	l.entries["idle"] = &hierarchyEntry{}

	l.mu.Lock()
	l.pruneLocked()
	l.mu.Unlock()

	if keys := sfKeys(l); len(keys) != 1 || keys[0] != "inflight" {
		t.Fatalf("entries = %v, want only the entry in flight", keys)
	}
}

// Either timestamp alone keeps an entry: guardedAt without loadedAt is a
// subaccount whose hierarchy calls fail while its commands are still served, and
// guard must keep reporting it.
func TestHierarchyPruneKeepsEntryWithOneRecentTimestamp(t *testing.T) {
	clk := &hierClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	l := newHierarchyLoader()
	l.now = clk.now
	recent := clk.now().Add(-time.Minute)
	l.entries["guarded"] = &hierarchyEntry{guardedAt: recent}
	l.entries["loaded"] = &hierarchyEntry{loadedAt: recent}
	l.entries["idle"] = &hierarchyEntry{}

	l.mu.Lock()
	l.pruneLocked()
	l.mu.Unlock()

	if keys := sfKeys(l); strings.Join(keys, ",") != "guarded,loaded" {
		t.Fatalf("entries = %v, want [guarded loaded]", keys)
	}
}

func TestHierarchyPruneSweepsAtMostOncePerInterval(t *testing.T) {
	l, clk := sfPruneLoader()
	l.mu.Lock()
	l.entries["idle"] = &hierarchyEntry{}
	l.mu.Unlock()

	clk.advance(hierarchyEntryMaxIdle - time.Second)
	l.mu.Lock()
	l.entryLocked("second")
	_, kept := l.entries["idle"]
	l.mu.Unlock()
	if !kept {
		t.Fatal("idle entry removed by a second sweep within maxIdle")
	}

	clk.advance(time.Second)
	l.mu.Lock()
	l.entryLocked("third")
	_, kept = l.entries["idle"]
	l.mu.Unlock()
	if kept {
		t.Fatal("idle entry kept after maxIdle has passed since the last sweep")
	}
}

func TestHierarchyPruneOffWithoutMaxIdle(t *testing.T) {
	clk := &hierClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	l := newHierarchyLoader()
	l.now = clk.now
	l.maxIdle = 0
	l.entries["idle"] = &hierarchyEntry{}

	for _, k := range []string{"a", "b", "c"} {
		clk.advance(time.Hour)
		l.mu.Lock()
		l.entryLocked(k)
		l.mu.Unlock()
	}
	if keys := sfKeys(l); len(keys) != 4 {
		t.Fatalf("entries = %v, want all 4 kept", keys)
	}
}

func TestHierarchyPrunedSubaccountIsLoadedAgain(t *testing.T) {
	base := &hierBase{}
	tr, clk := hierTransport(base, nil)
	sfSend(t, tr, hierSubaccount)
	clk.advance(hierarchyEntryMaxIdle + time.Minute)
	sfSend(t, tr, hierSubaccount2)
	for _, k := range sfKeys(tr.hierarchy) {
		if k == sfKey(hierSubaccount) {
			t.Fatalf("entry for %s not pruned", hierSubaccount)
		}
	}

	sfSend(t, tr, hierSubaccount)
	if h, c := base.counts(); h != 3 || c != 3 {
		t.Fatalf("hierarchy=%d commands=%d, want 3/3", h, c)
	}
	base.mu.Lock()
	last := base.hierBodies[len(base.hierBodies)-1]
	base.mu.Unlock()
	if !strings.Contains(last, hierSubaccount) {
		t.Fatalf("last hierarchy call body %q does not name %s", last, hierSubaccount)
	}
}
