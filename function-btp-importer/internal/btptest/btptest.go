// Package btptest is the in-process mock of the BTP APIs this function talks
// to (UAA token endpoint, Service Manager, Accounts Service, Provisioning
// Service). It is the shared test fixture for every HTTP-seam test in the
// repository and the server behind the --mock-btp flag used by crossplane
// render.
//
// The mock is data-driven: a Config describes the resources it serves and the
// faults it injects; Serve starts a fresh server per test case. Nothing in
// this package asserts — assertions belong to the tests.
package btptest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// smBindingPathRE matches the SM admin binding path for any subaccount ID.
var smBindingPathRE = regexp.MustCompile(`^/accounts/v1/subaccounts/[^/]+/serviceManagementBinding$`)

// Config holds the fake BTP API responses. The YAML-tagged fields are what
// the --mock-btp file carries; the untagged fields are test-only knobs.
type Config struct {
	ServiceInstances []SMResource      `yaml:"serviceInstances"`
	ServiceBindings  []SMResource      `yaml:"serviceBindings"`
	ServicePlans     []CatalogResource `yaml:"servicePlans"`
	ServiceOfferings []CatalogResource `yaml:"serviceOfferings"`
	Subaccounts      []Subaccount      `yaml:"subaccounts"`
	Environments     []Environment     `yaml:"environments"`

	// Advertise sets the service URLs the mock hands out in binding
	// responses. Empty fields mean the mock's own address, so clients that
	// follow them land back on the mock — what render needs.
	Advertise Advertised `yaml:"-"`

	// Faults are matched in order against every request before normal
	// routing; the first match answers the request.
	Faults []Fault `yaml:"-"`

	// Down makes Serve close the server immediately, so every request fails
	// at the network level.
	Down bool `yaml:"-"`
}

// SMResource is one SM service instance or binding list entry. The identity
// fields mirror what real SM list responses carry: instances link to their
// plan, bindings to their instance.
type SMResource struct {
	Name              string `yaml:"name"`
	ID                string `yaml:"id"`
	ServicePlanID     string `yaml:"servicePlanId"`     // instances
	ServiceInstanceID string `yaml:"serviceInstanceId"` // bindings
}

// CatalogResource is one service plan or offering detail entry. CatalogName
// is what the broker registered the entry under; empty means it equals Name,
// as real SM reports when a broker has not renamed a catalog entry.
type CatalogResource struct {
	ID                string `yaml:"id"`
	Name              string `yaml:"name"`
	CatalogName       string `yaml:"catalogName"`
	ServiceOfferingID string `yaml:"serviceOfferingId"` // plans only
}

// Subaccount is one entry in the Accounts Service subaccounts list.
type Subaccount struct {
	GUID      string `yaml:"guid"`
	Subdomain string `yaml:"subdomain"`
	Region    string `yaml:"region"`
}

// Environment is one entry in the Provisioning Service environments list.
type Environment struct {
	ID              string `yaml:"id"`
	Name            string `yaml:"name"`
	EnvironmentType string `yaml:"environmentType"`
	PlanName        string `yaml:"planName"`
}

// Advertised are the URLs served in binding responses: SM and UAA in the SM
// admin binding, UAA plus the Accounts and Provisioning endpoints in the
// cloud-management binding credentials.
type Advertised struct {
	SM           string
	UAA          string
	Accounts     string
	Provisioning string
}

// resolve fills empty fields with the address the request arrived on.
func (a Advertised) resolve(r *http.Request) Advertised {
	self := "http://" + r.Host
	if a.SM == "" {
		a.SM = self
	}
	if a.UAA == "" {
		a.UAA = self
	}
	if a.Accounts == "" {
		a.Accounts = self
	}
	if a.Provisioning == "" {
		a.Provisioning = self
	}
	return a
}

// Fault overrides the response for matching requests. Method and Path are
// matched exactly and by prefix respectively; empty means any. Status zero
// means 200; Body is written verbatim so malformed payloads are expressible.
type Fault struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (f Fault) matches(r *http.Request) bool {
	if f.Method != "" && f.Method != r.Method {
		return false
	}
	return strings.HasPrefix(r.URL.Path, f.Path)
}

