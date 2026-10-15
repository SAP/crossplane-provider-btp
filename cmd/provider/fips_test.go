package main

import (
	"crypto/fips140"
	"strings"
	"testing"
)

// TestFIPS140Enabled verifies that the binary was built with the Go
// Cryptographic Module in approved mode (GOFIPS140=certified at build time).
//
// When this test runs in a normal dev build (GOFIPS140 not set),
// fips140.Enabled() is false and the test is skipped — no failure.
//
// In a FIPS-mode build (GOFIPS140=certified), both assertions must pass:
//   - fips140.Enabled() == true   → approved mode is active
//   - fips140.Version() != ""     → module version is reported (e.g. "v1.0.0")
//
// The version string is captured as a CI artifact for evidence that
// CMVP-validated module #5247 is linked. See https://go.dev/doc/security/fips140
func TestFIPS140Enabled(t *testing.T) {
	if !fips140.Enabled() {
		t.Skip("binary not built with GOFIPS140=certified; skipping FIPS assertion (set GOFIPS140=certified at build time to enable)")
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
