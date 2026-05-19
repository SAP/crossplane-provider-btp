package main

import (
	"embed"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// templateFuncs contains functions available in templates.
var templateFuncs = template.FuncMap{
	"toLower":    strings.ToLower,
	"lowerFirst": lowerFirst,
	"splitLines": splitLines,
}

// lowerFirst lowercases the first character of a string.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// splitLines splits a string on newlines, returning a slice.
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

// mustParseTemplate parses a template from the embedded filesystem.
// It ensures all templates end with a newline for proper formatting.
func mustParseTemplate(name, filename string) *template.Template {
	content, err := templateFS.ReadFile("templates/" + filename)
	if err != nil {
		panic(err)
	}
	// Ensure template content ends with exactly one newline
	contentStr := strings.TrimRight(string(content), "\n") + "\n"
	return template.Must(template.New(name).Funcs(templateFuncs).Parse(contentStr))
}

// scopedTypeTemplate generates the scoped type definition.
var scopedTypeTemplate = mustParseTemplate("scopedType", "scoped_type.go.tmpl")

// baseImplTemplate generates the conversion methods to/from base types.
var baseImplTemplate = mustParseTemplate("baseImpl", "base_impl.go.tmpl")

// controllerTemplate generates the scoped controller implementation.
var controllerTemplate = mustParseTemplate("controller", "controller.go.tmpl")

// setupTemplate generates the setup.go file that registers all controllers.
var setupTemplate = mustParseTemplate("setup", "setup.go.tmpl")

// docTemplate generates the doc.go file.
var docTemplate = mustParseTemplate("doc", "doc.go.tmpl")

// groupVersionInfoTemplate generates the groupversion_info.go file.
var groupVersionInfoTemplate = mustParseTemplate("groupVersionInfo", "groupversion_info.go.tmpl")

// apisPackageTemplate generates the top-level apis/{scope}/aicore.go file.
var apisPackageTemplate = mustParseTemplate("apisPackage", "apis_package.go.tmpl")

// typesTemplate generates type aliases for exported base structs.
var typesTemplate = mustParseTemplate("types", "types.go.tmpl")
