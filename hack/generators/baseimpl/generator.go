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

// Generate runs the code generation process. Per-version files (types, baseimpl, controllers,
// doc, groupversion_info) are written eagerly; per-group aggregator files (apis_package,
// setup) are written once at the end with the union of base types across all versions.
//
// A resource defined in two versions of the same group fails fast — the controller dir is
// version-agnostic, so two versions would silently overwrite each other.
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

	type aggregate struct {
		versions  []GroupConfig
		baseTypes []BaseType
		seenRes   map[string]string // resourceName -> "<version> (<basePkg>)"
	}
	aggs := make(map[string]*aggregate)
	var groupOrder []string

	for _, gc := range groups {
		baseTypes, exportedStructs, exportedVars, err := parseAll(gc)
		if err != nil {
			return err
		}
		if len(baseTypes) == 0 {
			continue
		}

		a := aggs[gc.Name]
		if a == nil {
			a = &aggregate{seenRes: map[string]string{}}
			aggs[gc.Name] = a
			groupOrder = append(groupOrder, gc.Name)
		}
		for _, bt := range baseTypes {
			origin := gc.Version + " (" + gc.BasePkg + ")"
			if prior, ok := a.seenRes[bt.ResourceName]; ok {
				return fmt.Errorf("group %s: resource %s defined in multiple versions: %s and %s. Controller files for both versions would land in %s/<resource>/, overwriting each other", gc.Name, bt.ResourceName, prior, origin, gc.ClusterCtrlPkg)
			}
			a.seenRes[bt.ResourceName] = origin
		}
		a.versions = append(a.versions, gc)
		a.baseTypes = append(a.baseTypes, baseTypes...)

		for _, scope := range []string{ScopeCluster, ScopeNamespaced} {
			if err := g.generatePerVersion(gc, baseTypes, exportedStructs, exportedVars, scope); err != nil {
				return err
			}
		}
	}

	for _, name := range groupOrder {
		a := aggs[name]
		for _, scope := range []string{ScopeCluster, ScopeNamespaced} {
			if err := g.generateAggregatorFiles(a.versions, a.baseTypes, scope); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseAll runs every parser pass over a single base package.
func parseAll(gc GroupConfig) ([]BaseType, []string, []ExportedVar, error) {
	parser := NewParser(gc.BasePkg)
	baseTypes, err := parser.ParseBaseTypes()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse base types in %s: %w", gc.BasePkg, err)
	}
	if len(baseTypes) == 0 {
		log.Printf("No base types found with %s marker in %s", MarkerGenerateScoped, gc.BasePkg)
		return nil, nil, nil, nil
	}
	exportedStructs, err := parser.ParseExportedStructs()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse exported structs in %s: %w", gc.BasePkg, err)
	}
	exportedVars, err := parser.ParseExportedVarsAndConsts()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse exported vars in %s: %w", gc.BasePkg, err)
	}
	return baseTypes, exportedStructs, exportedVars, nil
}

// discoverGroups scans the base directory for (group, version) pairs. The layout is
// apis/base/<group>/<version>/doc.go containing a `+groupName=<group>.btp.sap.crossplane.io`
// marker. Each version directory becomes its own GroupConfig so the generator can emit
// scoped types into apis/<scope>/<group>/<version>/. Directories without a doc.go marker
// (or without a doc.go at all) are skipped.
//
// One group can contribute multiple GroupConfigs, one per version. Aggregator files
// (apis_package, setup) emit once per group across all versions; per-resource files emit
// per (group, version).
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
		groupPath := filepath.Join(g.BaseDir, groupDir)

		versionEntries, err := os.ReadDir(groupPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read group directory %s: %w", groupPath, err)
		}

		for _, ve := range versionEntries {
			if !ve.IsDir() {
				continue
			}
			version := ve.Name()
			basePkg := filepath.Join(groupPath, version)

			clusterGroupName, err := g.parseGroupName(basePkg)
			if err != nil {
				return nil, fmt.Errorf("failed to parse group name for %s/%s: %w", groupDir, version, err)
			}
			if clusterGroupName == "" {
				log.Printf("Skipping %s/%s: no %s marker found in doc.go", groupDir, version, MarkerGroupName)
				continue
			}

			gc := GroupConfig{
				Name:                groupDir,
				Version:             version,
				ClusterGroupName:    clusterGroupName,
				NamespacedGroupName: DeriveNamespacedGroup(clusterGroupName),
				BasePkg:             basePkg,
				ClusterPkg:          filepath.Join(g.ClusterDir, groupDir, version),
				NamespacedPkg:       filepath.Join(g.NamespacedDir, groupDir, version),
				ClusterCtrlPkg:      filepath.Join(g.ClusterCtrlDir, groupDir),
				NamespacedCtrlPkg:   filepath.Join(g.NamespacedCtrlDir, groupDir),
			}

			groups = append(groups, gc)
			log.Printf("Discovered group: %s/%s (cluster: %s, namespaced: %s)", groupDir, version, gc.ClusterGroupName, gc.NamespacedGroupName)
		}
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

