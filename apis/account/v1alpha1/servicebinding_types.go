package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// ServiceBindingParameters are the configurable fields of a ServiceBinding.
type ServiceBindingParameters struct {
	// Name of the service instance in btp, required
	Name string `json:"name"`

	// Parameters in JSON or YAML format, will be merged with yaml parameters and secret parameters, will overwrite duplicated keys from secrets
	// +kubebuilder:validation:Optional
	Parameters runtime.RawExtension `json:"parameters,omitempty"`

	// Parameters stored in secret, will be merged with spec parameters
	// +kubebuilder:validation:Optional
	ParameterSecretRefs []xpv1.SecretKeySelector `json:"parameterSecretRefs,omitempty"`

	// (String) The ID of the subaccount.
	// The ID of the subaccount.
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	SubaccountID *string `json:"subaccountId,omitempty" tf:"subaccount_id,omitempty"`

	// Reference to a Subaccount in account to populate subaccountId.
	// +kubebuilder:validation:Optional
	SubaccountRef *v1.Reference `json:"subaccountRef,omitempty" tf:"-"`

	// Selector for a Subaccount in account to populate subaccountId.
	// +kubebuilder:validation:Optional
	SubaccountSelector *v1.Selector `json:"subaccountSelector,omitempty" tf:"-"`

	// (String) The ID of the service instance associated with the binding.
	// The ID of the service instance associated with the binding.
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceInstance
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceInstanceUuid()
	// +crossplane:generate:reference:refFieldName=ServiceInstanceRef
	// +crossplane:generate:reference:selectorFieldName=ServiceInstanceSelector
	// +kubebuilder:validation:Optional
	ServiceInstanceID *string `json:"serviceInstanceId,omitempty" tf:"service_instance_id,omitempty"`

	// Reference to a ServiceInstance in account to populate serviceInstanceId.
	// +kubebuilder:validation:Optional
	ServiceInstanceRef *v1.Reference `json:"serviceInstanceRef,omitempty" tf:"-"`

	// Selector for a ServiceInstance in account to populate serviceInstanceId.
	// +kubebuilder:validation:Optional
	ServiceInstanceSelector *v1.Selector `json:"serviceInstanceSelector,omitempty" tf:"-"`
}

// ServiceBindingObservation are the observable fields of a ServiceBinding.
type ServiceBindingObservation struct {
	ID string `json:"id,omitempty"`
}

// A ServiceBindingSpec defines the desired state of a ServiceBinding.
type ServiceBindingSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ServiceBindingParameters `json:"forProvider"`
}

// A ServiceBindingStatus represents the observed state of a ServiceBinding.
type ServiceBindingStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ServiceBindingObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceBinding allows to manage a binding to a service instance in BTP
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type ServiceBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceBindingSpec   `json:"spec"`
	Status ServiceBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceBindingList contains a list of ServiceBinding
type ServiceBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceBinding `json:"items"`
}

// ServiceBinding type metadata.
var (
	ServiceBindingKind             = reflect.TypeOf(ServiceBinding{}).Name()
	ServiceBindingGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: ServiceBindingKind}.String()
	ServiceBindingKindAPIVersion   = ServiceBindingKind + "." + CRDGroupVersion.String()
	ServiceBindingGroupVersionKind = CRDGroupVersion.WithKind(ServiceBindingKind)
)

func init() {
	SchemeBuilder.Register(&ServiceBinding{}, &ServiceBindingList{})
}
