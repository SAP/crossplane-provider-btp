package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

const (
	KubeConfigSecreKey = "kubeconfig"
)

// KubeConfigGeneratorParameters are the configurable fields of a KubeConfigGenerator.
type KubeConfigGeneratorParameters struct {
	KubeconfigTemplate KubeconfigTemplate `json:"kubeconfigTemplate"`
	OIDCToken          OIDCToken          `json:"oidcToken"`
}

type KubeconfigTemplate struct {
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source                         xpv1.CredentialsSource `json:"source"`
	xpv1.CommonCredentialSelectors `json:",inline"`

	// ID of the entry in users of kubeconfig to inject the token to
	UserIndex int `json:"userIndex,omitempty"`
	// If not set to true it will clean the user entry and leave only the token, otherwise the token will just be added
	InjectInline bool `json:"injectInline,omitempty"`
}

type OIDCToken struct {
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source                         xpv1.CredentialsSource `json:"source"`
	xpv1.CommonCredentialSelectors `json:",inline"`
}

type KubeConfigGeneratorObservation struct {
	// Time of the last generation process, just for manual lookup right now
	LastUpdatedAt string `json:"lastUpdatedAt,omitempty"`
	// Hash of kubeconfig that has been used in the last generation process (referenced from the secret under kubeconfigTemplate)
	KubeConfigHash []byte `json:"kubeConfigHash,omitempty"`
	// Hash of token that has been used in the last generation process (referenced from the secret under oidcToken)
	TokenHash []byte `json:"tokenHash,omitempty"`
	// Generation (from object metadata) of the CR used for the last generation process, used to detect changes in the CR itself
	UpdatedGeneration int64 `json:"updatedGeneration,omitempty"`
	// ServerUrl parsed from generated Kubeconfig
	ServerUrl string `json:"serverUrl"`
}

// A KubeConfigGeneratorSpec defines the desired state of a KubeConfigGenerator.
type KubeConfigGeneratorSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       KubeConfigGeneratorParameters `json:"forProvider"`
}

// A KubeConfigGeneratorStatus represents the observed state of a KubeConfigGenerator.
type KubeConfigGeneratorStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          KubeConfigGeneratorObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A KubeConfigGenerator is a managed resource that controls the generation of a kubeconfig file
//
// External-Name Configuration:
//   - Follows Standard: no - This resource does not support external name as it does not represent an external resource. Instead of using external name for importing, you can just create an instance of this resource.
//   - Format: Not applicable
//
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp-account}
type KubeConfigGenerator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeConfigGeneratorSpec   `json:"spec"`
	Status KubeConfigGeneratorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeConfigGeneratorList contains a list of KubeConfigGenerator
type KubeConfigGeneratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeConfigGenerator `json:"items"`
}

// KubeConfigGenerator type metadata.
var (
	KubeConfigGeneratorKind             = reflect.TypeOf(KubeConfigGenerator{}).Name()
	KubeConfigGeneratorGroupKind        = schema.GroupKind{Group: Group, Kind: KubeConfigGeneratorKind}.String()
	KubeConfigGeneratorKindAPIVersion   = KubeConfigGeneratorKind + "." + SchemeGroupVersion.String()
	KubeConfigGeneratorGroupVersionKind = SchemeGroupVersion.WithKind(KubeConfigGeneratorKind)
)

func init() {
	SchemeBuilder.Register(&KubeConfigGenerator{}, &KubeConfigGeneratorList{})
}
