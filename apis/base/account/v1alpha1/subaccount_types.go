package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SubaccountParameters are the configurable fields of a Subaccount.
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

	// Labels, up to 10 user-defined labels to assign as key-value pairs to the subaccount. Each label has a name (key) that you specify, and to which you can assign up to 10 corresponding values or leave empty.
	// Keys and values are each limited to 63 characters.
	// +optional
	Labels map[string][]string `json:"labels,omitempty"`

	// Region
	// Change requires recreation
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="subaccountAdmins can't be updated once set"
	SubaccountAdmins []string `json:"subaccountAdmins"`

	// Subdomain
	// +kubebuilder:validation:MinLength=1
	Subdomain string `json:"subdomain"`

	// Used for production
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Enum=NOT_USED_FOR_PRODUCTION;USED_FOR_PRODUCTION;UNSET
	// +kubebuilder:default:=UNSET
	UsedForProduction string `json:"usedForProduction,omitempty"`

	// +optional
	GlobalAccountGuid string `json:"globalAccountGuid,omitempty"`

	// +optional
	DirectoryGuid string `json:"directoryGuid,omitempty"`
}

// SubaccountObservation are the observable fields of a Subaccount.
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

	// Labels, up to 10 user-defined labels to assign as key-value pairs to the subaccount. Each label has a name (key) that you specify, and to which you can assign up to 10 corresponding values or leave empty.
	// Keys and values are each limited to 63 characters.
	// +optional
	Labels *map[string][]string `json:"labels,omitempty"`

	// Region
	// Change requires recreation
	Region *string `json:"region,omitempty"`

	// Admins for the subaccount (service account user already included)
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
// A Subaccount is a managed resource that represents a subaccount in the SAP Business Technology Platform.
//
// External-Name Configuration:
//   - Follows Standard: yes
//   - Format: Subaccount GUID (UUID format)
//   - How to find:
//   - UI: Global Account → Account Explorer → Subaccounts → [Select Subaccount] → Subaccount ID
//   - CLI: btp list accounts/subaccount (field: guid)
//
// +codegen:generate:scoped
// +codegen:categories=sap
// +kubebuilder:object:generate=false
type BaseSubaccount struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BaseSubaccountSpec   `json:"spec"`
	Status            BaseSubaccountStatus `json:"status,omitempty"`
}

// SubaccountReferences defines reference configuration for code generation.
// +codegen:references
// +kubebuilder:object:generate=false
type SubaccountReferences struct {
	// +codegen:reference:target=GlobalAccount
	// +codegen:reference:group=account.btp.sap.crossplane.io
	// +codegen:reference:apiversion=v1alpha1
	// +codegen:reference:field=Spec.ForProvider.GlobalAccountGuid
	// +codegen:reference:refName=GlobalAccountRef
	// +codegen:reference:selectorName=GlobalAccountSelector
	// +codegen:reference:refDescription=GlobalAccountRef is deprecated, please use globalAccount field in the ProviderConfig spec instead and leave this field empty.
	GlobalAccountRef bool

	// +codegen:reference:target=Directory
	// +codegen:reference:group=account.btp.sap.crossplane.io
	// +codegen:reference:apiversion=v1alpha1
	// +codegen:reference:field=Spec.ForProvider.DirectoryGuid
	// +codegen:reference:refName=DirectoryRef
	// +codegen:reference:selectorName=DirectorySelector
	// +codegen:reference:refDescription=DirectoryRef allows grouping subaccounts into directories. If unset subaccount will be placed in globalaccount directly\nPlease note: The provider supports moving subaccounts between directories if you supply `resolve: Always` as a policy in this ref
	DirectoryRef bool
}
