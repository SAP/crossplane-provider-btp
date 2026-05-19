package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

const (
	ResourceCredentialsClientSecret      = "clientsecret"
	ResourceCredentialsClientId          = "clientid"
	ResourceCredentialsServiceManagerUrl = "sm_url"
	ResourceCredentialsXsuaaUrl          = "tokenurl"
	ResourceCredentialsXsappname         = "xsappname"
	ResourceCredentialsXsuaaUrlSufix     = "tokenurlsuffix"
)

const (
	ServiceManagerBound   = "BOUND"
	ServiceManagerUnbound = "UNBOUND"
)

// ServiceManagerParameters are the configurable fields of a ServiceManager.
type ServiceManagerParameters struct {
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/cluster/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()
	SubaccountGuid string `json:"subaccountGuid,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Subaccount" reference-apiversion:"v1alpha1"`
}

// ServiceManagerObservation are the observable fields of a ServiceManager.
type ServiceManagerObservation struct {
	Status string `json:"status,omitempty"`
}

// A ServiceManagerSpec defines the desired state of a ServiceManager.
type ServiceManagerSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ServiceManagerParameters `json:"forProvider"`
}

// A ServiceManagerStatus represents the observed state of a ServiceManager.
type ServiceManagerStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ServiceManagerObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceManager is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type ServiceManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceManagerSpec   `json:"spec"`
	Status ServiceManagerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceManagerList contains a list of ServiceManager
type ServiceManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceManager `json:"items"`
}

// ServiceManager type metadata.
var (
	ServiceManagerKind             = reflect.TypeOf(ServiceManager{}).Name()
	ServiceManagerGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: ServiceManagerKind}.String()
	ServiceManagerKindAPIVersion   = ServiceManagerKind + "." + CRDGroupVersion.String()
	ServiceManagerGroupVersionKind = CRDGroupVersion.WithKind(ServiceManagerKind)
)

func init() {
	SchemeBuilder.Register(&ServiceManager{}, &ServiceManagerList{})
}
