package btp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

// The tables in this package run against the shared btptest fixture. Rows say
// what differs from btptest.Default() through a setup mutator; these are the
// shorthands they use.

// down closes the mock before the call, so every request fails at the network
// level.
func down(c btptest.Config) btptest.Config {
	c.Down = true
	return c
}

// withFaults answers matching requests with the given faults instead of the
// mock's normal routing.
func withFaults(f ...btptest.Fault) func(btptest.Config) btptest.Config {
	return func(c btptest.Config) btptest.Config {
		c.Faults = append(c.Faults, f...)
		return c
	}
}

// equateErrors is cmpopts.EquateErrors plus one rule: two non-nil errors also
// match when their messages are equal, so a row can pin the exact
// operator-facing text of an error the code constructs on the spot. As with
// cmpopts, nil only matches nil and cmpopts.AnyError matches any non-nil
// error.
func equateErrors() cmp.Option {
	return cmp.FilterValues(func(x, y any) bool {
		_, ok1 := x.(error)
		_, ok2 := y.(error)
		return ok1 && ok2
	}, cmp.Comparer(func(x, y any) bool {
		xe, ye := x.(error), y.(error)
		if errors.Is(xe, cmpopts.AnyError) || errors.Is(ye, cmpopts.AnyError) {
			return true
		}
		return errors.Is(xe, ye) || errors.Is(ye, xe) || xe.Error() == ye.Error()
	}))
}

// writeJSON and newTestServer serve the two tests that stay on hand-rolled
// handlers because they assert something the data fixture cannot express: the
// outgoing Authorization header (TestNewOAuthClient) and the DELETE issued by
// Close (TestNewSMClientCloseDeletesBinding).

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("writeJSON: %v", err)
	}
}

func newTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}
