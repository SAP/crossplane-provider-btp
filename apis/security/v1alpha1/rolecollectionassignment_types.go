package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// RoleCollectionAssignmentParameters are the configurable fields of a RoleCollectionAssignment.
// +kubebuilder:validation:XValidation:rule=(has(self.userName) && !has(self.groupName)) || (!has(self.userName) && has(self.groupName)), message="use either userName or groupName, not both"
type RoleCollectionAssignmentParameters struct {
	// Origin of the user or group
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="origin can't be updated once set"
	Origin string `json:"origin"`
	// UserName of the user to assign the role collection to
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="userName can't be updated once set"
	UserName string `json:"userName,omitempty"`
	// GroupName of the group to assign the role collection to
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="groupName can't be updated once set"
	GroupName string `json:"groupName,omitempty"`
	// RoleCollectionName is the name of the role collection to assign
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="roleCollectionName can't be updated once set"
	RoleCollectionName string `json:"roleCollectionName"`
}

// APICredentials are the credentials to authenticate against the xsuaa api
type APICredentials struct {
	// Source of the credentials.
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source xpv1.CredentialsSource `json:"source"`

	xpv1.CommonCredentialSelectors `json:",inline"`
}

// RoleCollectionAssignmentObservation are the observable fields of a RoleCollectionAssignment.
type RoleCollectionAssignmentObservation struct {
}

// A RoleCollectionAssignmentSpec defines the desired state of a RoleCollectionAssignment.
type RoleCollectionAssignmentSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       RoleCollectionAssignmentParameters `json:"forProvider"`

	// xsuaa api credentials used to manage the assignment
	APICredentials APICredentials `json:"apiCredentials"`

	// +kubebuilder:validation:Optional
	SubaccountApiCredentialSelector *xpv1.Selector `json:"subaccountApiCredentialSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountApiCredentialRef *xpv1.Reference `json:"subaccountApiCredentialRef,omitempty" reference-group:"security.btp.sap.crossplane.io" reference-kind:"SubaccountApiCredential" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredential
	// +crossplane:generate:reference:refFieldName=SubaccountApiCredentialRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountApiCredentialSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredentialSecret()
	SubaccountApiCredentialSecret string `json:"subaccountApiCredentialSecret,omitempty"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredential
	// +crossplane:generate:reference:refFieldName=SubaccountApiCredentialRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountApiCredentialSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredentialSecretSecretNamespace()
	SubaccountApiCredentialSecretNamespace string `json:"subaccountApiCredentialSecretNamespace,omitempty"`
}

// A RoleCollectionAssignmentStatus represents the observed state of a RoleCollectionAssignment.
type RoleCollectionAssignmentStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          RoleCollectionAssignmentObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A RoleCollectionAssignment assigns a role collection to a user or group
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type RoleCollectionAssignment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoleCollectionAssignmentSpec   `json:"spec"`
	Status RoleCollectionAssignmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RoleCollectionAssignmentList contains a list of RoleCollectionAssignment
type RoleCollectionAssignmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoleCollectionAssignment `json:"items"`
}

// RoleCollectionAssignment type metadata.
var (
	RoleCollectionAssignmentKind             = reflect.TypeOf(RoleCollectionAssignment{}).Name()
	RoleCollectionAssignmentGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: RoleCollectionAssignmentKind}.String()
	RoleCollectionAssignmentKindAPIVersion   = RoleCollectionAssignmentKind + "." + CRDGroupVersion.String()
	RoleCollectionAssignmentGroupVersionKind = CRDGroupVersion.WithKind(RoleCollectionAssignmentKind)
)

func init() {
	SchemeBuilder.Register(&RoleCollectionAssignment{}, &RoleCollectionAssignmentList{})
}
