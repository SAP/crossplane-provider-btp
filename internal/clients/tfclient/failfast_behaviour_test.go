package tfclient

// Behavioural tests for fail-fast: a fake CLI server keeps refusing one subaccount
// with a bare 500 while serving another, and the tests check what the real
// terraform-provider-btp and the server see.

import (
	"context"
	"encoding/json"
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
	ffbFailing = "11111111-2222-4333-8444-555555555555"
	ffbHealthy = "66666666-7777-4888-8999-aaaaaaaaaaaa"

	// ffbFlaky refuses its first flakyGets instance reads after a create was served.
	ffbFlaky = "44444444-5555-4666-8777-888888888888"

	ffbInstanceID = "22222222-2222-4333-8444-555555555555"
	ffbCausePart  = "has not loaded subaccount "
)

type ffbRecord struct {
	path   string
	header http.Header
	body   []byte
}

type ffbServer struct {
	*httptest.Server
	// All are set before the server is used.
	loginBare500   bool
	commandBare500 bool
	flakyGets      int

	mu           sync.Mutex
	logins       []ffbRecord
	hierarchy    map[string]int
	commands     map[string]int
	gets         map[string]int
	commandsSeen map[string][]http.Header
}

func ffbNewServer(t *testing.T, configure func(*ffbServer)) *ffbServer {
	t.Helper()
	s := &ffbServer{
		hierarchy:    map[string]int{},
		commands:     map[string]int{},
		gets:         map[string]int{},
		commandsSeen: map[string][]http.Header{},
	}
	if configure != nil {
		configure(s)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *ffbServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/login/"):
		s.mu.Lock()
		s.logins = append(s.logins, ffbRecord{path: r.URL.Path, header: r.Header.Clone(), body: body})
		bare := s.loginBare500
		s.mu.Unlock()
		if bare {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Cpcli-Sessionid", behaviourSession)
		_, _ = w.Write([]byte(`{"mail":"u@example.test","issuer":"https://idp.example.test"}`))

	case strings.HasPrefix(r.URL.Path, "/client/") && strings.HasSuffix(r.URL.Path, "/globalAccountHierarchyForNodes"):
		var req struct {
			Nodes []struct {
				EntityGUID string `json:"entityGuid"`
			} `json:"nodes"`
		}
		_ = json.Unmarshal(body, &req)
		s.mu.Lock()
		for _, n := range req.Nodes {
			s.hierarchy[n.EntityGUID]++
		}
		s.mu.Unlock()
		w.Header().Set("X-Cpcli-Backend-Status", "200")
		_, _ = w.Write([]byte(`{"guid":"` + behaviourGA + `"}`))

	case strings.Contains(r.URL.Path, "/command/"):
		var req struct {
			ParamValues struct {
				Subaccount string `json:"subaccount"`
			} `json:"paramValues"`
		}
		_ = json.Unmarshal(body, &req)
		sa := req.ParamValues.Subaccount
		get := strings.HasSuffix(r.URL.Path, "/services/instance") && r.URL.RawQuery == "get"
		s.mu.Lock()
		s.commands[sa]++
		s.commandsSeen[sa] = append(s.commandsSeen[sa], r.Header.Clone())
		if get {
			s.gets[sa]++
		}
		bare := sa == ffbFailing || (sa == "" && s.commandBare500) || (sa == ffbFlaky && get && s.gets[sa] <= s.flakyGets)
		s.mu.Unlock()
		if bare {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		backend := "200"
		if r.URL.RawQuery == "create" {
			// Makes upstream read the instance right after the create.
			backend = "202"
		}
		w.Header().Set("X-Cpcli-Backend-Status", backend)
		_, _ = w.Write([]byte(`{"id":"` + ffbInstanceID + `","name":"inst","ready":true,"subaccount_id":"` + sa +
			`","last_operation":{"state":"succeeded"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`))

	default:
		http.NotFound(w, r)
	}
}

func (s *ffbServer) hierarchyCalls(sa string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hierarchy[sa]
}

func (s *ffbServer) totalHierarchyCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.hierarchy {
		n += c
	}
	return n
}

func (s *ffbServer) commandCalls(sa string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commands[sa]
}

func (s *ffbServer) instanceGets(sa string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gets[sa]
}

func (s *ffbServer) commandHeaders(sa string) []http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]http.Header(nil), s.commandsSeen[sa]...)
}

func (s *ffbServer) loginRecords() []ffbRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ffbRecord(nil), s.logins...)
}

