package tfclient

// Behavioural tests for the plan and offering lookup cache: a fake CLI server
// counts what reaches it, and the tests check what the real terraform-provider-btp
// and direct callers of cliTransport get back.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	lcbSubaccountA = "11111111-2222-4333-8444-555555555555"
	lcbSubaccountB = "66666666-7777-4888-8999-aaaaaaaaaaaa"
	lcbInstanceID  = "22222222-2222-4333-8444-555555555555"
	lcbPlanID      = "33333333-2222-4333-8444-555555555555"
	lcbOfferingID  = "44444444-2222-4333-8444-555555555555"

	lcbPlanQuery     = "services/plan?get"
	lcbOfferingQuery = "services/offering?get"
	lcbInstanceQuery = "services/instance?get"

	lcbResource = "btp_subaccount_service_instance"

	// Upstream reads the instance three times per read (once without parameters,
	// twice with); the lookups are what the cache must remove, not these.
	lcbInstanceGetsPerRead = 3

	lcbTimestamps = `"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"`
)

type lcbRecord struct {
	command    string // "services/plan?get"
	subaccount string
	id         string
	header     http.Header
	body       []byte
}

type lcbServer struct {
	*httptest.Server

	mu            sync.Mutex
	records       []lcbRecord
	hierarchy     int
	planName      string
	offeringName  string
	mode          map[string]string // command -> "404" | "bare500"
	beforeBare500 func()
	extraPlanHdr  http.Header
	planPadding   int
}

func lcbNewServer(t *testing.T) *lcbServer {
	t.Helper()
	s := &lcbServer{planName: "plan-a", offeringName: "offering-a", mode: map[string]string{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *lcbServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/login/"):
		w.Header().Set("X-Cpcli-Sessionid", behaviourSession)
		_, _ = w.Write([]byte(`{"mail":"u@example.test","issuer":"https://idp.example.test"}`))

	case strings.HasSuffix(r.URL.Path, "/globalAccountHierarchyForNodes"):
		s.mu.Lock()
		s.hierarchy++
		s.mu.Unlock()
		w.Header().Set("X-Cpcli-Backend-Status", "200")
		_, _ = w.Write([]byte(`{"guid":"` + behaviourGA + `"}`))

	case strings.Contains(r.URL.Path, "/command/"):
		s.command(w, r, body)

	default:
		http.NotFound(w, r)
	}
}

func (s *lcbServer) command(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		ParamValues map[string]any `json:"paramValues"`
	}
	_ = json.Unmarshal(body, &req)
	_, cmd, _ := strings.Cut(r.URL.Path, "/command/"+behaviourVersion+"/")
	command := cmd + "?" + r.URL.RawQuery
	sa, _ := req.ParamValues["subaccount"].(string)
	id, _ := req.ParamValues["id"].(string)

	s.mu.Lock()
	s.records = append(s.records, lcbRecord{command: command, subaccount: sa, id: id, header: r.Header.Clone(), body: body})
	mode := s.mode[command]
	hook := s.beforeBare500
	planName, offeringName := s.planName, s.offeringName
	extra := s.extraPlanHdr.Clone()
	padding := s.planPadding
	s.mu.Unlock()

	switch mode {
	case "bare500":
		if hook != nil {
			hook()
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	case "404":
		w.Header().Set("X-Cpcli-Backend-Status", "404")
		_, _ = w.Write([]byte(`{"error":"not found"}`))
		return
	}

	w.Header().Set("X-Cpcli-Backend-Status", "200")
	switch command {
	case lcbInstanceQuery:
		_, _ = w.Write([]byte(`{"id":"` + lcbInstanceID + `","name":"inst","ready":true,"subaccount_id":"` + sa +
			`","service_plan_id":"` + lcbPlanID + `","last_operation":{"state":"succeeded"},` + lcbTimestamps + `}`))
	case lcbPlanQuery:
		for k, v := range extra {
			w.Header()[k] = v
		}
		pad := ""
		if padding > 0 {
			pad = `,"description":"` + strings.Repeat("x", padding) + `"`
		}
		_, _ = w.Write([]byte(`{"id":"` + id + `","name":"` + planName + `","service_offering_id":"` + lcbOfferingID + `"` + pad + `,` + lcbTimestamps + `}`))
	case lcbOfferingQuery:
		_, _ = w.Write([]byte(`{"id":"` + id + `","name":"` + offeringName + `",` + lcbTimestamps + `}`))
	default:
		_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
	}
}

func (s *lcbServer) setPlanName(n string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planName = n
}

func (s *lcbServer) setMode(command, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mode == "" {
		delete(s.mode, command)
		return
	}
	s.mode[command] = mode
}

func (s *lcbServer) setBeforeBare500(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beforeBare500 = f
}

func (s *lcbServer) setPlanExtras(h http.Header, padding int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extraPlanHdr, s.planPadding = h, padding
}

func (s *lcbServer) count(command string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.records {
		if r.command == command {
			n++
		}
	}
	return n
}

func (s *lcbServer) countFor(command, subaccount, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.records {
		if r.command == command && r.subaccount == subaccount && r.id == id {
			n++
		}
	}
	return n
}

func (s *lcbServer) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records) + s.hierarchy
}

