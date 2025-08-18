package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

var DirectoryEntityStateOk = "OK"

// DirectoryParameters are the configurable fields of a Directory.
type DirectoryParameters struct {

	// Description of the Directory
	// +optional
	Description *string `json:"description,omitempty"`

	// Additional admins of the directory. Applies only to directories that have the user authorization management feature enabled. Do not add yourself as you are assigned as a directory admin by default. Example: ["admin1@example.com", "admin2@example.com"]
	// +kubebuilder:validation:MinItems=2
	DirectoryAdmins []string `json:"directoryAdmins"`

	// <b>The features to be enabled in the directory. The available features are:</b>
	// -	<b>DEFAULT</b>: (Mandatory) All directories provide the following basic features: (1) Group and filter subaccounts for reports and filters, (2) monitor usage and costs on a directory level (costs only available for contracts that use the consumption-based commercial model), and (3) set custom properties and tags to the directory for identification and reporting purposes.
	// -	<b>ENTITLEMENTS</b>: (Optional) Enables the assignment of a quota for services and applications to the directory from the global account quota for distribution to the subaccounts under this directory.
	// -	<b>AUTHORIZATIONS</b>: (Optional) Allows you to assign users as administrators or viewers of this directory. You must apply this feature in combination with the ENTITLEMENTS feature.
	//
	//
	// IMPORTANT: Your multi-level account hierarchy can have more than one directory enabled with user authorization and/or entitlement management; however, only one directory in any directory path can have these features enabled. In other words, other directories above or below this directory in the same path can only have the default features specified. If you are not sure which features to enable, we recommend that you set only the default features, and then add features later on as they are needed.
	// <br/><b>Valid values:</b>
	// [DEFAULT]
	// [DEFAULT,ENTITLEMENTS]
	// [DEFAULT,ENTITLEMENTS,AUTHORIZATIONS]<br/>
	// Unique: true
	// +optional
	DirectoryFeatures []string `json:"directoryFeatures"`

	// The display name of the directory.
	DisplayName *string `json:"displayName"`

	// JSON array of up to 10 user-defined labels to assign as key-value pairs to the directory. Each label has a name (key) that you specify, and to which you can assign up to 10 corresponding values or leave empty.
	// Keys and values are each limited to 63 characters.
	// Label keys and values are case-sensitive. Try to avoid creating duplicate variants of the same keys or values with a different casing (example: "myValue" and "MyValue").
	//
	// Example:
	// {
	//   "Cost Center": ["19700626"],
	//   "Department": ["Sales"],
	//   "Contacts": ["name1@example.com","name2@example.com"],
	//   "EMEA":[]
	// }
	//
	// +optional
	Labels map[string][]string `json:"labels,omitempty"`

	// Subdomain Applies only to directories that have the user authorization management feature enabled.  The subdomain becomes part of the path used to access the authorization tenant of the directory. Must be unique within the defined region. Use only letters (a-z), digits (0-9), and hyphens (not at start or end). Maximum length is 63 characters. Cannot be changed after the directory has been created.
	// +optional
	Subdomain *string `json:"subdomain,omitempty"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Directory
	// +crossplane:generate:reference:refFieldName=DirectoryRef
	// +crossplane:generate:reference:selectorFieldName=DirectorySelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.DirectoryUuid()
	DirectoryGuid string `json:"directoryGuid,omitempty"`

	// +kubebuilder:validation:Optional
	DirectorySelector *xpv1.Selector `json:"directorySelector,omitempty"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="directoryRef name can't be updated once set"
	DirectoryRef *xpv1.Reference `json:"directoryRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Directory" reference-apiversion:"v1alpha1"`
}

// DirectoryObservation are the observable fields of a Directory.
type DirectoryObservation struct {
	// The GUID of the directory
	Guid *string `json:"guid,omitempty"`

	// Processing state in external	system
	EntityState *string `json:"entityState,omitempty"`
	// Details related to external processing state
	StateMessage *string `json:"stateMessage,omitempty"`
	// Subdomain currently present in external system
	Subdomain *string `json:"subdomain,omitempty"`
	// Features currently present in external system
	DirectoryFeatures []string `json:"directoryFeatures"`
}

// A DirectorySpec defines the desired state of a Directory.
type DirectorySpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       DirectoryParameters `json:"forProvider"`
}

// A DirectoryStatus represents the observed state of a Directory.
type DirectoryStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          DirectoryObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Directory is a managed resource that allows grouping of subaccounts in the SAP Business Technology Platform
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp-account}
type Directory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DirectorySpec   `json:"spec"`
	Status DirectoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DirectoryList contains a list of Directory
type DirectoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Directory `json:"items"`
}

// Directory type metadata.
var (
	DirectoryKind             = reflect.TypeOf(Directory{}).Name()
	DirectoryGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: DirectoryKind}.String()
	DirectoryKindAPIVersion   = DirectoryKind + "." + CRDGroupVersion.String()
	DirectoryGroupVersionKind = CRDGroupVersion.WithKind(DirectoryKind)
)

func init() {
	SchemeBuilder.Register(&Directory{}, &DirectoryList{})
}
