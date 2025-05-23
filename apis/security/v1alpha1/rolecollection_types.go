package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
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
type RoleCollectionParameters struct {
	// Name of the role collection
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	Description *string `json:"description,omitempty"`
	// RoleReferences are the roles that are part of the role collection
	RoleReferences []RoleReference `json:"roles"`
}

// RoleCollectionObservation are the observable fields of a RoleCollection.
type RoleCollectionObservation struct {
	// Name of the role collection as saved in external system
	// +kubebuilder:validation:Optional
	Name *string `json:"name,omitempty"`
	// Description of the role collection as saved in external system
	Description *string `json:"description,omitempty"`
	// RoleReferences roles as saved in the external system
	// +kubebuilder:validation:Optional
	RoleReferences *[]RoleReference `json:"roles"`
}

// A RoleCollectionSpec defines the desired state of a RoleCollection.
type RoleCollectionSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       RoleCollectionParameters `json:"forProvider"`

	XSUAACredentialsReference `json:",inline"`
}

// A RoleCollectionStatus represents the observed state of a RoleCollection.
type RoleCollectionStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          RoleCollectionObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A RoleCollection aggregates roles into a single entity to assign it to users / groups
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type RoleCollection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoleCollectionSpec   `json:"spec"`
	Status RoleCollectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RoleCollectionList contains a list of RoleCollection
type RoleCollectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoleCollection `json:"items"`
}

// RoleCollection type metadata.
var (
	RoleCollectionKind             = reflect.TypeOf(RoleCollection{}).Name()
	RoleCollectionGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: RoleCollectionKind}.String()
	RoleCollectionKindAPIVersion   = RoleCollectionKind + "." + CRDGroupVersion.String()
	RoleCollectionGroupVersionKind = CRDGroupVersion.WithKind(RoleCollectionKind)
)

func init() {
	SchemeBuilder.Register(&RoleCollection{}, &RoleCollectionList{})
}
