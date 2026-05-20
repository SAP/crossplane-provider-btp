package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// A ProviderConfigSpec defines the desired state of a ProviderConfig.
type ProviderConfigSpec struct {
	// Credentials required to authenticate to this provider.
	// Reference to a secret containing the CIS Accounts service credentials.
	// The Cloud Management (CIS) instance must be of plan `central`.
	// The Service Binding should be created with the following parameters `{"grantType": "clientCredentials"}`
	// See [Setup](https://pages.github.tools.sap/cloud-orchestration/docs/sap-services/btp-services/account-managment/provider) for more details
	CISSecret ProviderCredentials `json:"cisCredentials"`

	// A user available in BTP.
	// The Credentials in the ServiceAccountSecret are relevant for two reasons
	// (1) On environment creation (Kyma & CloudFoundry) the APIs require a users email address
	// (2) For updating the managers of a CloudFoundry Environment it is required to have a user and a password
	// The structure is pretty basic, a json object with email, username and password. Username & Password must not be filled if there is no need for CloudFoundry Environments.
	// Example:
	//   {
	//      "email": "<EMAIL>",
	//      "username": "PUserID",
	//      "password": "--"
	//    }
	ServiceAccountSecret ProviderCredentials `json:"serviceAccountSecret,omitempty"`

	CliServerUrl string `json:"cliServerUrl,omitempty"`

	// GlobalAccount is the Global Account Subdomain.
	GlobalAccount string `json:"globalAccount,omitempty"`
}

// ProviderCredentials required to authenticate.
type ProviderCredentials struct {
	// Source of the provider credentials.
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source xpv1.CredentialsSource `json:"source"`

	xpv1.CommonCredentialSelectors `json:",inline"`
}

// A ProviderConfigStatus reflects the observed state of a ProviderConfig.
type ProviderConfigStatus struct {
	xpv1.ProviderConfigStatus `json:",inline"`
}

// +kubebuilder:object:root=true

// A ProviderConfig configures a Template provider.
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SECRET-NAME",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1
// +kubebuilder:resource:scope=Cluster
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig.
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// ProviderConfig type metadata.
var (
	ProviderConfigKind             = reflect.TypeOf(ProviderConfig{}).Name()
	ProviderConfigGroupKind        = schema.GroupKind{Group: Group, Kind: ProviderConfigKind}.String()
	ProviderConfigKindAPIVersion   = ProviderConfigKind + "." + SchemeGroupVersion.String()
	ProviderConfigGroupVersionKind = SchemeGroupVersion.WithKind(ProviderConfigKind)
)

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
}
