package v1beta1

// External-Name Configuration:
//   - Resource: ServiceManager
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format: `<service-instance-id>/<service-binding-id>` (e.g. "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f/9c2b1f80-3d4e-4a11-8f2c-7b5d6e1a4c33"), both canonical 36-character GUIDs; a bare `<service-instance-id>` is the valid transient form while the binding is still being created
//   - Note: `subaccountGuid`, `planName`, `serviceInstanceName` and `serviceBindingName` are immutable once set (v1beta1); changing one strands the instance/binding pair, so delete and recreate instead. Once `subaccountGuid` is resolved, `subaccountRef`/`subaccountSelector` can no longer be repointed, though dropping them is allowed; a replace-style sync must still carry the resolved `subaccountGuid` and any non-default names.
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Services → Instances and Subscriptions → [Select the service manager instance] → the preview pane shows its ID; take the binding ID from the CLI
//     - CLI: `btp list services/instance --subaccount <subaccount-guid>` (field: id), then `btp list services/binding --subaccount <subaccount-guid>` (field: id) for the binding on that instance

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

const (
	ResourceCredentialsClientSecret      = "clientsecret"
	ResourceCredentialsClientId          = "clientid"
	ResourceCredentialsServiceManagerUrl = "sm_url"
	ResourceCredentialsXsuaaUrl          = "tokenurl"
	ResourceCredentialsXsappname         = "xsappname"
	ResourceCredentialsXsuaaUrlSufix     = "tokenurlsuffix"

	DefaultPlanName = "service-operator-access"

	DefaultServiceInstanceName = "managed-service-manager"
	DefaultServiceBindingName  = "managed-service-manager-binding"
)

const (
	ServiceManagerBound   = "BOUND"
	ServiceManagerUnbound = "UNBOUND"
)

// Detached so it stays out of the CRD description. subaccountGuid uses a
// struct-level rule, not "self == oldSelf": the resolver fills it in after
// admission, so a transition rule would reject that write. The subaccountRef and
// subaccountSelector rules stop a retarget looping against that rule under
// `policy.resolve: Always`, where ResolutionRequest.IsNoOp() re-resolves even
// though subaccountGuid is already set. Both are gated on subaccountGuid being
// resolved and only forbid repointing a field present on both sides: removal
// stays legal (subaccountGuid is pinned, so nothing can retarget), and
// subaccountRef is compared by name only, because the resolver's own write-back
// rebuilds it as a bare Reference and would otherwise reject itself. The name
// fields need defaults, as a field-level transition rule is skipped when the
// field is absent.

// ServiceManagerParameters are the configurable fields of a ServiceManager.
//
// ADR(external-name): every field that selects the external resource is immutable
// once set. Changing one would leave crossplane.io/external-name pointing at the
// instance/binding pair created for the old value, stranding it in BTP.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.subaccountGuid) || size(oldSelf.subaccountGuid) == 0 || (has(self.subaccountGuid) && self.subaccountGuid == oldSelf.subaccountGuid)",message="subaccountGuid can't be updated once resolved"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.subaccountGuid) || size(oldSelf.subaccountGuid) == 0 || !has(self.subaccountRef) || !has(oldSelf.subaccountRef) || self.subaccountRef.name == oldSelf.subaccountRef.name",message="subaccountRef can't be repointed after subaccountGuid is resolved"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.subaccountGuid) || size(oldSelf.subaccountGuid) == 0 || !has(self.subaccountSelector) || !has(oldSelf.subaccountSelector) || self.subaccountSelector == oldSelf.subaccountSelector",message="subaccountSelector can't be repointed after subaccountGuid is resolved"
type ServiceManagerParameters struct {
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()
	SubaccountGuid string `json:"subaccountGuid,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Subaccount" reference-apiversion:"v1alpha1"`

	// Planname for service manager instance
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Enum=subaccount-admin;service-operator-access;container;subaccount-audit
	// +kubebuilder:default:=service-operator-access
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="planName can't be updated once set"
	PlanName string `json:"planName,omitempty"`

	// Name of created service instance, Defaults to "managed-service-manager"
	// +kubebuilder:default:=managed-service-manager
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceInstanceName can't be updated once set"
	ServiceInstanceName string `json:"serviceInstanceName,omitempty"`
	// Name of created service binding, Defaults to "managed-service-manager-binding"
	// +kubebuilder:default:=managed-service-manager-binding
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceBindingName can't be updated once set"
	ServiceBindingName string `json:"serviceBindingName,omitempty"`
}

type DataSourceLookup struct {
	ServiceManagerPlanID string `json:"serviceManagerPlanID,omitempty"`
}

// ServiceManagerObservation are the observable fields of a ServiceManager.
type ServiceManagerObservation struct {
	// currently bound to a service manager instance or not (BOUND/UNBOUND)
	Status string `json:"status,omitempty"`
	// currently bound service instance id
	ServiceInstanceID string `json:"serviceInstanceID,omitempty"`
	// currently bound service binding id
	ServiceBindingID string `json:"serviceBindingID,omitempty"`

	DataSourceLookup *DataSourceLookup `json:"dataSourceLookup,omitempty"`
}

// A ServiceManagerSpec defines the desired state of a ServiceManager.
type ServiceManagerSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ServiceManagerParameters `json:"forProvider"`
}

// A ServiceManagerStatus represents the observed state of a ServiceManager.
type ServiceManagerStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ServiceManagerObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceManager is a managed resource that represents a service manager instance and its API credentials in the SAP Business Technology Platform
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
// +kubebuilder:storageversion
type ServiceManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceManagerSpec   `json:"spec"`
	Status ServiceManagerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceManagerList contains a list of ServiceManager
type ServiceManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceManager `json:"items"`
}

// ServiceManager type metadata.
var (
	ServiceManagerKind             = reflect.TypeOf(ServiceManager{}).Name()
	ServiceManagerGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceManagerKind}.String()
	ServiceManagerKindAPIVersion   = ServiceManagerKind + "." + SchemeGroupVersion.String()
	ServiceManagerGroupVersionKind = SchemeGroupVersion.WithKind(ServiceManagerKind)
)

func init() {
	SchemeBuilder.Register(&ServiceManager{}, &ServiceManagerList{})
}
