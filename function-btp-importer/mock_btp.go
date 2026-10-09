package main

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

const mockBTPPort = 18888

// startMockBTPServer reads the config file at path and starts an in-process
// mock BTP server on mockBTPPort. Panics on startup errors (port in use, bad
// config) since this is a dev-only tool.
func startMockBTPServer(path string) {
	cfg, err := btptest.Load(path)
	if err != nil {
		panic(fmt.Sprintf("mock-btp: cannot load config %q: %v", path, err))
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", mockBTPPort)) //nolint:noctx // dev-only tool; no context required
	if err != nil {
		panic(fmt.Sprintf("mock-btp: cannot listen on port %d: %v", mockBTPPort, err))
	}
	go func() {
		_ = http.Serve(ln, btptest.Handler(cfg)) //nolint:gosec // G114: dev-only server; timeouts not required
	}()
	log.Printf("mock-btp: listening on http://127.0.0.1:%d (config: %s)", mockBTPPort, path)
}
