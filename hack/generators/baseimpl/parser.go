/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Parser handles parsing of Go source files to extract type information.
type Parser struct {
	basePkg string
}

// NewParser creates a new Parser for the given base package path.
func NewParser(basePkg string) *Parser {
	return &Parser{basePkg: basePkg}
}

// ParseBaseTypes parses the base package and extracts type definitions.
func (p *Parser) ParseBaseTypes() ([]BaseType, error) {
	fset := token.NewFileSet()
	// packages.Load is used instead of the deprecated parser.ParseDir (deprecated since Go 1.25).
	// It correctly handles build tags and uses Dir to resolve the package from a filesystem path.
	// NeedSyntax populates pkg.Syntax ([]*ast.File) and NeedFiles ensures filenames are available.
	// ParseFile is overridden to retain comments in the AST, which are required for codegen markers.
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles,
		Dir:  p.basePkg,
		Fset: fset,
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.ParseComments)
		},
	}
	pkgList, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to parse base package %s: %w", p.basePkg, err)
	}

	var baseTypes []BaseType
	references := make(map[string][]Reference)
	customMethods := make(map[string][]CustomMethod)

	for _, pkg := range pkgList {
		for _, file := range pkg.Syntax {
			p.collectMetadata(file, references, customMethods)
			baseTypes = append(baseTypes, p.collectBaseTypes(file, references, customMethods)...)
		}
	}

	return baseTypes, nil
}

// collectMetadata extracts reference configurations and custom methods from interfaces.
func (p *Parser) collectMetadata(file *ast.File, references map[string][]Reference, customMethods map[string][]CustomMethod) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			p.extractReferences(genDecl, typeSpec, references)
			p.extractCustomMethods(typeSpec, customMethods)
		}
	}
}

// extractReferences extracts reference configurations from a type with +codegen:references marker.
func (p *Parser) extractReferences(genDecl *ast.GenDecl, typeSpec *ast.TypeSpec, references map[string][]Reference) {
	if genDecl.Doc == nil {
		return
	}

	for _, comment := range genDecl.Doc.List {
		if strings.Contains(comment.Text, MarkerReferences) {
			refs := p.parseReferencesStruct(typeSpec)
			// Extract resource name from type name (e.g., "DeploymentReferences" -> "Deployment")
			resourceName := strings.TrimSuffix(typeSpec.Name.Name, "References")
			references[resourceName] = refs
		}
	}
}

// extractCustomMethods extracts custom methods from Managed<Resource> interfaces.
func (p *Parser) extractCustomMethods(typeSpec *ast.TypeSpec, customMethods map[string][]CustomMethod) {
	// Check for Managed<Resource> interface pattern
	if !strings.HasPrefix(typeSpec.Name.Name, PrefixManaged) || strings.HasSuffix(typeSpec.Name.Name, SuffixList) {
		return
	}

	resourceName := strings.TrimPrefix(typeSpec.Name.Name, PrefixManaged)
	methods := p.parseInterfaceMethods(typeSpec)
	if len(methods) > 0 {
		customMethods[resourceName] = methods
	}
}

// collectBaseTypes extracts base types with +codegen:generate:scoped marker.
func (p *Parser) collectBaseTypes(file *ast.File, references map[string][]Reference, customMethods map[string][]CustomMethod) []BaseType {
	var baseTypes []BaseType

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if !p.hasGenerateScopedMarker(genDecl) {
				continue
			}

			bt := NewBaseType(typeSpec.Name.Name)
			if refs, ok := references[bt.ResourceName]; ok {
				bt.References = refs
			}
			if methods, ok := customMethods[bt.ResourceName]; ok {
				bt.CustomMethods = methods
			}
			baseTypes = append(baseTypes, bt)
			log.Printf("Found base type: %s -> %s (custom methods: %d)", bt.Name, bt.ResourceName, len(bt.CustomMethods))
		}
	}

	return baseTypes
}

