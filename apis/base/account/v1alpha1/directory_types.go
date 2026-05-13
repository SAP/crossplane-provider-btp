package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var DirectoryEntityStateOk = "OK"

// BaseDirectoryParameters are the configurable fields of a Directory.
type BaseDirectoryParameters struct {
	// Description of the Directory
	// +optional
	Description *string `json:"description,omitempty"`

	// Additional admins of the directory.
	// +kubebuilder:validation:MinItems=2
	DirectoryAdmins []string `json:"directoryAdmins"`

	// The features to be enabled in the directory.
	// +optional
	DirectoryFeatures []string `json:"directoryFeatures"`

	// The display name of the directory.
	DisplayName *string `json:"displayName"`

	// Labels
	// +optional
	Labels map[string][]string `json:"labels,omitempty"`

	// Subdomain
	// +optional
	Subdomain *string `json:"subdomain,omitempty"`

	// DirectoryGuid is the GUID of the parent directory (empty = direct under global account)
	DirectoryGuid string `json:"directoryGuid,omitempty"`
}

// BaseDirectoryObservation are the observable fields of a Directory.
type BaseDirectoryObservation struct {
	// The GUID of the directory
	Guid *string `json:"guid,omitempty"`

	// Processing state in external system
	EntityState *string `json:"entityState,omitempty"`
	// Details related to external processing state
	StateMessage *string `json:"stateMessage,omitempty"`
	// Subdomain currently present in external system
	Subdomain *string `json:"subdomain,omitempty"`
	// Features currently present in external system
	DirectoryFeatures []string `json:"directoryFeatures"`
}

// BaseDirectorySpec defines the desired state of a Directory.
type BaseDirectorySpec struct {
	ForProvider BaseDirectoryParameters `json:"forProvider"`
}

// BaseDirectoryStatus represents the observed state of a Directory.
type BaseDirectoryStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BaseDirectoryObservation `json:"atProvider,omitempty"`
}

// BaseDirectory is the base resource definition for Directory.
// +codegen:generate:scoped
// +kubebuilder:skip
// +kubebuilder:object:generate=false
type BaseDirectory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BaseDirectorySpec   `json:"spec"`
	Status            BaseDirectoryStatus `json:"status,omitempty"`
}
