package tfclient

// Behavioural tests for the hierarchy call: instead of scripting responses, a fake
// CLI server reproduces the observed rule (a subaccount command is answered with a
// bare 500 unless the subaccount was loaded by a hierarchy call within `keep`) and
// the tests check what callers and the server actually see.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const (
	behaviourSubaccount = "11111111-2222-4333-8444-555555555555"
	behaviourInstance   = "22222222-2222-4333-8444-555555555555"
	behaviourSession    = "session-placeholder"
	behaviourGA         = "ga-placeholder"
	behaviourCorrID     = "33333333-2222-4333-8444-555555555555"
	behaviourVersion    = "v2.106.1"

	behaviourInstanceJSON = `{"id":"` + behaviourInstance + `","name":"inst","ready":true,"subaccount_id":"` + behaviourSubaccount +
		`","last_operation":{"state":"succeeded"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`
)

type behaviourClock struct {
	mu sync.Mutex
	t  time.Time
}

func newBehaviourClock() *behaviourClock {
	return &behaviourClock{t: time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *behaviourClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *behaviourClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type behaviourRecord struct {
	path   string
	query  string
	header http.Header
	body   []byte
}

type behaviourCLIServer struct {
	*httptest.Server
	clock *behaviourClock
	keep  time.Duration
	// gate, when set, holds every hierarchy answer until it is closed; arrived is
	// closed when the first hierarchy request has reached the handler.
	gate        chan struct{}
	arrived     chan struct{}
	arrivedOnce sync.Once

	mu           sync.Mutex
	loadedAt     map[string]time.Time
	records      []behaviourRecord
	hierarchies  int
	commands     int
	bare500Count int
}

func newBehaviourCLIServer(t *testing.T, clock *behaviourClock, keep time.Duration) *behaviourCLIServer {
	t.Helper()
	s := &behaviourCLIServer{
		clock:    clock,
		keep:     keep,
		arrived:  make(chan struct{}),
		loadedAt: map[string]time.Time{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *behaviourCLIServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rec := behaviourRecord{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: body}
	isPost := r.Method == http.MethodPost

	switch {
	case isPost && strings.HasPrefix(r.URL.Path, "/login/"):
		s.record(rec)
		w.Header().Set(hierWireSessionID, behaviourSession)
		_, _ = w.Write([]byte(`{"mail":"u@example.test","issuer":"https://idp.example.test"}`))

	case isPost && r.URL.Path == hierWireHierarchyPath:
		s.mu.Lock()
		s.records = append(s.records, rec)
		s.hierarchies++
		s.mu.Unlock()
		if s.gate != nil {
			s.arrivedOnce.Do(func() { close(s.arrived) })
			<-s.gate
		}
		var req struct {
			Nodes []struct {
				EntityGUID string `json:"entityGuid"`
				EntityType string `json:"entityType"`
			} `json:"nodes"`
		}
		_ = json.Unmarshal(body, &req)
		s.mu.Lock()
		for _, n := range req.Nodes {
			if n.EntityType == "SUBACCOUNT" {
				s.loadedAt[n.EntityGUID] = s.clock.Now()
			}
		}
		s.mu.Unlock()
		w.Header().Set(hierWireBackendStatus, "200")
		_, _ = w.Write([]byte(`{"guid":"` + behaviourGA + `"}`))

	case isPost && strings.Contains(r.URL.Path, "/command/"):
		var req struct {
			ParamValues struct {
				Subaccount string `json:"subaccount"`
			} `json:"paramValues"`
		}
		_ = json.Unmarshal(body, &req)
		sa := req.ParamValues.Subaccount
		s.mu.Lock()
		s.records = append(s.records, rec)
		s.commands++
		at, ok := s.loadedAt[sa]
		if sa != "" && (!ok || s.clock.Now().Sub(at) >= s.keep) {
			s.bare500Count++
			s.mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.mu.Unlock()
		w.Header().Set(hierWireBackendStatus, "200")
		_, _ = w.Write([]byte(behaviourInstanceJSON))

	default:
		http.NotFound(w, r)
	}
}

func (s *behaviourCLIServer) record(rec behaviourRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rec)
}

func (s *behaviourCLIServer) forget(subaccount string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.loadedAt, subaccount)
}

func (s *behaviourCLIServer) counts() (hierarchies, commands, bare500 int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hierarchies, s.commands, s.bare500Count
}

func (s *behaviourCLIServer) recorded() []behaviourRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]behaviourRecord(nil), s.records...)
}

func behaviourCommandBody(subaccount string) string {
	return `{"paramValues":{"id":"` + behaviourInstance + `","parameters":"false","subaccount":"` + subaccount + `"}}`
}

func behaviourPost(t *testing.T, serverURL, path, body string, header map[string]string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, serverURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range header {
		req.Header[k] = []string{v}
	}
	return req
}

