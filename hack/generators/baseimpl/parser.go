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
			bt.DocComment = p.extractDocComment(genDecl)
			bt.Categories = p.extractCategories(genDecl)
			bt.DeprecatedWarning = p.extractDeprecatedWarning(genDecl)
			if refs, ok := references[bt.ResourceName]; ok {
				bt.References = refs
			}
			if methods, ok := customMethods[bt.ResourceName]; ok {
				bt.CustomMethods = methods
			}
			bt.SpecFields = p.extractSpecFields(file, bt.ResourceName)
			bt.ForProviderOmitTag = p.hasForProviderOmitEmpty(file, bt.ResourceName)
			if len(bt.References) > 0 {
				bt.ParameterFields = p.extractParameterFields(file, bt.ResourceName, bt.References)
			}
			baseTypes = append(baseTypes, bt)
			log.Printf("Found base type: %s -> %s (custom methods: %d, spec fields: %d)", bt.Name, bt.ResourceName, len(bt.CustomMethods), len(bt.SpecFields))
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

// extractCategories extracts the +codegen:categories=<value> marker from a type declaration.
func (p *Parser) extractCategories(genDecl *ast.GenDecl) string {
	if genDecl.Doc == nil {
		return ""
	}
	for _, comment := range genDecl.Doc.List {
		if idx := strings.Index(comment.Text, MarkerCategories); idx >= 0 {
			return strings.TrimSpace(comment.Text[idx+len(MarkerCategories):])
		}
	}
	return ""
}

// extractDeprecatedWarning extracts the +codegen:deprecatedversion:warning="..." marker.
func (p *Parser) extractDeprecatedWarning(genDecl *ast.GenDecl) string {
	if genDecl.Doc == nil {
		return ""
	}
	for _, comment := range genDecl.Doc.List {
		if idx := strings.Index(comment.Text, MarkerDeprecatedVersion); idx >= 0 {
			val := strings.TrimSpace(comment.Text[idx+len(MarkerDeprecatedVersion):])
			// Strip surrounding quotes
			val = strings.Trim(val, "\"")
			return val
		}
	}
	return ""
}

// hasForProviderOmitEmpty checks if the Base<Resource>Spec has ForProvider with ,omitempty tag.
func (p *Parser) hasForProviderOmitEmpty(file *ast.File, resourceName string) bool {
	specTypeName := PrefixBase + resourceName + "Spec"

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != specTypeName {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			for _, field := range structType.Fields.List {
				if len(field.Names) > 0 && field.Names[0].Name == "ForProvider" {
					if field.Tag != nil && strings.Contains(field.Tag.Value, "omitempty") {
						return true
					}
				}
			}
		}
	}
	return false
}

// extractDocComment extracts non-marker comment lines from a type declaration's doc block.
// Skips the conventional "BaseX is the base resource definition for X." line.
func (p *Parser) extractDocComment(genDecl *ast.GenDecl) []string {
	if genDecl.Doc == nil {
		return nil
	}
	var lines []string
	for _, comment := range genDecl.Doc.List {
		text := strings.TrimPrefix(comment.Text, "//")
		text = strings.TrimPrefix(text, " ")
		if strings.HasPrefix(text, "+") {
			continue
		}
		if strings.HasPrefix(text, "Base") && strings.Contains(text, "is the base resource definition for") {
			continue
		}
		lines = append(lines, text)
	}
	// Trim leading and trailing empty lines
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// parseReferencesStruct parses a references struct type.
// Each field in the struct represents one reference. All +codegen:reference: comment lines
// above a field are merged into a single Reference (one key=value per line).
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

		var ref Reference
		for _, comment := range field.Doc.List {
			if strings.Contains(comment.Text, MarkerReference) {
				p.mergeReferenceParam(&ref, comment.Text)
			}
		}
		if ref.TargetType != "" {
			if ref.RefName == "" && ref.FieldName != "" {
				ref.RefName = ref.FieldName + "Ref"
			}
			if ref.SelectorName == "" && ref.FieldName != "" {
				ref.SelectorName = ref.FieldName + "Selector"
			}
			refs = append(refs, ref)
		}
	}

	return refs
}

