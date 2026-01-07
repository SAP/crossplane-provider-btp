package v1alpha1

import (
	"reflect"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ConDetailsIDToken       = "IDToken"
	ConDetailsRefresh       = "RefreshToken"
	RotationStrategyDynamic = "Dynamic"
)

// CertBasedOIDCLoginParameters are the configurable fields of a CertBasedOIDCLogin.
type CertBasedOIDCLoginParameters struct {
	Issuer   string `json:"issuer,omitempty"`
	ClientId string `json:"clientId,omitempty"`
	// Reference to p12 certificate, encoded as secret
	Certificate Certificate `json:"certificate,omitempty"`
	// Certificate Password used in the auth process
	Password Password `json:"password,omitempty"`
}

type Certificate struct {
	// Type of certificate, currently just used for manual bookkepping
	Type string `json:"type,omitempty"`
	// ContainsCertificateChain indicates whether the provided certificate should be parsed as the full certificate chain (true) or just the leaf certificate (false). Default is false to keep backwards compatibility.
	ContainsCertificateChain bool `json:"containsCertificateChain,omitempty"`
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source                         xpv1.CredentialsSource `json:"source"`
	xpv1.CommonCredentialSelectors `json:",inline"`
}

type Password struct {
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source                         xpv1.CredentialsSource `json:"source"`
	xpv1.CommonCredentialSelectors `json:",inline"`
}

// JwtStatus status of the retrieved Json Web Token
type JwtStatus struct {
	// Issuer is the IDP which issued the JWT
	Issuer *string `json:"issuer,omitempty"`
	// IssuedAt timestamp of creation of the JWT
	IssuedAt *metav1.Time `json:"issuedAt,omitempty"`
	// expiresAt timestamp when JWT will expire
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
	// RotationNotBefore timestamp after which rotation will be started
	RotationNotBefore *metav1.Time `json:"rotationNotBefore,omitempty"`
	// RotationStrategy returns which strategy the controller chose to rotate the secret. Currently not configurable. Dynamic refers to rotate the jwt at 2/3 of its duration.
	// +kubebuilder:validation:Enum=Dynamic
	RotationStrategy *string `json:"rotationStrategy,omitempty"`
	// RotationDuration threshold value (depending on RotationStrategy) used to calculate RotationNotBefore
	RotationDuration *metav1.Duration `json:"rotationDuration,omitempty"`
}

// CertBasedOIDCLoginObservation are the observable fields of a CertBasedOIDCLogin.
type CertBasedOIDCLoginObservation struct {
	JwtStatus `json:",inline"`
}

// A CertBasedOIDCLoginSpec defines the desired state of a CertBasedOIDCLogin.
type CertBasedOIDCLoginSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       CertBasedOIDCLoginParameters `json:"forProvider"`
}

// A CertBasedOIDCLoginStatus represents the observed state of a CertBasedOIDCLogin.
type CertBasedOIDCLoginStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          CertBasedOIDCLoginObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A CertBasedOIDCLogin is a managed resource that represents a OIDC login flow using a certificate for authentication
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp-account}
type CertBasedOIDCLogin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CertBasedOIDCLoginSpec   `json:"spec"`
	Status CertBasedOIDCLoginStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CertBasedOIDCLoginList contains a list of CertBasedOIDCLogin
type CertBasedOIDCLoginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CertBasedOIDCLogin `json:"items"`
}

// CertBasedOIDCLogin type metadata.
var (
	CertBasedOIDCLoginKind             = reflect.TypeOf(CertBasedOIDCLogin{}).Name()
	CertBasedOIDCLoginGroupKind        = schema.GroupKind{Group: Group, Kind: CertBasedOIDCLoginKind}.String()
	CertBasedOIDCLoginKindAPIVersion   = CertBasedOIDCLoginKind + "." + SchemeGroupVersion.String()
	CertBasedOIDCLoginGroupVersionKind = SchemeGroupVersion.WithKind(CertBasedOIDCLoginKind)
)

func init() {
	SchemeBuilder.Register(&CertBasedOIDCLogin{}, &CertBasedOIDCLoginList{})
}

const Introspection xpv1.ConditionType = "TokenIntrospection"
const CannotIntrospect xpv1.ConditionReason = "ErrIntrospect"
const IntrospectSuccess xpv1.ConditionReason = "IntrospectSuccess"

func IntrospectError(msg string) xpv1.Condition {
	return xpv1.Condition{
		Type:               Introspection,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             CannotIntrospect,
		Message:            msg,
	}
}

func IntrospectOk() xpv1.Condition {
	return xpv1.Condition{
		Type:               Introspection,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             IntrospectSuccess,
	}
}
