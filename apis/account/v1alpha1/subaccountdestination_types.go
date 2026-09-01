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
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name can't be updated once set"
	Name string `json:"name,omitempty"`

	// Type of the destination.
	// +optional
	// +kubebuilder:validation:Enum=HTTP;RFC;MAIL;LDAP
	Type string `json:"type,omitempty"`

	// AdditionalProperties are sent as-is to the Destination Service API property bag.
	// Use for all destination-specific fields (e.g. URL, Authentication, ProxyType,
	// TokenServiceURL). For the full list of supported properties see:
	// https://help.sap.com/docs/connectivity/sap-btp-connectivity-cf/destination-service-rest-api-23ccafbea18f4b65919a2799f2cd20e6-150
	// Note: keys "Name" and "Type" override the typed fields above if present.
	// +optional
	AdditionalProperties map[string]string `json:"additionalProperties,omitempty"`

	// AdditionalConfigurationSecretRefs points to Kubernetes Secrets whose
	// values are JSON objects merged into the destination property bag.
	// All values are coerced to strings (BTP Destination Service expects string-only
	// property values). Use for sensitive properties that must not appear in the CR spec.
	// +optional
	AdditionalConfigurationSecretRefs []xpv1.SecretKeySelector `json:"additionalConfigurationSecretRefs,omitempty"`

	// DestinationServiceBindingSecretRef points to a Kubernetes Secret containing
	// Destination Service OAuth2 credentials.
	// +optional
	DestinationServiceBindingSecretRef *xpv1.SecretKeySelector `json:"destinationServiceBindingSecretRef,omitempty"`
}

// SubaccountDestinationParameters are the configurable fields of a SubaccountDestination.
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

	// Type of the destination.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=HTTP;RFC;MAIL;LDAP
	Type string `json:"type"`

	// AdditionalProperties are sent as-is to the Destination Service API property bag.
	// Use for all destination-specific fields (e.g. URL, Authentication, ProxyType,
	// TokenServiceURL). For the full list of supported properties see:
	// https://help.sap.com/docs/connectivity/sap-btp-connectivity-cf/destination-service-rest-api-23ccafbea18f4b65919a2799f2cd20e6-150
	// Note: keys "Name" and "Type" override the typed fields above if present.
	// +optional
	AdditionalProperties map[string]string `json:"additionalProperties,omitempty"`

	// AdditionalConfigurationSecretRefs points to Kubernetes Secrets whose
	// values are JSON objects merged into the destination property bag.
	// All values are coerced to strings (BTP Destination Service expects string-only
	// property values). Use for sensitive properties that must not appear in the CR spec.
	// +optional
	AdditionalConfigurationSecretRefs []xpv1.SecretKeySelector `json:"additionalConfigurationSecretRefs,omitempty"`

	// DestinationServiceBindingSecretRef points to a Kubernetes Secret containing
	// Destination Service OAuth2 credentials. The secret can be created by a
	// ServiceBinding CR (recommended) or manually.
	//
	// Two secret formats are accepted:
	//   - Flat keys (leave key empty): the secret has individual keys clientid,
	//     clientsecret, tokenurl/token_url, uri/url — the format written by
	//     SubaccountServiceBinding when secretKey is not set.
	//   - Single JSON key (set key): the named key holds a JSON object with
	//     clientid, clientsecret, tokenurl, uri — the format written by
	//     SubaccountServiceBinding when secretKey is set (e.g. secretKey: credentials).
	//
	// +optional
	DestinationServiceBindingSecretRef *xpv1.SecretKeySelector `json:"destinationServiceBindingSecretRef,omitempty"`
}

// SubaccountDestinationObservation holds the fields observed from the Destination Service API.
type SubaccountDestinationObservation struct {
	// Name of the destination as reported by the API.
	// +optional
	Name *string `json:"name,omitempty"`

	// ETag for optimistic concurrency on updates.
	// +optional
	ETag *string `json:"etag,omitempty"`
}

// SubaccountDestinationSpec defines the desired state.
type SubaccountDestinationSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       SubaccountDestinationParameters     `json:"forProvider"`
	InitProvider      SubaccountDestinationInitParameters `json:"initProvider,omitempty"`
}

// SubaccountDestinationStatus defines the observed state.
type SubaccountDestinationStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          SubaccountDestinationObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// SubaccountDestination manages a destination in a SAP BTP subaccount via
// the Destination Service REST API.
//
// External-Name Configuration:
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format: `<subaccount-id>/<destination-name>`
//   - How to find:
//     - UI: SAP BTP Cockpit → Subaccount → Connectivity → Destinations (field: Name)
//     - API: GET /v1/subaccountDestinations/\{destination name\} (fields: subaccount_id + Name)
//
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,account}
// +kubebuilder:subresource:status
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
