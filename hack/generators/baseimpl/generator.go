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

	exportedStructs, err := parser.ParseExportedStructs()
	if err != nil {
		return fmt.Errorf("failed to parse exported structs in %s: %w", gc.BasePkg, err)
	}

	exportedVars, err := parser.ParseExportedVarsAndConsts()
	if err != nil {
		return fmt.Errorf("failed to parse exported vars in %s: %w", gc.BasePkg, err)
	}

	for _, scope := range []string{ScopeCluster, ScopeNamespaced} {
		if err := g.generateForScope(gc, baseTypes, exportedStructs, exportedVars, scope); err != nil {
			return err
		}
	}

	return nil
}

// generateForScope generates all files for a single scope within a group.
func (g *Generator) generateForScope(gc GroupConfig, baseTypes []BaseType, exportedStructs []string, exportedVars []ExportedVar, scope string) error {
	// Clean stale generated files before writing new ones.
	pkgPath := gc.GetPackagePath(scope)
	apisParentDir := filepath.Dir(pkgPath)
	ctrlBasePath := gc.GetControllerPackagePath(scope)
	for _, dir := range []string{pkgPath, apisParentDir, ctrlBasePath} {
		if err := cleanGeneratedFiles(dir); err != nil {
			return fmt.Errorf("failed to clean generated files in %s: %w", dir, err)
		}
	}
	// Also clean per-resource controller subdirectories.
	for _, bt := range baseTypes {
		ctrlPkgPath := filepath.Join(ctrlBasePath, strings.ToLower(bt.ResourceName))
		if err := cleanGeneratedFiles(ctrlPkgPath); err != nil {
			return fmt.Errorf("failed to clean generated files in %s: %w", ctrlPkgPath, err)
		}
	}

	generators := []struct {
		name string
		fn   func() error
	}{
		{"API package files", func() error { return g.generateAPIPackageFiles(gc, scope) }},
		{"types", func() error { return g.generateScopedTypes(gc, baseTypes, scope) }},
		{"type aliases", func() error { return g.generateTypeAliases(gc, exportedStructs, exportedVars, scope) }},
		{"baseimpl", func() error { return g.generateBaseImpl(gc, baseTypes, scope) }},
		{"controllers", func() error { return g.generateControllers(gc, baseTypes, scope) }},
		{"setup", func() error { return g.generateSetup(gc, baseTypes, scope) }},
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

	if err := os.MkdirAll(pkgPath, 0750); err != nil {
		return fmt.Errorf("failed to create API directory %s: %w", pkgPath, err)
	}

	groupName := gc.GetGroupName(scope)

	files := []templateFile{
		{docTemplate, nil, filepath.Join(pkgPath, GeneratedFilePrefix+"doc.go")},
		{groupVersionInfoTemplate, GroupVersionTemplateData{GroupName: groupName, ProviderName: g.ProviderName}, filepath.Join(pkgPath, GeneratedFilePrefix+"groupversion_info.go")},
	}

	apisParentDir := filepath.Dir(pkgPath) // e.g., apis/cluster/account
	files = append(files,
		templateFile{apisPackageTemplate, APIsPackageTemplateData{Scope: scope, ModulePath: g.ModulePath, GroupDir: gc.Name}, filepath.Join(apisParentDir, GeneratedFilePrefix+gc.Name+".go")},
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
		scopedBt := bt
		if scope != ScopeCluster {
			scopedBt.ParameterFields = deriveNamespacedReferenceGroups(bt.ParameterFields)
		}
		data := ScopedTemplateData{
			BaseType:      scopedBt,
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

		filename := filepath.Join(pkgPath, GeneratedFilePrefix+strings.ToLower(bt.ResourceName)+"_types.go")
		if err := writeFile(filename, content); err != nil {
			return err
		}
		log.Printf("Generated %s", filename)
	}

	return nil
}

// deriveNamespacedReferenceGroups returns a copy of fields with ReferenceGroup
// converted from cluster group to namespaced group (e.g., "account.btp.sap.crossplane.io" → "account.btp.sap.m.crossplane.io").
// This is correct because namespaced output uses *xpv1.NamespacedReference, which always
// points to namespaced-scoped resources in the derived group.
func deriveNamespacedReferenceGroups(fields []ParameterField) []ParameterField {
	out := make([]ParameterField, len(fields))
	copy(out, fields)
	for i := range out {
		if out[i].IsReferenceValue && out[i].ReferenceGroup != "" {
			out[i].ReferenceGroup = DeriveNamespacedGroup(out[i].ReferenceGroup)
		}
	}
	return out
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

	filename := filepath.Join(pkgPath, GeneratedFilePrefix+"baseimpl.go")
	if err := writeFile(filename, content); err != nil {
		return err
	}
	log.Printf("Generated %s", filename)

	return nil
}

// generateTypeAliases generates a file with type aliases for exported base structs.
func (g *Generator) generateTypeAliases(gc GroupConfig, exportedStructs []string, exportedVars []ExportedVar, scope string) error {
	if len(exportedStructs) == 0 && len(exportedVars) == 0 {
		return nil
	}

	pkgPath := gc.GetPackagePath(scope)

	data := TypesTemplateData{
		BasePkgImport: g.GetBasePkgImport(gc),
		TypeNames:     exportedStructs,
		Vars:          exportedVars,
	}

	content, err := executeTemplate(typesTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to execute types template: %w", err)
	}

	filename := filepath.Join(pkgPath, GeneratedFilePrefix+"types.go")
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
			LogicCtrlDir:  g.LogicCtrlDir,
		}

		content, err := executeTemplate(controllerTemplate, data)
		if err != nil {
			return fmt.Errorf("failed to execute controller template for %s: %w", bt.ResourceName, err)
		}

		filename := filepath.Join(ctrlPkgPath, GeneratedFilePrefix+strings.ToLower(bt.ResourceName)+".go")
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
		BaseTypes:  baseTypes,
		Scope:      scope,
		ModulePath: g.ModulePath,
		GroupDir:   gc.Name,
	}

	content, err := executeTemplate(setupTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to execute setup template: %w", err)
	}

	filename := filepath.Join(ctrlBasePath, GeneratedFilePrefix+"setup.go")
	if err := writeFile(filename, content); err != nil {
		return err
	}
	log.Printf("Generated %s", filename)

	return nil
}

// cleanGeneratedFiles removes all files with the GeneratedFilePrefix from the given directory.
// It is a no-op if the directory does not exist.
func cleanGeneratedFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), GeneratedFilePrefix) {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove stale file %s: %w", path, err)
			}
			log.Printf("Cleaned stale file: %s", path)
		}
	}
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
