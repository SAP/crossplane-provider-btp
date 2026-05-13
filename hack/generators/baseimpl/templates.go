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
}

// lowerFirst lowercases the first character of a string.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
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

// utilsLegacyTrackerTemplate generates the utils/legacy_tracker.go for cluster scope.
var utilsLegacyTrackerTemplate = mustParseTemplate("utilsLegacyTracker", "utils_legacy_tracker.go.tmpl")

// utilsModernTrackerTemplate generates the utils/utils.go for namespaced scope.
var utilsModernTrackerTemplate = mustParseTemplate("utilsModernTracker", "utils_modern_tracker.go.tmpl")

// docTemplate generates the doc.go file.
var docTemplate = mustParseTemplate("doc", "doc.go.tmpl")

// groupVersionInfoTemplate generates the groupversion_info.go file.
var groupVersionInfoTemplate = mustParseTemplate("groupVersionInfo", "groupversion_info.go.tmpl")

// providerConfigTypesTemplate generates the providerconfig_types.go file.
var providerConfigTypesTemplate = mustParseTemplate("providerConfigTypes", "providerconfig_types.go.tmpl")

// providerConfigUsageTypesTemplate generates the providerconfigusage_types.go file.
var providerConfigUsageTypesTemplate = mustParseTemplate("providerConfigUsageTypes", "providerconfigusage_types.go.tmpl")

// apisPackageTemplate generates the top-level apis/{scope}/aicore.go file.
var apisPackageTemplate = mustParseTemplate("apisPackage", "apis_package.go.tmpl")

// providerConfigTemplate generates the providerconfig controller.
var providerConfigTemplate = mustParseTemplate("providerconfig", "providerconfig.go.tmpl")
