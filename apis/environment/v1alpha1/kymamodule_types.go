package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// KymaModuleParameters are the configurable fields of a KymaModule.
type KymaModuleParameters struct {

	// The name of the standard module to be activated.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// The channel of the module to be activated. Note: this is activated on module level and overrides the channel of the KymaEnvironment.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="regular"
	Channel *string `json:"channel,omitempty"`
}

// A KymaModuleSpec defines the desired state of a KymaModule.
type KymaModuleSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       KymaModuleParameters `json:"forProvider"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1.KymaEnvironmentBinding
	// +crossplane:generate:reference:refFieldName=KymaEnvironmentBindingRef
	// +crossplane:generate:reference:selectorFieldName=KymaEnvironmentBindingSelector
	KymaEnvironmentBindingId string `json:"kymaEnvironmentBindingId,omitempty"`
	// +kubebuilder:validation:Optional
	KymaEnvironmentBindingSelector *xpv1.Selector `json:"kymaEnvironmentBindingSelector,omitempty"`
	// +kubebuilder:validation:Optional
	KymaEnvironmentBindingRef *xpv1.Reference `json:"kymaEnvironmentBindingRef,omitempty" reference-group:"environment.btp.sap.crossplane.io" reference-kind:"KymaEnvironmentBinding" reference-apiversion:"v1alpha1"`

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

// A KymaModuleStatus represents the observed state of a KymaModule.
type KymaModuleStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          KymaModuleObservation `json:"atProvider,omitempty"`
}

type KymaModuleObservation struct {
	// The ID of the module.
	ID string `json:"id,omitempty"`
	// The name of the module.
	Name string `json:"name,omitempty"`
	// The channel of the module.
	Channel string `json:"channel,omitempty"`
	// The status of the module.
	Status string `json:"status,omitempty"`
	// The description of the module.
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true

// A KymaModule is an API to retrieve a binding for a specific Kyma Instance.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type KymaModule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KymaModuleSpec   `json:"spec"`
	Status KymaModuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KymaModuleList contains a list of KymaModules
type KymaModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KymaModule `json:"items"`
}

// KymaModule type metadata.
var (
	KymaModuleKind             = reflect.TypeOf(KymaModule{}).Name()
	KymaModuleGroupKind        = schema.GroupKind{Group: Group, Kind: KymaModuleKind}.String()
	KymaModuleKindAPIVersion   = KymaModuleKind + "." + SchemeGroupVersion.String()
	KymaModuleGroupVersionKind = SchemeGroupVersion.WithKind(KymaModuleKind)
)

func init() {
	SchemeBuilder.Register(&KymaModule{}, &KymaModuleList{})
}
