package kymaserviceinstance

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// This matches services.cloud.sap.com/v1/ServiceInstance in Kyma
type ServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServiceInstanceSpec   `json:"spec"`
	Status            ServiceInstanceStatus `json:"status,omitempty"`
}

type ServiceInstanceSpec struct {
	ServiceOfferingName string                `json:"serviceOfferingName"`
	ServicePlanName     string                `json:"servicePlanName"`
	ExternalName        string                `json:"externalName,omitempty"`
	Parameters          *runtime.RawExtension `json:"parameters,omitempty"`
}

type ServiceInstanceStatus struct {
	Ready      bool   `json:"ready"`
	InstanceID string `json:"instanceID,omitempty"`
	// ... more fields
}
