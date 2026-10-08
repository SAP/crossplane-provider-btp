package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// A ProviderConfigSpec defines the desired state of a ProviderConfig.
type ProviderConfigSpec struct {
	// Credentials required to authenticate to this provider.
	// Reference to a secret containing the CIS Accounts service credentials.
	// The Cloud Management (CIS) instance must be of plan `central`.
	// The Service Binding should be created with the following parameters `{"grantType": "clientCredentials"}`
	// See [Setup](https://sap.github.io/crossplane-provider-docs/docs/crossplane-provider-btp/docs/end-user-guides/setup/configure-provider-btp) for more details
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

	// WorkloadIdentity replaces user/password authentication with projected assertions.
	// Native clients still require manually provisioned CIS credentials.
	// +optional
	WorkloadIdentity *WorkloadIdentityConfiguration `json:"workloadIdentity,omitempty"`

	CliServerUrl string `json:"cliServerUrl,omitempty"`

	// GlobalAccount is the Global Account Subdomain. It must be the subdomain
	// of the same global account that the cisCredentials binding points at.
	GlobalAccount string `json:"globalAccount,omitempty"`
}

// WorkloadIdentityConfiguration identifies a rotating projected workload assertion.
type WorkloadIdentityConfiguration struct {
	// TokenFile is the mounted projected ServiceAccount JWT path.
	// +kubebuilder:validation:MinLength=1
	TokenFile string `json:"tokenFile"`
	// IdentityProvider is the BTP platform trust origin.
	// +kubebuilder:validation:MinLength=1
	IdentityProvider string `json:"identityProvider"`
	// UserEmail supplies non-secret identity metadata required by native operations.
	// +kubebuilder:validation:MinLength=1
	UserEmail string `json:"userEmail"`
	// IASURL, IASClientID and IASResource enable user-preserving native CIS JWT exchange.
	// The confidential IAS client authenticates with the projected JWT; CIS bindings remain manual.
	// +optional
	IASURL string `json:"iasUrl,omitempty"`
	// +optional
	IASClientID string `json:"iasClientId,omitempty"`
	// +optional
	IASResource string `json:"iasResource,omitempty"`
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
