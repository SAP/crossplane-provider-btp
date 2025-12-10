package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// GlobalAccountParameters are the configurable fields of a GlobalAccount.
type GlobalAccountParameters struct {
}

// GlobalAccountObservation are the observable fields of a GlobalAccount.
type GlobalAccountObservation struct {
	// BTP Global Account GUID
	// +optional
	Guid string `json:"guid,omitempty"`
}

// A GlobalAccountSpec defines the desired state of a GlobalAccount.
type GlobalAccountSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       GlobalAccountParameters `json:"forProvider,omitempty"`
}

// A GlobalAccountStatus represents the observed state of a GlobalAccount.
type GlobalAccountStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          GlobalAccountObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A GlobalAccount is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,sap}
// +kubebuilder:deprecatedversion:warning="Use globalaccount reference in providerconfig instead"
type GlobalAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlobalAccountSpec   `json:"spec"`
	Status GlobalAccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GlobalAccountList contains a list of GlobalAccount
type GlobalAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalAccount `json:"items"`
}

// GlobalAccount type metadata.
var (
	GlobalAccountKind             = reflect.TypeOf(GlobalAccount{}).Name()
	GlobalAccountGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: GlobalAccountKind}.String()
	GlobalAccountKindAPIVersion   = GlobalAccountKind + "." + CRDGroupVersion.String()
	GlobalAccountGroupVersionKind = CRDGroupVersion.WithKind(GlobalAccountKind)
)

func init() {
	SchemeBuilder.Register(&GlobalAccount{}, &GlobalAccountList{})
}