func (s *lcbServer) hierarchyCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hierarchy
}

// lcbExpect fails when the server saw a different number of a command.
func lcbExpect(t *testing.T, s *lcbServer, when string, want map[string]int) {
	t.Helper()
	for command, n := range want {
		if got := s.count(command); got != n {
			t.Errorf("%s: server saw %s %d times, want %d", when, command, got, n)
		}
	}
}

type lcbEnv struct {
	ctx     context.Context
	server  tfprotov6.ProviderServer
	schemas *tfprotov6.GetProviderSchemaResponse
}

func lcbProvider(t *testing.T, serverURL string, tr http.RoundTripper) *lcbEnv {
	t.Helper()
	ctx := context.Background()
	p := tfprovider.NewWithClient(&http.Client{Transport: tr})
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
		"cli_server_url": serverURL,
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
	return &lcbEnv{ctx: ctx, server: server, schemas: schemas}
}

// readResource reads a service instance whose state has neither plan name nor
// offering name, which makes upstream look up the plan and then the offering.
func (e *lcbEnv) readResource(subaccount string) (*tfprotov6.ReadResourceResponse, error) {
	rs, ok := e.schemas.ResourceSchemas[lcbResource]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	state, err := behaviourDynamicValue(map[string]any{
		"subaccount_id":  subaccount,
		"id":             lcbInstanceID,
		"name":           "n",
		"serviceplan_id": lcbPlanID,
	}, rs.ValueType())
	if err != nil {
		return nil, err
	}
	return e.server.ReadResource(e.ctx, &tfprotov6.ReadResourceRequest{TypeName: lcbResource, CurrentState: state})
}

func (e *lcbEnv) readPlanDataSource(config map[string]any) (*tfprotov6.ReadDataSourceResponse, error) {
	const name = "btp_subaccount_service_plan"
	ds, ok := e.schemas.DataSourceSchemas[name]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	cfg, err := behaviourDynamicValue(config, ds.ValueType())
	if err != nil {
		return nil, err
	}
	return e.server.ReadDataSource(e.ctx, &tfprotov6.ReadDataSourceRequest{TypeName: name, Config: cfg})
}

// names returns the plan and offering name of a resource state.
func (e *lcbEnv) names(t *testing.T, state *tfprotov6.DynamicValue) (plan, offering string) {
	t.Helper()
	if state == nil {
		t.Fatal("new state is nil")
	}
	v, err := state.Unmarshal(e.schemas.ResourceSchemas[lcbResource].ValueType())
	if err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	var attrs map[string]tftypes.Value
	if err := v.As(&attrs); err != nil {
		t.Fatalf("state attributes: %v", err)
	}
	if err := attrs["serviceplan_name"].As(&plan); err != nil {
		t.Fatalf("plan name: %v", err)
	}
	if err := attrs["service_offering_name"].As(&offering); err != nil {
		t.Fatalf("offering name: %v", err)
	}
	return plan, offering
}

