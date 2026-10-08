package main

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestPinnedOverlaysCompileAndPreserveCancellation(t *testing.T) {
	out := t.TempDir()
	args := os.Args
	os.Args = []string{"diagnostic-overlay", out}
	t.Cleanup(func() { os.Args = args })
	if err := run(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"client.go", "external_async_tfpluginfw.go"} {
		data, err := os.ReadFile(out + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), name, data, parser.AllErrors); err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if name == "client.go" {
			if !strings.Contains(s, "v2.session.Lock()") || !strings.Contains(s, "v2.session.Unlock()") || !strings.Contains(s, "diagnostics.BeginRequest(ctx)") {
				t.Fatal("lock or request counter hook missing")
			}
		} else if strings.Count(s, "diagnostics.Copy(context.Background(), parentCtx)") != 3 {
			t.Fatal("async operations lost trace propagation")
		}
	}
}

func TestHooksRejectUnexpectedDependencySources(t *testing.T) {
	if _, err := patchCLI("package unexpected"); err == nil {
		t.Fatal("accepted incompatible client")
	}
	if _, err := patchAsync("package unexpected"); err == nil {
		t.Fatal("accepted incompatible async client")
	}
}
