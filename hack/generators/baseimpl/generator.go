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
	"bytes"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// Generate runs the code generation process.
func (g *Generator) Generate() error {
	groups, err := g.discoverGroups()
	if err != nil {
		return fmt.Errorf("failed to discover groups: %w", err)
	}

	if len(groups) == 0 {
		log.Println("No groups found in base directory")
		return nil
	}

	g.Groups = groups

	for _, gc := range groups {
		if err := g.generateForGroup(gc); err != nil {
			return fmt.Errorf("failed to generate for group %s: %w", gc.Name, err)
		}
	}

	return nil
}

// discoverGroups scans the base directory for API group subdirectories.
func (g *Generator) discoverGroups() ([]GroupConfig, error) {
	entries, err := os.ReadDir(g.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read base directory %s: %w", g.BaseDir, err)
	}

	var groups []GroupConfig
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		groupDir := entry.Name()
		basePkg := filepath.Join(g.BaseDir, groupDir, "v1alpha1")

		if _, err := os.Stat(basePkg); os.IsNotExist(err) {
			continue
		}

		clusterGroupName, err := g.parseGroupName(basePkg)
		if err != nil {
			return nil, fmt.Errorf("failed to parse group name for %s: %w", groupDir, err)
		}

		if clusterGroupName == "" {
			log.Printf("Skipping %s: no %s marker found in doc.go", groupDir, MarkerGroupName)
			continue
		}

		gc := GroupConfig{
			Name:                groupDir,
			ClusterGroupName:    clusterGroupName,
			NamespacedGroupName: DeriveNamespacedGroup(clusterGroupName),
			BasePkg:             basePkg,
			ClusterPkg:          filepath.Join(g.ClusterDir, groupDir, "v1alpha1"),
			NamespacedPkg:       filepath.Join(g.NamespacedDir, groupDir, "v1alpha1"),
			ClusterCtrlPkg:      filepath.Join(g.ClusterCtrlDir, groupDir),
			NamespacedCtrlPkg:   filepath.Join(g.NamespacedCtrlDir, groupDir),
		}

		groups = append(groups, gc)
		log.Printf("Discovered group: %s (cluster: %s, namespaced: %s)", groupDir, gc.ClusterGroupName, gc.NamespacedGroupName)
	}

	return groups, nil
}

// parseGroupName reads the +groupName marker from doc.go in the given package directory.
func (g *Generator) parseGroupName(pkgDir string) (string, error) {
	docFile := filepath.Join(pkgDir, "doc.go")
	content, err := os.ReadFile(docFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read %s: %w", docFile, err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, MarkerGroupName) {
			idx := strings.Index(line, MarkerGroupName)
			return strings.TrimSpace(line[idx+len(MarkerGroupName):]), nil
		}
	}

	return "", nil
}

// generateForGroup generates all files for a single API group (both scopes).
func (g *Generator) generateForGroup(gc GroupConfig) error {
	parser := NewParser(gc.BasePkg)

	baseTypes, err := parser.ParseBaseTypes()
	if err != nil {
		return fmt.Errorf("failed to parse base types in %s: %w", gc.BasePkg, err)
	}

	if len(baseTypes) == 0 {
		log.Printf("No base types found with %s marker in %s", MarkerGenerateScoped, gc.BasePkg)
		return nil
	}

	for _, scope := range []string{ScopeCluster, ScopeNamespaced} {
		if err := g.generateForScope(gc, baseTypes, scope); err != nil {
			return err
		}
	}

	return nil
}

// generateForScope generates all files for a single scope within a group.
func (g *Generator) generateForScope(gc GroupConfig, baseTypes []BaseType, scope string) error {
	generators := []struct {
		name string
		fn   func() error
	}{
		{"API package files", func() error { return g.generateAPIPackageFiles(gc, scope) }},
		{"types", func() error { return g.generateScopedTypes(gc, baseTypes, scope) }},
		{"baseimpl", func() error { return g.generateBaseImpl(gc, baseTypes, scope) }},
		{"controllers", func() error { return g.generateControllers(gc, baseTypes, scope) }},
		{"setup", func() error { return g.generateSetup(gc, baseTypes, scope) }},
	}

	if !g.SkipProviderConfig {
		generators = append(generators, struct {
			name string
			fn   func() error
		}{"providerconfig controller", func() error { return g.generateProviderConfigController(gc, scope) }})
	}

	for _, gen := range generators {
		if err := gen.fn(); err != nil {
			return fmt.Errorf("failed to generate %s %s for group %s: %w", scope, gen.name, gc.Name, err)
		}
	}

	return nil
}

