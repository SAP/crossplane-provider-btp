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
	"strings"
)

// GroupConfig holds the discovered configuration for a single API group.
type GroupConfig struct {
	Name              string // directory name, e.g., "account"
	ClusterGroupName  string // e.g., "account.btp.sap.crossplane.io"
	NamespacedGroupName string // e.g., "account.btp.sap.m.crossplane.io"
	BasePkg           string // e.g., "apis/base/account/v1alpha1"
	ClusterPkg        string // e.g., "apis/cluster/account/v1alpha1"
	NamespacedPkg     string // e.g., "apis/namespaced/account/v1alpha1"
	ClusterCtrlPkg    string // e.g., "internal/controller/cluster/account"
	NamespacedCtrlPkg string // e.g., "internal/controller/namespaced/account"
}

// Generator generates scope-specific types from base type definitions.
type Generator struct {
	BaseDir           string // e.g., "apis/base"
	ClusterDir        string // e.g., "apis/cluster"
	NamespacedDir     string // e.g., "apis/namespaced"
	ClusterCtrlDir    string // e.g., "internal/controller/cluster"
	NamespacedCtrlDir string // e.g., "internal/controller/namespaced"
	ModulePath        string
	ProviderName      string
	SkipProviderConfig bool
	Groups            []GroupConfig
}

// BaseType represents a parsed base type definition.
type BaseType struct {
	Name            string         // e.g., "BaseDeployment"
	ResourceName    string         // e.g., "Deployment" (without "Base" prefix)
	ParametersType  string         // e.g., "BaseDeploymentParameters"
	ObservationType string         // e.g., "BaseDeploymentObservation"
	References      []Reference    // Cross-resource references
	CustomMethods   []CustomMethod // Custom methods to generate
}

// Reference represents a reference field configuration.
type Reference struct {
	TargetType   string // e.g., "Configuration"
	FieldPath    string // e.g., "Spec.ForProvider.Configuration"
	FieldName    string // e.g., "Configuration" (extracted from FieldPath)
	RefName      string // e.g., "GlobalAccountRef" (override for ref field name, defaults to FieldName + "Ref")
	SelectorName string // e.g., "GlobalAccountSelector" (override for selector field name, defaults to FieldName + "Selector")
}

// CustomMethod represents a custom method defined in the base interface.
type CustomMethod struct {
	Name       string // e.g., "GetID"
	ReturnType string // e.g., "string"
	FieldPath  string // e.g., "Status.AtProvider.ID"
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
}

// ControllerTemplateData holds data passed to controller templates.
type ControllerTemplateData struct {
	BaseType
	Scope         string
	IsCluster     bool
	ModulePath    string
	BasePkgImport string
	GroupDir      string // e.g., "account"
}

// SetupTemplateData holds data passed to setup.go templates.
type SetupTemplateData struct {
	BaseTypes          []BaseType
	Scope              string
	ModulePath         string
	GroupDir           string // e.g., "account"
	SkipProviderConfig bool
}

// ProviderConfigTemplateData holds data passed to providerconfig templates.
type ProviderConfigTemplateData struct {
	Scope        string
	IsCluster    bool
	ModulePath   string
	GroupDir     string // e.g., "account"
	ProviderName string
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
