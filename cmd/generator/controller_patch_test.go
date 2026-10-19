package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchSubaccountApiCredentialController(t *testing.T) {
	path := filepath.Join(t.TempDir(), "internal/controller/security/subaccountapicredential/zz_controller.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(generatedCredentialConnector), 0o644); err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSuffix(path, "/internal/controller/security/subaccountapicredential/zz_controller.go")
	if err := patchSubaccountApiCredentialController(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != adoptionGuardedCredentialConnector {
		t.Fatalf("generated controller was not patched as expected:\n%s", got)
	}
	if err := patchSubaccountApiCredentialController(root); err != nil {
		t.Fatalf("patch must be idempotent: %v", err)
	}
}

func TestPatchSubaccountApiCredentialControllerRejectsUnknownTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "internal/controller/security/subaccountapicredential/zz_controller.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("unexpected"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSuffix(path, "/internal/controller/security/subaccountapicredential/zz_controller.go")
	if err := patchSubaccountApiCredentialController(root); err == nil {
		t.Fatal("expected unexpected generated input to fail")
	}
}
