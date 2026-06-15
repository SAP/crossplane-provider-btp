package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var DirectoryEntityStateOk = "OK"

// DirectoryParameters are the configurable fields of a Directory.
type BaseDirectoryParameters struct {
	// Description of the Directory
	// +optional
	Description *string `json:"description,omitempty"`

	// Additional admins of the directory. Applies only to directories that have the user authorization management feature enabled. Do not add yourself as you are assigned as a directory admin by default. Example: ["admin1@example.com", "admin2@example.com"]
	// +kubebuilder:validation:MinItems=2
	DirectoryAdmins []string `json:"directoryAdmins"`

	// <b>The features to be enabled in the directory. The available features are:</b>
	// -	<b>DEFAULT</b>: (Mandatory) All directories provide the following basic features: (1) Group and filter subaccounts for reports and filters, (2) monitor usage and costs on a directory level (costs only available for contracts that use the consumption-based commercial model), and (3) set custom properties and tags to the directory for identification and reporting purposes.
	// -	<b>ENTITLEMENTS</b>: (Optional) Enables the assignment of a quota for services and applications to the directory from the global account quota for distribution to the subaccounts under this directory.
	// -	<b>AUTHORIZATIONS</b>: (Optional) Allows you to assign users as administrators or viewers of this directory. You must apply this feature in combination with the ENTITLEMENTS feature.
	//
	//
	// IMPORTANT: Your multi-level account hierarchy can have more than one directory enabled with user authorization and/or entitlement management; however, only one directory in any directory path can have these features enabled. In other words, other directories above or below this directory in the same path can only have the default features specified. If you are not sure which features to enable, we recommend that you set only the default features, and then add features later on as they are needed.
	// <br/><b>Valid values:</b>
	// [DEFAULT]
	// [DEFAULT,ENTITLEMENTS]
	// [DEFAULT,ENTITLEMENTS,AUTHORIZATIONS]<br/>
	// Unique: true
	// +optional
	DirectoryFeatures []string `json:"directoryFeatures"`

	// The display name of the directory.
	DisplayName *string `json:"displayName"`

	// JSON array of up to 10 user-defined labels to assign as key-value pairs to the directory. Each label has a name (key) that you specify, and to which you can assign up to 10 corresponding values or leave empty.
	// Keys and values are each limited to 63 characters.
	// Label keys and values are case-sensitive. Try to avoid creating duplicate variants of the same keys or values with a different casing (example: "myValue" and "MyValue").
	//
	// Example:
	// {
	//   "Cost Center": ["19700626"],
	//   "Department": ["Sales"],
	//   "Contacts": ["name1@example.com","name2@example.com"],
	//   "EMEA":[]
	// }
	//
	// +optional
	Labels map[string][]string `json:"labels,omitempty"`

	// Subdomain Applies only to directories that have the user authorization management feature enabled.  The subdomain becomes part of the path used to access the authorization tenant of the directory. Must be unique within the defined region. Use only letters (a-z), digits (0-9), and hyphens (not at start or end). Maximum length is 63 characters. Cannot be changed after the directory has been created.
	// +optional
	Subdomain *string `json:"subdomain,omitempty"`

	// +optional
	DirectoryGuid string `json:"directoryGuid,omitempty"`
}

// DirectoryObservation are the observable fields of a Directory.
type BaseDirectoryObservation struct {
	// The GUID of the directory
	Guid *string `json:"guid,omitempty"`

	// Processing state in external	system
	EntityState *string `json:"entityState,omitempty"`
	// Details related to external processing state
	StateMessage *string `json:"stateMessage,omitempty"`
	// Subdomain currently present in external system
	Subdomain *string `json:"subdomain,omitempty"`
	// Features currently present in external system
	DirectoryFeatures []string `json:"directoryFeatures"`
}

// BaseDirectorySpec defines the desired state of a Directory.
type BaseDirectorySpec struct {
	ForProvider BaseDirectoryParameters `json:"forProvider"`
}

// BaseDirectoryStatus represents the observed state of a Directory.
type BaseDirectoryStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BaseDirectoryObservation `json:"atProvider,omitempty"`
}

// BaseDirectory is the base resource definition for Directory.
// A Directory is a managed resource that allows grouping of subaccounts in the SAP Business Technology Platform
//
// External-Name Configuration:
//   - Follows Standard: yes
//   - Format: Directory GUID (UUID format)
//   - How to find:
//   - UI: Global Account → Account Explorer → Directories → [Select Directory] → Directory ID
//   - CLI: btp list accounts/directory (field: guid)
//
// +codegen:generate:scoped
// +codegen:categories=btp-account
// +kubebuilder:object:generate=false
type BaseDirectory struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BaseDirectorySpec   `json:"spec"`
	Status            BaseDirectoryStatus `json:"status,omitempty"`
}

// DirectoryReferences defines reference configuration for code generation.
// +codegen:references
// +kubebuilder:object:generate=false
type DirectoryReferences struct {
	// +codegen:reference:target=Directory
	// +codegen:reference:group=account.btp.sap.crossplane.io
	// +codegen:reference:apiversion=v1alpha1
	// +codegen:reference:field=Spec.ForProvider.DirectoryGuid
	// +codegen:reference:refName=DirectoryRef
	// +codegen:reference:selectorName=DirectorySelector
	// +codegen:reference:immutableRef=true
	DirectoryRef bool
}
