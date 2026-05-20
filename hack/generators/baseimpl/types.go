package main

import (
	"strings"
)

// GroupConfig holds the discovered configuration for a single API group.
type GroupConfig struct {
	Name                string // directory name, e.g., "account"
	ClusterGroupName    string // e.g., "account.btp.sap.crossplane.io"
	NamespacedGroupName string // e.g., "account.btp.sap.m.crossplane.io"
	BasePkg             string // e.g., "apis/base/account/v1alpha1"
	ClusterPkg          string // e.g., "apis/cluster/account/v1alpha1"
	NamespacedPkg       string // e.g., "apis/namespaced/account/v1alpha1"
	ClusterCtrlPkg      string // e.g., "internal/controller/cluster/account"
	NamespacedCtrlPkg   string // e.g., "internal/controller/namespaced/account"
}

// Generator generates scope-specific types from base type definitions.
type Generator struct {
	BaseDir           string // e.g., "apis/base"
	ClusterDir        string // e.g., "apis/cluster"
	NamespacedDir     string // e.g., "apis/namespaced"
	LogicCtrlDir      string // e.g., "internal/controller/logic"
	ClusterCtrlDir    string // e.g., "internal/controller/cluster"
	NamespacedCtrlDir string // e.g., "internal/controller/namespaced"
	ModulePath        string
	ProviderName      string
	Groups            []GroupConfig
}

// SpecField represents an extra field in the BaseXxxSpec struct beyond ForProvider.
type SpecField struct {
	Name     string // field name (empty for anonymous/embedded fields)
	TypeName string // type name, e.g., "XSUAACredentialsReference"
	JSONTag  string // raw struct tag string, e.g., `json:",inline"`
	Embedded bool   // true if anonymous embed

	// Refs is the list of reference fields whose value/ref/selector live inside the struct named TypeName.
	// When non-empty, the scoped types generator emits a LOCAL copy of TypeName in the scoped package
	// (with angryjet `+crossplane:generate:reference:*` markers) instead of embedding the base type.
	// Fields then describes ALL fields of the local struct so the template can render them faithfully.
	Refs   []Reference
	Fields []ParameterField
}

// AccessName returns the Go expression to access this field (type name for embeds, field name otherwise).
func (sf SpecField) AccessName() string {
	if sf.Embedded {
		return sf.TypeName
	}
	return sf.Name
}

// BaseType represents a parsed base type definition.
type BaseType struct {
	Name               string           // e.g., "BaseDeployment"
	ResourceName       string           // e.g., "Deployment" (without "Base" prefix)
	ParametersType     string           // e.g., "BaseDeploymentParameters"
	ObservationType    string           // e.g., "BaseDeploymentObservation"
	DocComment         []string         // Multi-line doc comment for the generated type (without // prefix)
	References         []Reference      // Cross-resource references
	CustomMethods      []CustomMethod   // Custom methods to generate
	SpecFields         []SpecField      // Extra fields in BaseXxxSpec beyond ForProvider
	ParameterFields    []ParameterField // All fields from BaseXxxParameters (used when References exist)
	Categories         string           // Override for resource categories (empty = use ProviderName)
	DeprecatedWarning  string           // Deprecation warning message (empty = not deprecated)
	ForProviderOmitTag bool             // True if ForProvider has ,omitempty in the base spec
}

// Reference represents a reference field configuration.
type Reference struct {
	TargetType     string // e.g., "Directory" or fully-qualified "github.com/.../v1alpha1.SubaccountApiCredential"
	Group          string // e.g., "account.btp.sap.crossplane.io" (required, used for tracking struct tags)
	ApiVersion     string // e.g., "v1alpha1" (required, used for tracking struct tags)
	FieldPath      string // e.g., "Spec.ForProvider.DirectoryGuid" or "Spec.XSUAACredentialsReference.SubaccountApiCredentialSecret"
	FieldName      string // e.g., "DirectoryGuid" (extracted from FieldPath; last path segment)
	SpecFieldName  string // empty if reference lives in ForProvider; otherwise the embedded SpecField type name (e.g., "XSUAACredentialsReference")
	Extractor      string // optional fully-qualified extractor expression, e.g. "github.com/.../v1alpha1.SubaccountApiCredentialSecret()". Empty means default ExternalName().
	RefName        string // e.g., "DirectoryRef" (override for ref field name, defaults to FieldName + "Ref")
	SelectorName   string // e.g., "DirectorySelector" (override for selector field name, defaults to FieldName + "Selector")
	RefDescription string // Optional description for the Ref field (overrides the default from xpv1.Reference)
	ImmutableRef   bool   // If true, emit XValidation marker on the Ref field to prevent updates
}

// CustomMethod represents a custom method defined in the base interface.
type CustomMethod struct {
	Name       string // e.g., "GetID"
	ReturnType string // e.g., "string"
	FieldPath  string // e.g., "Status.AtProvider.ID"
}