// Default is the canonical small dataset, mirroring example/auto/mock.yaml:
// one instance on one plan and offering, one binding on that instance, one
// subaccount, one Kyma and one CloudFoundry environment.
func Default() Config {
	return Config{
		ServiceInstances: []SMResource{{Name: "my-instance", ID: "si-uuid-1", ServicePlanID: "plan-uuid-1"}},
		ServiceBindings:  []SMResource{{Name: "my-binding", ID: "sb-uuid-1", ServiceInstanceID: "si-uuid-1"}},
		ServicePlans:     []CatalogResource{{ID: "plan-uuid-1", Name: "hana", ServiceOfferingID: "off-uuid-1"}},
		ServiceOfferings: []CatalogResource{{ID: "off-uuid-1", Name: "hana-cloud"}},
		Subaccounts:      []Subaccount{{GUID: "sa-uuid-1", Subdomain: "my-sub", Region: "eu10"}},
		Environments: []Environment{
			{ID: "kyma-uuid-1", Name: "my-kyma", EnvironmentType: "kyma", PlanName: "aws"},
			{ID: "cf-uuid-1", Name: "my-cf", EnvironmentType: "cloudfoundry"},
		},
	}
}

// Load reads a Config from a YAML file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from CLI flag, not user input
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Serve starts a fresh mock server for cfg, closed via t.Cleanup. When
// cfg.Down is set the server is closed before returning.
func Serve(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(Handler(cfg))
	t.Cleanup(srv.Close)
	if cfg.Down {
		srv.Close()
	}
	return srv
}

