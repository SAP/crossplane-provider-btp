// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=import.btp.sap.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Mode controls which resources the function attempts to import.
// +kubebuilder:validation:Enum=auto;explicit
type Mode string

const (
	// ModeAuto attempts import for all supported resources without external-name.
	ModeAuto Mode = "auto"
	// ModeExplicit only attempts import for resources annotated with
	// import.btp.sap.crossplane.io/lookup: "true".
	ModeExplicit Mode = "explicit"
)

// ValueSource resolves a string value from one of three sources.
// Exactly one field must be set — enforced at runtime (Fatal) and by the
// schema's CEL rule below.
// +kubebuilder:validation:XValidation:rule="[has(self.value), has(self.fromFieldPath), has(self.fromContextKey)].filter(x, x).size() == 1",message="exactly one of value, fromFieldPath, or fromContextKey must be set"
type ValueSource struct {
	// Value is a static literal string.
	// +optional
	Value *string `json:"value,omitempty"`

	// FromFieldPath resolves the value from the observed composite resource
	// at the given field path (e.g. "metadata.labels[crossplane.io/claim-namespace]").
	// +optional
	FromFieldPath *string `json:"fromFieldPath,omitempty"`

	// FromContextKey resolves the value from the Crossplane pipeline context
	// at the given key. A prior function in the pipeline must have set this key.
	// +optional
	FromContextKey *string `json:"fromContextKey,omitempty"`
}

// SecretReference tells the function how to locate the CIS credentials secret.
// Both name and namespace must be configured.
type SecretReference struct {
	// Name resolves the Kubernetes Secret name.
	Name ValueSource `json:"name"`

	// Namespace resolves the Kubernetes Secret namespace.
	Namespace ValueSource `json:"namespace"`

	// Key is the data key within the Secret that holds the CIS credentials JSON.
	Key string `json:"key"`
}

// Input is the configuration for function-import-btp.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Mode controls lookup behavior. "auto" attempts import for all supported
	// resources without external-name. "explicit" only attempts import for
	// resources annotated with import.btp.sap.crossplane.io/lookup: "true".
	// +kubebuilder:default=auto
	// +optional
	Mode Mode `json:"mode,omitempty"`

	// Exclude is a list of RE2 regex patterns matched against the composition
	// resource name (the key in the desired map). Matching resources are skipped.
	// +optional
	Exclude []string `json:"exclude,omitempty"`

	// Include is a list of RE2 regex patterns. When non-empty, only resources
	// whose composition resource name matches at least one pattern are considered.
	// Evaluated before Exclude.
	// +optional
	Include []string `json:"include,omitempty"`

	// SecretRef configures how the function locates the CIS provider secret.
	SecretRef SecretReference `json:"secretRef"`
}
