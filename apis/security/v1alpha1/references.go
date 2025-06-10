package v1alpha1

import xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

// CredentialsReference contains the credentials needed to access the xsuaa api
type XSUAACredentialsReference struct {
	// xsuaa api credentials used to manage the assignment
	// +kubebuilder:validation:Optional
	APICredentials APICredentials `json:"apiCredentials"`

	// +kubebuilder:validation:Optional
	SubaccountApiCredentialSelector *xpv1.Selector `json:"subaccountApiCredentialSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountApiCredentialRef *xpv1.Reference `json:"subaccountApiCredentialRef,omitempty" reference-group:"security.btp.sap.crossplane.io" reference-kind:"SubaccountApiCredential" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredential
	// +crossplane:generate:reference:refFieldName=SubaccountApiCredentialRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountApiCredentialSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredentialSecret()
	SubaccountApiCredentialSecret string `json:"subaccountApiCredentialSecret,omitempty"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredential
	// +crossplane:generate:reference:refFieldName=SubaccountApiCredentialRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountApiCredentialSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/security/v1alpha1.SubaccountApiCredentialSecretSecretNamespace()
	SubaccountApiCredentialSecretNamespace string `json:"subaccountApiCredentialSecretNamespace,omitempty"`
}

// APICredentials are the credentials to authenticate against the xsuaa api
type APICredentials struct {
	// Source of the credentials.
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem;""
	Source xpv1.CredentialsSource `json:"source"`

	xpv1.CommonCredentialSelectors `json:",inline"`
}
