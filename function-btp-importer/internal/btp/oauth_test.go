package btp

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNewOAuthClient(t *testing.T) {
	type args struct {
		handler http.HandlerFunc
	}
	type want struct {
		statusCode int
		err        error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ValidTokenEndpoint": {
			reason: "valid token endpoint → client injects Bearer token into subsequent requests",
			args: args{
				handler: func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/oauth/token" {
						writeJSON(t, w, map[string]any{
							"access_token": "test-token",
							"token_type":   "Bearer",
							"expires_in":   3600,
						})
						return
					}
					if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
						http.Error(w, "missing or wrong auth header", http.StatusUnauthorized)
						return
					}
					w.WriteHeader(http.StatusOK)
				},
			},
			want: want{statusCode: http.StatusOK},
		},
		"TokenEndpointReturns401": {
			reason: "token endpoint returns 401 → client request fails with oauth error",
			args: args{
				handler: func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				},
			},
			want: want{err: cmpopts.AnyError},
		},
		"TokenEndpointReturnsMalformedJSON": {
			reason: "token endpoint returns invalid JSON → client request fails",
			args: args{
				handler: func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{not json`))
				},
			},
			want: want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newTestServer(t, tc.args.handler)

			client := NewOAuthClient(context.Background(), srv.URL+"/oauth/token", "cid", "csecret")
			if client == nil {
				t.Fatal("NewOAuthClient() returned nil")
			}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/resource", nil)
			if err != nil {
				t.Fatalf("http.NewRequestWithContext: %v", err)
			}
			resp, err := client.Do(req)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nNewOAuthClient().Do(): -want err, +got err:\n%s", tc.reason, diff)
			}
			if tc.want.err != nil {
				return
			}
			if err != nil {
				t.Fatalf("%s\nNewOAuthClient().Do(): unexpected error: %v", tc.reason, err)
			}
			defer resp.Body.Close()
			if diff := cmp.Diff(tc.want.statusCode, resp.StatusCode); diff != "" {
				t.Errorf("%s\nNewOAuthClient().Do(): -want status, +got status:\n%s", tc.reason, diff)
			}
		})
	}
}