// ParameterField represents a field from BaseXxxParameters, used for full struct generation.
type ParameterField struct {
	Name                string   // e.g., "DisplayName"
	TypeExpr            string   // e.g., "string", "map[string][]string", "[]string"
	Tag                 string   // raw struct tag, e.g., `json:"displayName"`
	Comments            []string // doc comment lines (without // prefix)
	IsReferenceValue    bool     // true if this field is a reference value (matches a Reference.FieldName)
	ReferenceTarget     string   // e.g., "GlobalAccount" or fully-qualified path (only set if IsReferenceValue)
	ReferenceGroup      string   // e.g., "account.btp.sap.crossplane.io" (only set if IsReferenceValue)
	ReferenceApiVersion string   // e.g., "v1alpha1" (only set if IsReferenceValue)
	RefName             string   // e.g., "GlobalAccountRef" (only set if IsReferenceValue)
	SelectorName        string   // e.g., "GlobalAccountSelector" (only set if IsReferenceValue)
	RefDescription      string   // e.g., "DirectoryRef allows..." (only set if IsReferenceValue)
	ImmutableRef        bool     // If true, emit XValidation on Ref field (only set if IsReferenceValue)
	Extractor           string   // optional fully-qualified extractor expression (only set if IsReferenceValue)

	// Embedded indicates this is an anonymous (embedded) field on a sub-struct (e.g. xpv1.CommonCredentialSelectors).
	// When true, the template emits the type name only (no field name) with the JSON tag.
	Embedded bool
	// IsRefField indicates this field is the *Ref pointer (xpv1.Reference) — used so the template can
	// emit it without the +crossplane:generate:reference markers but with the reference-* struct tags.
	IsRefField bool
	// IsSelectorField indicates this field is the *Selector pointer (xpv1.Selector).
	IsSelectorField bool
}

// ScopedTemplateData holds data passed to scoped type templates.
type ScopedTemplateData struct {
	BaseType
	Scope         string
	IsCluster     bool
	ModulePath    string
	BasePkgImport string
	ProviderName  string
}

// BaseImplTemplateData holds data passed to base implementation templates.
type BaseImplTemplateData struct {
	BaseTypes     []BaseType
	ModulePath    string
	BasePkgImport string
	IsCluster     bool
	// NeedsRefConverters is true when at least one BaseType has a SpecField with Refs,
	// in which case the namespaced base_impl template emits ref/selector conversion helpers.
	NeedsRefConverters bool
}

// ControllerTemplateData holds data passed to controller templates.
type ControllerTemplateData struct {
	BaseType
	Scope         string
	IsCluster     bool
	ModulePath    string
	BasePkgImport string
	GroupDir      string // e.g., "account"
	LogicCtrlDir  string // e.g., "internal/controller/logic"
}

// SetupTemplateData holds data passed to setup.go templates.
type SetupTemplateData struct {
	BaseTypes  []BaseType
	Scope      string
	ModulePath string
	GroupDir   string // e.g., "account"
}

// GroupVersionTemplateData holds data passed to groupversion_info templates.
type GroupVersionTemplateData struct {
	GroupName    string
	ProviderName string
}

// APIsPackageTemplateData holds data passed to apis package templates.
type APIsPackageTemplateData struct {
	Scope      string
	ModulePath string
	GroupDir   string // e.g., "account"
}

// ExportedVar represents an exported var or const from the base package.
type ExportedVar struct {
	Name    string
	IsConst bool
}

// TypesTemplateData holds data passed to the types alias template.
type TypesTemplateData struct {
	BasePkgImport string
	TypeNames     []string
	Vars          []ExportedVar
}

// NewBaseType creates a BaseType from a type name.
func NewBaseType(name string) BaseType {
	resourceName := strings.TrimPrefix(name, PrefixBase)
	return BaseType{
		Name:            name,
		ResourceName:    resourceName,
		ParametersType:  PrefixBase + resourceName + "Parameters",
		ObservationType: PrefixBase + resourceName + "Observation",
	}
}

// GetPackagePath returns the API package path for the given group and scope.
func (g *GroupConfig) GetPackagePath(scope string) string {
	if scope == ScopeCluster {
		return g.ClusterPkg
	}
	return g.NamespacedPkg
}

// GetControllerPackagePath returns the controller package path for the given group and scope.
func (g *GroupConfig) GetControllerPackagePath(scope string) string {
	if scope == ScopeCluster {
		return g.ClusterCtrlPkg
	}
	return g.NamespacedCtrlPkg
}

// GetGroupName returns the API group name for the given scope.
func (g *GroupConfig) GetGroupName(scope string) string {
	if scope == ScopeCluster {
		return g.ClusterGroupName
	}
	return g.NamespacedGroupName
}

// GetBasePkgImport returns the full import path for the base package.
func (gen *Generator) GetBasePkgImport(gc GroupConfig) string {
	return gen.ModulePath + "/" + gc.BasePkg
}

// DeriveNamespacedGroup derives the namespaced group name from a cluster group name.
// e.g., "account.btp.sap.crossplane.io" → "account.btp.sap.m.crossplane.io"
func DeriveNamespacedGroup(clusterGroup string) string {
	const suffix = "crossplane.io"
	if idx := strings.LastIndex(clusterGroup, suffix); idx > 0 {
		return clusterGroup[:idx] + NamespacedGroupInfix + suffix
	}
	return clusterGroup + "." + NamespacedGroupInfix + "namespaced"
}
