package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseSubaccountParameters are the configurable fields of a Subaccount.
type BaseSubaccountParameters struct {
	// enable beta services and applications?
	// +optional
	// +immutable
	BetaEnabled bool `json:"betaEnabled,omitempty"`

	// Description
	// +optional
	// +kubebuilder:validation:MinLength=1
	Description string `json:"description,omitempty"`

	// Display name
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// Labels, up to 10 user-defined labels to assign as key-value pairs to the subaccount.
	// +optional
	Labels map[string][]string `json:"labels,omitempty"`

	// Region
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// Admins for the subaccount (service account user already included)
	// +kubebuilder:validation:MinItems=1
	SubaccountAdmins []string `json:"subaccountAdmins"`

	// Subdomain
	// +kubebuilder:validation:MinLength=1
	Subdomain string `json:"subdomain"`

	// Used for production
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Enum=NOT_USED_FOR_PRODUCTION;USED_FOR_PRODUCTION;UNSET
	// +kubebuilder:default:=UNSET
	UsedForProduction string `json:"usedForProduction,omitempty"`

	// GlobalAccountGuid is the GUID of the global account
	GlobalAccountGuid string `json:"globalAccountGuid,omitempty"`

	// DirectoryGuid is the GUID of the parent directory (empty = direct under global account)
	DirectoryGuid string `json:"directoryGuid,omitempty"`
}

// BaseSubaccountObservation are the observable fields of a Subaccount.
type BaseSubaccountObservation struct {
	// Subaccount ID
	// +optional
	SubaccountGuid *string `json:"subaccountGuid,omitempty"`
	// Subaccount Status
	// +optional
	Status *string `json:"status,omitempty"`
	// Subaccount StatusMessage
	// +optional
	StatusMessage *string `json:"statusMessage,omitempty"`

	// enable beta services and applications?
	// +optional
	BetaEnabled *bool `json:"betaEnabled,omitempty"`

	// Description
	// +optional
	Description *string `json:"description,omitempty"`

	// Display name
	DisplayName *string `json:"displayName,omitempty"`

	// Labels
	// +optional
	Labels *map[string][]string `json:"labels,omitempty"`

	// Region
	Region *string `json:"region,omitempty"`

	// Admins for the subaccount
	SubaccountAdmins *[]string `json:"subaccountAdmins,omitempty"`

	// Subdomain
	Subdomain *string `json:"subdomain,omitempty"`

	// Used for production
	UsedForProduction *string `json:"usedForProduction,omitempty"`

	// Guid of directory the subaccount is stored in or otherwise ID of the globalaccount
	ParentGuid *string `json:"parentGuid,omitempty"`

	// The unique ID of the subaccount's global account.
	GlobalAccountGUID *string `json:"globalAccountGUID,omitempty"`
}

// BaseSubaccountSpec defines the desired state of a Subaccount.
type BaseSubaccountSpec struct {
	ForProvider BaseSubaccountParameters `json:"forProvider"`
}

// BaseSubaccountStatus represents the observed state of a Subaccount.
type BaseSubaccountStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BaseSubaccountObservation `json:"atProvider,omitempty"`
}

// BaseSubaccount is the base resource definition for Subaccount.
// +codegen:generate:scoped
// +kubebuilder:skip
// +kubebuilder:object:generate=false
type BaseSubaccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BaseSubaccountSpec   `json:"spec"`
	Status            BaseSubaccountStatus `json:"status,omitempty"`
}

// SubaccountReferences defines reference configuration for code generation.
// TODO: Activate by adding + prefix to codegen:references marker once the baseimpl
// generator supports generating ResolveReferences (angryjet cannot handle promoted fields).
// codegen:references
// +kubebuilder:object:generate=false
type SubaccountReferences struct {
	// codegen:reference:target=GlobalAccount,field=Spec.ForProvider.GlobalAccountGuid,refName=GlobalAccountRef,selectorName=GlobalAccountSelector
	GlobalAccountRef bool

	// codegen:reference:target=Directory,field=Spec.ForProvider.DirectoryGuid,refName=DirectoryRef,selectorName=DirectorySelector
	DirectoryRef bool
}