// generateAPIPackageFiles generates all API package infrastructure files.
func (g *Generator) generateAPIPackageFiles(gc GroupConfig, scope string) error {
	pkgPath := gc.GetPackagePath(scope)
	isCluster := scope == ScopeCluster

	if err := os.MkdirAll(pkgPath, 0750); err != nil {
		return fmt.Errorf("failed to create API directory %s: %w", pkgPath, err)
	}

	groupName := gc.GetGroupName(scope)

	files := []templateFile{
		{docTemplate, nil, filepath.Join(pkgPath, "zz_generated.doc.go")},
		{groupVersionInfoTemplate, GroupVersionTemplateData{GroupName: groupName, ProviderName: g.ProviderName}, filepath.Join(pkgPath, "zz_generated.groupversion_info.go")},
	}

	if !g.SkipProviderConfig {
		pcData := ProviderConfigTemplateData{
			Scope:        scope,
			IsCluster:    isCluster,
			ModulePath:   g.ModulePath,
			GroupDir:     gc.Name,
			ProviderName: g.ProviderName,
		}
		files = append(files,
			templateFile{providerConfigTypesTemplate, pcData, filepath.Join(pkgPath, "zz_generated.providerconfig_types.go")},
			templateFile{providerConfigUsageTypesTemplate, pcData, filepath.Join(pkgPath, "zz_generated.providerconfigusage_types.go")},
		)
	}

	apisParentDir := filepath.Dir(pkgPath) // e.g., apis/cluster/account
	files = append(files,
		templateFile{apisPackageTemplate, APIsPackageTemplateData{Scope: scope, ModulePath: g.ModulePath, GroupDir: gc.Name}, filepath.Join(apisParentDir, fmt.Sprintf("zz_generated.%s.go", gc.Name))},
	)

	return generateFiles(files)
}

// templateFile represents a file to be generated from a template.
type templateFile struct {
	tmpl *template.Template
	data any
	path string
}

// generateFiles generates multiple files from templates.
func generateFiles(files []templateFile) error {
	for _, f := range files {
		content, err := executeTemplate(f.tmpl, f.data)
		if err != nil {
			return fmt.Errorf("failed to execute template for %s: %w", f.path, err)
		}
		if err := writeFile(f.path, content); err != nil {
			return err
		}
		log.Printf("Generated %s", f.path)
	}
	return nil
}

// generateScopedTypes generates the scoped type file for cluster or namespaced.
func (g *Generator) generateScopedTypes(gc GroupConfig, baseTypes []BaseType, scope string) error {
	pkgPath := gc.GetPackagePath(scope)

	for _, bt := range baseTypes {
		data := ScopedTemplateData{
			BaseType:      bt,
			Scope:         scope,
			IsCluster:     scope == ScopeCluster,
			ModulePath:    g.ModulePath,
			BasePkgImport: g.GetBasePkgImport(gc),
			ProviderName:  g.ProviderName,
		}

		content, err := executeTemplate(scopedTypeTemplate, data)
		if err != nil {
			return fmt.Errorf("failed to execute template for %s: %w", bt.ResourceName, err)
		}

		filename := filepath.Join(pkgPath, fmt.Sprintf("zz_generated.%s.go", strings.ToLower(bt.ResourceName)))
		if err := writeFile(filename, content); err != nil {
			return err
		}
		log.Printf("Generated %s", filename)
	}

	return nil
}

// generateBaseImpl generates the interface implementation file.
func (g *Generator) generateBaseImpl(gc GroupConfig, baseTypes []BaseType, scope string) error {
	pkgPath := gc.GetPackagePath(scope)
	data := BaseImplTemplateData{
		BaseTypes:     baseTypes,
		ModulePath:    g.ModulePath,
		BasePkgImport: g.GetBasePkgImport(gc),
	}

	content, err := executeTemplate(baseImplTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to execute baseimpl template: %w", err)
	}

	filename := filepath.Join(pkgPath, "zz_generated.baseimpl.go")
	if err := writeFile(filename, content); err != nil {
		return err
	}
	log.Printf("Generated %s", filename)

	return nil
}

