package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// GeneratedCertificateValidity is the validity requested for a generated certificate.
type GeneratedCertificateValidity struct {
	// TimeUnit of Value.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=DAYS;MONTHS;YEARS
	TimeUnit string `json:"timeUnit"`

	// Value in TimeUnit. The Destination service enforces the SAP Cloud PKI maximum.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Value int32 `json:"value"`
}

// SubaccountDestinationGeneratedCertificateParameters are the configurable fields.
// Certificate fields are immutable: changing a generated certificate would turn
// it into an uploaded one. The binding and subaccount references stay mutable.
// +kubebuilder:validation:XValidation:rule="has(self.commonName) == has(oldSelf.commonName) && (!has(self.commonName) || self.commonName == oldSelf.commonName)",message="commonName can't be added, changed or removed once set"
// +kubebuilder:validation:XValidation:rule="has(self.validity) == has(oldSelf.validity) && (!has(self.validity) || self.validity == oldSelf.validity)",message="validity can't be added, changed or removed once set"
// +kubebuilder:validation:XValidation:rule="has(self.passwordSecretRef) == has(oldSelf.passwordSecretRef) && (!has(self.passwordSecretRef) || self.passwordSecretRef == oldSelf.passwordSecretRef)",message="passwordSecretRef can't be added, changed or removed once set"
type SubaccountDestinationGeneratedCertificateParameters struct {
	// Name of the certificate in the subaccount certificate store, ending in
	// .pem (private key and certificate chain) or .p12 (PKCS#12 keystore).
	// Destinations reference it via the KeyStoreLocation property.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^.+\.(pem|p12)$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name can't be updated once set"
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

	// DestinationServiceBindingSecretRef points to a Kubernetes Secret containing
	// Destination Service OAuth2 credentials of a binding in the same subaccount
	// as subaccountId. The secret can be created by a ServiceBinding CR
	// (recommended) or manually.
	//
	// Two secret formats are accepted:
	//   - Flat keys (set key to ""): clientid, clientsecret, tokenurl/token_url, uri/url
	//   - Single JSON key (set key): the named key holds a JSON object with those fields
	//
	// +kubebuilder:validation:Required
	DestinationServiceBindingSecretRef *xpv1.SecretKeySelector `json:"destinationServiceBindingSecretRef"`

	// CommonName (CN) of the certificate. Server default applies when unset.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	CommonName *string `json:"commonName,omitempty"`

	// Validity of the certificate. Server default applies when unset.
	// +optional
	Validity *GeneratedCertificateValidity `json:"validity,omitempty"`

	// AutomaticRenew lets the Destination service renew the certificate before
	// it expires, keeping name, password and type.
	// +optional
	// +kubebuilder:default=false
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="automaticRenew can't be updated once set"
	AutomaticRenew *bool `json:"automaticRenew,omitempty"`

	// PasswordSecretRef points to the Secret key holding the password for the
	// private key (.pem) or keystore (.p12). The value must not be empty. Read
	// only when the certificate is generated. If unset, the key isn't
	// password-protected, as when generated in the cockpit.
	// +optional
	PasswordSecretRef *xpv1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
}

// GeneratedCertificateObservation holds public data of the leaf certificate.
type GeneratedCertificateObservation struct {
	// Subject DN, RFC 2253.
	Subject string `json:"subject"`
	// CommonName of the subject.
	CommonName string `json:"commonName"`
	// Issuer DN, RFC 2253.
	Issuer string `json:"issuer"`
	// SerialNumber as uppercase hex. Changes on renewal.
	SerialNumber string `json:"serialNumber"`
	// NotBefore of the leaf certificate.
	NotBefore metav1.Time `json:"notBefore"`
	// NotAfter of the leaf certificate. Moves forward on renewal.
	NotAfter metav1.Time `json:"notAfter"`
	// SHA256Fingerprint of the DER-encoded leaf certificate, lowercase hex.
	SHA256Fingerprint string `json:"sha256Fingerprint"`
}

