package v1beta1

import (
	"reflect"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	CisStatusBound   = "BOUND"
	CisStatusUnbound = "UNBOUND"

	DefaultCloudManagementInstanceName string = "managed-cloud-management"
	DefaultCloudManagementBindingName  string = "managed-cloud-management-binding"
)

// CloudManagementParameters are the configurable fields of a CloudManagement.
type CloudManagementParameters struct {
	// +kubebuilder:validation:Optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Subaccount" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()
	SubaccountGuid string `json:"subaccountGuid,omitempty"`

	// +kubebuilder:validation:Optional
	ServiceManagerSelector *xpv1.Selector `json:"serviceManagerSelector,omitempty"`
	// +kubebuilder:validation:Optional
	ServiceManagerRef *xpv1.Reference `json:"serviceManagerRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"ServiceManager" reference-apiversion:"v1alpha1"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManager
	// +crossplane:generate:reference:refFieldName=ServiceManagerRef
	// +crossplane:generate:reference:selectorFieldName=ServiceManagerSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManagerSecret()
	ServiceManagerSecret string `json:"serviceManagerSecret,omitempty"`
	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManager
	// +crossplane:generate:reference:refFieldName=ServiceManagerRef
	// +crossplane:generate:reference:selectorFieldName=ServiceManagerSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceManagerSecretNamespace()
	ServiceManagerSecretNamespace string `json:"serviceManagerSecretNamespace,omitempty"`

	// Name of created service instance, Defaults to "managed-cloud-management"
	ServiceInstanceName string `json:"serviceInstanceName,omitempty"`
	// Name of created service binding, Defaults to "managed-cloud-management-binding"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceBindingName can't be updated once set"
	ServiceBindingName string `json:"serviceBindingName,omitempty"`
}

type CloudManagementDataSourceLookup struct {
	CloudManagementPlanID string `json:"cloudManagementPlanID,omitempty"`
}

// CloudManagementObservation are the observable fields of a CloudManagement.
type CloudManagementObservation struct {
	Status   string    `json:"status"`
	Instance *Instance `json:"instance,omitempty"`
	Binding  *Binding  `json:"binding,omitempty"`

	// currently bound service instance id
	ServiceInstanceID string `json:"serviceInstanceID,omitempty"`
	// currently bound service binding id
	ServiceBindingID string `json:"serviceBindingID,omitempty"`

	DataSourceLookup *CloudManagementDataSourceLookup `json:"dataSourceLookup,omitempty"`
}

type Instance struct {
	// The ID of the service instance.
	Id *string `json:"id,omitempty"`
	// Whether the service instance is ready.
	Ready *bool `json:"ready,omitempty"`
	// The name of the service instance.
	Name *string `json:"name,omitempty"`
	// The ID of the service plan associated with the service instance.
	ServicePlanId *string `json:"service_plan_id,omitempty"`
	// The ID of the platform to which the service instance belongs.
	PlatformId *string `json:"platform_id,omitempty"`
	// The URL of the web-based management UI for the service instance.
	DashboardUrl *string `json:"dashboard_url,omitempty"`
	// The ID of the instance to which the service instance refers.
	ReferencedInstanceId *string `json:"referenced_instance_id,omitempty"`
	// Whether the service instance is shared.
	Shared *bool `json:"shared,omitempty"`
	// Contextual data for the resource.
	Context *map[string]string `json:"context,omitempty"`
	// The maintenance information associated with the service instance.
	MaintenanceInfo *map[string]string `json:"maintenance_info,omitempty"`
	// Whether the service instance can be used.
	Usable *bool `json:"usable,omitempty"`
	// The time the service instance was created.<br/>In ISO 8601 format:</br> YYYY-MM-DDThh:mm:ssTZD
	CreatedAt *string `json:"created_at,omitempty"`
	// The last time the service instance was updated.<br/> In ISO 8601 format.
	UpdatedAt *string `json:"updated_at,omitempty"`
	// Additional data associated with the resource entity. <br><br>Can be an empty object.
	Labels *map[string][]string `json:"labels,omitempty"`
}

type Binding struct {
	// The ID of the service binding.
	Id *string `json:"id,omitempty"`
	// Whether the service binding is ready.
	Ready *bool `json:"ready,omitempty"`
	// The name of the service binding.
	Name *string `json:"name,omitempty"`
	// The ID of the service instance associated with the binding.
	ServiceInstanceId *string `json:"service_instance_id,omitempty"`
	// Contextual data for the resource.
	Context *map[string]string `json:"context,omitempty"`
	// Contains the resources associated with the binding.
	BindResource *map[string]string `json:"bind_resource,omitempty"`
	// The time the binding was created.<br/>In ISO 8601 format:</br> YYYY-MM-DDThh:mm:ssTZD
	CreatedAt *string `json:"created_at,omitempty"`
	// The last time the binding was updated.<br/> In ISO 8601 format.
	UpdatedAt *string `json:"updated_at,omitempty"`
	// Additional data associated with the resource entity. <br><br>Can be an empty object.
	Labels *map[string][]string `json:"labels,omitempty"`
}

// A CloudManagementSpec defines the desired state of a CloudManagement.
type CloudManagementSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       CloudManagementParameters `json:"forProvider,omitempty"`
}

// A CloudManagementStatus represents the observed state of a CloudManagement.
type CloudManagementStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          CloudManagementObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A CloudManagement is a managed resource that represents a cloud management instance and its api credentials in the SAP Business Technology Platform
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
// +kubebuilder:storageversion
type CloudManagement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudManagementSpec   `json:"spec"`
	Status CloudManagementStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudManagementList contains a list of CloudManagement
type CloudManagementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudManagement `json:"items"`
}

// CloudManagement type metadata.
var (
	CloudManagementKind             = reflect.TypeOf(CloudManagement{}).Name()
	CloudManagementGroupKind        = schema.GroupKind{Group: Group, Kind: CloudManagementKind}.String()
	CloudManagementKindAPIVersion   = CloudManagementKind + "." + SchemeGroupVersion.String()
	CloudManagementGroupVersionKind = SchemeGroupVersion.WithKind(CloudManagementKind)
)

func init() {
	SchemeBuilder.Register(&CloudManagement{}, &CloudManagementList{})
}
