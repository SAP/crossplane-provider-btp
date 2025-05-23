/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// ServiceInstanceParameters are the configurable fields of a ServiceInstance.
type ServiceInstanceParameters struct {

	// Name of the service offering
	OfferingName string `json:"offeringName,omitempty"`

	// Name of the service plan of that offering
	PlanName string `json:"planName,omitempty"`

	// All parameters of the tf resource are included here as well
	SubaccountServiceInstanceParameters `json:",inline"`

	// Parameters stored in secret, will be merged with spec parameters
	// +kubebuilder:validation:Optional
	ParameterSecretRefs []xpv1.SecretKeySelector `json:"parameterSecretRefs,omitempty"`

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

// ServiceInstanceObservation are the observable fields of a ServiceInstance.
type ServiceInstanceObservation struct {
	ID string `json:"id,omitempty"`
}

// A ServiceInstanceSpec defines the desired state of a ServiceInstance.
type ServiceInstanceSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ServiceInstanceParameters `json:"forProvider"`
}

// A ServiceInstanceStatus represents the observed state of a ServiceInstance.
type ServiceInstanceStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ServiceInstanceObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceInstance is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type ServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceInstanceSpec   `json:"spec"`
	Status ServiceInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceInstanceList contains a list of ServiceInstance
type ServiceInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceInstance `json:"items"`
}

// ServiceInstance type metadata.
var (
	ServiceInstanceKind             = reflect.TypeOf(ServiceInstance{}).Name()
	ServiceInstanceGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: ServiceInstanceKind}.String()
	ServiceInstanceKindAPIVersion   = ServiceInstanceKind + "." + CRDGroupVersion.String()
	ServiceInstanceGroupVersionKind = CRDGroupVersion.WithKind(ServiceInstanceKind)
)

func init() {
	SchemeBuilder.Register(&ServiceInstance{}, &ServiceInstanceList{})
}