// hasGenerateScopedMarker checks if a declaration has the +codegen:generate:scoped marker.
func (p *Parser) hasGenerateScopedMarker(genDecl *ast.GenDecl) bool {
	if genDecl.Doc == nil {
		return false
	}
	for _, comment := range genDecl.Doc.List {
		if strings.Contains(comment.Text, MarkerGenerateScoped) {
			return true
		}
	}
	return false
}

// parseReferencesStruct parses a references struct type.
func (p *Parser) parseReferencesStruct(typeSpec *ast.TypeSpec) []Reference {
	var refs []Reference

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return refs
	}

	for _, field := range structType.Fields.List {
		if field.Doc == nil {
			continue
		}

		for _, comment := range field.Doc.List {
			if strings.Contains(comment.Text, MarkerReference) {
				ref := p.parseReferenceComment(comment.Text)
				if ref.TargetType != "" {
					refs = append(refs, ref)
				}
			}
		}
	}

	return refs
}

// parseReferenceComment parses a reference comment like:
// +codegen:reference:target=Configuration,field=Spec.ForProvider.Configuration
func (p *Parser) parseReferenceComment(comment string) Reference {
	ref := Reference{}

	parts := strings.Split(comment, MarkerReference)
	if len(parts) < 2 {
		return ref
	}

	params := strings.Split(parts[1], ",")
	for _, param := range params {
		param = strings.TrimSpace(param)
		switch {
		case strings.HasPrefix(param, "target="):
			ref.TargetType = strings.TrimPrefix(param, "target=")
		case strings.HasPrefix(param, "field="):
			ref.FieldPath = strings.TrimPrefix(param, "field=")
			// Extract field name from path (e.g., "Spec.ForProvider.Configuration" -> "Configuration")
			pathParts := strings.Split(ref.FieldPath, ".")
			if len(pathParts) > 0 {
				ref.FieldName = pathParts[len(pathParts)-1]
			}
		case strings.HasPrefix(param, "refName="):
			ref.RefName = strings.TrimPrefix(param, "refName=")
		case strings.HasPrefix(param, "selectorName="):
			ref.SelectorName = strings.TrimPrefix(param, "selectorName=")
		}
	}

	// Default ref/selector names if not explicitly overridden
	if ref.RefName == "" && ref.FieldName != "" {
		ref.RefName = ref.FieldName + "Ref"
	}
	if ref.SelectorName == "" && ref.FieldName != "" {
		ref.SelectorName = ref.FieldName + "Selector"
	}

	return ref
}

// parseInterfaceMethods extracts custom methods from an interface type.
func (p *Parser) parseInterfaceMethods(typeSpec *ast.TypeSpec) []CustomMethod {
	var methods []CustomMethod

	interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		return methods
	}

	for _, method := range interfaceType.Methods.List {
		if len(method.Names) == 0 {
			continue // embedded interface
		}

		methodName := method.Names[0].Name

		// Skip standard methods
		if standardMethods[methodName] {
			continue
		}

		// Check for +codegen:method comment
		if method.Doc == nil {
			continue
		}

		for _, comment := range method.Doc.List {
			if strings.Contains(comment.Text, MarkerMethod) {
				cm := p.parseMethodComment(methodName, comment.Text, method.Type)
				if cm.Name != "" {
					methods = append(methods, cm)
				}
			}
		}
	}

	return methods
}

// parseMethodComment parses a method comment like:
// +codegen:method:field=Status.AtProvider.ID
func (p *Parser) parseMethodComment(methodName, comment string, funcType ast.Expr) CustomMethod {
	cm := CustomMethod{Name: methodName}

	parts := strings.Split(comment, MarkerMethod)
	if len(parts) < 2 {
		return cm
	}

	params := strings.Split(parts[1], ",")
	for _, param := range params {
		param = strings.TrimSpace(param)
		if strings.HasPrefix(param, "field=") {
			cm.FieldPath = strings.TrimPrefix(param, "field=")
		}
	}

	// Extract return type from function signature
	if ft, ok := funcType.(*ast.FuncType); ok {
		if ft.Results != nil && len(ft.Results.List) > 0 {
			cm.ReturnType = exprToString(ft.Results.List[0].Type)
		}
	}

	return cm
}

// exprToString converts an ast.Expr to its string representation.
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}
