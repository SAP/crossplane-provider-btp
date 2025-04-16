package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

const (
	KubeConfigSecretKey = "kubeconfig"
	KubeConfigLabelKey  = "KubeconfigURL"
)

// KymaEnvironmentParameters are the configurable fields of a KymaEnvironment.
type KymaEnvironmentParameters struct {
	PlanName string `json:"planName"`

	// Provisioning parameters for the instance.
	//
	// The Parameters field is NOT secret or secured in any way and should
	// NEVER be used to hold sensitive information. To set parameters that
	// contain secret information, you should ALWAYS store that information
	// in a Secret and use the ParametersFrom field.
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters runtime.RawExtension `json:"parameters,omitempty"`
}

// KymaEnvironmentObservation are the observable fields of a KymaEnvironment.
type KymaEnvironmentObservation struct {
	EnvironmentObservation `json:",inline"`
}

// A KymaEnvironmentSpec defines the desired state of a KymaEnvironment.
type KymaEnvironmentSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       KymaEnvironmentParameters `json:"forProvider"`
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

// A KymaEnvironmentStatus represents the observed state of a KymaEnvironment.
type KymaEnvironmentStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          KymaEnvironmentObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A KymaEnvironment is a managed resource that represents a Kyma environment in the SAP Business Technology Platform
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type KymaEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KymaEnvironmentSpec   `json:"spec"`
	Status KymaEnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KymaEnvironmentList contains a list of KymaEnvironment
type KymaEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KymaEnvironment `json:"items"`
}

// KymaEnvironment type metadata.
var (
	KymaEnvironmentKind             = reflect.TypeOf(KymaEnvironment{}).Name()
	KymaEnvironmentGroupKind        = schema.GroupKind{Group: Group, Kind: KymaEnvironmentKind}.String()
	KymaEnvironmentKindAPIVersion   = KymaEnvironmentKind + "." + SchemeGroupVersion.String()
	KymaEnvironmentGroupVersionKind = SchemeGroupVersion.WithKind(KymaEnvironmentKind)
)

func init() {
	SchemeBuilder.Register(&KymaEnvironment{}, &KymaEnvironmentList{})
}
