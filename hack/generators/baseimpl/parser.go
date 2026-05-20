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

	// Build a package-wide index of struct type declarations across all files,
	// so we can look up types referenced from a Spec field that live in a different file
	// of the same package (e.g. XSUAACredentialsReference defined in xsuaa_credentials.go).
	pkgStructs := make(map[string]*ast.StructType)
	for _, pkg := range pkgList {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if st, ok := ts.Type.(*ast.StructType); ok {
						pkgStructs[ts.Name.Name] = st
					}
				}
			}
		}
	}

	for _, pkg := range pkgList {
		for _, file := range pkg.Syntax {
			p.collectMetadata(file, references, customMethods)
			baseTypes = append(baseTypes, p.collectBaseTypes(file, references, customMethods, pkgStructs)...)
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
func (p *Parser) collectBaseTypes(file *ast.File, references map[string][]Reference, customMethods map[string][]CustomMethod, pkgStructs map[string]*ast.StructType) []BaseType {
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
			bt.SpecFields = p.extractSpecFields(pkgStructs, bt.ResourceName)
			bt.ForProviderOmitTag = p.hasForProviderOmitEmpty(file, bt.ResourceName)

			// Discover SpecField-level references from embedded fields tagged with
			// `+codegen:references`. Each discovered Reference is appended to bt.References
			// and processed by the same splitting/annotation pipeline as sidecar-declared refs.
			discovered := p.discoverEmbedReferences(pkgStructs, bt.ResourceName)
			bt.References = append(bt.References, discovered...)

			// Split references into ForProvider-level (existing path) and SpecField-level (new path).
			// SpecField-level references attach to the matching SpecField; the parser also reads the
			// embedded type's full field list so the scoped types template can emit a local copy.
			if len(bt.References) > 0 {
				var fpRefs []Reference
				for _, ref := range bt.References {
					if ref.SpecFieldName == "" {
						fpRefs = append(fpRefs, ref)
						continue
					}
					for i := range bt.SpecFields {
						if bt.SpecFields[i].TypeName != ref.SpecFieldName {
							continue
						}
						bt.SpecFields[i].Refs = append(bt.SpecFields[i].Refs, ref)
						if bt.SpecFields[i].Fields == nil {
							bt.SpecFields[i].Fields = p.extractStructFields(pkgStructs, ref.SpecFieldName)
						}
						break
					}
				}
				// Annotate the SpecField sub-fields with reference info so the template can emit markers.
				for i := range bt.SpecFields {
					sf := &bt.SpecFields[i]
					if len(sf.Refs) == 0 {
						continue
					}
					annotateSpecFieldRefs(sf)
				}
				// Reduce bt.References to ForProvider-level only — keeps existing template branches unchanged.
				bt.References = fpRefs
			}
			if len(bt.References) > 0 {
				bt.ParameterFields = p.extractParameterFields(pkgStructs, bt.ResourceName, bt.References)
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
			parseReferenceMarkerLine(&ref, comment.Text, MarkerReference)
		}
		if ref.TargetType == "" {
			continue
		}
		finalizeReferenceDefaults(&ref, "")
		refs = append(refs, ref)
	}

	return refs
}

// parseReferenceMarkerLine parses a single reference marker line and merges its key=value pair
// into ref. The marker prefix is supplied by the caller — either MarkerReference
// ("+codegen:reference:") for the sidecar dialect, or "+crossplane:generate:reference:" for
// markers harvested from an embedded reference struct.
//
// The two dialects use different keys for the same concepts; both are accepted in one switch
// so each caller doesn't need its own merge function:
//
//	target=        / type=               -> Reference.TargetType
//	refName=       / refFieldName=       -> Reference.RefName
//	selectorName=  / selectorFieldName=  -> Reference.SelectorName
//
// Sidecar-only keys (group, apiversion, field, refDescription, immutableRef) are also handled.
// Lines that don't start with the supplied prefix or that carry an unknown key are ignored.
func parseReferenceMarkerLine(ref *Reference, comment, prefix string) {
	idx := strings.Index(comment, prefix)
	if idx < 0 {
		return
	}
	param := strings.TrimSpace(comment[idx+len(prefix):])
	switch {
	case strings.HasPrefix(param, "target="):
		ref.TargetType = strings.TrimPrefix(param, "target=")
	case strings.HasPrefix(param, "type="):
		ref.TargetType = strings.TrimPrefix(param, "type=")
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
		// Detect a SpecField-embedded reference, e.g. "Spec.XSUAACredentialsReference.SubaccountApiCredentialSecret".
		// pathParts[0] == "Spec", pathParts[1] is the embedded SpecField type name, pathParts[2] is the value field.
		// "Spec.ForProvider.X" stays as a parameter-level reference (SpecFieldName empty).
		if len(pathParts) == 3 && pathParts[0] == "Spec" && pathParts[1] != "ForProvider" {
			ref.SpecFieldName = pathParts[1]
		}
	case strings.HasPrefix(param, "extractor="):
		ref.Extractor = strings.TrimPrefix(param, "extractor=")
	case strings.HasPrefix(param, "refName="):
		ref.RefName = strings.TrimPrefix(param, "refName=")
	case strings.HasPrefix(param, "refFieldName="):
		ref.RefName = strings.TrimPrefix(param, "refFieldName=")
	case strings.HasPrefix(param, "selectorName="):
		ref.SelectorName = strings.TrimPrefix(param, "selectorName=")
	case strings.HasPrefix(param, "selectorFieldName="):
		ref.SelectorName = strings.TrimPrefix(param, "selectorFieldName=")
	case strings.HasPrefix(param, "refDescription="):
		desc := strings.TrimPrefix(param, "refDescription=")
		ref.RefDescription = strings.ReplaceAll(desc, "\\n", "\n")
	case param == "immutableRef=true":
		ref.ImmutableRef = true
	}
}

// finalizeReferenceDefaults fills in derived fields after marker parsing:
//   - if FieldName is unset, fall back to the caller-supplied fallbackFieldName (used by the
//     embed-discovery path, where the field name comes from the AST node itself rather than a
//     `field=` marker)
//   - if RefName is unset, default to FieldName + "Ref"
//   - if SelectorName is unset, default to FieldName + "Selector"
func finalizeReferenceDefaults(ref *Reference, fallbackFieldName string) {
	if ref.FieldName == "" {
		ref.FieldName = fallbackFieldName
	}
	if ref.FieldName == "" {
		return
	}
	if ref.RefName == "" {
		ref.RefName = ref.FieldName + "Ref"
	}
	if ref.SelectorName == "" {
		ref.SelectorName = ref.FieldName + "Selector"
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
//
// Field type expressions are NOT package-qualified (e.g. "XSUAACredentialsReference", not
// "base.XSUAACredentialsReference") because callers consume `TypeName` and prepend the
// "base." selector themselves in the scoped template. The struct walker used internally
// preserves the raw identifier when no qualification is requested.
func (p *Parser) extractSpecFields(pkgStructs map[string]*ast.StructType, resourceName string) []SpecField {
	specTypeName := PrefixBase + resourceName + "Spec"
	st, ok := pkgStructs[specTypeName]
	if !ok {
		return nil
	}
	raw := walkStructFields(st, nil) // nil pkgStructs => no qualification
	var out []SpecField
	for _, pf := range raw {
		// Skip ForProvider — handled by the dedicated ForProvider rendering path.
		if pf.Name == "ForProvider" {
			continue
		}
		sf := SpecField{
			Name:     pf.Name,
			TypeName: pf.TypeExpr,
			JSONTag:  pf.Tag,
			Embedded: pf.Embedded,
		}
		if sf.TypeName != "" {
			out = append(out, sf)
		}
	}
	return out
}

// extractParameterFields extracts all fields from Base<Resource>Parameters and annotates reference fields.
// Embedded fields are skipped — ForProvider parameter rendering currently expects only named fields.
func (p *Parser) extractParameterFields(pkgStructs map[string]*ast.StructType, resourceName string, refs []Reference) []ParameterField {
	paramsTypeName := PrefixBase + resourceName + "Parameters"
	st, ok := pkgStructs[paramsTypeName]
	if !ok {
		return nil
	}

	refLookup := make(map[string]Reference)
	for _, ref := range refs {
		refLookup[ref.FieldName] = ref
	}

	var fields []ParameterField
	for _, pf := range walkStructFields(st, nil) {
		if pf.Embedded {
			continue
		}
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
	return fields
}

// discoverEmbedReferences walks BaseXxxSpec for embedded fields tagged with `+codegen:references`
// and synthesizes Reference entries from the existing `+crossplane:generate:reference:*` markers
// (and `reference-{group,apiversion}:` struct tags) already present on the embedded type's fields.
//
// This lets a base type opt in to scoped reference generation without declaring a sidecar
// `XxxReferences` struct: the marker on the embed field is the only authored input — every other
// datum (target, refName, selectorName, extractor, group, apiversion) is read from the embedded
// type's existing field markers/tags, which are also consumed at runtime by the legacy resolver.
func (p *Parser) discoverEmbedReferences(pkgStructs map[string]*ast.StructType, resourceName string) []Reference {
	specTypeName := PrefixBase + resourceName + "Spec"
	specSt, ok := pkgStructs[specTypeName]
	if !ok {
		return nil
	}

	var out []Reference
	for _, field := range specSt.Fields.List {
		// Only embedded (anonymous) fields are candidates.
		if len(field.Names) != 0 {
			continue
		}
		if field.Doc == nil {
			continue
		}
		hasMarker := false
		for _, c := range field.Doc.List {
			if strings.Contains(c.Text, MarkerReferences) {
				hasMarker = true
				break
			}
		}
		if !hasMarker {
			continue
		}

		embedName := fieldTypeToString(field.Type)
		// Strip leading `*` if anyone embeds a pointer (unusual but defensive).
		embedName = strings.TrimPrefix(embedName, "*")
		embedSt, ok := pkgStructs[embedName]
		if !ok {
			continue
		}

		// Index ref/selector tags in the embedded struct so we can look up group/apiversion/target
		// when we see a value field with reference markers.
		refTags := indexReferenceTags(embedSt)

		for _, ef := range embedSt.Fields.List {
			if len(ef.Names) == 0 || ef.Doc == nil {
				continue
			}
			fieldName := ef.Names[0].Name
			ref := Reference{
				FieldPath:     "Spec." + embedName + "." + fieldName,
				SpecFieldName: embedName,
			}
			for _, c := range ef.Doc.List {
				parseReferenceMarkerLine(&ref, c.Text, MarkerAngryjetReference)
			}
			if ref.TargetType == "" || ref.RefName == "" {
				continue
			}
			finalizeReferenceDefaults(&ref, fieldName)
			if t, ok := refTags[ref.RefName]; ok {
				if ref.Group == "" {
					ref.Group = t.group
				}
				if ref.ApiVersion == "" {
					ref.ApiVersion = t.apiversion
				}
			}
			out = append(out, ref)
		}
	}
	return out
}

// referenceTagInfo holds the values parsed out of `reference-group:` and `reference-apiversion:`
// struct tag entries on a Ref pointer field.
type referenceTagInfo struct {
	group      string
	apiversion string
}

// indexReferenceTags walks an embedded reference-bearing struct and returns a map from each
// `*xpv1.Reference` (or `*xpv1.NamespacedReference`) field name to the values parsed out of its
// `reference-{group,apiversion}:` struct tag entries.
func indexReferenceTags(st *ast.StructType) map[string]referenceTagInfo {
	out := make(map[string]referenceTagInfo)
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 || f.Tag == nil {
			continue
		}
		tag := f.Tag.Value
		if !strings.Contains(tag, "reference-group") && !strings.Contains(tag, "reference-apiversion") {
			continue
		}
		out[f.Names[0].Name] = referenceTagInfo{
			group:      extractTagValue(tag, "reference-group"),
			apiversion: extractTagValue(tag, "reference-apiversion"),
		}
	}
	return out
}

// extractTagValue pulls the value of a single `key:"…"` entry out of a raw struct tag literal
// (the literal still has its surrounding backticks). Returns "" if the key is absent.
func extractTagValue(rawTag, key string) string {
	idx := strings.Index(rawTag, key+":\"")
	if idx < 0 {
		return ""
	}
	rest := rawTag[idx+len(key)+2:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// extractStructFields parses every field of the named struct from the package-wide index.
// Used to read the embedded SpecField type (e.g. XSUAACredentialsReference) so the scoped
// types template can render a local copy with angryjet markers.
//
// Field type expressions are package-qualified with "base." for any same-package struct
// (so the scoped file can compile), and `+crossplane:generate:reference:*` markers from
// the source comments are filtered out (the template re-emits them from parsed sidecar Refs).
func (p *Parser) extractStructFields(pkgStructs map[string]*ast.StructType, structName string) []ParameterField {
	st, ok := pkgStructs[structName]
	if !ok {
		return nil
	}
	return walkStructFields(st, pkgStructs)
}

// walkStructFields walks the fields of a struct type and returns ParameterField metadata.
// When pkgStructs is non-nil, type expressions naming a same-package struct are qualified
// with the "base." selector. When pkgStructs is nil, raw identifiers are preserved.
//
// Doc comments are copied verbatim. Filtering of `+crossplane:generate:reference:*` lines
// (so they don't get re-emitted alongside the scoped template's regenerated marker block)
// happens later in annotateSpecFieldRefs, scoped to ref-value fields only.
func walkStructFields(st *ast.StructType, pkgStructs map[string]*ast.StructType) []ParameterField {
	var fields []ParameterField
	for _, field := range st.Fields.List {
		raw := fieldTypeToString(field.Type)
		typeExpr := raw
		if pkgStructs != nil {
			typeExpr = qualifyBaseType(raw, pkgStructs)
		}
		pf := ParameterField{TypeExpr: typeExpr}
		if len(field.Names) == 0 {
			pf.Embedded = true
		} else {
			pf.Name = field.Names[0].Name
		}
		if field.Tag != nil {
			pf.Tag = field.Tag.Value
		}
		if field.Doc != nil {
			for _, comment := range field.Doc.List {
				text := strings.TrimPrefix(comment.Text, "//")
				text = strings.TrimPrefix(text, " ")
				pf.Comments = append(pf.Comments, text)
			}
		}
		fields = append(fields, pf)
	}
	return fields
}

// qualifyBaseType prefixes any leading identifier of a type expression with "base." when that
// identifier names a struct in the base package. Pointer / slice / map prefixes and map
// key+value positions are recursively qualified.
//
// Examples (assuming "APICredentials" is a base-pkg struct):
//
//	"APICredentials"           -> "base.APICredentials"
//	"*APICredentials"          -> "*base.APICredentials"
//	"[]APICredentials"         -> "[]base.APICredentials"
//	"map[string]APICredentials"-> "map[string]base.APICredentials"
//	"map[Foo]APICredentials"   -> "map[base.Foo]base.APICredentials"  (when Foo is also base-pkg)
//	"xpv1.Selector"            -> "xpv1.Selector"      (already qualified, untouched)
//	"*xpv1.Reference"          -> "*xpv1.Reference"    (already qualified, untouched)
//	"string"                   -> "string"             (built-in, untouched)
func qualifyBaseType(typeExpr string, pkgStructs map[string]*ast.StructType) string {
	switch {
	case strings.HasPrefix(typeExpr, "*"):
		return "*" + qualifyBaseType(typeExpr[1:], pkgStructs)
	case strings.HasPrefix(typeExpr, "[]"):
		return "[]" + qualifyBaseType(typeExpr[2:], pkgStructs)
	case strings.HasPrefix(typeExpr, "map["):
		// Find the matching closing bracket for the key, accounting for nested map[…].
		rest := typeExpr[len("map["):]
		depth := 1
		end := -1
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return typeExpr // malformed — leave untouched
		}
		key := rest[:end]
		value := rest[end+1:]
		return "map[" + qualifyBaseType(key, pkgStructs) + "]" + qualifyBaseType(value, pkgStructs)
	}
	// Atom: already qualified (has a dot) or unknown — leave untouched.
	if strings.Contains(typeExpr, ".") {
		return typeExpr
	}
	if _, ok := pkgStructs[typeExpr]; ok {
		return "base." + typeExpr
	}
	return typeExpr
}

// annotateSpecFieldRefs marks the value/ref/selector sub-fields of a SpecField according to its Refs list,
// so the scoped types template can emit angryjet `+crossplane:generate:reference:*` markers and
// reference-* struct tags in the right places.
//
// For each Reference, three sub-fields participate:
//   - the value field (matches Reference.FieldName) — gets the marker block + reference info
//   - the ref field (matches Reference.RefName) — flagged so the template can swap the type
//     between *xpv1.Reference and *xpv1.NamespacedReference depending on scope; the
//     reference-{group,kind,apiversion} struct tag itself is inherited verbatim from the
//     base type's field tag
//   - the selector field (matches Reference.SelectorName) — flagged so the template can swap
//     between *xpv1.Selector and *xpv1.NamespacedSelector
//
// When multiple refs share the same RefName/SelectorName (e.g. two values extracted from the
// same target), only the type-swap flag is needed on the shared field — the per-ref Group /
// ApiVersion / TargetType only matter at the value field.
func annotateSpecFieldRefs(sf *SpecField) {
	for _, ref := range sf.Refs {
		for i := range sf.Fields {
			f := &sf.Fields[i]
			switch f.Name {
			case ref.FieldName:
				f.IsReferenceValue = true
				f.ReferenceTarget = ref.TargetType
				f.ReferenceGroup = ref.Group
				f.ReferenceApiVersion = ref.ApiVersion
				f.RefName = ref.RefName
				f.SelectorName = ref.SelectorName
				f.RefDescription = ref.RefDescription
				f.ImmutableRef = ref.ImmutableRef
				f.Extractor = ref.Extractor
				// Strip inline `+crossplane:generate:reference:*` markers from this value
				// field's comments — the scoped template re-emits them from the parsed
				// sidecar Refs, so passing them through here would produce a duplicated
				// marker block above the field. Non-value fields are untouched so their
				// authored doc comments survive unchanged.
				f.Comments = filterReferenceMarkers(f.Comments)
			case ref.RefName:
				f.IsRefField = true
			case ref.SelectorName:
				f.IsSelectorField = true
			}
		}
	}
}

// filterReferenceMarkers removes `+crossplane:generate:reference:*` lines from a comment
// slice, returning the rest in order.
func filterReferenceMarkers(comments []string) []string {
	out := comments[:0]
	for _, c := range comments {
		if strings.HasPrefix(c, MarkerAngryjetReference) {
			continue
		}
		out = append(out, c)
	}
	return out
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
