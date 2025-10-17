// +kubebuilder:object:generate=true
// +groupName=account.btp.sap.com
package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// KymaServiceInstanceParameters are the configurable fields of a KymaServiceInstance.
type KymaServiceInstanceParameters struct {
	// Name of the ServiceInstance resource in Kyma cluster
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace in Kyma cluster where ServiceInstance will be created
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// BTP service offering name (e.g., "destination", "xsuaa")
	// +kubebuilder:validation:Required
	ServiceOfferingName string `json:"serviceOfferingName"`

	// Service plan name (e.g., "lite", "standard")
	// +kubebuilder:validation:Required
	ServicePlanName string `json:"servicePlanName"`

	// External name for display in BTP cockpit (optional)
	// If not specified, uses the Name field
	// +kubebuilder:validation:Optional
	ExternalName string `json:"externalName,omitempty"`

	// Service configuration parameters (inline JSON/YAML).
	// These are typically merged with values discovered or from secrets by the controller.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Optional
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// KymaServiceInstanceObservation are the observable fields of a KymaServiceInstance.
type KymaServiceInstanceObservation struct {
	// Ready status from Kyma ServiceInstance
	Ready bool `json:"ready,omitempty"`

	// Instance ID from BTP
	InstanceID string `json:"instanceID,omitempty"`
}

// A KymaServiceInstanceSpec defines the desired state of a KymaServiceInstance.
type KymaServiceInstanceSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       KymaServiceInstanceParameters `json:"forProvider"`

	// Reference to KymaEnvironmentBinding (for rotating kubeconfig)
	// +kubebuilder:validation:Optional
	KymaEnvironmentBindingRef *xpv1.Reference `json:"kymaEnvironmentBindingRef,omitempty"`

	// +kubebuilder:validation:Optional
	KymaEnvironmentBindingSelector *xpv1.Selector `json:"kymaEnvironmentBindingSelector,omitempty"`

	// Extracted values populated by Crossplane reference resolution:
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1.KymaEnvironmentBinding
	// +crossplane:generate:reference:refFieldName=KymaEnvironmentBindingRef
	// +crossplane:generate:reference:selectorFieldName=KymaEnvironmentBindingSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1.KymaEnvironmentBindingSecret()
	KymaEnvironmentBindingSecret string `json:"kymaEnvironmentBindingSecret,omitempty"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1.KymaEnvironmentBinding
	// +crossplane:generate:reference:refFieldName=KymaEnvironmentBindingRef
	// +crossplane:generate:reference:selectorFieldName=KymaEnvironmentBindingSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1.KymaEnvironmentBindingSecretNamespace()
	KymaEnvironmentBindingSecretNamespace string `json:"kymaEnvironmentBindingSecretNamespace,omitempty"`
}

// A KymaServiceInstanceStatus represents the observed state of a KymaServiceInstance.
type KymaServiceInstanceStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          KymaServiceInstanceObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A KymaServiceInstance is a managed resource that represents a BTP Service Instance in a Kyma cluster
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type KymaServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KymaServiceInstanceSpec   `json:"spec"`
	Status KymaServiceInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KymaServiceInstanceList contains a list of KymaServiceInstance
type KymaServiceInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KymaServiceInstance `json:"items"`
}

// KymaServiceInstance type metadata.
const (
	Group = "account.btp.sap.com"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: "v1alpha1"}

	KymaServiceInstanceKind             = reflect.TypeOf(KymaServiceInstance{}).Name()
	KymaServiceInstanceGroupKind        = schema.GroupKind{Group: Group, Kind: KymaServiceInstanceKind}.String()
	KymaServiceInstanceKindAPIVersion   = KymaServiceInstanceKind + "." + SchemeGroupVersion.String()
	KymaServiceInstanceGroupVersionKind = SchemeGroupVersion.WithKind(KymaServiceInstanceKind)
)

func init() {
	SchemeBuilder.Register(&KymaServiceInstance{}, &KymaServiceInstanceList{})
}