// lcbRead reads and fails the test on an error diagnostic.
func lcbRead(t *testing.T, env *lcbEnv, subaccount string) (plan, offering string) {
	t.Helper()
	resp, err := env.readResource(subaccount)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	behaviourFailOnErrors(t, "read", resp.Diagnostics)
	return env.names(t, resp.NewState)
}

func lcbTransport(clock *behaviourClock, ttl time.Duration) *cliTransport {
	tr := ffbTransport(true, true)
	tr.lookups = newLookupCache(ttl, nil)
	if tr.lookups != nil {
		tr.lookups.now = clock.Now
	}
	return tr
}

func TestLookupCacheBehaviourObserveAsksPlanAndOfferingOnce(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	env := lcbProvider(t, srv.URL, lcbTransport(newBehaviourClock(), time.Hour))

	for i := 0; i < 3; i++ {
		plan, offering := lcbRead(t, env, lcbSubaccountA)
		if plan != "plan-a" || offering != "offering-a" {
			t.Errorf("read %d: names = %q/%q, want plan-a/offering-a", i, plan, offering)
		}
	}
	lcbExpect(t, srv, "three reads", map[string]int{lcbInstanceQuery: 3 * lcbInstanceGetsPerRead, lcbPlanQuery: 1, lcbOfferingQuery: 1})
}

func TestLookupCacheBehaviourSwitchedOff(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	tr := ffbTransport(true, true)
	tr.lookups = newLookupCache(0, nil)
	if tr.lookups != nil {
		t.Fatal("a zero duration must give no cache")
	}
	env := lcbProvider(t, srv.URL, tr)

	for i := 0; i < 3; i++ {
		lcbRead(t, env, lcbSubaccountA)
	}
	lcbExpect(t, srv, "three reads", map[string]int{lcbInstanceQuery: 3 * lcbInstanceGetsPerRead, lcbPlanQuery: 3, lcbOfferingQuery: 3})
}

func TestLookupCacheBehaviourExpires(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := lcbNewServer(t)
	env := lcbProvider(t, srv.URL, lcbTransport(clock, time.Hour))

	lcbRead(t, env, lcbSubaccountA)
	clock.Advance(59 * time.Minute)
	srv.setPlanName("plan-b")
	if plan, _ := lcbRead(t, env, lcbSubaccountA); plan != "plan-a" {
		t.Errorf("plan name inside the duration = %q, want the remembered plan-a", plan)
	}
	lcbExpect(t, srv, "inside the duration", map[string]int{lcbPlanQuery: 1, lcbOfferingQuery: 1})

	clock.Advance(2 * time.Minute)
	if plan, _ := lcbRead(t, env, lcbSubaccountA); plan != "plan-b" {
		t.Errorf("plan name after expiry = %q, want plan-b", plan)
	}
	lcbExpect(t, srv, "after expiry", map[string]int{lcbPlanQuery: 2, lcbOfferingQuery: 2})
}

func TestLookupCacheBehaviourPerSubaccount(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	env := lcbProvider(t, srv.URL, lcbTransport(newBehaviourClock(), time.Hour))

	for _, sa := range []string{lcbSubaccountA, lcbSubaccountB} {
		lcbRead(t, env, sa)
	}
	lcbExpect(t, srv, "first read of each", map[string]int{lcbPlanQuery: 2, lcbOfferingQuery: 2})
	for _, sa := range []string{lcbSubaccountA, lcbSubaccountB} {
		lcbRead(t, env, sa)
	}
	lcbExpect(t, srv, "second read of each", map[string]int{lcbPlanQuery: 2, lcbOfferingQuery: 2})
	for _, sa := range []string{lcbSubaccountA, lcbSubaccountB} {
		if n := srv.countFor(lcbPlanQuery, sa, lcbPlanID); n != 1 {
			t.Errorf("plan lookups for %s = %d, want 1", sa, n)
		}
	}
}

