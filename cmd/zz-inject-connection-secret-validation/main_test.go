package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPatchFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zz_controller.go")
	input := `package example

func Setup(mgr manager) {
	opts := []option{
		managed.WithExternalConnecter(tjcontroller.NewConnector(mgr.GetClient(), o.WorkspaceStore, o.SetupFn, resource, tjcontroller.WithLogger(o.Logger))),
	}
}
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := patchFile(path)
	if err != nil {
		t.Fatalf("patchFile() error = %v", err)
	}
	if !changed {
		t.Fatal("patchFile() changed = false, want true")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `package example

func Setup(mgr manager) {
	opts := []option{
		managed.WithExternalConnecter(NewConnectionSecretValidatingConnector(
			tjcontroller.NewConnector(mgr.GetClient(), o.WorkspaceStore, o.SetupFn, resource, tjcontroller.WithLogger(o.Logger)),
			mgr.GetClient(),
		)),
	}
}
`
	if string(got) != want {
		t.Fatalf("patched source mismatch (-got +want):\n%s", string(got))
	}

	changed, err = patchFile(path)
	if err != nil {
		t.Fatalf("second patchFile() error = %v", err)
	}
	if changed {
		t.Fatal("second patchFile() changed = true, want false")
	}
}

func TestMatchingParen(t *testing.T) {
	raw := []byte("foo(a, bar(c), d)")
	close, err := matchingParen(raw, 3)
	if err != nil {
		t.Fatalf("matchingParen() error = %v", err)
	}
	if want := len(raw) - 1; close != want {
		t.Fatalf("matchingParen() = %d, want %d", close, want)
	}
}
