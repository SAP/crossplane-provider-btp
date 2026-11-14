package btptest

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseNameFromFieldQuery(t *testing.T) {
	type args struct {
		fieldQuery string
	}
	type want struct {
		name string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Valid": {
			reason: "a well-formed fieldQuery yields the quoted name",
			args:   args{fieldQuery: "name eq 'my-instance'"},
			want:   want{name: "my-instance"},
		},
		"Empty": {
			reason: "an empty fieldQuery yields no name",
			args:   args{fieldQuery: ""},
			want:   want{name: ""},
		},
		"NoQuotes": {
			reason: "an unquoted value is not a valid fieldQuery and yields no name",
			args:   args{fieldQuery: "name eq my-instance"},
			want:   want{name: ""},
		},
		"WithSpaces": {
			reason: "surrounding whitespace is trimmed before parsing",
			args:   args{fieldQuery: "  name eq 'trimmed'  "},
			want:   want{name: "trimmed"},
		},
		"EmptyValue": {
			reason: "a lone opening quote must not be read as a value (this input used to panic the parser)",
			args:   args{fieldQuery: "name eq '"},
			want:   want{name: ""},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := parseNameFromFieldQuery(tc.args.fieldQuery)
			if diff := cmp.Diff(tc.want.name, got); diff != "" {
				t.Errorf("%s\nparseNameFromFieldQuery(%q): -want, +got:\n%s", tc.reason, tc.args.fieldQuery, diff)
			}
		})
	}
}

// TestSMAdminBinding checks the binding handler's own contract. Against the
// mock, production only ever GETs (the mock always reports an existing
// binding), so POST and DELETE are exercised here directly.
func TestSMAdminBinding(t *testing.T) {
	type args struct {
		method string
	}
	type want struct {
		status int
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GETReturnsBinding": {
			reason: "GET returns the binding with 200",
			args:   args{method: http.MethodGet},
			want:   want{status: http.StatusOK},
		},
		"POSTCreatesBinding": {
			reason: "POST creates the binding with 201",
			args:   args{method: http.MethodPost},
			want:   want{status: http.StatusCreated},
		},
		"DELETERemovesBinding": {
			reason: "DELETE removes the binding with 204",
			args:   args{method: http.MethodDelete},
			want:   want{status: http.StatusNoContent},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := Serve(t, Default())

			req, err := http.NewRequestWithContext(context.Background(), tc.args.method, srv.URL+"/accounts/v1/subaccounts/test-sub/serviceManagementBinding", nil)
			if err != nil {
				t.Fatalf("cannot build request: %v", err)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if diff := cmp.Diff(tc.want.status, resp.StatusCode); diff != "" {
				t.Errorf("%s\n%s serviceManagementBinding: -want status, +got:\n%s", tc.reason, tc.args.method, diff)
			}
		})
	}
}

// TestLoad exercises loading a Config from a real file. It stays a focused
// test: the file format has one shape to pin, and the parser is yaml.v3's.
func TestLoad(t *testing.T) {
	path := writeConfig(t, `
serviceInstances:
  - name: svc-a
    id: id-a
serviceBindings:
  - name: bind-b
    id: id-b
subaccounts:
  - guid: guid-c
    subdomain: sub-c
    region: eu10
environments:
  - id: env-d
    name: kyma-d
    environmentType: kyma
`)
	want := Config{
		ServiceInstances: []SMResource{{Name: "svc-a", ID: "id-a"}},
		ServiceBindings:  []SMResource{{Name: "bind-b", ID: "id-b"}},
		Subaccounts:      []Subaccount{{GUID: "guid-c", Subdomain: "sub-c", Region: "eu10"}},
		Environments:     []Environment{{ID: "env-d", Name: "kyma-d", EnvironmentType: "kyma"}},
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Load(): -want, +got:\n%s", diff)
	}
}

// TestWriteConfigPath ensures the test helper returns an absolute path.
func TestWriteConfigPath(t *testing.T) {
	path := writeConfig(t, "serviceInstances: []\n")
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
}

func writeConfig(t *testing.T, cfg string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mock-btp-*.yaml")
	if err != nil {
		t.Fatalf("cannot create temp file: %v", err)
	}
	if _, err := f.WriteString(cfg); err != nil {
		t.Fatalf("cannot write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("cannot close temp file: %v", err)
	}
	return f.Name()
}