// Handler serves all mock BTP endpoints for cfg on one mux:
//   - POST /oauth/token            → fake Bearer token
//   - GET  /v1/service_instances   → SM instance lookup by fieldQuery
//   - GET  /v1/service_bindings    → SM binding lookup by fieldQuery
//   - GET  /v1/service_plans/{id}, /v1/service_offerings/{id} → catalog detail
//   - GET  /v1/service_bindings/{id} → binding detail with CIS-shaped credentials
//   - GET/POST/DELETE /accounts/v1/subaccounts/{id}/serviceManagementBinding
//   - GET  /accounts/v1/subaccounts
//   - GET  /provisioning/v1/environments
func Handler(cfg Config) http.Handler {
	mux := http.NewServeMux()

	// OAuth token — serves any path ending in /oauth/token.
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"access_token": "mock-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})

	// SM: service instances.
	mux.HandleFunc("/v1/service_instances", func(w http.ResponseWriter, r *http.Request) {
		serveSMList(w, r, cfg.ServiceInstances)
	})

	// SM: service bindings list.
	mux.HandleFunc("/v1/service_bindings", func(w http.ResponseWriter, r *http.Request) {
		serveSMList(w, r, cfg.ServiceBindings)
	})

	// SM: service plan and offering detail (for GetServicePlanIdentity).
	mux.HandleFunc("/v1/service_plans/", func(w http.ResponseWriter, r *http.Request) {
		serveCatalogDetail(w, r, "/v1/service_plans/", cfg.ServicePlans)
	})
	mux.HandleFunc("/v1/service_offerings/", func(w http.ResponseWriter, r *http.Request) {
		serveCatalogDetail(w, r, "/v1/service_offerings/", cfg.ServiceOfferings)
	})

	// SM: service binding detail (for GetBindingCredentials).
	mux.HandleFunc("/v1/service_bindings/", func(w http.ResponseWriter, r *http.Request) {
		adv := cfg.Advertise.resolve(r)
		writeJSON(w, map[string]any{
			"id": strings.TrimPrefix(r.URL.Path, "/v1/service_bindings/"),
			"credentials": map[string]any{
				"uaa": map[string]any{
					"clientid":     "mock-cm-cid",
					"clientsecret": "mock-cm-csecret",
					"url":          adv.UAA,
					"subaccountid": "7b3f9a2e-4d1c-4f8a-b5e6-2c9d8f0a1b3e",
				},
				"endpoints": map[string]any{
					"accounts_service_url":     adv.Accounts,
					"provisioning_service_url": adv.Provisioning,
				},
			},
		})
	})

	// Accounts: SM admin binding GET/POST/DELETE.
	mux.HandleFunc("/accounts/v1/subaccounts/", func(w http.ResponseWriter, r *http.Request) {
		if !smBindingPathRE.MatchString(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		adv := cfg.Advertise.resolve(r)
		switch r.Method {
		case http.MethodGet, http.MethodPost:
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
			}
			writeJSON(w, map[string]any{
				"clientid":     "mock-sm-cid",
				"clientsecret": "mock-sm-csecret",
				"sm_url":       adv.SM,
				"url":          adv.UAA,
				"xsappname":    "mock-sm-app",
			})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Accounts: subaccounts list.
	mux.HandleFunc("/accounts/v1/subaccounts", func(w http.ResponseWriter, _ *http.Request) {
		items := make([]map[string]any, 0, len(cfg.Subaccounts))
		for _, sa := range cfg.Subaccounts {
			items = append(items, map[string]any{
				"guid":      sa.GUID,
				"subdomain": sa.Subdomain,
				"region":    sa.Region,
			})
		}
		writeJSON(w, map[string]any{"value": items})
	})

	// Provisioning: environments list.
	mux.HandleFunc("/provisioning/v1/environments", func(w http.ResponseWriter, _ *http.Request) {
		items := make([]map[string]any, 0, len(cfg.Environments))
		for _, e := range cfg.Environments {
			items = append(items, map[string]any{
				"id":              e.ID,
				"name":            e.Name,
				"environmentType": e.EnvironmentType,
				"planName":        e.PlanName,
			})
		}
		writeJSON(w, map[string]any{"environmentInstances": items})
	})

	if len(cfg.Faults) == 0 {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, f := range cfg.Faults {
			if f.matches(r) {
				status := f.Status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(f.Body))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

// serveSMList handles SM list endpoints. It filters by the fieldQuery parameter
// (format: `name eq '<n>'`) and returns matching items as an SM list response.
func serveSMList(w http.ResponseWriter, r *http.Request, resources []SMResource) {
	fq := r.URL.Query().Get("fieldQuery")
	name := parseNameFromFieldQuery(fq)

	// Use make([]T, 0) rather than var — json.Marshal encodes nil slices as null
	// but the SM API always returns [] for empty results.
	items := make([]map[string]any, 0)
	for _, res := range resources {
		if name == "" || res.Name == name {
			item := map[string]any{"id": res.ID}
			if res.ServicePlanID != "" {
				item["service_plan_id"] = res.ServicePlanID
			}
			if res.ServiceInstanceID != "" {
				item["service_instance_id"] = res.ServiceInstanceID
			}
			items = append(items, item)
		}
	}
	writeJSON(w, map[string]any{"items": items, "num_items": len(items)})
}

// serveCatalogDetail handles SM plan/offering detail endpoints; an unknown ID
// is a 404, like real SM.
func serveCatalogDetail(w http.ResponseWriter, r *http.Request, prefix string, entries []CatalogResource) {
	id := strings.TrimPrefix(r.URL.Path, prefix)
	for _, e := range entries {
		if e.ID == id {
			catalogName := e.CatalogName
			if catalogName == "" {
				catalogName = e.Name
			}
			item := map[string]any{"id": e.ID, "name": e.Name, "catalog_name": catalogName}
			if e.ServiceOfferingID != "" {
				item["service_offering_id"] = e.ServiceOfferingID
			}
			writeJSON(w, item)
			return
		}
	}
	http.NotFound(w, r)
}

// parseNameFromFieldQuery extracts the name value from a fieldQuery of the
// form `name eq '<n>'`. Returns empty string if the format doesn't match.
func parseNameFromFieldQuery(fq string) string {
	// fieldQuery=name eq 'my-instance'
	fq = strings.TrimSpace(fq)
	prefix := "name eq '"
	if !strings.HasPrefix(fq, prefix) || !strings.HasSuffix(fq, "'") {
		return ""
	}
	// A bare `name eq '` satisfies both prefix and suffix checks with the SAME
	// quote character; slicing would panic with fq[9:8].
	if len(fq) <= len(prefix) {
		return ""
	}
	return fq[len(prefix) : len(fq)-1]
}

// writeJSON writes v as a JSON response with Content-Type application/json.
// The body is buffered before writing headers so that encode errors produce a
// correct 500 rather than a partial 200 response.
func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
