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

// Scope constants define the supported Kubernetes resource scopes.
const (
	ScopeCluster    = "cluster"
	ScopeNamespaced = "namespaced"
)

// Marker constants define the code generation markers used in source files.
const (
	MarkerGenerateScoped = "+codegen:generate:scoped"
	MarkerReferences     = "+codegen:references"
	MarkerReference      = "+codegen:reference:"
	MarkerMethod         = "+codegen:method:"
)

// Prefix constants for type name parsing.
const (
	PrefixBase    = "Base"
	PrefixManaged = "Managed"
	SuffixList    = "List"
)

// NamespacedGroupInfix is inserted before "crossplane.io" to derive the namespaced group name.
// For example: "account.btp.sap.crossplane.io" → "account.btp.sap.m.crossplane.io"
const NamespacedGroupInfix = "m."

// MarkerGroupName is the kubebuilder marker used in doc.go to declare the API group name.
const MarkerGroupName = "+groupName="

// Standard method names that should be skipped during custom method parsing.
var standardMethods = map[string]bool{
	"GetBaseParameters":  true,
	"SetBaseParameters":  true,
	"GetBaseObservation": true,
	"SetBaseObservation": true,
}
