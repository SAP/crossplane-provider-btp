package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoleReference describes a role that belongs to a role collection.
type RoleReference struct {
	// RoleTemplateAppId The name of the referenced template app id
	RoleTemplateAppId string `json:"roleTemplateAppId"`
	// RoleTemplateName The name of the referenced role template
	RoleTemplateName string `json:"roleTemplateName"`
	// Name The name of the referenced role
	Name string `json:"name"`
}

// BaseRoleCollectionParameters are the configurable fields of a RoleCollection.
type BaseRoleCollectionParameters struct {
	// Name of the role collection
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name can't be updated once set"
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	Description *string `json:"description,omitempty"`
	// RoleReferences are the roles that are part of the role collection
	RoleReferences []RoleReference `json:"roles"`
}

// BaseRoleCollectionObservation are the observable fields of a RoleCollection.
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
}

// BaseRoleCollectionStatus represents the observed state of a RoleCollection.
type BaseRoleCollectionStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BaseRoleCollectionObservation `json:"atProvider,omitempty"`
}

// BaseRoleCollection is the base resource definition for RoleCollection.
// +codegen:generate:scoped
// +kubebuilder:skip
// +kubebuilder:object:generate=false
type BaseRoleCollection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BaseRoleCollectionSpec   `json:"spec"`
	Status            BaseRoleCollectionStatus `json:"status,omitempty"`
}