// SubaccountDestinationGeneratedCertificateObservation holds fields observed from the API.
type SubaccountDestinationGeneratedCertificateObservation struct {
	// Name of the certificate.
	// +optional
	Name *string `json:"name,omitempty"`

	// Certificate is the public data of the current leaf certificate.
	// +optional
	Certificate *GeneratedCertificateObservation `json:"certificate,omitempty"`
}

// SubaccountDestinationGeneratedCertificateSpec defines the desired state.
type SubaccountDestinationGeneratedCertificateSpec struct {
	xpv1.ClusterManagedResourceSpec `json:",inline"`
	ForProvider                     SubaccountDestinationGeneratedCertificateParameters `json:"forProvider"`
}

// SubaccountDestinationGeneratedCertificateStatus defines the observed state.
type SubaccountDestinationGeneratedCertificateStatus struct {
	xpv1.ManagedResourceStatus `json:",inline"`
	AtProvider                 SubaccountDestinationGeneratedCertificateObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// SubaccountDestinationGeneratedCertificate manages a certificate that the
// Destination service generates with SAP Cloud PKI in a SAP BTP subaccount.
// The key pair stays in BTP; status reports only public data of the leaf
// certificate.
//
// The certificate is never regenerated. To rotate, create a resource with a
// new name, repoint the destinations, then delete the old one. SAP Cloud PKI
// doesn't revoke certificates: a deleted or renewed certificate stays valid
// until it expires. .p12 keystores aren't supported in FIPS environments.
//
// Use it from a ClientCertificateAuthentication SubaccountDestination: set
// KeyStoreLocation to the certificate name in additionalProperties and
// KeyStorePassword via additionalConfigurationSecretRefs.
//
// The Destination REST API can't tell a generated certificate from an uploaded
// one and doesn't return auto-renew or validity, so a re-upload outside
// Crossplane drops auto-renew unnoticed. For the same reason, don't adopt a
// generated certificate into SubaccountDestinationCertificate.
//
// The binding in destinationServiceBindingSecretRef must belong to the
// subaccount in subaccountId; otherwise the resource goes Synced=False and a
// delete blocks. If the binding can't be fixed and the resource must go, set
// deletionPolicy: Orphan.
//
// External-Name Configuration:
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format: `<subaccount-id>/<certificate-name>`
//   - How to find:
//   - UI: SAP BTP Cockpit → Subaccount → Connectivity → Certificates (field: Name)
//   - API: certificate name = path parameter of GET /v1/subaccountCertificates/\{certificate name\}/publicKeys; subaccount ID = the Subaccount's GUID (cockpit or Subaccount status.atProvider)
//
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="CN",type="string",JSONPath=".status.atProvider.certificate.commonName"
// +kubebuilder:printcolumn:name="NOT-AFTER",type="string",JSONPath=".status.atProvider.certificate.notAfter"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,account}
// +kubebuilder:subresource:status
type SubaccountDestinationGeneratedCertificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SubaccountDestinationGeneratedCertificateSpec   `json:"spec"`
	Status            SubaccountDestinationGeneratedCertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubaccountDestinationGeneratedCertificateList contains a list of SubaccountDestinationGeneratedCertificate.
type SubaccountDestinationGeneratedCertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SubaccountDestinationGeneratedCertificate `json:"items"`
}

var (
	SubaccountDestinationGeneratedCertificate_Kind             = "SubaccountDestinationGeneratedCertificate"
	SubaccountDestinationGeneratedCertificate_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: SubaccountDestinationGeneratedCertificate_Kind}.String()
	SubaccountDestinationGeneratedCertificate_GroupVersionKind = CRDGroupVersion.WithKind(SubaccountDestinationGeneratedCertificate_Kind)
)

func init() {
	SchemeBuilder.Register(&SubaccountDestinationGeneratedCertificate{}, &SubaccountDestinationGeneratedCertificateList{})
}
