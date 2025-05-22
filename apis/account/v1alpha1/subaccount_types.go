package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// SubaccountParameters are the configurable fields of a Subaccount.
type SubaccountParameters struct {
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

	// Admins for the subaccount (service account user already included)
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

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.GlobalAccount
	// +crossplane:generate:reference:refFieldName=GlobalAccountRef
	// +crossplane:generate:reference:selectorFieldName=GlobalAccountSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.GlobalAccountUuid()
	GlobalAccountGuid string `json:"globalAccountGuid,omitempty"`

	// +kubebuilder:validation:Optional
	GlobalAccountSelector *xpv1.Selector `json:"globalAccountSelector,omitempty"`
	// GlobalAccountRef is deprecated, please use globalAccount field in the ProviderConfig spec instead and leave this field empty.
	// +kubebuilder:validation:Optional
	GlobalAccountRef *xpv1.Reference `json:"globalAccountRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"GlobalAccount" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Directory
	// +crossplane:generate:reference:refFieldName=DirectoryRef
	// +crossplane:generate:reference:selectorFieldName=DirectorySelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.DirectoryUuid()
	DirectoryGuid string `json:"directoryGuid,omitempty"`

	// +kubebuilder:validation:Optional
	DirectorySelector *xpv1.Selector `json:"directorySelector,omitempty"`
	// DirectoryRef allows grouping subaccounts into directories. If unset subaccount will be placed in globalaccount directly
	// Please note: The provider supports moving subaccounts between directories if you supply `resolve: Always` as a policy in this ref
	// +kubebuilder:validation:Optional
	DirectoryRef *xpv1.Reference `json:"directoryRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Directory" reference-apiversion:"v1alpha1"`
}

// SubaccountObservation are the observable fields of a Subaccount.
type SubaccountObservation struct {
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
	// +immutable
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

// A SubaccountSpec defines the desired state of a Subaccount.
type SubaccountSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       SubaccountParameters `json:"forProvider"`
}

// A SubaccountStatus represents the observed state of a Subaccount.
type SubaccountStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          SubaccountObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Subaccount is a managed resource that represents a subaccount in the SAP Business Technology Platform
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,sap}
type Subaccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubaccountSpec   `json:"spec"`
	Status SubaccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubaccountList contains a list of Subaccount
type SubaccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subaccount `json:"items"`
}

// Subaccount type metadata.
var (
	SubaccountKind             = reflect.TypeOf(Subaccount{}).Name()
	SubaccountGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: SubaccountKind}.String()
	SubaccountKindAPIVersion   = SubaccountKind + "." + CRDGroupVersion.String()
	SubaccountGroupVersionKind = CRDGroupVersion.WithKind(SubaccountKind)
)

func init() {
	SchemeBuilder.Register(&Subaccount{}, &SubaccountList{})
}