func TestLookupCacheBehaviourErrorsAreNotRemembered(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	srv.setMode(lcbPlanQuery, "404")
	env := lcbProvider(t, srv.URL, lcbTransport(newBehaviourClock(), time.Hour))

	resp, err := env.readResource(lcbSubaccountA)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(ffbErrors(resp.Diagnostics)) == 0 {
		t.Fatal("read with a 404 on the plan lookup reported no error")
	}

	srv.setMode(lcbPlanQuery, "")
	lcbRead(t, env, lcbSubaccountA)
	lcbExpect(t, srv, "after recovery", map[string]int{lcbPlanQuery: 2, lcbOfferingQuery: 1})
	lcbRead(t, env, lcbSubaccountA)
	lcbExpect(t, srv, "third read", map[string]int{lcbPlanQuery: 2, lcbOfferingQuery: 1})
}

func TestLookupCacheBehaviourBare500IsNotRemembered(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := lcbNewServer(t)
	tr := lcbTransport(clock, time.Hour)
	tr.hierarchy.now = clock.Now
	// The instance get just before marks the subaccount as served, and a bare 500
	// for a subaccount served recently is left to btpcli's retry chain. Time
	// passing on the server between the two commands models it forgetting.
	srv.setBeforeBare500(func() { clock.Advance(2 * time.Minute) })
	srv.setMode(lcbPlanQuery, "bare500")
	env := lcbProvider(t, srv.URL, tr)

	var (
		resp *tfprotov6.ReadResourceResponse
		err  error
		done = make(chan struct{})
	)
	go func() {
		defer close(done)
		resp, err = env.readResource(lcbSubaccountA)
	}()
	hierAwait(t, done, "read with a bare 500 on the plan lookup")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(ffbErrors(resp.Diagnostics)) == 0 {
		t.Fatal("read with a bare 500 on the plan lookup reported no error")
	}

	srv.setMode(lcbPlanQuery, "")
	plan, offering := lcbRead(t, env, lcbSubaccountA)
	if plan != "plan-a" || offering != "offering-a" {
		t.Errorf("names = %q/%q, want plan-a/offering-a", plan, offering)
	}
	before := srv.count(lcbPlanQuery)
	lcbRead(t, env, lcbSubaccountA)
	if got := srv.count(lcbPlanQuery); got != before {
		t.Errorf("plan lookups grew from %d to %d, want the recovered answer served from the cache", before, got)
	}
}

func TestLookupCacheBehaviourLookupByNameAlwaysAsks(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	env := lcbProvider(t, srv.URL, lcbTransport(newBehaviourClock(), time.Hour))

	byName := map[string]any{"subaccount_id": lcbSubaccountA, "name": "plan-a", "offering_name": "offering-a"}
	byID := map[string]any{"subaccount_id": lcbSubaccountA, "id": lcbPlanID}
	read := func(cfg map[string]any) {
		t.Helper()
		resp, err := env.readPlanDataSource(cfg)
		if err != nil {
			t.Fatalf("read data source: %v", err)
		}
		behaviourFailOnErrors(t, "read data source", resp.Diagnostics)
	}

	read(byName)
	read(byName)
	lcbExpect(t, srv, "two lookups by name", map[string]int{lcbPlanQuery: 2})
	read(byID)
	read(byID)
	lcbExpect(t, srv, "two lookups by id", map[string]int{lcbPlanQuery: 3})
}

func lcbLookup(t *testing.T, serverURL, command, session, subaccount, id string) *http.Request {
	t.Helper()
	req := behaviourPost(t, serverURL, "/command/"+behaviourVersion+"/"+command,
		`{"paramValues":{"subaccount":"`+subaccount+`","id":"`+id+`"}}`,
		map[string]string{
			"Content-Type":      "application/json",
			"X-Cpcli-Format":    "json",
			"X-Cpcli-Sessionid": session,
			"X-Cpcli-Subdomain": behaviourGA,
			"X-Cpcli-Customidp": "",
			"X-Correlationid":   behaviourCorrID,
		})
	return req
}

