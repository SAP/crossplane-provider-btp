package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseGlobalAccountParameters are the configurable fields of a GlobalAccount.
type BaseGlobalAccountParameters struct {
}

// BaseGlobalAccountObservation are the observable fields of a GlobalAccount.
type BaseGlobalAccountObservation struct {
	// BTP Global Account GUID
	// +optional
	Guid string `json:"guid,omitempty"`
}

// BaseGlobalAccountSpec defines the desired state of a GlobalAccount.
type BaseGlobalAccountSpec struct {
	ForProvider BaseGlobalAccountParameters `json:"forProvider,omitempty"`
}

// BaseGlobalAccountStatus represents the observed state of a GlobalAccount.
type BaseGlobalAccountStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BaseGlobalAccountObservation `json:"atProvider,omitempty"`
}

// BaseGlobalAccount is the base resource definition for GlobalAccount.
// +codegen:generate:scoped
// +kubebuilder:skip
// +kubebuilder:object:generate=false
type BaseGlobalAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BaseGlobalAccountSpec   `json:"spec"`
	Status            BaseGlobalAccountStatus `json:"status,omitempty"`
}
