package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RoleReference struct {
	// RoleTemplateAppId The name of the referenced template app id
	RoleTemplateAppId string `json:"roleTemplateAppId"`
	// RemoteRoleTemplateAppId The name of the referenced remote template
	RoleTemplateName string `json:"roleTemplateName"`
	// Name The name of the referenced role template
	Name string `json:"name"`
}

// RoleCollectionParameters are the configurable fields of a RoleCollection
type BaseRoleCollectionParameters struct {
	// Name of the role collection
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name can't be updated once set"
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	Description *string `json:"description,omitempty"`
	// RoleReferences are the roles that are part of the role collection
	RoleReferences []RoleReference `json:"roles"`
}

// RoleCollectionObservation are the observable fields of a RoleCollection.
type BaseRoleCollectionObservation struct {
	// Name of the role collection as saved in external system
	// +kubebuilder:validation:Optional
	Name *string `json:"name,omitempty"`
	// Description of the role collection as saved in external system
	Description *string `json:"description,omitempty"`
	// RoleReferences roles as saved in the external system
	// +kubebuilder:validation:Optional
	RoleReferences *[]RoleReference `json:"roles"`
}

// BaseRoleCollectionSpec defines the desired state of a RoleCollection.
type BaseRoleCollectionSpec struct {
	ForProvider BaseRoleCollectionParameters `json:"forProvider"`

	XSUAACredentialsReference `json:",inline"`
}

// BaseRoleCollectionStatus represents the observed state of a RoleCollection.
type BaseRoleCollectionStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BaseRoleCollectionObservation `json:"atProvider,omitempty"`
}

// BaseRoleCollection is the base resource definition for RoleCollection.
// A RoleCollection aggregates roles into a single entity to assign it to users / groups
//
// External-Name Configuration:
//   - Follows Standard: no (uses name as identifier, not a GUID)
//   - Format: Role Collection Name (string)
//   - How to find:
//   - UI: BTP Cockpit → Subaccount → Security → Role Collections → [Role Collection Name]
//   - CLI: btp get security/role-collection `"<name>"` → `name`
// +codegen:generate:scoped
// +kubebuilder:object:generate=false
type BaseRoleCollection struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BaseRoleCollectionSpec   `json:"spec"`
	Status            BaseRoleCollectionStatus `json:"status,omitempty"`
}
