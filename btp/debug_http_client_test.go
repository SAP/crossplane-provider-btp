package btp

import (
	"net/http"
	"reflect"
	"testing"
)

func TestRedactSensitiveHeaders(t *testing.T) {
	redacted := []string{"<REDACTED>"}
	tests := []struct {
		name   string
		header http.Header
		want   http.Header
	}{
		{
			name: "credentials are redacted",
			header: http.Header{
				"Authorization":     {"Bearer token-placeholder"},
				"X-Cpcli-Sessionid": {"session-placeholder"},
				"X-Id-Token":        {"id-token-placeholder"},
			},
			want: http.Header{
				"Authorization":     redacted,
				"X-Cpcli-Sessionid": redacted,
				"X-Id-Token":        redacted,
			},
		},
		{
			name: "non-canonical keys are redacted",
			header: http.Header{
				"x-cpcli-sessionid": {"session-placeholder"},
				"X-ID-TOKEN":        {"id-token-placeholder"},
				"authorization":     {"Bearer token-placeholder"},
			},
			want: http.Header{
				"x-cpcli-sessionid": redacted,
				"X-ID-TOKEN":        redacted,
				"authorization":     redacted,
			},
		},
		{
			name: "other headers keep their values",
			header: http.Header{
				"X-Cpcli-Subdomain":      {"subdomain-placeholder"},
				"X-Correlationid":        {"correlation-placeholder"},
				"X-Cpcli-Backend-Status": {"200"},
				"Content-Type":           {"application/json"},
				"X-Cpcli-Sessionid":      {"session-placeholder"},
			},
			want: http.Header{
				"X-Cpcli-Subdomain":      {"subdomain-placeholder"},
				"X-Correlationid":        {"correlation-placeholder"},
				"X-Cpcli-Backend-Status": {"200"},
				"Content-Type":           {"application/json"},
				"X-Cpcli-Sessionid":      redacted,
			},
		},
		{
			name:   "several values yield one placeholder",
			header: http.Header{"X-Cpcli-Sessionid": {"session-placeholder-1", "session-placeholder-2"}},
			want:   http.Header{"X-Cpcli-Sessionid": redacted},
		},
		{
			name:   "nil header",
			header: nil,
			want:   http.Header{},
		},
		{
			name:   "empty header",
			header: http.Header{},
			want:   http.Header{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.header.Clone()
			got := redactSensitiveHeaders(tt.header)
			if got == nil {
				t.Fatal("got nil header, want non-nil")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("redactSensitiveHeaders() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.header, before) {
				t.Errorf("input header changed to %v, want %v", tt.header, before)
			}
		})
	}
}
