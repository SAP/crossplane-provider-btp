package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// SubaccountDestinationInitParameters holds the reference fields needed by
// the crossplane reference resolver generator. Mirrors the reference fields
// in SubaccountDestinationParameters.
type SubaccountDestinationInitParameters struct {
	// SubaccountID is the GUID of the subaccount that owns this destination.
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:extractor=github.com/crossplane/crossplane-runtime/v2/pkg/reference.ExternalName()
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +optional
	SubaccountID *string `json:"subaccountId,omitempty"`

	// Reference to a Subaccount to populate subaccountId.
	// +optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty"`

	// Selector for a Subaccount to populate subaccountId.
	// +optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`

	// Name of the destination. Immutable after creation.
	// +optional
	Name string `json:"name,omitempty"`

	// Type of the destination. e.g. HTTP, LDAP, RFC, MAIL.
	// +optional
	Type string `json:"type,omitempty"`

	// URL of the destination.
	// +optional
	URL *string `json:"url,omitempty"`

	// Authentication type.
	// +optional
	Authentication *string `json:"authentication,omitempty"`

	// ProxyType.
	// +optional
	ProxyType *string `json:"proxyType,omitempty"`

	// Description of the destination.
	// +optional
	Description *string `json:"description,omitempty"`

	// ServiceInstanceID scopes this destination to a service instance.
	// +optional
	ServiceInstanceID *string `json:"serviceInstanceId,omitempty"`

	// AdditionalProperties merged into the destination property bag.
	// +optional
	AdditionalProperties map[string]string `json:"additionalProperties,omitempty"`

	// AdditionalConfigurationSecretRef for sensitive destination properties.
	// +optional
	AdditionalConfigurationSecretRef *xpv1.SecretKeySelector `json:"additionalConfigurationSecretRef,omitempty"`
}

// SubaccountDestinationParameters are the configurable fields of a SubaccountDestination.
//
// External-Name Configuration:
//   - Follow Standard: yes
//   - Format: <subaccount-id>/<destination-name>
//   - How to find:
//     - UI: SAP BTP Cockpit → Subaccount → Connectivity → Destinations (field: Name)
//     - API: GET /v1/subaccountDestinations/{destination name} (fields: subaccount_id + Name)
type SubaccountDestinationParameters struct {
	// Name of the destination. Immutable after creation.
	// Used as the second segment of the external-name annotation.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// SubaccountID is the GUID of the subaccount that owns this destination.
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:extractor=github.com/crossplane/crossplane-runtime/v2/pkg/reference.ExternalName()
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +optional
	SubaccountID *string `json:"subaccountId,omitempty"`

	// Reference to a Subaccount to populate subaccountId.
	// +optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty"`

	// Selector for a Subaccount to populate subaccountId.
	// +optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`

	// Type of the destination. e.g. HTTP, LDAP, RFC, MAIL.
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// URL of the destination.
	// +optional
	URL *string `json:"url,omitempty"`

	// Authentication type. e.g. NoAuthentication, BasicAuthentication, OAuth2ClientCredentials.
	// +optional
	Authentication *string `json:"authentication,omitempty"`

	// ProxyType. e.g. Internet, OnPremise, PrivateLink.
	// +optional
	ProxyType *string `json:"proxyType,omitempty"`

	// Description of the destination.
	// +optional
	Description *string `json:"description,omitempty"`

	// ServiceInstanceID scopes this destination to a service instance.
	// Not yet implemented — setting this field returns an error.
	// +optional
	ServiceInstanceID *string `json:"serviceInstanceId,omitempty"`

	// AdditionalProperties are merged on top of typed fields when building
	// the destination property bag sent to the Destination Service API.
	// Use for authentication-specific fields such as User, Password,
	// ClientId, ClientSecret, TokenServiceURL, etc.
	// +optional
	AdditionalProperties map[string]string `json:"additionalProperties,omitempty"`

	// AdditionalConfigurationSecretRef points to a Kubernetes Secret whose
	// value (a JSON object) is merged into the destination property bag.
	// Use for sensitive properties that must not appear in the CR spec.
	// +optional
	AdditionalConfigurationSecretRef *xpv1.SecretKeySelector `json:"additionalConfigurationSecretRef,omitempty"`
}

// SubaccountDestinationObservation holds the fields observed from the Destination Service API.
type SubaccountDestinationObservation struct {
	// Name of the destination as reported by the API.
	// +optional
	Name *string `json:"name,omitempty"`

	// SubaccountID as reported by the API.
	// +optional
	SubaccountID *string `json:"subaccountId,omitempty"`

	// Authentication type as reported by the API.
	// +optional
	Authentication *string `json:"authentication,omitempty"`

	// ProxyType as reported by the API.
	// +optional
	ProxyType *string `json:"proxyType,omitempty"`

	// Description as reported by the API.
	// +optional
	Description *string `json:"description,omitempty"`

	// URL as reported by the API.
	// +optional
	URL *string `json:"url,omitempty"`

	// CreationTime metadata from the API.
	// +optional
	CreationTime *string `json:"creationTime,omitempty"`

	// ModificationTime metadata from the API.
	// +optional
	ModificationTime *string `json:"modificationTime,omitempty"`

	// ETag for optimistic concurrency on updates.
	// +optional
	ETag *string `json:"etag,omitempty"`

	// RawProperties is the full property bag as returned by the Destination Service API.
	// Used internally to determine whether an update is needed.
	// +optional
	RawProperties map[string]string `json:"rawProperties,omitempty"`
}

// SubaccountDestinationSpec defines the desired state.
type SubaccountDestinationSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       SubaccountDestinationParameters     `json:"forProvider"`
	InitProvider      SubaccountDestinationInitParameters `json:"initProvider,omitempty"`
}

// SubaccountDestinationStatus defines the observed state.
type SubaccountDestinationStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             SubaccountDestinationObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,account}

// SubaccountDestination manages a destination in a SAP BTP subaccount via
// the Destination Service REST API.
type SubaccountDestination struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SubaccountDestinationSpec   `json:"spec"`
	Status            SubaccountDestinationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubaccountDestinationList contains a list of SubaccountDestination.
type SubaccountDestinationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SubaccountDestination `json:"items"`
}

var (
	SubaccountDestination_Kind             = "SubaccountDestination"
	SubaccountDestination_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: SubaccountDestination_Kind}.String()
	SubaccountDestination_GroupVersionKind = CRDGroupVersion.WithKind(SubaccountDestination_Kind)
)

func init() {
	SchemeBuilder.Register(&SubaccountDestination{}, &SubaccountDestinationList{})
}