func behaviourCommandWithBody(t *testing.T, serverURL, session, body string) *http.Request {
	t.Helper()
	return behaviourPost(t, serverURL, "/command/"+behaviourVersion+"/services/instance?get", body, map[string]string{
		"Content-Type":        "application/json",
		hierWireFormat:        "json",
		hierWireSessionID:     session,
		hierWireSubdomain:     behaviourGA,
		hierWireCustomIDP:     "",
		hierWireCorrelationID: behaviourCorrID,
	})
}

func behaviourCommand(t *testing.T, serverURL, session, subaccount string) *http.Request {
	t.Helper()
	return behaviourCommandWithBody(t, serverURL, session, behaviourCommandBody(subaccount))
}

// behaviourDo returns the status the caller sees, or -1 on a transport error.
func behaviourDo(t *testing.T, tr http.RoundTripper, req *http.Request) int {
	t.Helper()
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Errorf("round trip: %v", err)
		return -1
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func behaviourTransport(clock *behaviourClock, hierarchyCall bool) *cliTransport {
	tr := newCLITransport(&cachingProvider{entries: map[string]*cacheEntry{}}, http.DefaultTransport, nil, hierarchyCall)
	if tr.hierarchy != nil {
		tr.hierarchy.now = clock.Now
	}
	return tr
}

func behaviourExpectCounts(t *testing.T, s *behaviourCLIServer, hierarchies, commands, bare500 int) {
	t.Helper()
	h, c, b := s.counts()
	if h != hierarchies || c != commands || b != bare500 {
		t.Errorf("server saw hierarchy=%d commands=%d bare500=%d, want %d/%d/%d", h, c, b, hierarchies, commands, bare500)
	}
}

func behaviourJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("unmarshal got %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestCLIServerBehaviourWithoutFeature(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	tr := behaviourTransport(clock, false)

	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusInternalServerError {
		t.Errorf("caller status = %d, want 500", got)
	}
	behaviourExpectCounts(t, srv, 0, 1, 1)
}

func TestCLIServerBehaviourHierarchyCallFirst(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	tr := behaviourTransport(clock, true)

	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
		t.Fatalf("caller status = %d, want 200", got)
	}
	recs := srv.recorded()
	if len(recs) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(recs))
	}
	hier, cmd := recs[0], recs[1]
	if hier.path != hierWireHierarchyPath {
		t.Errorf("first request path = %q, want the hierarchy endpoint", hier.path)
	}
	if !strings.HasPrefix(cmd.path, "/command/") {
		t.Errorf("second request path = %q, want a command", cmd.path)
	}
	if hier.query != "" {
		t.Errorf("hierarchy query = %q, want none", hier.query)
	}
	behaviourJSONEqual(t, hier.body, `{"nodes":[{"entityGuid":"`+behaviourSubaccount+`","entityType":"SUBACCOUNT"}]}`)
	if got := hier.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := hier.header.Get(hierWireFormat); got != "json" {
		t.Errorf("format = %q", got)
	}
	if got := hier.header.Get(hierWireSessionID); got != behaviourSession {
		t.Errorf("session = %q, want the command's", got)
	}
	for _, k := range []string{hierWireSubdomain, hierWireCustomIDP} {
		v, ok := hier.header[k]
		if !ok || len(v) != 1 || v[0] != "" {
			t.Errorf("header %s = %q (present=%v), want present and empty", k, v, ok)
		}
	}
	if hier.header.Get(hierWireCorrelationID) == "" {
		t.Error("hierarchy call carries no correlation id")
	}
	if string(cmd.body) != behaviourCommandBody(behaviourSubaccount) {
		t.Errorf("command body = %s, want it unchanged", cmd.body)
	}
	behaviourExpectCounts(t, srv, 1, 1, 0)
}

func TestCLIServerBehaviourSecondCommandReusesCall(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	tr := behaviourTransport(clock, true)

	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
		t.Fatalf("first status = %d", got)
	}
	clock.Advance(30 * time.Second)
	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
		t.Errorf("second status = %d", got)
	}
	// The loaded state is server-wide, so another session needs no call either.
	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, "other-session-placeholder", behaviourSubaccount)); got != http.StatusOK {
		t.Errorf("other session status = %d", got)
	}
	behaviourExpectCounts(t, srv, 1, 3, 0)
}

func TestCLIServerBehaviourAfterRememberedTimeExpires(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	tr := behaviourTransport(clock, true)

	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
		t.Fatalf("first status = %d", got)
	}
	clock.Advance(hierarchyCallTTL - time.Second)
	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
		t.Errorf("status before expiry = %d", got)
	}
	if h, _, _ := srv.counts(); h != 1 {
		t.Errorf("hierarchy calls just before the TTL = %d, want 1", h)
	}
	clock.Advance(time.Second)
	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
		t.Errorf("status after expiry = %d", got)
	}
	behaviourExpectCounts(t, srv, 2, 3, 0)
}