func ffbTransport(hierarchyCall, failFast bool) *cliTransport {
	return newCLITransport(&cachingProvider{entries: map[string]*cacheEntry{}}, http.DefaultTransport, nil, hierarchyCall, failFast)
}

type ffbEnv struct {
	ctx     context.Context
	srv     *ffbServer
	probe   *behaviourProbe
	server  tfprotov6.ProviderServer
	schemas *tfprotov6.GetProviderSchemaResponse
}

const ffbResource = "btp_subaccount_service_instance"

// ffbProvider does all the setup work up front so that timings around a read
// measure the read only.
func ffbProvider(t *testing.T, srv *ffbServer, tr http.RoundTripper) *ffbEnv {
	t.Helper()
	ctx := context.Background()
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
	return &ffbEnv{ctx: ctx, srv: srv, probe: probe, server: server, schemas: schemas}
}

func (e *ffbEnv) readResource(subaccount string) (*tfprotov6.ReadResourceResponse, error) {
	rs, ok := e.schemas.ResourceSchemas[ffbResource]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	state, err := behaviourDynamicValue(map[string]any{
		"subaccount_id":  subaccount,
		"id":             ffbInstanceID,
		"name":           "n",
		"serviceplan_id": "p",
	}, rs.ValueType())
	if err != nil {
		return nil, err
	}
	return e.server.ReadResource(e.ctx, &tfprotov6.ReadResourceRequest{TypeName: ffbResource, CurrentState: state})
}

func (e *ffbEnv) readDataSource(subaccount string) (*tfprotov6.ReadDataSourceResponse, error) {
	ds, ok := e.schemas.DataSourceSchemas[ffbResource]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	cfg, err := behaviourDynamicValue(map[string]any{"subaccount_id": subaccount, "id": ffbInstanceID}, ds.ValueType())
	if err != nil {
		return nil, err
	}
	return e.server.ReadDataSource(e.ctx, &tfprotov6.ReadDataSourceRequest{TypeName: ffbResource, Config: cfg})
}

// applyResourceChange creates when prior is nil and deletes when planned is nil.
func (e *ffbEnv) applyResourceChange(t *testing.T, prior, planned map[string]any) *tfprotov6.ApplyResourceChangeResponse {
	t.Helper()
	typ := e.schemas.ResourceSchemas[ffbResource].ValueType()
	value := func(data map[string]any) *tfprotov6.DynamicValue {
		t.Helper()
		if data == nil {
			dv, err := tfprotov6.NewDynamicValue(typ, tftypes.NewValue(typ, nil))
			if err != nil {
				t.Fatal(err)
			}
			return &dv
		}
		dv, err := behaviourDynamicValue(data, typ)
		if err != nil {
			t.Fatal(err)
		}
		return dv
	}
	plannedState := value(planned)
	resp, err := e.server.ApplyResourceChange(e.ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName:     ffbResource,
		PriorState:   value(prior),
		PlannedState: plannedState,
		Config:       plannedState,
	})
	if err != nil {
		t.Fatalf("apply resource change: %v", err)
	}
	return resp
}

// stateID returns the id in a resource state, or "" when the state is null.
func (e *ffbEnv) stateID(t *testing.T, state *tfprotov6.DynamicValue) string {
	t.Helper()
	if state == nil {
		return ""
	}
	v, err := state.Unmarshal(e.schemas.ResourceSchemas[ffbResource].ValueType())
	if err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if v.IsNull() {
		return ""
	}
	var attrs map[string]tftypes.Value
	if err := v.As(&attrs); err != nil {
		t.Fatalf("state attributes: %v", err)
	}
	var id string
	if err := attrs["id"].As(&id); err != nil {
		t.Fatalf("state id: %v", err)
	}
	return id
}

func ffbErrors(diags []*tfprotov6.Diagnostic) []*tfprotov6.Diagnostic {
	var out []*tfprotov6.Diagnostic
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			out = append(out, d)
		}
	}
	return out
}

// ffbCause checks that the read failed with exactly the fail-fast cause and
// returns the correlation id the text names.
func ffbCause(t *testing.T, diags []*tfprotov6.Diagnostic) string {
	t.Helper()
	errs := ffbErrors(diags)
	if len(errs) != 1 {
		t.Fatalf("got %d error diagnostics, want 1: %v", len(errs), diags)
	}
	detail := errs[0].Detail
	if !strings.Contains(detail, ffbCausePart+ffbFailing) {
		t.Errorf("detail = %q, want the not-loaded cause for %s", detail, ffbFailing)
	}
	for _, bad := range []string{"unexpected status", "couldn't find resource"} {
		if strings.Contains(detail, bad) {
			t.Errorf("detail = %q, must not contain %q", detail, bad)
		}
	}
	_, after, ok := strings.Cut(detail, "Correlation ID: ")
	if !ok {
		t.Fatalf("detail = %q, want a correlation id", detail)
	}
	corr, _, _ := strings.Cut(after, "]")
	return corr
}

