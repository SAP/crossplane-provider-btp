package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestExtendPreservesStatusSubresourceMarker(t *testing.T) {
	src := []byte(`package v1alpha1

type SubaccountApiCredentialStatus struct {
	AtProvider Observation
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type SubaccountApiCredential struct {
	Status SubaccountApiCredentialStatus
}
`)
	got, err := extend(src, "generated.go")
	if err != nil {
		t.Fatal(err)
	}
	marker := strings.Index(string(got), "// +kubebuilder:subresource:status")
	resource := strings.Index(string(got), "type SubaccountApiCredential struct")
	lifecycle := strings.Index(string(got), "Lifecycle")
	if marker < 0 || resource < 0 || lifecycle < 0 || marker > resource || lifecycle > marker {
		t.Fatalf("generated marker/field order is invalid:\n%s", got)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "generated.go", got, parser.ParseComments)
	if err != nil {
		t.Fatalf("generated Go does not parse: %v", err)
	}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "SubaccountApiCredential" {
				continue
			}
			attached := false
			for _, comment := range gen.Doc.List {
				attached = attached || bytes.Contains([]byte(comment.Text), []byte("kubebuilder:subresource:status"))
			}
			if !attached {
				t.Fatal("status-subresource marker is no longer attached to the resource declaration")
			}
			return
		}
	}
	t.Fatal("resource type declaration not found")
}
