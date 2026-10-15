package main

import (
	"crypto/fips140"
	"strings"
	"testing"
)

// TestFIPS140Enabled verifies that the binary was built with the Go
// Cryptographic Module in approved mode (GOFIPS140=certified at build time).
//
// This test MUST pass for every production build. It fails if the binary was
// built without GOFIPS140=certified, which means the CMVP-validated module
// (#5247) is not linked and FIPS 140-3 approved mode is not active.
//
// To run locally without FIPS: GOFIPS140=certified go test ./cmd/provider/...
// See https://go.dev/doc/security/fips140
func TestFIPS140Enabled(t *testing.T) {
	if !fips140.Enabled() {
		t.Fatal("FIPS 140-3 mode is NOT active: binary must be built with GOFIPS140=certified (go build env var). " +
			"The Go Cryptographic Module CMVP #5247 is not linked in approved mode.")
	}

	ver := fips140.Version()
	if ver == "" {
		t.Error("fips140.Version() returned empty string; expected a module version (e.g. \"v1.0.0\")")
	}

	// "certified" currently resolves to v1.0.0 (CMVP #5247). Accept any "v1."
	// prefix so the test stays green when v1.26.0 certifies and the flag rolls
	// forward. If the version is something completely unexpected, surface it.
	if !strings.HasPrefix(ver, "v1.") {
		t.Errorf("fips140.Version() = %q; expected a v1.x.y module version", ver)
	}

	t.Logf("FIPS 140-3 module active: enabled=%v version=%s (CMVP #5247)", fips140.Enabled(), ver)
}