func TestFailFastBehaviourRealProviderFailsFast(t *testing.T) {
	t.Parallel()
	srv := ffbNewServer(t, nil)
	env := ffbProvider(t, srv, ffbTransport(true, true))

	start := time.Now()
	resp, err := env.readResource(ffbFailing)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Logf("failing read took %v; commands=%d hierarchy calls=%d", elapsed, srv.commandCalls(ffbFailing), srv.hierarchyCalls(ffbFailing))

	corr := ffbCause(t, resp.Diagnostics)
	seen := srv.commandHeaders(ffbFailing)
	if len(seen) != 1 {
		t.Fatalf("server saw %d commands for the failing subaccount, want 1", len(seen))
	}
	if got := seen[0].Get("X-Correlationid"); corr == "" || got != corr {
		t.Errorf("correlation id in detail = %q, header the server saw = %q", corr, got)
	}
	// The single command is what proves no retry chain ran; timings vary too much.
	if n := srv.hierarchyCalls(ffbFailing); n != 1 {
		t.Errorf("hierarchy calls = %d, want 1", n)
	}

	// A removed state would make crossplane create the resource again.
	if resp.NewState == nil {
		t.Fatal("new state is nil")
	}
	v, err := resp.NewState.Unmarshal(env.schemas.ResourceSchemas[ffbResource].ValueType())
	if err != nil {
		t.Fatalf("unmarshal new state: %v", err)
	}
	if v.IsNull() {
		t.Error("state was removed, want it kept")
	}

	env.probe.mu.Lock()
	defer env.probe.mu.Unlock()
	if len(env.probe.statuses) == 0 {
		t.Fatal("probe saw no responses")
	}
	for _, s := range env.probe.statuses {
		if s == http.StatusInternalServerError {
			t.Errorf("provider side saw a 500; statuses = %v", env.probe.statuses)
		}
	}
	if last := env.probe.statuses[len(env.probe.statuses)-1]; last != http.StatusOK {
		t.Errorf("status the provider saw for the failing command = %d, want 200", last)
	}
}

func TestFailFastBehaviourHealthySubaccountIsServedAtOnce(t *testing.T) {
	t.Parallel()
	srv := ffbNewServer(t, nil)
	env := ffbProvider(t, srv, ffbTransport(true, true))

	resp, err := env.readResource(ffbFailing)
	if err != nil {
		t.Fatalf("failing read: %v", err)
	}
	ffbCause(t, resp.Diagnostics)

	start := time.Now()
	dresp, err := env.readDataSource(ffbHealthy)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("healthy read: %v", err)
	}
	t.Logf("healthy read after a failing one took %v", elapsed)
	if errs := ffbErrors(dresp.Diagnostics); len(errs) != 0 {
		t.Fatalf("healthy read failed: %v", errs)
	}

	var (
		failDiags    []*tfprotov6.Diagnostic
		healthyDiags []*tfprotov6.Diagnostic
		failErr      error
		healthyErr   error
		failDone     = make(chan struct{})
		healthyDone  = make(chan struct{})
	)
	go func() {
		defer close(failDone)
		r, err := env.readResource(ffbFailing)
		failErr = err
		if r != nil {
			failDiags = r.Diagnostics
		}
	}()
	go func() {
		defer close(healthyDone)
		r, err := env.readDataSource(ffbHealthy)
		healthyErr = err
		if r != nil {
			healthyDiags = r.Diagnostics
		}
	}()
	hierAwait(t, failDone, "failing read")
	hierAwait(t, healthyDone, "healthy read")

	if failErr != nil || healthyErr != nil {
		t.Fatalf("read errors: failing=%v healthy=%v", failErr, healthyErr)
	}
	if errs := ffbErrors(healthyDiags); len(errs) != 0 {
		t.Errorf("concurrent healthy read failed: %v", errs)
	}
	ffbCause(t, failDiags)
	if n := srv.commandCalls(ffbFailing); n != 2 {
		t.Errorf("commands for the failing subaccount = %d, want 1 per failing read (2)", n)
	}
}

