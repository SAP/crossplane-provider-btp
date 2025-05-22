package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

const (
	SubscriptionStateInProcess              = "IN_PROCESS"
	SubscriptionStateSubscribed             = "SUBSCRIBED"
	SubscriptionStateSubscribeFailed        = "SUBSCRIBE_FAILED"
	SubscriptionStateUnsubscribeFailed      = "UNSUBSCRIBE_FAILED"
	SubscriptionStateUpdateFailed           = "UPDATE_FAILED"
	SubscriptionStateUpdateParametersFailed = "UPDATE_PARAMETERS_FAILED"
	SubscriptionStateNotSubscribed          = "NOT_SUBSCRIBED"
)

// SubscriptionParameters are the configurable fields of a Subscription.
type SubscriptionParameters struct {
	// AppName of the app to subscribe to
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="appName can't be updated once set"
	AppName string `json:"appName"`
	// PlanName to subscribe to, empty plannames are shown as "default" in cockpit, use "" instead
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="planName can't be updated once set"
	PlanName string `json:"planName"`
}

// SubscriptionObservation are the observable fields of a Subscription.
type SubscriptionObservation struct {
	// State as received from the API instance
	// +optional
	State *string `json:"state,omitempty"`
}

// A SubscriptionSpec defines the desired state of a Subscription.
type SubscriptionSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       SubscriptionParameters `json:"forProvider"`

	// +kubebuilder:validation:Optional
	CloudManagementSelector *xpv1.Selector `json:"cloudManagementSelector,omitempty"`
	// Reference to CloudManagement instance of plan type "local" used for authentication
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
}

// A SubscriptionStatus represents the observed state of a Subscription.
type SubscriptionStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          SubscriptionObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Subscription encodes a subscription of a subaccount to a service
// It requires a references CloudManagement instance of plan type "local" to authenticate and map to subaccount.
// To import a subscription use the pattern <app name>/<plan name> as externalName annotation
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type Subscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubscriptionSpec   `json:"spec"`
	Status SubscriptionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubscriptionList contains a list of Subscription
type SubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subscription `json:"items"`
}

// Subscription type metadata.
var (
	SubscriptionKind             = reflect.TypeOf(Subscription{}).Name()
	SubscriptionGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: SubscriptionKind}.String()
	SubscriptionKindAPIVersion   = SubscriptionKind + "." + CRDGroupVersion.String()
	SubscriptionGroupVersionKind = CRDGroupVersion.WithKind(SubscriptionKind)
)

func init() {
	SchemeBuilder.Register(&Subscription{}, &SubscriptionList{})
}