func TestCLIServerBehaviourServerForgotEarly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		keep   time.Duration
		forget func(clock *behaviourClock, srv *behaviourCLIServer)
	}{
		{"expired on server", 60 * time.Second, func(c *behaviourClock, _ *behaviourCLIServer) { c.Advance(70 * time.Second) }},
		{"dropped on server", 3 * time.Minute, func(_ *behaviourClock, s *behaviourCLIServer) { s.forget(behaviourSubaccount) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := newBehaviourClock()
			srv := newBehaviourCLIServer(t, clock, tc.keep)
			tr := behaviourTransport(clock, true)

			if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
				t.Fatalf("first status = %d", got)
			}
			tc.forget(clock, srv)
			if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, behaviourSubaccount)); got != http.StatusOK {
				t.Errorf("caller status = %d, want 200 and never the 500", got)
			}
			behaviourExpectCounts(t, srv, 2, 3, 1)

			var cmdBodies [][]byte
			for _, r := range srv.recorded() {
				if strings.HasPrefix(r.path, "/command/") {
					cmdBodies = append(cmdBodies, r.body)
				}
			}
			if len(cmdBodies) != 3 {
				t.Fatalf("command requests = %d, want 3", len(cmdBodies))
			}
			if !bytes.Equal(cmdBodies[1], cmdBodies[2]) || string(cmdBodies[2]) != behaviourCommandBody(behaviourSubaccount) {
				t.Errorf("resent body = %s, want the original %s", cmdBodies[2], cmdBodies[1])
			}
		})
	}
}

func TestCLIServerBehaviourPassThrough(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		path     string
		body     string
		header   map[string]string
		commands int
	}{
		{
			name:   "login without session headers",
			path:   "/login/" + behaviourVersion,
			body:   `{"userName":"technical-user","password":"pw"}`,
			header: map[string]string{"Content-Type": "application/json", hierWireFormat: "json"},
		},
		{
			name: "command without subaccount",
			path: "/command/" + behaviourVersion + "/accounts/global-account?get",
			body: `{"paramValues":{"globalAccount":"` + behaviourGA + `"}}`,
			header: map[string]string{
				"Content-Type": "application/json", hierWireFormat: "json", hierWireSessionID: behaviourSession,
				hierWireSubdomain: behaviourGA, hierWireCustomIDP: "", hierWireCorrelationID: behaviourCorrID,
			},
			commands: 1,
		},
		{
			name: "command scoped to a directory",
			path: "/command/" + behaviourVersion + "/accounts/directory?get",
			body: `{"paramValues":{"directory":"` + behaviourInstance + `"}}`,
			header: map[string]string{
				"Content-Type": "application/json", hierWireFormat: "json", hierWireSessionID: behaviourSession,
				hierWireSubdomain: behaviourGA, hierWireCustomIDP: "", hierWireCorrelationID: behaviourCorrID,
			},
			commands: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := newBehaviourClock()
			srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
			tr := behaviourTransport(clock, true)

			if got := behaviourDo(t, tr, behaviourPost(t, srv.URL, tc.path, tc.body, tc.header)); got != http.StatusOK {
				t.Errorf("caller status = %d, want the server's 200", got)
			}
			behaviourExpectCounts(t, srv, 0, tc.commands, 0)
			recs := srv.recorded()
			if len(recs) != 1 {
				t.Fatalf("server saw %d requests, want 1", len(recs))
			}
			if string(recs[0].body) != tc.body {
				t.Errorf("body = %s, want %s", recs[0].body, tc.body)
			}
			for k, v := range tc.header {
				got, ok := recs[0].header[http.CanonicalHeaderKey(k)]
				if !ok || len(got) != 1 || got[0] != v {
					t.Errorf("header %s = %q (present=%v), want %q", k, got, ok, v)
				}
			}
		})
	}
}

func TestCLIServerBehaviourConcurrentCommandsOneCall(t *testing.T) {
	t.Parallel()
	const n = 50
	clock := newBehaviourClock()
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	srv.gate = make(chan struct{})
	release := sync.OnceFunc(func() { close(srv.gate) })
	t.Cleanup(release)
	tr := behaviourTransport(clock, true)

	var started, done sync.WaitGroup
	statuses := make([]int, n)
	for i := 0; i < n; i++ {
		req := behaviourCommand(t, srv.URL, fmt.Sprintf("session-placeholder-%d", i), behaviourSubaccount)
		started.Add(1)
		done.Add(1)
		go func() {
			defer done.Done()
			started.Done()
			statuses[i] = behaviourDo(t, tr, req)
		}()
	}
	started.Wait()
	// Bounded so a hierarchy call that never reaches the server fails the test
	// instead of hanging it.
	hierAwait(t, srv.arrived, "hierarchy call reaching the server")
	release()
	done.Wait()

	for i, s := range statuses {
		if s != http.StatusOK {
			t.Errorf("caller %d status = %d, want 200", i, s)
		}
	}
	behaviourExpectCounts(t, srv, 1, n, 0)
}