// generatePerVersion emits all per-version files for one (group, version, scope) triple:
// scoped types, type aliases, baseimpl, controllers, and the version's doc.go +
// groupversion_info.go. The per-group aggregator files (apis_package, setup) are emitted
// separately by generateAggregatorFiles.
func (g *Generator) generatePerVersion(gc GroupConfig, baseTypes []BaseType, exportedStructs []string, exportedVars []ExportedVar, scope string) error {
	pkgPath := gc.GetPackagePath(scope)
	ctrlBasePath := gc.GetControllerPackagePath(scope)

	// Clean stale generated files. The version dir is per-version; the per-resource controller
	// dirs below ctrlBasePath are version-agnostic — cleaning them on every version is safe
	// because cleanGeneratedFiles is idempotent.
	if err := cleanGeneratedFiles(pkgPath); err != nil {
		return fmt.Errorf("failed to clean generated files in %s: %w", pkgPath, err)
	}
	for _, bt := range baseTypes {
		if err := cleanGeneratedFiles(filepath.Join(ctrlBasePath, strings.ToLower(bt.ResourceName))); err != nil {
			return fmt.Errorf("failed to clean generated files in %s: %w", ctrlBasePath, err)
		}
	}

	if err := g.generateVersionPackageFiles(gc, scope); err != nil {
		return err
	}
	if err := g.generateScopedTypes(gc, baseTypes, scope); err != nil {
		return err
	}
	if err := g.generateTypeAliases(gc, filterShadowedStructs(exportedStructs, baseTypes), exportedVars, scope); err != nil {
		return err
	}
	if err := g.generateBaseImpl(gc, baseTypes, scope); err != nil {
		return err
	}
	return g.generateControllers(gc, baseTypes, scope)
}

// generateAggregatorFiles emits the per-group, per-scope files that span every version:
// the apis_package file at apis/<scope>/<group>/zz_baseimpl_gen.<group>.go (registers all
// versions' SchemeBuilders) and the setup file at internal/controller/<scope>/<group>/
// zz_baseimpl_gen.setup.go (calls Setup on every resource across all versions).
func (g *Generator) generateAggregatorFiles(versions []GroupConfig, unionBaseTypes []BaseType, scope string) error {
	if len(versions) == 0 {
		return nil
	}
	groupName := versions[0].Name
	apisParentDir := filepath.Dir(versions[0].GetPackagePath(scope))
	ctrlBasePath := versions[0].GetControllerPackagePath(scope)

	for _, dir := range []string{apisParentDir, ctrlBasePath} {
		if err := cleanGeneratedFiles(dir); err != nil {
			return fmt.Errorf("failed to clean generated files in %s: %w", dir, err)
		}
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	versionStrings := make([]string, 0, len(versions))
	for _, gc := range versions {
		versionStrings = append(versionStrings, gc.Version)
	}

	return generateFiles([]templateFile{
		{apisPackageTemplate, APIsPackageTemplateData{Scope: scope, ModulePath: g.ModulePath, GroupDir: groupName, Versions: versionStrings}, filepath.Join(apisParentDir, GeneratedFilePrefix+groupName+".go")},
		{setupTemplate, SetupTemplateData{BaseTypes: unionBaseTypes, Scope: scope, ModulePath: g.ModulePath, GroupDir: groupName}, filepath.Join(ctrlBasePath, GeneratedFilePrefix+"setup.go")},
	})
}

// generateVersionPackageFiles emits the per-version doc.go + groupversion_info.go that live
// inside apis/<scope>/<group>/<version>/. (The per-group apis_package file is emitted by
// generateAggregatorFiles; this function only writes files under the version dir.)
func (g *Generator) generateVersionPackageFiles(gc GroupConfig, scope string) error {
	pkgPath := gc.GetPackagePath(scope)

	if err := os.MkdirAll(pkgPath, 0750); err != nil {
		return fmt.Errorf("failed to create API directory %s: %w", pkgPath, err)
	}

	groupName := gc.GetGroupName(scope)

	files := []templateFile{
		{docTemplate, DocTemplateData{Version: gc.Version}, filepath.Join(pkgPath, GeneratedFilePrefix+"doc.go")},
		{groupVersionInfoTemplate, GroupVersionTemplateData{GroupName: groupName, Version: gc.Version, ProviderName: g.ProviderName}, filepath.Join(pkgPath, GeneratedFilePrefix+"groupversion_info.go")},
	}

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
			Version:       gc.Version,
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
		BaseTypes:          baseTypes,
		Version:            gc.Version,
		ModulePath:         g.ModulePath,
		BasePkgImport:      g.GetBasePkgImport(gc),
		IsCluster:          scope == ScopeCluster,
		NeedsRefConverters: anySpecFieldHasRefs(baseTypes),
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
		Version:       gc.Version,
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
			Version:       gc.Version,
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

// filterShadowedStructs returns the exported base struct names with any names that the
// scoped types template will emit as local copies removed. Without this filter the
// re-export `type X = base.X` collides with the locally-declared `type X struct { ... }`.
func filterShadowedStructs(names []string, baseTypes []BaseType) []string {
	shadowed := make(map[string]struct{})
	for _, bt := range baseTypes {
		for _, sf := range bt.SpecFields {
			if len(sf.Refs) > 0 {
				shadowed[sf.TypeName] = struct{}{}
			}
		}
	}
	if len(shadowed) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if _, skip := shadowed[n]; skip {
			continue
		}
		out = append(out, n)
	}
	return out
}

// anySpecFieldHasRefs reports whether any BaseType has a SpecField with reference
// markers (i.e. a sidecar entry pointed at it). Drives whether the namespaced base_impl
// template emits the *xpv1.Reference / *xpv1.NamespacedReference conversion helpers.
func anySpecFieldHasRefs(baseTypes []BaseType) bool {
	for _, bt := range baseTypes {
		for _, sf := range bt.SpecFields {
			if len(sf.Refs) > 0 {
				return true
			}
		}
	}
	return false
}