// mergeReferenceParam parses a single +codegen:reference:key=value line and sets the
// corresponding field on the Reference.
func (p *Parser) mergeReferenceParam(ref *Reference, comment string) {
	parts := strings.SplitN(comment, MarkerReference, 2)
	if len(parts) < 2 {
		return
	}
	param := strings.TrimSpace(parts[1])

	switch {
	case strings.HasPrefix(param, "target="):
		ref.TargetType = strings.TrimPrefix(param, "target=")
	case strings.HasPrefix(param, "group="):
		ref.Group = strings.TrimPrefix(param, "group=")
	case strings.HasPrefix(param, "apiversion="):
		ref.ApiVersion = strings.TrimPrefix(param, "apiversion=")
	case strings.HasPrefix(param, "field="):
		ref.FieldPath = strings.TrimPrefix(param, "field=")
		pathParts := strings.Split(ref.FieldPath, ".")
		if len(pathParts) > 0 {
			ref.FieldName = pathParts[len(pathParts)-1]
		}
	case strings.HasPrefix(param, "refName="):
		ref.RefName = strings.TrimPrefix(param, "refName=")
	case strings.HasPrefix(param, "selectorName="):
		ref.SelectorName = strings.TrimPrefix(param, "selectorName=")
	case strings.HasPrefix(param, "refDescription="):
		desc := strings.TrimPrefix(param, "refDescription=")
		ref.RefDescription = strings.ReplaceAll(desc, "\\n", "\n")
	case param == "immutableRef=true":
		ref.ImmutableRef = true
	}
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

// extractSpecFields finds the Base<Resource>Spec struct and extracts fields beyond ForProvider.
func (p *Parser) extractSpecFields(file *ast.File, resourceName string) []SpecField {
	specTypeName := PrefixBase + resourceName + "Spec"
	var fields []SpecField

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != specTypeName {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			for _, field := range structType.Fields.List {
				// Skip ForProvider field
				if len(field.Names) > 0 && field.Names[0].Name == "ForProvider" {
					continue
				}

				sf := SpecField{}

				// Determine if embedded (anonymous) or named
				if len(field.Names) == 0 {
					sf.Embedded = true
					sf.TypeName = exprToString(field.Type)
				} else {
					sf.Name = field.Names[0].Name
					sf.TypeName = exprToString(field.Type)
				}

				// Extract struct tag
				if field.Tag != nil {
					sf.JSONTag = field.Tag.Value
				}

				if sf.TypeName != "" {
					fields = append(fields, sf)
				}
			}
		}
	}

	return fields
}

// extractParameterFields extracts all fields from Base<Resource>Parameters and annotates reference fields.
func (p *Parser) extractParameterFields(file *ast.File, resourceName string, refs []Reference) []ParameterField {
	paramsTypeName := PrefixBase + resourceName + "Parameters"
	var fields []ParameterField

	// Build lookup of reference field names
	refLookup := make(map[string]Reference)
	for _, ref := range refs {
		refLookup[ref.FieldName] = ref
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != paramsTypeName {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 {
					continue // skip embedded fields
				}

				pf := ParameterField{
					Name:     field.Names[0].Name,
					TypeExpr: fieldTypeToString(field.Type),
				}

				if field.Tag != nil {
					pf.Tag = field.Tag.Value
				}

				// Extract doc comments
				if field.Doc != nil {
					for _, comment := range field.Doc.List {
						text := strings.TrimPrefix(comment.Text, "//")
						text = strings.TrimPrefix(text, " ")
						pf.Comments = append(pf.Comments, text)
					}
				}

				// Check if this is a reference value field
				if ref, ok := refLookup[pf.Name]; ok {
					pf.IsReferenceValue = true
					pf.ReferenceTarget = ref.TargetType
					pf.ReferenceGroup = ref.Group
					pf.ReferenceApiVersion = ref.ApiVersion
					pf.RefName = ref.RefName
					pf.SelectorName = ref.SelectorName
					pf.RefDescription = ref.RefDescription
					pf.ImmutableRef = ref.ImmutableRef
				}

				fields = append(fields, pf)
			}
		}
	}

	return fields
}

// ParseExportedStructs returns all exported struct type names from the base package,
// excluding types with the "Base" prefix and types ending in "References" (codegen markers).
func (p *Parser) ParseExportedStructs() ([]string, error) {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles,
		Dir:  p.basePkg,
		Fset: fset,
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, 0)
		},
	}
	pkgList, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to parse base package %s: %w", p.basePkg, err)
	}

	var names []string
	for _, pkg := range pkgList {
		for _, file := range pkg.Syntax {
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
					name := typeSpec.Name.Name
					if !ast.IsExported(name) {
						continue
					}
					if _, ok := typeSpec.Type.(*ast.StructType); !ok {
						continue
					}
					if strings.HasPrefix(name, PrefixBase) || strings.HasSuffix(name, "References") {
						continue
					}
					names = append(names, name)
				}
			}
		}
	}

	return names, nil
}

// ParseExportedVarsAndConsts returns all exported var and const names from the base package.
func (p *Parser) ParseExportedVarsAndConsts() ([]ExportedVar, error) {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles,
		Dir:  p.basePkg,
		Fset: fset,
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, 0)
		},
	}
	pkgList, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to parse base package %s: %w", p.basePkg, err)
	}

	var vars []ExportedVar
	for _, pkg := range pkgList {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				if genDecl.Tok != token.VAR && genDecl.Tok != token.CONST {
					continue
				}
				isConst := genDecl.Tok == token.CONST
				for _, spec := range genDecl.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range valueSpec.Names {
						if ast.IsExported(name.Name) {
							vars = append(vars, ExportedVar{Name: name.Name, IsConst: isConst})
						}
					}
				}
			}
		}
	}

	return vars, nil
}

// fieldTypeToString converts a field type AST expression to its Go source representation.
func fieldTypeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + fieldTypeToString(t.X)
	case *ast.SelectorExpr:
		return fieldTypeToString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + fieldTypeToString(t.Elt)
		}
		return "[" + fieldTypeToString(t.Len) + "]" + fieldTypeToString(t.Elt)
	case *ast.MapType:
		return "map[" + fieldTypeToString(t.Key) + "]" + fieldTypeToString(t.Value)
	default:
		return ""
	}
}
