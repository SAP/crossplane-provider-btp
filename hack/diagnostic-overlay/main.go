// Command diagnostic-overlay generates narrowly scoped timing hooks for the pinned dependencies.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: diagnostic-overlay OUTPUT_DIRECTORY")
	}
	out, err := filepath.Abs(os.Args[1])
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	replace := map[string]string{}
	patches := []struct{ module, version, path, hash string }{
		{"github.com/SAP/terraform-provider-btp", "v1.27.0", "internal/btpcli/client.go", "a6281dbf390137798528927f76e86e7ae5d2f180b3f7a004d6fefaab0484263e"},
		{"github.com/crossplane/upjet/v2", "v2.5.1", "pkg/controller/external_async_tfpluginfw.go", "e7e6550fc138136d5d9015e7a2234bf0f29620ab5afe75ab942eac2b679ee440"},
	}
	for _, patch := range patches {
		cmd := exec.Command("go", "mod", "download", "-json", patch.module+"@"+patch.version)
		data, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("resolve diagnostic dependency: %w", err)
		}
		var info struct{ Dir string }
		if err := json.Unmarshal(data, &info); err != nil {
			return err
		}
		original := filepath.Join(info.Dir, patch.path)
		data, err = os.ReadFile(original)
		if err != nil {
			return err
		}
		if patch.hash != "" && fmt.Sprintf("%x", sha256.Sum256(data)) != patch.hash {
			return fmt.Errorf("dependency source changed: %s", original)
		}
		source := string(data)
		if strings.Contains(patch.path, "btpcli") {
			source, err = patchCLI(source)
		} else {
			source, err = patchAsync(source)
		}
		if err != nil {
			return err
		}
		target := filepath.Join(out, filepath.Base(patch.path))
		formatted, err := format.Source([]byte(source))
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, formatted, 0644); err != nil {
			return err
		}
		vendored, err := filepath.Abs(filepath.Join("vendor", patch.module, patch.path))
		if err != nil {
			return err
		}
		replace[vendored] = target
	}
	data, err := json.MarshalIndent(struct{ Replace map[string]string }{replace}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "overlay.json"), data, 0644)
}

func change(source, old, new string, count int) (string, error) {
	if strings.Count(source, old) != count {
		return "", fmt.Errorf("diagnostic hook mismatch: %q", old)
	}
	return strings.ReplaceAll(source, old, new), nil
}

func patchCLI(source string) (string, error) {
	changes := [][2]string{
		{"\"github.com/hashicorp/go-retryablehttp\"", "\"github.com/hashicorp/go-retryablehttp\"\n\"github.com/sap/crossplane-provider-btp/pkg/diagnostics\""},
		{"req, err := http.NewRequestWithContext(ctx, method", "ctx = diagnostics.BeginRequest(ctx)\nrequestStart := time.Now()\nreq, err := http.NewRequestWithContext(ctx, method"},
		{"if v2.session != nil {\n\t\tv2.session.Lock()\n\t\tdefer v2.session.Unlock()", `diagnostics.Record(ctx, "btp request queued", nil, "path", req.URL.Path, "action", req.URL.RawQuery, "correlationID", ctx.Value(v2ContextKey(HeaderCorrelationID)))
lockStart := time.Now()
var acquired time.Time
if v2.session != nil {
 v2.session.Lock()
 acquired = time.Now()
 diagnostics.Record(ctx, "btp session lock acquired", nil, "waitMs", acquired.Sub(lockStart).Milliseconds(), "correlationID", ctx.Value(v2ContextKey(HeaderCorrelationID)))
 defer func() {
  held := time.Since(acquired)
  v2.session.Unlock()
  diagnostics.Record(ctx, "btp session lock released", ctx.Err(), "holdMs", held.Milliseconds(), "correlationID", ctx.Value(v2ContextKey(HeaderCorrelationID)))
 }()`},
		{"res, err := v2.httpClient.Do(req)\n\n\treturn res, err", `diagnostics.Record(ctx, "btp request started", nil, "action", req.URL.RawQuery, "path", req.URL.Path, "correlationID", req.Header.Get(HeaderCorrelationID))
res, err := v2.httpClient.Do(req)
diagnostics.Record(ctx, "btp request finished", err, "durationMs", time.Since(requestStart).Milliseconds(), "action", req.URL.RawQuery, "path", req.URL.Path, "correlationID", req.Header.Get(HeaderCorrelationID))
return res, err`},
	}
	var err error
	for _, c := range changes {
		source, err = change(source, c[0], c[1], 1)
		if err != nil {
			return "", err
		}
	}
	return source, nil
}

func patchAsync(source string) (string, error) {
	var err error
	source, err = change(source, "import (", "import (\n\"github.com/sap/crossplane-provider-btp/pkg/diagnostics\"", 1)
	if err != nil {
		return "", err
	}
	source, err = change(source, "(_ context.Context, mg xpresource.Managed)", "(parentCtx context.Context, mg xpresource.Managed)", 3)
	if err != nil {
		return "", err
	}
	return change(source, "context.WithDeadline(context.Background(), n.opTracker.LastOperation.StartTime().Add(defaultAsyncTimeout))", "context.WithDeadline(diagnostics.Copy(context.Background(), parentCtx), n.opTracker.LastOperation.StartTime().Add(defaultAsyncTimeout))", 3)
}