// behaviourProbe sits between the provider and the transport, standing in for
// what btpcli's retry layer would see.
type behaviourProbe struct {
	next http.RoundTripper

	mu          sync.Mutex
	noGetBody   int
	statuses    []int
	requestSeen int
}

func (p *behaviourProbe) RoundTrip(r *http.Request) (*http.Response, error) {
	p.mu.Lock()
	p.requestSeen++
	if r.GetBody == nil {
		p.noGetBody++
	}
	p.mu.Unlock()
	resp, err := p.next.RoundTrip(r)
	if resp != nil {
		p.mu.Lock()
		p.statuses = append(p.statuses, resp.StatusCode)
		p.mu.Unlock()
	}
	return resp, err
}

func behaviourDynamicValue(data map[string]any, typ tftypes.Type) (*tfprotov6.DynamicValue, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	v, err := tftypes.ValueFromJSONWithOpts(raw, typ, tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true})
	if err != nil {
		return nil, fmt.Errorf("tf value from json: %w", err)
	}
	dv, err := tfprotov6.NewDynamicValue(typ, v)
	if err != nil {
		return nil, fmt.Errorf("dynamic value: %w", err)
	}
	return &dv, nil
}

func behaviourFailOnErrors(t *testing.T, what string, diags []*tfprotov6.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s: %s: %s", what, d.Summary, d.Detail)
		}
	}
}

func TestCLIServerBehaviourRealProviderNeverSeesBare500(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := newBehaviourClock()
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	tr := behaviourTransport(clock, true)
	probe := &behaviourProbe{next: tr}

	p := tfprovider.NewWithClient(&http.Client{Transport: probe})
	var sr fwprovider.SchemaResponse
	p.Schema(ctx, fwprovider.SchemaRequest{}, &sr)
	if sr.Diagnostics.HasError() {
		t.Fatalf("provider schema: %v", sr.Diagnostics)
	}
	server := providerserver.NewProtocol6(p)()

	cfg, err := behaviourDynamicValue(map[string]any{
		"username":       "technical-user",
		"password":       "pw",
		"globalaccount":  behaviourGA,
		"cli_server_url": srv.URL,
	}, sr.Schema.Type().TerraformType(ctx))
	if err != nil {
		t.Fatal(err)
	}
	cresp, err := server.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{TerraformVersion: "crossTF000", Config: cfg})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	behaviourFailOnErrors(t, "configure", cresp.Diagnostics)

	schemas, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("get provider schema: %v", err)
	}
	behaviourFailOnErrors(t, "get provider schema", schemas.Diagnostics)
	const dataSource = "btp_subaccount_service_instance"
	ds, ok := schemas.DataSourceSchemas[dataSource]
	if !ok {
		t.Fatalf("data source %s not in provider schema", dataSource)
	}
	dsCfg, err := behaviourDynamicValue(map[string]any{
		"subaccount_id": behaviourSubaccount,
		"id":            behaviourInstance,
	}, ds.ValueType())
	if err != nil {
		t.Fatal(err)
	}

	read := func(what string) {
		t.Helper()
		resp, err := server.ReadDataSource(ctx, &tfprotov6.ReadDataSourceRequest{TypeName: dataSource, Config: dsCfg})
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		behaviourFailOnErrors(t, what, resp.Diagnostics)
	}
	read("first read")
	// One read issues several commands; the count is measured rather than assumed.
	_, perRead, _ := srv.counts()
	srv.forget(behaviourSubaccount)
	read("read after the server forgot the subaccount")

	probe.mu.Lock()
	defer probe.mu.Unlock()
	// Only the first command of the second read is repeated after its bare 500.
	behaviourExpectCounts(t, srv, 2, 2*perRead+1, 1)
	for _, s := range probe.statuses {
		if s == http.StatusInternalServerError {
			t.Errorf("provider side saw a 500 response; statuses = %v", probe.statuses)
			break
		}
	}
	if probe.noGetBody != 0 {
		t.Errorf("%d of %d provider requests had no GetBody", probe.noGetBody, probe.requestSeen)
	}
	for _, r := range srv.recorded() {
		if !strings.Contains(r.path, "/client/") {
			continue
		}
		if got := r.header.Get(hierWireSessionID); got != behaviourSession {
			t.Errorf("hierarchy call session = %q, want the one the login returned", got)
		}
	}
}
