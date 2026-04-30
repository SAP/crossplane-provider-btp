package v1beta1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

const (
	ResourceCredentialsClientSecret      = "clientsecret"
	ResourceCredentialsClientId          = "clientid"
	ResourceCredentialsServiceManagerUrl = "sm_url"
	ResourceCredentialsXsuaaUrl          = "tokenurl"
	ResourceCredentialsXsappname         = "xsappname"
	ResourceCredentialsXsuaaUrlSufix     = "tokenurlsuffix"

	DefaultPlanName = "service-operator-access"

	DefaultServiceInstanceName = "managed-service-manager"
	DefaultServiceBindingName  = "managed-service-manager-binding"
)

const (
	ServiceManagerBound   = "BOUND"
	ServiceManagerUnbound = "UNBOUND"
)

// ServiceManagerParameters are the configurable fields of a ServiceManager.
type ServiceManagerParameters struct {
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()
	SubaccountGuid string `json:"subaccountGuid,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Subaccount" reference-apiversion:"v1alpha1"`

	// Planname for service manager instance
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Enum=subaccount-admin;service-operator-access;container;subaccount-audit
	// +kubebuilder:default:=service-operator-access
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="planName can't be updated once set"
	PlanName string `json:"planName,omitempty"`

	// Name of created service instance, Defaults to "managed-service-manager"
	ServiceInstanceName string `json:"serviceInstanceName,omitempty"`
	// Name of created service binding, Defaults to "managed-service-manager-binding"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceBindingName can't be updated once set"
	ServiceBindingName string `json:"serviceBindingName,omitempty"`
}

type DataSourceLookup struct {
	ServiceManagerPlanID string `json:"serviceManagerPlanID,omitempty"`
}

// ServiceManagerObservation are the observable fields of a ServiceManager.
type ServiceManagerObservation struct {
	// currently bound to a service manager instance or not (BOUND/UNBOUND)
	Status string `json:"status,omitempty"`
	// currently bound service instance id
	ServiceInstanceID string `json:"serviceInstanceID,omitempty"`
	// currently bound service binding id
	ServiceBindingID string `json:"serviceBindingID,omitempty"`

	DataSourceLookup *DataSourceLookup `json:"dataSourceLookup,omitempty"`
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

// A ServiceManager is a managed resource that represents a service manager instance and its API credentials in the SAP Business Technology Platform
//
// External-Name Configuration:
//   - Follows Standard: yes
//   - Format: `<serviceInstanceID>/<serviceBindingID>` (both UUIDs)
//   - How to find:
//     - UI: BTP Cockpit → Subaccounts → [Select Subaccount] → Instances and Subscriptions → Look for service-manager instance and binding IDs
//     - CLI: `btp list services/instance` and `btp list services/binding` in the subaccount context
//
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
// +kubebuilder:storageversion
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
	ServiceManagerGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceManagerKind}.String()
	ServiceManagerKindAPIVersion   = ServiceManagerKind + "." + SchemeGroupVersion.String()
	ServiceManagerGroupVersionKind = SchemeGroupVersion.WithKind(ServiceManagerKind)
)

func init() {
	SchemeBuilder.Register(&ServiceManager{}, &ServiceManagerList{})
}