func TestFailFastBehaviourSwitchedOff(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		hierarchyCall bool
		failFast      bool
		wantHierarchy int
		replaced      bool
	}{
		{"fail-fast off", true, false, 1, false},
		{"hierarchy call off", false, true, 0, false},
		{"both on", true, true, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := ffbNewServer(t, nil)
			tr := ffbTransport(tc.hierarchyCall, tc.failFast)
			req := behaviourPost(t, srv.URL, "/command/"+behaviourVersion+"/services/instance?get",
				`{"paramValues":{"id":"`+ffbInstanceID+`","parameters":"false","subaccount":"`+ffbFailing+`"}}`,
				map[string]string{
					"Content-Type":      "application/json",
					"X-Cpcli-Format":    "json",
					"X-Cpcli-Sessionid": behaviourSession,
					"X-Cpcli-Subdomain": behaviourGA,
					"X-Cpcli-Customidp": "",
					"X-Correlationid":   behaviourCorrID,
				})
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatalf("round trip: %v", err)
			}
			defer resp.Body.Close() //nolint:errcheck
			body, _ := io.ReadAll(resp.Body)

			if got := srv.totalHierarchyCalls(); got != tc.wantHierarchy {
				t.Errorf("hierarchy calls = %d, want %d", got, tc.wantHierarchy)
			}
			if got := srv.commandCalls(ffbFailing); got != 1 {
				t.Errorf("commands = %d, want 1", got)
			}
			if tc.replaced {
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("status = %d, want 200", resp.StatusCode)
				}
				if got := resp.Header.Get("X-Cpcli-Backend-Status"); got != "500" {
					t.Errorf("backend status = %q, want 500 (and never 404)", got)
				}
				var parsed struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(body, &parsed); err != nil {
					t.Fatalf("body %q: %v", body, err)
				}
				want := "cli server has not loaded subaccount " + ffbFailing +
					": it answered HTTP 500 without a backend status; not retried in-process [Correlation ID: " + behaviourCorrID + "]"
				if parsed.Error != want {
					t.Errorf("error = %q, want %q", parsed.Error, want)
				}
				return
			}
			if resp.StatusCode != http.StatusInternalServerError {
				t.Errorf("status = %d, want the bare 500", resp.StatusCode)
			}
			if len(body) != 0 {
				t.Errorf("body = %q, want empty", body)
			}
			for k := range resp.Header {
				if strings.HasPrefix(http.CanonicalHeaderKey(k), "X-Cpcli-") {
					t.Errorf("response header %s present, want none", k)
				}
			}
		})
	}
}

func TestFailFastBehaviourLoginUntouched(t *testing.T) {
	t.Parallel()
	loginHeader := map[string]string{"Content-Type": "application/json", "X-Cpcli-Format": "json"}
	const loginBody = `{"userName":"technical-user","password":"pw"}`
	path := "/login/" + behaviourVersion

	t.Run("normal login", func(t *testing.T) {
		t.Parallel()
		srv := ffbNewServer(t, nil)
		resp, err := ffbTransport(true, true).RoundTrip(behaviourPost(t, srv.URL, path, loginBody, loginHeader))
		if err != nil {
			t.Fatalf("round trip: %v", err)
		}
		defer resp.Body.Close() //nolint:errcheck
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		if want := `{"mail":"u@example.test","issuer":"https://idp.example.test"}`; string(body) != want {
			t.Errorf("body = %s, want %s", body, want)
		}
		if got := resp.Header.Get("X-Cpcli-Sessionid"); got != behaviourSession {
			t.Errorf("session header = %q, want %q", got, behaviourSession)
		}
		recs := srv.loginRecords()
		if len(recs) != 1 {
			t.Fatalf("server saw %d logins, want 1", len(recs))
		}
		if string(recs[0].body) != loginBody {
			t.Errorf("login body = %s, want it unchanged", recs[0].body)
		}
		for k, v := range loginHeader {
			if got := recs[0].header.Get(k); got != v {
				t.Errorf("login header %s = %q, want %q", k, got, v)
			}
		}
		if n := srv.totalHierarchyCalls(); n != 0 {
			t.Errorf("hierarchy calls = %d, want 0", n)
		}
	})

	t.Run("bare 500 login", func(t *testing.T) {
		t.Parallel()
		srv := ffbNewServer(t, func(s *ffbServer) { s.loginBare500 = true })
		resp, err := ffbTransport(true, true).RoundTrip(behaviourPost(t, srv.URL, path, loginBody, loginHeader))
		if err != nil {
			t.Fatalf("round trip: %v", err)
		}
		defer resp.Body.Close() //nolint:errcheck
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("status = %d, want the bare 500", resp.StatusCode)
		}
		if len(body) != 0 {
			t.Errorf("body = %q, want empty", body)
		}
		if _, ok := resp.Header["X-Cpcli-Backend-Status"]; ok {
			t.Error("a backend status header was added to the login answer")
		}
		if n := len(srv.loginRecords()); n != 1 {
			t.Errorf("logins = %d, want 1", n)
		}
		if n := srv.totalHierarchyCalls(); n != 0 {
			t.Errorf("hierarchy calls = %d, want 0", n)
		}
	})
}

