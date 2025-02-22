package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// ServicePlanParameters are the configurable fields of a ServicePlan.
type ServicePlanParameters struct {
	OfferingName string `json:"offeringName"`
	PlanName     string `json:"planName"`

	// +kubebuilder:validation:Optional
	ServiceManagerSelector *xpv1.Selector `json:"serviceManagerSelector,omitempty"`
	// +kubebuilder:validation:Optional
	ServiceManagerRef *xpv1.Reference `json:"serviceManagerRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"ServiceManager" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManager
	// +crossplane:generate:reference:refFieldName=ServiceManagerRef
	// +crossplane:generate:reference:selectorFieldName=ServiceManagerSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManagerSecret()
	ServiceManagerSecret string `json:"serviceManagerSecret,omitempty"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManager
	// +crossplane:generate:reference:refFieldName=ServiceManagerRef
	// +crossplane:generate:reference:selectorFieldName=ServiceManagerSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManagerSecretNamespace()
	ServiceManagerSecretNamespace string `json:"serviceManagerSecretNamespace,omitempty"`
}

// ServicePlanObservation are the observable fields of a ServicePlan.
type ServicePlanObservation struct {
	ServicePlanId string `json:"servicePlanId,omitempty"`
}

// A ServicePlanSpec defines the desired state of a ServicePlan.
type ServicePlanSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ServicePlanParameters `json:"forProvider"`
}

// A ServicePlanStatus represents the observed state of a ServicePlan.
type ServicePlanStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ServicePlanObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServicePlan is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type ServicePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServicePlanSpec   `json:"spec"`
	Status ServicePlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServicePlanList contains a list of ServicePlan
type ServicePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServicePlan `json:"items"`
}

// ServicePlan type metadata.
var (
	ServicePlanKind             = reflect.TypeOf(ServicePlan{}).Name()
	ServicePlanGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: ServicePlanKind}.String()
	ServicePlanKindAPIVersion   = ServicePlanKind + "." + CRDGroupVersion.String()
	ServicePlanGroupVersionKind = CRDGroupVersion.WithKind(ServicePlanKind)
)

func init() {
	SchemeBuilder.Register(&ServicePlan{}, &ServicePlanList{})
}
