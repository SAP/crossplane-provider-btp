// Command zz-inject-identity patches upjet-generated zz_controller.go files
// to wrap o.WorkspaceStore with tfclient.NewIdentityInjectingStore at the
// tjcontroller.NewConnector call site. Workaround for issue #521.
//
// Why this exists: upjet's controller template hardcodes
//
//	tjcontroller.NewConnector(mgr.GetClient(), o.WorkspaceStore, ...)
//
// and there's no public template-override hook in pipeline.Run. We can't
// change the type of o.WorkspaceStore (it's also used by NewWorkspaceFinalizer
// which needs the concrete *terraform.WorkspaceStore). So instead, after the
// upjet code generator runs, this tool rewrites only the NewConnector argument
// per generated file to wrap it in our identity-injecting Store.
//
// Hooked in via a //go:generate directive in apis/generate.go so it runs
// every time `make generate` (i.e. `go generate ./apis/...`) regenerates the
// zz_*.go files, right after the upjet generator and before controller-gen.
//
// Idempotent: re-running on already-patched files is a no-op.
//
// Removal: when no-fork (PR #680 / issue #207) lands, delete this command,
// the apis/generate.go directive, and the tfclient.NewIdentityInjectingStore
// wrapper.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

const (
	tfclientImportLine = `	tfclient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"`
	wrapPrefix         = `tfclient.NewIdentityInjectingStore(o.WorkspaceStore, o.Logger)`
)

// matchNewConnector finds the exact `o.WorkspaceStore` arg position inside
// the upjet-generated `tjcontroller.NewConnector(...)` call. The template is
// single-line, so we anchor on the call name + `mgr.GetClient(), ` and
// capture up to the next `,`.
var matchNewConnector = regexp.MustCompile(`(tjcontroller\.NewConnector\(mgr\.GetClient\(\), )(o\.WorkspaceStore)(,)`)

func main() {
	root := flag.String("root", "internal/controller", "directory to scan for zz_controller.go files")
	flag.Parse()

	var patched, skipped int
	err := filepath.WalkDir(*root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) != "zz_controller.go" {
			return nil
		}
		changed, perr := patchFile(path)
		if perr != nil {
			return fmt.Errorf("%s: %w", path, perr)
		}
		if changed {
			patched++
			fmt.Printf("patched %s\n", path)
		} else {
			skipped++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("identity-injector patcher: %d patched, %d already-patched\n", patched, skipped)
}

// patchFile rewrites a single zz_controller.go. Returns (changed, err).
func patchFile(path string) (bool, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is filtered by WalkDir
	if err != nil {
		return false, err
	}
	if bytes.Contains(raw, []byte(wrapPrefix)) {
		// Already patched.
		return false, nil
	}
	loc := matchNewConnector.FindSubmatchIndex(raw)
	if loc == nil {
		// File doesn't contain the upjet NewConnector call (could be a future
		// shape change or a non-generated stray). Don't error — let the build
		// catch unexpected shapes; report skipped.
		return false, nil
	}
	// loc[2:4] = prefix span, loc[4:6] = "o.WorkspaceStore" span,
	// loc[6:8] = trailing "," span. Replace the argument span only.
	var buf bytes.Buffer
	buf.Write(raw[:loc[4]])
	buf.WriteString(wrapPrefix)
	buf.Write(raw[loc[5]:])
	out := buf.Bytes()

	out, err = ensureImport(out)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, out, mode); err != nil {
		return false, err
	}
	return true, nil
}

// ensureImport adds the tfclient import inside the existing `import ( ... )`
// block if it isn't already present. Returns the (possibly unchanged) bytes.
func ensureImport(raw []byte) ([]byte, error) {
	if bytes.Contains(raw, []byte(`"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"`)) {
		return raw, nil
	}
	// Insert before the closing `)` of the import block. Anchor on the
	// upjet-generated tjcontroller import line — it's stable across all
	// generated files.
	anchor := []byte(`tjcontroller "github.com/crossplane/upjet/pkg/controller"`)
	idx := bytes.Index(raw, anchor)
	if idx == -1 {
		return nil, fmt.Errorf("tjcontroller import anchor not found")
	}
	// Insert the new import line directly after the anchor line.
	eol := bytes.IndexByte(raw[idx:], '\n')
	if eol == -1 {
		return nil, fmt.Errorf("unterminated tjcontroller import line")
	}
	insertAt := idx + eol + 1
	var buf bytes.Buffer
	buf.Write(raw[:insertAt])
	buf.WriteString(tfclientImportLine)
	buf.WriteByte('\n')
	buf.Write(raw[insertAt:])
	return buf.Bytes(), nil
}
