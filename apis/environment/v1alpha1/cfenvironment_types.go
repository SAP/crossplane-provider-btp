package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

const (
	InstanceStateOk       = "OK"
	InstanceStateCreating = "CREATING"
	InstanceStateDeleting = "DELETING"
	InstanceStateUpdating = "UPDATING"
)

const (
	ResourceAPIEndpoint = "apiEndpoint"
	ResourceOrgId       = "orgId"
	ResourceOrgName     = "orgName"
	ResourceRaw         = "__raw"
)

// User identifies a user by username and origin
type User struct {
	// Username at the identity provider
	Username string `json:"username"`
	// +kubebuilder:default=sap.ids
	// Origin picks the IDP
	Origin string `json:"origin,omitempty"`
}

// String return a formatted string of User
func (u *User) String() string {
	// todo: default origin to "sap.ids", replace this with scim lookup
	if u.Origin == "" {
		u.Origin = "sap.ids"
	}
	return u.Username + " (" + u.Origin + ")"
}

// CfEnvironmentParameters are the configurable fields of a CloudFoundryEnvironment.
type CfEnvironmentParameters struct {
	// A list of users (with username/email and origin) to assign as the Org Manager role.
	// Cannot be updated after creation --> initial creation only
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="OrgManagers can't be updated once set"
	// +optional
	Managers []string `json:"initialOrgManagers,omitempty"`

	// Landscape, region of the cloud foundry org, e.g. cf-eu12
	// must be set, when cloud foundry name is set
	// +kubebuilder:validation:MinLength=1
	// +optional
	Landscape string `json:"landscape,omitempty"`

	// Org name of the Cloud Foundry environment
	// +optional
	OrgName string `json:"orgName,omitempty"`

	// CF environment instance name
	// +optional
	EnvironmentName string `json:"environmentName,omitempty"`
}

// CfEnvironmentObservation  are the observable fields of a CloudFoundryEnvironment.
type CfEnvironmentObservation struct {
	EnvironmentObservation `json:",inline"`
	Managers               []User `json:"managers,omitempty"`
}

// A CfEnvironmentSpec defines the desired state of a CloudFoundryEnvironment.
type CfEnvironmentSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       CfEnvironmentParameters `json:"forProvider"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()
	SubaccountGuid string `json:"subaccountGuid,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Subaccount" reference-apiversion:"v1alpha1"`

	// +kubebuilder:validation:Optional
	CloudManagementSelector *xpv1.Selector `json:"cloudManagementSelector,omitempty"`
	// +kubebuilder:validation:Optional
	CloudManagementRef *xpv1.Reference `json:"cloudManagementRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"CloudManagement" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagement
	// +crossplane:generate:reference:refFieldName=CloudManagementRef
	// +crossplane:generate:reference:selectorFieldName=CloudManagementSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagementSecret()
	CloudManagementSecret string `json:"cloudManagemxentSecret,omitempty"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagement
	// +crossplane:generate:reference:refFieldName=CloudManagementRef
	// +crossplane:generate:reference:selectorFieldName=CloudManagementSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagementSecretSecretNamespace()
	CloudManagementSecretNamespace string `json:"cloudManagementSecretNamespace,omitempty"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagement
	// +crossplane:generate:reference:refFieldName=CloudManagementRef
	// +crossplane:generate:reference:selectorFieldName=CloudManagementSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagementSubaccountUuid()
	CloudManagementSubaccountGuid string `json:"cloudManagementSubaccountGuid,omitempty"`
}

// A EnvironmentStatus represents the observed state of a CloudFoundryEnvironment.
type EnvironmentStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          CfEnvironmentObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A CloudFoundryEnvironment is a managed resource that represents a Cloud Foundry environment in the SAP Business Technology Platform
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,sap}
type CloudFoundryEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CfEnvironmentSpec `json:"spec"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudFoundryEnvironmentList contains a list of CloudFoundryEnvironment
type CloudFoundryEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudFoundryEnvironment `json:"items"`
}

// CloudFoundryEnvironment type metadata.
var (
	CfEnvironmentKind             = reflect.TypeOf(CloudFoundryEnvironment{}).Name()
	CfEnvironmentGroupKind        = schema.GroupKind{Group: Group, Kind: CfEnvironmentKind}.String()
	CfEnvironmentKindAPIVersion   = CfEnvironmentKind + "." + SchemeGroupVersion.String()
	CfEnvironmentGroupVersionKind = SchemeGroupVersion.WithKind(CfEnvironmentKind)
)

func init() {
	SchemeBuilder.Register(&CloudFoundryEnvironment{}, &CloudFoundryEnvironmentList{})
}