func lcbDo(t *testing.T, tr http.RoundTripper, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}

func TestLookupCacheBehaviourNoHierarchyCallOnHit(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := lcbNewServer(t)
	tr := lcbTransport(clock, time.Hour)
	tr.hierarchy.now = clock.Now

	lcbDo(t, tr, lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID))
	if h, p := srv.hierarchyCalls(), srv.count(lcbPlanQuery); h != 1 || p != 1 {
		t.Fatalf("first lookup: hierarchy calls=%d plan lookups=%d, want 1/1", h, p)
	}

	// Past the time a hierarchy call is trusted, inside the cache duration.
	clock.Advance(2 * time.Minute)
	seen := srv.total()
	resp, _ := lcbDo(t, tr, lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := srv.total(); got != seen {
		t.Errorf("server saw %d further requests for a cached lookup, want none", got-seen)
	}
}

func TestLookupCacheBehaviourListsAndOtherCommandsUntouched(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	tr := lcbTransport(newBehaviourClock(), time.Hour)

	for _, command := range []string{"services/plan?list", "services/offering?list", lcbInstanceQuery, "services/binding?get"} {
		for i := 0; i < 2; i++ {
			resp, _ := lcbDo(t, tr, lcbLookup(t, srv.URL, command, behaviourSession, lcbSubaccountA, lcbPlanID))
			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", command, resp.StatusCode)
			}
		}
		if got := srv.count(command); got != 2 {
			t.Errorf("server saw %s %d times, want 2", command, got)
		}
	}
	if n := tr.lookups.len(); n != 0 {
		t.Errorf("cache holds %d entries, want none", n)
	}
}

func TestLookupCacheBehaviourSessionsDoNotShare(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		vary func(t *testing.T, req *http.Request, v string)
	}{
		{"session", func(_ *testing.T, r *http.Request, v string) { r.Header.Set("X-Cpcli-Sessionid", v) }},
		{"subdomain", func(_ *testing.T, r *http.Request, v string) { r.Header.Set("X-Cpcli-Subdomain", v) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := lcbNewServer(t)
			tr := lcbTransport(newBehaviourClock(), time.Hour)
			lookup := func(v string) {
				req := lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID)
				tc.vary(t, req, v)
				lcbDo(t, tr, req)
			}

			lookup("value-one")
			lookup("value-two")
			if got := srv.count(lcbPlanQuery); got != 2 {
				t.Fatalf("plan lookups with two values = %d, want 2", got)
			}
			lookup("value-one")
			lookup("value-two")
			if got := srv.count(lcbPlanQuery); got != 2 {
				t.Errorf("plan lookups after repeating both = %d, want still 2", got)
			}
		})
	}
}

func TestLookupCacheBehaviourCallersShareNothing(t *testing.T) {
	t.Parallel()
	const callers = 16
	srv := lcbNewServer(t)
	tr := lcbTransport(newBehaviourClock(), time.Hour)

	_, want := lcbDo(t, tr, lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID))

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		mu    sync.Mutex
		resps = map[*http.Response]bool{}
	)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			corr := fmt.Sprintf("corr-%d", i)
			req := lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID)
			req.Header.Set("X-Correlationid", corr)
			<-start
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Errorf("caller %d: %v", i, err)
				return
			}
			defer resp.Body.Close() //nolint:errcheck
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("caller %d: read body: %v", i, err)
			}
			if string(body) != string(want) {
				t.Errorf("caller %d: body = %q, want %q", i, body, want)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("caller %d: status = %d, want 200", i, resp.StatusCode)
			}
			if got := resp.Header.Get("X-Correlationid"); got != corr {
				t.Errorf("caller %d: correlation id = %q, want its own %q", i, got, corr)
			}
			mu.Lock()
			resps[resp] = true
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if len(resps) != callers {
		t.Errorf("got %d distinct responses, want %d", len(resps), callers)
	}
	if got := srv.count(lcbPlanQuery); got != 1 {
		t.Errorf("server saw %d plan lookups, want 1", got)
	}
}

