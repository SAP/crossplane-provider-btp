package v1alpha1

import (
	"github.com/crossplane/crossplane-runtime/pkg/reference"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

type EnvironmentObservation struct {

	// The ID of the associated environment broker.
	BrokerID *string `json:"brokerId,omitempty"`

	// The commercial type of the environment broker.
	CommercialType *string `json:"commercialType,omitempty"`

	// The date the environment instance was created. Dates and times are in UTC format.
	CreatedDate *string `json:"createdDate,omitempty"`

	// Custom labels that are defined by a user and assigned as key-value pairs in a JSON array to the environment instance.
	// Example:
	// {
	//   "Cost Center": ["19700626"],
	//   "Department": ["Sales"],
	//   "Contacts": ["name1@example.com","name2@example.com"],
	//   "EMEA":[]
	// }
	// NOTE: Custom labels apply only to SAP BTP. They are not the same labels that might be defined by your environment broker (see "labels" field).
	CustomLabels *map[string][]string `json:"customLabels,omitempty"`

	// The URL of the service dashboard, which is a web-based management user interface for the service instances.
	DashboardURL *string `json:"dashboardUrl,omitempty"`

	// The description of the environment instance.
	Description *string `json:"description,omitempty"`

	// Type of the environment instance that is used.
	// Example: cloudfoundry
	// Enum: [cloudfoundry kubernetes neo]
	EnvironmentType *string `json:"environmentType,omitempty"`

	// The GUID of the global account that is associated with the environment instance.
	GlobalAccountGUID *string `json:"globalAccountGUID,omitempty"`

	// Automatically generated unique identifier for the environment instance.
	ID *string `json:"id,omitempty"`

	// Broker-specified key-value pairs that specify attributes of an environment instance.
	Labels *string `json:"labels,omitempty"`

	// The name of the landscape within the logged-in region on which the environment instance is created.
	LandscapeLabel *string `json:"landscapeLabel,omitempty"`

	// The last date the environment instance was last modified. Dates and times are in UTC format.
	ModifiedDate *string `json:"modifiedDate,omitempty"`

	// Name of the environment instance.
	Name *string `json:"name,omitempty"`

	// An identifier that represents the last operation. This ID is returned by the environment brokers.
	Operation *string `json:"operation,omitempty"`

	// Configuration parameters for the environment instance.
	Parameters *string `json:"parameters,omitempty"`

	// ID of the service plan for the environment instance in the corresponding service broker's catalog.
	PlanID *string `json:"planId,omitempty"`

	// Name of the service plan for the environment instance in the corresponding service broker's catalog.
	PlanName *string `json:"planName,omitempty"`

	// ID of the platform for the environment instance in the corresponding service broker's catalog.
	PlatformID *string `json:"platformId,omitempty"`

	// ID of the service for the environment instance in the corresponding service broker's catalog.
	ServiceID *string `json:"serviceId,omitempty"`

	// Name of the service for the environment instance in the corresponding service broker's catalog.
	ServiceName *string `json:"serviceName,omitempty"`

	// Current state of the environment instance.
	// Example: cloudfoundry
	// Enum: [CREATING UPDATING DELETING OK CREATION_FAILED DELETION_FAILED UPDATE_FAILED]
	State *string `json:"state,omitempty"`

	// Information about the current state of the environment instance.
	StateMessage *string `json:"stateMessage,omitempty"`

	// The GUID of the subaccount associated with the environment instance.
	SubaccountGUID *string `json:"subaccountGUID,omitempty"`

	// The ID of the tenant that owns the environment instance.
	TenantID *string `json:"tenantId,omitempty"`

	// The last provisioning operation on the environment instance.
	// * <b>Provision:</b> CloudFoundryEnvironment instance created.
	// * <b>Update:</b> CloudFoundryEnvironment instance changed.
	// * <b>Deprovision:</b> CloudFoundryEnvironment instance deleted.
	// Example: Provision
	// Enum: [Provision Update Deprovision]
	Type *string `json:"type,omitempty"`
}

// KymaInstanceId is a function that extracts the ID of the KymaEnvironment instance from the managed resource.
func KymaInstanceId() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*KymaEnvironment)
		if !ok {
			return ""
		}
		if sg.Status.AtProvider.ID == nil {
			return ""
		}
		return *sg.Status.AtProvider.ID
	}
}
