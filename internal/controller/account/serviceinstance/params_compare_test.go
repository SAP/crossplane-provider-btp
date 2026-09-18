package serviceinstance

import (
	"encoding/json"
	"testing"
)

func mustMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("bad test json %q: %v", s, err)
	}
	return m
}

func TestParamsSubsetMatch(t *testing.T) {
	cases := []struct {
		name    string
		desired string
		server  string
		want    bool
	}{
		{
			name:    "exact match",
			desired: `{"ingest_otlp":{"enabled":true}}`,
			server:  `{"ingest_otlp":{"enabled":true}}`,
			want:    true,
		},
		{
			name:    "server has extra defaults (must NOT be drift)",
			desired: `{"ingest_otlp":{"enabled":true}}`,
			server:  `{"ingest_otlp":{"enabled":true},"retention_period":0,"saml":{"enabled":true},"rotate_root_ca":true}`,
			want:    true,
		},
		{
			name:    "key order irrelevant",
			desired: `{"backend":{"api_enabled":true,"max_data_nodes":2},"ingest_otlp":{"enabled":true}}`,
			server:  `{"ingest_otlp":{"enabled":true},"backend":{"max_data_nodes":2,"api_enabled":true}}`,
			want:    true,
		},
		{
			name:    "desired adds nested field the server lacks (IS drift)",
			desired: `{"ingest_otlp":{"enabled":true,"span_passthrough":true}}`,
			server:  `{"ingest_otlp":{"enabled":true}}`,
			want:    false,
		},
		{
			name:    "desired value differs from server (IS drift)",
			desired: `{"ingest_otlp":{"enabled":true}}`,
			server:  `{"ingest_otlp":{"enabled":false}}`,
			want:    false,
		},
		{
			name:    "desired top-level key missing on server (IS drift)",
			desired: `{"backend":{"api_enabled":true}}`,
			server:  `{"ingest_otlp":{"enabled":true}}`,
			want:    false,
		},
		{
			name:    "empty desired always matches",
			desired: `{}`,
			server:  `{"ingest_otlp":{"enabled":true}}`,
			want:    true,
		},
		{
			name:    "number type tolerance (int vs float)",
			desired: `{"backend":{"max_data_nodes":2}}`,
			server:  `{"backend":{"max_data_nodes":2.0}}`,
			want:    true,
		},
		{
			name:    "array equality required",
			desired: `{"feature_flags":["a","b"]}`,
			server:  `{"feature_flags":["a","b"]}`,
			want:    true,
		},
		{
			name:    "array order matters (IS drift)",
			desired: `{"feature_flags":["a","b"]}`,
			server:  `{"feature_flags":["b","a"]}`,
			want:    false,
		},
		{
			// Empirically-motivated: object-store-style instance where the
			// broker materializes many defaults the spec never set. Only the
			// one desired key is asserted; the defaults are ignored, so no
			// phantom drift.
			name:    "object-store defaults ignored, single desired key present",
			desired: `{"backupEnabled":true}`,
			server:  `{"backupEnabled":true,"backupRetentionPeriod":20,"versioning":true,"preventDeletion":false,"autoExpiration":[{"id":"x"}]}`,
			want:    true,
		},
		{
			// DELETE is intentionally NOT detected (see paramsSubsetMatch doc):
			// a key removed from desired but still present on the server is not
			// drift, because it cannot be cleared via the update API.
			name:    "key removed from desired still on server -> NOT drift (by design)",
			desired: `{"ingest_otlp":{"enabled":true}}`,
			server:  `{"ingest_otlp":{"enabled":true,"span_passthrough":true}}`,
			want:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := paramsSubsetMatch(mustMap(t, tc.desired), mustMap(t, tc.server))
			if got != tc.want {
				t.Errorf("paramsSubsetMatch(%s, %s) = %v, want %v", tc.desired, tc.server, got, tc.want)
			}
		})
	}
}