func TestLookupCacheBehaviourNothingSecretReplayed(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	srv.setPlanExtras(http.Header{
		"Set-Cookie":        {"sid=cookie-placeholder"},
		"X-Cpcli-Sessionid": {"other-session-placeholder"},
	}, 0)
	tr := lcbTransport(newBehaviourClock(), time.Hour)

	lcbDo(t, tr, lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID))
	resp, _ := lcbDo(t, tr, lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID))

	if got := srv.count(lcbPlanQuery); got != 1 {
		t.Fatalf("server saw %d plan lookups, want 1 (the second must be a hit)", got)
	}
	for _, k := range []string{"Set-Cookie", "X-Cpcli-Sessionid"} {
		if v, ok := resp.Header[k]; ok {
			t.Errorf("cached answer carries %s = %q, want none", k, v)
		}
	}
	if got := resp.Header.Get("X-Cpcli-Backend-Status"); got != "200" {
		t.Errorf("backend status = %q, want 200", got)
	}
}

func TestLookupCacheBehaviourLargeAnswer(t *testing.T) {
	t.Parallel()
	srv := lcbNewServer(t)
	srv.setPlanExtras(nil, lookupCacheMaxBodyBytes+1024)
	tr := lcbTransport(newBehaviourClock(), time.Hour)

	var bodies [2][]byte
	for i := range bodies {
		resp, body := lcbDo(t, tr, lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, lcbPlanID))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("lookup %d: status = %d, want 200", i, resp.StatusCode)
		}
		if len(body) <= lookupCacheMaxBodyBytes || !json.Valid(body) {
			t.Fatalf("lookup %d: body has %d bytes (valid JSON: %v), want the complete answer", i, len(body), json.Valid(body))
		}
		bodies[i] = body
	}
	if string(bodies[0]) != string(bodies[1]) {
		t.Error("the two answers differ")
	}
	if got := srv.count(lcbPlanQuery); got != 2 {
		t.Errorf("server saw %d plan lookups, want 2 (too large to keep)", got)
	}
	if n := tr.lookups.len(); n != 0 {
		t.Errorf("cache holds %d entries, want none", n)
	}
}

func TestLookupCacheBehaviourEntryLimit(t *testing.T) {
	t.Parallel()
	clock := newBehaviourClock()
	srv := lcbNewServer(t)
	tr := lcbTransport(clock, time.Hour)
	tr.lookups.maxEntries = 4

	id := func(i int) string { return fmt.Sprintf("plan-%d", i) }
	lookup := func(i int) {
		lcbDo(t, tr, lcbLookup(t, srv.URL, lcbPlanQuery, behaviourSession, lcbSubaccountA, id(i)))
		clock.Advance(time.Second)
	}
	asked := func(i int) int { return srv.countFor(lcbPlanQuery, lcbSubaccountA, id(i)) }

	for i := 1; i <= 6; i++ {
		lookup(i)
	}
	if n := tr.lookups.len(); n != 4 {
		t.Fatalf("cache holds %d entries, want 4", n)
	}
	// The newest are checked first: asking for an old id stores it again and
	// pushes out the then oldest.
	for i := 3; i <= 6; i++ {
		lookup(i)
		if got := asked(i); got != 1 {
			t.Errorf("id %d was asked %d times, want 1 (still cached)", i, got)
		}
	}
	for i := 1; i <= 2; i++ {
		lookup(i)
		if got := asked(i); got != 2 {
			t.Errorf("id %d was asked %d times, want 2 (pushed out)", i, got)
		}
	}
}
