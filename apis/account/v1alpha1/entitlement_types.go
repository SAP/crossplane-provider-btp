package v1alpha1

import (
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

const (
	EntitlementStatusOk               = "OK"
	EntitlementStatusProcessingFailed = "PROCESSING_FAILED"
	EntitlementStatusProcessing       = "PROCESSING"
	EntitlementStatusStarted          = "STARTED"
)

type EntitlementParameters struct {
	ServicePlanName string `json:"servicePlanName"`
	ServiceName     string `json:"serviceName"`
	//+kubebuilder:validation:Optional
	// The unique identifier of the service plan. This is a unique identifier for service plans that can distinguish between the same service plans with different hosting datacenters. Options Include `hana-cloud-hana` or `hana-cloud-hana-sap_eu-de-1`.
	ServicePlanUniqueIdentifier *string `json:"servicePlanUniqueIdentifier,omitempty"`
	// Whether to enable the service plan assignment to the specified subaccount without quantity restrictions. Relevant and mandatory only for plans that do not have a numeric quota. Do not set if amount is specified.
	Enable *bool `json:"enable,omitempty"`
	// The quantity of the plan that is assigned to the specified subaccount. Relevant and mandatory only for plans that have a numeric quota. Do not set if enable=TRUE is specified.
	Amount *int `json:"amount,omitempty"`
	// External resources to assign to subaccount
	Resources []*Resource `json:"resources,omitempty"`

	// +crossplane:generate:reference:type=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount
	// +crossplane:generate:reference:refFieldName=SubaccountRef
	// +crossplane:generate:reference:selectorFieldName=SubaccountSelector
	// +crossplane:generate:reference:extractor=github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()
	SubaccountGuid string `json:"subaccountGuid,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountSelector *xpv1.Selector `json:"subaccountSelector,omitempty"`
	// +kubebuilder:validation:Optional
	SubaccountRef *xpv1.Reference `json:"subaccountRef,omitempty" reference-group:"account.btp.sap.crossplane.io" reference-kind:"Subaccount" reference-apiversion:"v1alpha1"`
}

// EntitlementObservation are the observable fields of an Entitlement.
type EntitlementObservation struct {
	// Required is a calculated field from all entitlements for the same subaccount, service plan and service.
	Required *EntitlementSummary `json:"summary,omitempty"`
	// Assigned is the return value from the service
	Assigned *Assignable `json:"assigned,omitempty"`
	// Entitled is the overall available quota for the global account / directory which is available to assign
	Entitled Entitled `json:"entitled,omitempty"`
}

type Assignable struct {

	// The quantity of the entitlement that is assigned to the root global account or directory.
	Amount *int `json:"amount,omitempty"`

	// Whether the plan is automatically distributed to the subaccounts that are located in the directory.
	AutoAssign bool `json:"autoAssign,omitempty"`

	// Specifies if the plan was automatically assigned regardless of any action by an admin. This applies to entitlements that are always available to subaccounts and cannot be removed.
	AutoAssigned bool `json:"autoAssigned,omitempty"`

	// The amount of the entitlement to automatically assign to subaccounts that are added in the future to the entitlement's assigned directory.
	// Requires that autoAssign is set to TRUE, and there is remaining quota for the entitlement. To automatically distribute to subaccounts that are added in the future to the directory, distribute must be set to TRUE.
	AutoDistributeAmount int32 `json:"autoDistributeAmount,omitempty"`

	// The unique ID of the global account or directory to which the entitlement is assigned.
	// Example: GUID of GLOBAL_ACCOUNT or SUBACCOUNT
	EntityID string `json:"entityId,omitempty"`

	// The current state of the service plan assignment.
	// * <b>STARTED:</b> CRUD operation on an entity has started.
	// * <b>PROCESSING:</b> A series of operations related to the entity is in progress.
	// * <b>PROCESSING_FAILED:</b> The processing operations failed.
	// * <b>OK:</b> The CRUD operation or series of operations completed successfully.
	// Enum: [STARTED PROCESSING PROCESSING_FAILED OK]
	EntityState string `json:"entityState,omitempty"`

	// The type of entity to which the entitlement is assigned.
	// * <b>SUBACCOUNT:</b> The entitlement is assigned to a subaccount.
	// * <b>GLOBAL_ACCOUNT:</b> The entitlement is assigned to a root global account.
	// * <b>DIRECTORY:</b> The entitlement is assigned to a directory.
	// Example: GLOBAL_ACCOUNT or SUBACCOUNT
	// Enum: [SUBACCOUNT GLOBAL_ACCOUNT DIRECTORY]
	EntityType string `json:"entityType,omitempty"`

	// The requested amount when it is different from the actual amount because the request state is still in process or failed.
	RequestedAmount int `json:"requestedAmount,omitempty"`

	// Information about the current state.
	StateMessage string `json:"stateMessage,omitempty"`

	// True, if an unlimited quota of this service plan assigned to the directory or subaccount in the global account. False, if the service plan is assigned to the directory or subaccount with a limited numeric quota, even if the service plan has an unlimited usage entitled on the level of the global account.
	UnlimitedAmountAssigned bool `json:"unlimitedAmountAssigned,omitempty"`
	//resource details
	Resources []*Resource `json:"resources"`
}

type Entitled struct {
	// The assigned quota for maximum allowed consumption of the plan. Relevant for services that have a numeric quota assignment.
	Amount int `json:"amount,omitempty"`

	// Whether to automatically assign a quota of the entitlement to a subaccount when the subaccount is created in the entitlement's assigned directory.
	AutoAssign bool `json:"autoAssign,omitempty"`

	// The amount of the entitlement to automatically assign to a subaccount when the subaccount is created in the entitlement's assigned directory.
	// Requires that autoAssign is set to TRUE, and there is remaining quota for the entitlement.
	AutoDistributeAmount int `json:"autoDistributeAmount,omitempty"`

	// Whether the service plan is available internally to SAP users.
	AvailableForInternal bool `json:"availableForInternal,omitempty"`

	// Whether the service plan is a beta feature.
	Beta bool `json:"beta,omitempty"`

	// The type of service offering. Possible values:
	// * <b>PLATFORM:</b> A service required for using a specific platform; for example, Application Runtime is required for the Cloud Foundry platform.
	// * <b>SERVICE:</b> A commercial or technical service. that has a numeric quota (amount) when entitled or assigned to a resource. When assigning entitlements of this type, use the 'amount' option instead of 'enable'. See: PUT/entitlements/v1/directories/{directoryGUID}/assignments.
	// * <b>ELASTIC_SERVICE:</b> A commercial or technical service that has no numeric quota (amount) when entitled or assigned to a resource. Generally this type of service can be as many times as needed when enabled, but may in some cases be restricted by the service owner. When assigning entitlements of this type, use the 'enable' option instead of 'amount'. See: PUT/entitlements/v1/directories/{directoryGUID}/assignments.
	// * <b>ELASTIC_LIMITED:</b> An elastic service that can be enabled for only one subaccount per global account.
	// * <b>APPLICATION:</b> A multitenant application to which consumers can subscribe. As opposed to applications defined as a 'QUOTA_BASED_APPLICATION', these applications do not have a numeric quota and are simply enabled or disabled as entitlements per subaccount.
	// * <b>QUOTA_BASED_APPLICATION:</b> A multitenant application to which consumers can subscribe. As opposed to applications defined as 'APPLICATION', these applications have an numeric quota that limits consumer usage of the subscribed application per subaccount. When maxAllowedSubaccountQuota is > 0, this is the limit that can be set when assigning the max quota entitlement of the app to any subaccount. If maxAllowedSubaccountQuota is = 0 or null, the max quota that can be entitled to any subaccount is the amount purchased by the customer (the global account quota).
	// * <b>ENVIRONMENT:</b> An environment service; for example, Cloud Foundry.
	// Enum: [APPLICATION ELASTIC_LIMITED ELASTIC_SERVICE ENVIRONMENT PLATFORM QUOTA_BASED_APPLICATION SERVICE]
	Category string `json:"category,omitempty"`

	// Description of the service plan for customer-facing UIs.
	Description string `json:"description,omitempty"`

	// Display name of the service plan for customer-facing UIs.
	DisplayName string `json:"displayName,omitempty"`

	// The quota limit that is allowed for this service plan for SAP internal users.
	// If null, the default quota limit is set to 200.
	// Applies only when the availableForInternal property is set to TRUE.
	InternalQuotaLimit int `json:"internalQuotaLimit,omitempty"`

	// The maximum allowed usage quota per subaccount for multitenant applications and environments that are defined as "quota-based". This quota limits the usage of the application and/or environment per subaccount per a given usage metric that is defined within the application or environment by the service provider. If null, the usage limit per subaccount is the maximum free quota in the global account.
	// For example, a value of 1 could: (1) limit the number of subscriptions to a quota-based multitenant application within a global account according to the purchased quota, or (2) restrict the enablement of a single instance of an environment per subaccount.
	MaxAllowedSubaccountQuota int `json:"maxAllowedSubaccountQuota,omitempty"`

	// The unique registration name of the service plan.
	Name string `json:"name,omitempty"`

	// [DEPRECATED] The source that added the service. Possible values:
	// * <b>VENDOR:</b> The product has been added by SAP or the cloud operator to the product catalog for general use.
	// * <b>GLOBAL_ACCOUNT_OWNER:</b> Custom services that are added by a customer and are available only for that customer’s global account.
	// * <b>PARTNER:</b> Service that are added by partners. And only available to its customers.
	//
	// Note: This property is deprecated. Please use the ownerType attribute on the entitledService level instead.
	// Enum: [GLOBAL_ACCOUNT_OWNER PARTNER VENDOR]
	ProvidedBy string `json:"providedBy,omitempty"`

	// The method used to provision the service plan.
	// * <b>SERVICE_BROKER:</b> Provisioning of NEO or CF quotas done by the service broker.
	// * <b>NONE_REQUIRED:</b> Provisioning of CF quotas done by setting amount at provisioning-service.
	// * <b>COMMERCIAL_SOLUTION_SCRIPT:</b> Provisioning is done by a script provided by the service owner and run by the Core Commercial Foundation service.
	// * <b>GLOBAL_COMMERCIAL_SOLUTION_SCRIPT:</b> Provisioning is done by a script provided by the service owner and run by the Core Commercial Foundation service used for Global Account level.
	// * <b>GLOBAL_QUOTA_DOMAIN_DB:</b> Provisioning is done by setting amount at Domain DB, this is relevant for non-ui quotas only.
	// * <b>CLOUD_AUTOMATION:</b> Provisioning is done by the cloud automation service. This is relevant only for provisioning that requires external providers that are not within the scope of CIS.
	//
	// Enum: [CLOUD_AUTOMATION COMMERCIAL_SOLUTION_SCRIPT GLOBAL_COMMERCIAL_SOLUTION_SCRIPT GLOBAL_QUOTA_DOMAIN_DB NONE_REQUIRED SERVICE_BROKER]
	ProvisioningMethod string `json:"provisioningMethod,omitempty"`

	// The remaining amount of the plan that can still be assigned. For plans that don't have a numeric quota, the remaining amount is always the maximum allowed quota.
	RemainingAmount int `json:"remainingAmount,omitempty"`

	// Remote service resources provided by non-SAP cloud vendors, and which are offered by this plan.
	Resources []*Resource `json:"resources"`

	// A unique identifier for service plans that can distinguish between the same service plans with different pricing plans.
	UniqueIdentifier string `json:"uniqueIdentifier,omitempty"`

	// unlimited
	Unlimited bool `json:"unlimited,omitempty"`
}

type Resource struct {
	// The name of the resource.
	ResourceName string `json:"name,omitempty"`

	// The name of the provider.
	ResourceProvider string `json:"provider,omitempty"`

	// The unique name of the resource.
	ResourceTechnicalName string `json:"technicalName,omitempty"`

	// The type of the provider. For example infrastructure-as-a-service (IaaS).
	ResourceType string `json:"type,omitempty"`
}

// An EntitlementSpec defines the desired state of an Entitlement.
type EntitlementSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       EntitlementParameters `json:"forProvider"`
}

// EntitlementSummary represents the required properties for all entitlements of the same kind / service / serviceplan
type EntitlementSummary struct {
	// Whether to enable the service plan assignment to the specified subaccount without quantity restrictions. Relevant and mandatory only for plans that do not have a numeric quota. Do not set if amount is specified.
	Enable *bool `json:"enable,omitempty"`
	// The quantity of the plan that is assigned to the specified subaccount. Relevant and mandatory only for plans that have a numeric quota. Do not set if enable=TRUE is specified.
	Amount *int `json:"amount,omitempty"`
	// External resources to assign to subaccount
	Resources []*Resource `json:"resources,omitempty"`
	// Amount of managed entitlements of the same kind / service / serviceplan
	EntitlementsCount *int `json:"entitlementsCount"`
}

// An EntitlementStatus represents the observed state of an Entitlement.
type EntitlementStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          *EntitlementObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// An Entitlement is a managed resource that represents an entitlement in the SAP Business Technology Platform
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="VALIDATION",type="string",JSONPath=".status.conditions[?(@.type=='SoftValidation')].reason"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,btp}
type Entitlement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EntitlementSpec   `json:"spec"`
	Status EntitlementStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EntitlementList contains a list of Entitlement
type EntitlementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Entitlement `json:"items"`
}

// Entitlement type metadata.
var (
	EntitlementKind             = reflect.TypeOf(Entitlement{}).Name()
	EntitlementGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: EntitlementKind}.String()
	EntitlementKindAPIVersion   = EntitlementKind + "." + CRDGroupVersion.String()
	EntitlementGroupVersionKind = CRDGroupVersion.WithKind(EntitlementKind)
)

func init() {
	SchemeBuilder.Register(&Entitlement{}, &EntitlementList{})
}

const SoftValidationCondition xpv1.ConditionType = "SoftValidation"
const HasValidationIssues xpv1.ConditionReason = "ValidationIssuesFound"
const NoValidationIssues xpv1.ConditionReason = "NoValidationIssuesFound"

func ValidationError(msg string) xpv1.Condition {
	return xpv1.Condition{
		Type:               SoftValidationCondition,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             HasValidationIssues,
		Message:            msg,
	}
}

func ValidationOk() xpv1.Condition {
	return xpv1.Condition{
		Type:               SoftValidationCondition,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             NoValidationIssues,
	}
}

func ValidationCondition(validationIssues []string) xpv1.Condition {
	if validationIssues == nil {
		return ValidationOk()
	}

	return ValidationError(strings.Join(validationIssues, "\n"))
}
