package v1alpha1

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

const (
	LabelKeySourceUid          = "ref.orchestrate.cloud.sap/source-uid"
	LabelKeyTargetUid          = "ref.orchestrate.cloud.sap/target-uid"
	AnnotationIgnoreReferences = "ref.orchestrate.cloud.sap/ignore"
	ErrResourceInUse           = "Resource cannot be deleted, still has usages"
	Finalizer                  = "finalizer.orchestrate.cloud.sap"
)

// +kubebuilder:object:root=true

// A ResourceUsage indicates that a resource is using a another resource. It is used to track dependencies between objects.
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SOURCE-KIND",type="string",JSONPath=".spec.sourceRef.kind"
// +kubebuilder:printcolumn:name="SOURCE",type="string",JSONPath=".spec.sourceRef.name"
// +kubebuilder:printcolumn:name="TARGET-KIND",type="string",JSONPath=".spec.targetRef.kind"
// +kubebuilder:printcolumn:name="TARGET",type="string",JSONPath=".spec.targetRef.name"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,provider,template}
type ResourceUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	//Spec ResourceUsageSpec `json:"spec"`
	Spec ResourceUsageSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// ResourceUsageList contains a list of ProviderResourceUsage
type ResourceUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceUsage `json:"items"`
}

// A ResourceUsageSpec is a record that a particular managed resource is using
// a particular provider configuration.
type ResourceUsageSpec struct {

	// ResourceReference to the source managed resource.
	SourceReference xpv1.TypedReference `json:"sourceRef"`

	// ResourceReference to the source managed resource.
	TargetReference xpv1.TypedReference `json:"targetRef"`
}

// ResourceUsage type metadata.
var (
	ResourceUsageKind             = reflect.TypeOf(ResourceUsage{}).Name()
	ResourceUsageGroupKind        = schema.GroupKind{Group: Group, Kind: ResourceUsageKind}.String()
	ResourceUsageKindAPIVersion   = ResourceUsageKind + "." + SchemeGroupVersion.String()
	ResourceUsageGroupVersionKind = SchemeGroupVersion.WithKind(ResourceUsageKind)

	ResourceUsageListKind             = reflect.TypeOf(ResourceUsageList{}).Name()
	ResourceUsageListGroupKind        = schema.GroupKind{Group: Group, Kind: ResourceUsageListKind}.String()
	ResourceUsageListKindAPIVersion   = ResourceUsageListKind + "." + SchemeGroupVersion.String()
	ResourceUsageListGroupVersionKind = SchemeGroupVersion.WithKind(ResourceUsageListKind)
)

func init() {
	SchemeBuilder.Register(&ResourceUsage{}, &ResourceUsageList{})
}

const UseCondition xpv1.ConditionType = "ResourceUsage"
const InUseReason xpv1.ConditionReason = "ResourceUsagesFound"
const NotInUseReason xpv1.ConditionReason = "NoResourceUsagesFound"

func InUse() xpv1.Condition {
	return xpv1.Condition{
		Type:               UseCondition,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             InUseReason,
	}
}

func InUseError(err error) xpv1.Condition {
	return xpv1.Condition{
		Type:               UseCondition,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             InUseReason,
		Message:            err.Error(),
	}
}

func NotInUse() xpv1.Condition {
	return xpv1.Condition{
		Type:               UseCondition,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             NotInUseReason,
	}
}

// TODO: What about ignored condition?

func NewInUseCondition(isUsed bool, err error) xpv1.Condition {
	if err != nil {
		return InUseError(err)
	}
	if isUsed {
		return InUse()
	}
	return NotInUse()
}