// generateControllers generates the controller files for each base type.
func (g *Generator) generateControllers(gc GroupConfig, baseTypes []BaseType, scope string) error {
	ctrlBasePath := gc.GetControllerPackagePath(scope)

	for _, bt := range baseTypes {
		ctrlPkgPath := filepath.Join(ctrlBasePath, strings.ToLower(bt.ResourceName))
		if err := os.MkdirAll(ctrlPkgPath, 0750); err != nil {
			return fmt.Errorf("failed to create controller directory %s: %w", ctrlPkgPath, err)
		}

		data := ControllerTemplateData{
			BaseType:      bt,
			Scope:         scope,
			IsCluster:     scope == ScopeCluster,
			ModulePath:    g.ModulePath,
			BasePkgImport: g.GetBasePkgImport(gc),
			GroupDir:      gc.Name,
		}

		content, err := executeTemplate(controllerTemplate, data)
		if err != nil {
			return fmt.Errorf("failed to execute controller template for %s: %w", bt.ResourceName, err)
		}

		filename := filepath.Join(ctrlPkgPath, fmt.Sprintf("zz_generated.%s.go", strings.ToLower(bt.ResourceName)))
		if err := writeFile(filename, content); err != nil {
			return err
		}
		log.Printf("Generated %s", filename)
	}

	return nil
}

// generateSetup generates the setup.go file for controller registration.
func (g *Generator) generateSetup(gc GroupConfig, baseTypes []BaseType, scope string) error {
	ctrlBasePath := gc.GetControllerPackagePath(scope)

	if err := os.MkdirAll(ctrlBasePath, 0750); err != nil {
		return fmt.Errorf("failed to create controller directory %s: %w", ctrlBasePath, err)
	}

	data := SetupTemplateData{
		BaseTypes:          baseTypes,
		Scope:              scope,
		ModulePath:         g.ModulePath,
		GroupDir:           gc.Name,
		SkipProviderConfig: g.SkipProviderConfig,
	}

	content, err := executeTemplate(setupTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to execute setup template: %w", err)
	}

	filename := filepath.Join(ctrlBasePath, "zz_generated.setup.go")
	if err := writeFile(filename, content); err != nil {
		return err
	}
	log.Printf("Generated %s", filename)

	return nil
}

// generateUtils generates the utils package for the scope.
func (g *Generator) generateUtils(gc GroupConfig, scope string) error {
	ctrlBasePath := gc.GetControllerPackagePath(scope)
	utilsPath := filepath.Join(ctrlBasePath, "utils")

	if err := os.MkdirAll(utilsPath, 0750); err != nil {
		return fmt.Errorf("failed to create utils directory %s: %w", utilsPath, err)
	}

	var tmpl *template.Template
	if scope == ScopeCluster {
		tmpl = utilsLegacyTrackerTemplate
	} else {
		tmpl = utilsModernTrackerTemplate
	}

	content, err := executeTemplate(tmpl, nil)
	if err != nil {
		return fmt.Errorf("failed to execute utils template: %w", err)
	}

	filename := filepath.Join(utilsPath, "zz_generated.utils.go")
	if err := writeFile(filename, content); err != nil {
		return err
	}
	log.Printf("Generated %s", filename)

	return nil
}

// generateProviderConfigController generates the providerconfig controller for the scope.
func (g *Generator) generateProviderConfigController(gc GroupConfig, scope string) error {
	ctrlBasePath := gc.GetControllerPackagePath(scope)
	pcPath := filepath.Join(ctrlBasePath, "providerconfig")

	if err := os.MkdirAll(pcPath, 0750); err != nil {
		return fmt.Errorf("failed to create providerconfig directory %s: %w", pcPath, err)
	}

	data := ProviderConfigTemplateData{
		Scope:        scope,
		IsCluster:    scope == ScopeCluster,
		ModulePath:   g.ModulePath,
		GroupDir:     gc.Name,
		ProviderName: g.ProviderName,
	}

	content, err := executeTemplate(providerConfigTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to execute providerconfig template: %w", err)
	}

	filename := filepath.Join(pcPath, "zz_generated.providerconfig.go")
	if err := writeFile(filename, content); err != nil {
		return err
	}
	log.Printf("Generated %s", filename)

	return nil
}

// executeTemplate executes a template with the given data and returns the result.
func executeTemplate(tmpl *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(io.Writer(&buf), data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeFile writes content to a file with standard permissions.
// For .go files, it formats the code using gofmt before writing.
func writeFile(filename string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0750); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", filename, err)
	}

	if strings.HasSuffix(filename, ".go") {
		formatted, err := format.Source(content)
		if err != nil {
			return fmt.Errorf("failed to format %s: %w", filename, err)
		}
		content = formatted
	}

	if err := os.WriteFile(filename, content, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}
	return nil
}