func TestFailFastBehaviourOtherCommandsUntouched(t *testing.T) {
	t.Parallel()
	srv := ffbNewServer(t, func(s *ffbServer) { s.commandBare500 = true })
	req := behaviourPost(t, srv.URL, "/command/"+behaviourVersion+"/accounts/global-account?get",
		`{"paramValues":{"globalAccount":"`+behaviourGA+`"}}`,
		map[string]string{
			"Content-Type":      "application/json",
			"X-Cpcli-Format":    "json",
			"X-Cpcli-Sessionid": behaviourSession,
			"X-Cpcli-Subdomain": behaviourGA,
			"X-Cpcli-Customidp": "",
			"X-Correlationid":   behaviourCorrID,
		})
	resp, err := ffbTransport(true, true).RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want the bare 500 handed up", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
	if _, ok := resp.Header["X-Cpcli-Backend-Status"]; ok {
		t.Error("a backend status header was added")
	}
	if n := srv.totalHierarchyCalls(); n != 0 {
		t.Errorf("hierarchy calls = %d, want 0", n)
	}
	if n := srv.commandCalls(""); n != 1 {
		t.Errorf("commands = %d, want 1", n)
	}
}

// Upstream deletes with one command and polls only after it succeeded, so a
// never-served subaccount fails the delete at once and keeps the state.
func TestFailFastBehaviourDeleteFailsFast(t *testing.T) {
	t.Parallel()
	srv := ffbNewServer(t, nil)
	env := ffbProvider(t, srv, ffbTransport(true, true))

	resp := env.applyResourceChange(t, map[string]any{
		"subaccount_id":  ffbFailing,
		"id":             ffbInstanceID,
		"name":           "n",
		"serviceplan_id": "p",
	}, nil)

	ffbCause(t, resp.Diagnostics)
	if n := srv.commandCalls(ffbFailing); n != 1 {
		t.Errorf("commands = %d, want 1 (no retry chain)", n)
	}
	if n := srv.hierarchyCalls(ffbFailing); n != 1 {
		t.Errorf("hierarchy calls = %d, want 1", n)
	}
	if id := env.stateID(t, resp.NewState); id != ffbInstanceID {
		t.Errorf("state id after the failed delete = %q, want %q kept", id, ffbInstanceID)
	}
}

// A create whose follow-up read hits a bare 500 must not fail fast: the instance
// exists in the backend, and a create reported as failed leaves no state to find
// it by. btpcli's retry has to carry the read through.
func TestFailFastBehaviourCreateReadAfterServedWriteIsRetried(t *testing.T) {
	t.Parallel()
	// Two refusals outlast cliTransport's own resend.
	srv := ffbNewServer(t, func(s *ffbServer) { s.flakyGets = 2 })
	env := ffbProvider(t, srv, ffbTransport(true, true))

	resp := env.applyResourceChange(t, nil, map[string]any{
		"subaccount_id":  ffbFlaky,
		"name":           "n",
		"serviceplan_id": "p",
		// Upstream waits timeout/100 before polling; keep that at zero.
		"timeouts": map[string]any{"create": "10s"},
	})

	behaviourFailOnErrors(t, "create", resp.Diagnostics)
	if id := env.stateID(t, resp.NewState); id != ffbInstanceID {
		t.Errorf("state id after create = %q, want %q", id, ffbInstanceID)
	}
	if n := srv.instanceGets(ffbFlaky); n < 3 {
		t.Errorf("instance reads = %d, want at least 3 (refused, resent, retried by btpcli)", n)
	}

	env.probe.mu.Lock()
	defer env.probe.mu.Unlock()
	saw500 := false
	for _, s := range env.probe.statuses {
		saw500 = saw500 || s == http.StatusInternalServerError
	}
	if !saw500 {
		t.Errorf("btpcli never saw the bare 500, so it was replaced; statuses = %v", env.probe.statuses)
	}
}
