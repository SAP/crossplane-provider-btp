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

// ServiceBindingParameters are the configurable fields of a ServiceBinding.
type ServiceBindingParameters struct {
	// All parameters of the tf resource are included here as well
	SubaccountServiceBindingParameters `json:",inline"`

	// Parameters in YAML format, will be merged with json parameters and secret parameters
	// +kubebuilder:validation:Optional
	ParametersYaml *string `json:"parametersYaml,omitempty"`

	// Parameters stored in secret, will be merged with spec parameters
	// +kubebuilder:validation:Optional
	ParameterSecretRefs []xpv1.SecretKeySelector `json:"parameterSecretRefs,omitempty"`
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

// A ServiceBinding is an example API type.
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
