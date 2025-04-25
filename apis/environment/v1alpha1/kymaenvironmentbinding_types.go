package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// KymaEnvironmentBindingParameters are the configurable fields of a KymaEnvironmentBinding.
type KymaEnvironmentBindingParameters struct {
	// The interval at which the binding secret is rotated.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="1h"
	RotationInterval metav1.Duration `json:"rotationInterval,omitempty"`

	// The time to live of the binding secret. Should be greater than the rotation interval.
	// The margin between the two values allows systems to settle down and pickup the new secret
	// The binding secret will be deleted after this time.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="1h15m"
	BindingTTl metav1.Duration `json:"ttl,omitempty"`
}

type Binding struct {
	Id        string      `json:"id"`
	IsActive  bool        `json:"isActive"`
	CreatedAt metav1.Time `json:"createdAt"`
	ExpiresAt metav1.Time `json:"expiresAt"`
}

// KymaEnvironmentBindingObservation are the observable fields of a KymaEnvironmentBinding.
type KymaEnvironmentBindingObservation struct {
	Bindings []Binding `json:"bindings,omitempty"`
}

// A KymaEnvironmentBindingSpec defines the desired state of a KymaEnvironmentBinding.
type KymaEnvironmentBindingSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       KymaEnvironmentBindingParameters `json:"forProvider"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1.KymaEnvironment
	// +crossplane:generate:reference:refFieldName=KymaEnvironmentRef
	// +crossplane:generate:reference:selectorFieldName=KymaEnvironmentSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1.KymaInstanceId()
	KymaEnvironmentId string `json:"kymaEnvironmentId,omitempty"`
	// +kubebuilder:validation:Optional
	KymaEnvironmentSelector *xpv1.Selector `json:"kymaEnvironmentSelector,omitempty"`
	// +kubebuilder:validation:Optional
	KymaEnvironmentRef *xpv1.Reference `json:"kymaEnvironmentRef,omitempty" reference-group:"environment.btp.sap.crossplane.io" reference-kind:"KymaEnvironment" reference-apiversion:"v1alpha1"`

	// +kubebuilder:validation:Optional
	CloudManagementSelector *xpv1.Selector `json:"cloudManagementSelector,omitempty"`
	// +kubebuilder:validation:Optional
	CloudManagementRef *xpv1.Reference `json:"cloudManagementRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"CloudManagement" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagement
	// +crossplane:generate:reference:refFieldName=CloudManagementRef
	// +crossplane:generate:reference:selectorFieldName=CloudManagementSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.CloudManagementSecret()
	CloudManagementSecret string `json:"cloudManagementSecret,omitempty"`
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

// A KymaEnvironmentBindingStatus represents the observed state of a KymaEnvironmentBinding.
type KymaEnvironmentBindingStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          KymaEnvironmentBindingObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A KymaEnvironmentBinding is an API to retrieve a binding for a specific Kyma Instance.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type KymaEnvironmentBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KymaEnvironmentBindingSpec   `json:"spec"`
	Status KymaEnvironmentBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KymaEnvironmentBindingList contains a list of KymaEnvironmentBinding
type KymaEnvironmentBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KymaEnvironmentBinding `json:"items"`
}

// KymaEnvironmentBinding type metadata.
var (
	KymaEnvironmentBindingKind             = reflect.TypeOf(KymaEnvironmentBinding{}).Name()
	KymaEnvironmentBindingGroupKind        = schema.GroupKind{Group: Group, Kind: KymaEnvironmentBindingKind}.String()
	KymaEnvironmentBindingKindAPIVersion   = KymaEnvironmentBindingKind + "." + SchemeGroupVersion.String()
	KymaEnvironmentBindingGroupVersionKind = SchemeGroupVersion.WithKind(KymaEnvironmentBindingKind)
)

func init() {
	SchemeBuilder.Register(&KymaEnvironmentBinding{}, &KymaEnvironmentBindingList{})
}
