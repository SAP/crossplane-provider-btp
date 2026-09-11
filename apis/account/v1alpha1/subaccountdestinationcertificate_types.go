package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// SubaccountDestinationCertificateInitParameters holds reference-resolver fields.
// Mirrors the reference fields in SubaccountDestinationCertificateParameters.
type SubaccountDestinationCertificateInitParameters struct {
	// SubaccountID is the GUID of the subaccount that owns this certificate.
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

	// Name of the certificate. Immutable after creation.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name can't be updated once set"
	Name string `json:"name,omitempty"`

	// Content is the base64-encoded certificate (PEM or DER).
	// +optional
	Content string `json:"content,omitempty"`

	// Type is the certificate type (e.g. PEM). Optional; API defaults apply when absent.
	// +optional
	Type string `json:"type,omitempty"`

	// DestinationServiceBindingSecretRef points to a Kubernetes Secret containing
	// Destination Service OAuth2 credentials.
	// +optional
	DestinationServiceBindingSecretRef *xpv1.SecretKeySelector `json:"destinationServiceBindingSecretRef,omitempty"`
}

// SubaccountDestinationCertificateParameters are the configurable fields.
type SubaccountDestinationCertificateParameters struct {
	// Name of the certificate. Immutable after creation.
	// Used as the second segment of the external-name annotation.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// SubaccountID is the GUID of the subaccount that owns this certificate.
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

	// Content is the base64-encoded certificate (PEM or DER).
	// +kubebuilder:validation:Required
	Content string `json:"content"`

	// Type is the certificate type (e.g. PEM). Optional; API defaults apply when absent.
	// +optional
	Type string `json:"type,omitempty"`

	// DestinationServiceBindingSecretRef points to a Kubernetes Secret containing
	// Destination Service OAuth2 credentials. The secret can be created by a
	// ServiceBinding CR (recommended) or manually.
	//
	// Two secret formats are accepted:
	//   - Flat keys (leave key empty): clientid, clientsecret, tokenurl/token_url, uri/url
	//   - Single JSON key (set key): the named key holds a JSON object with those fields
	//
	// +kubebuilder:validation:Required
	DestinationServiceBindingSecretRef *xpv1.SecretKeySelector `json:"destinationServiceBindingSecretRef"`
}

// SubaccountDestinationCertificateObservation holds fields observed from the API.
type SubaccountDestinationCertificateObservation struct {
	// Name of the certificate as reported by the API.
	// +optional
	Name *string `json:"name,omitempty"`

	// Type of the certificate as reported by the API.
	// +optional
	Type *string `json:"type,omitempty"`
}

// SubaccountDestinationCertificateSpec defines the desired state.
type SubaccountDestinationCertificateSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       SubaccountDestinationCertificateParameters     `json:"forProvider"`
	InitProvider      SubaccountDestinationCertificateInitParameters `json:"initProvider,omitempty"`
}

// SubaccountDestinationCertificateStatus defines the observed state.
type SubaccountDestinationCertificateStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          SubaccountDestinationCertificateObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// SubaccountDestinationCertificate manages a certificate in a SAP BTP subaccount via
// the Destination Service REST API.
//
// External-Name Configuration:
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format: `<subaccount-id>/<certificate-name>`
//   - How to find:
//   - UI: SAP BTP Cockpit → Subaccount → Connectivity → Certificates (field: Name)
//   - API: GET /v1/subaccountCertificates/\{certificate name\} (fields: subaccount_id + Name)
//
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,account}
// +kubebuilder:subresource:status
type SubaccountDestinationCertificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SubaccountDestinationCertificateSpec   `json:"spec"`
	Status            SubaccountDestinationCertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubaccountDestinationCertificateList contains a list of SubaccountDestinationCertificate.
type SubaccountDestinationCertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SubaccountDestinationCertificate `json:"items"`
}

var (
	SubaccountDestinationCertificate_Kind             = "SubaccountDestinationCertificate"
	SubaccountDestinationCertificate_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: SubaccountDestinationCertificate_Kind}.String()
	SubaccountDestinationCertificate_GroupVersionKind = CRDGroupVersion.WithKind(SubaccountDestinationCertificate_Kind)
)

func init() {
	SchemeBuilder.Register(&SubaccountDestinationCertificate{}, &SubaccountDestinationCertificateList{})
}
