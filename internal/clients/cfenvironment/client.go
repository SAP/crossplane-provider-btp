package environments

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

type Client interface {
	DescribeInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) (
		*provisioningclient.BusinessEnvironmentInstanceResponseObject,
		[]v1alpha1.User,
		error,
	)
	CreateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error
	UpdateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error
	DeleteInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error

	NeedsUpdate(cr v1alpha1.CloudFoundryEnvironment) bool
}

func ExternalName(environment *provisioningclient.BusinessEnvironmentInstanceResponseObject) *string {
	if environment == nil {
		return nil
	}
	details, err := GetConnectionDetails(environment)
	if err != nil {
		return nil
	}
	orgnameb := details[v1alpha1.ResourceOrgName]
	if orgnameb == nil {
		return nil
	}
	orgname := string(orgnameb)
	return &orgname
}

func GenerateObservation(
	environment *provisioningclient.BusinessEnvironmentInstanceResponseObject,
	managers []v1alpha1.User,
) v1alpha1.CfEnvironmentObservation {
	observation := v1alpha1.CfEnvironmentObservation{}

	if environment == nil {
		return observation
	}

	observation.BrokerID = environment.BrokerId
	observation.CommercialType = environment.CommercialType
	if environment.CreatedDate != nil {
		observation.CreatedDate = internal.Ptr(fmt.Sprintf("%f", *environment.CreatedDate))
	}
	observation.CustomLabels = environment.CustomLabels
	observation.DashboardURL = environment.DashboardUrl
	observation.Description = environment.Description
	observation.EnvironmentType = environment.EnvironmentType
	observation.GlobalAccountGUID = environment.GlobalAccountGUID
	observation.ID = environment.Id
	observation.Labels = environment.Labels
	observation.LandscapeLabel = environment.LandscapeLabel
	if environment.ModifiedDate != nil {
		observation.ModifiedDate = internal.Ptr(fmt.Sprintf("%f", *environment.ModifiedDate))
	}
	observation.Name = environment.Name
	observation.Operation = environment.Operation
	observation.Parameters = environment.Parameters
	observation.PlanID = environment.PlanId
	observation.PlanName = environment.PlanName
	observation.PlatformID = environment.PlatformId
	observation.ServiceID = environment.ServiceId
	observation.ServiceName = environment.ServiceName
	observation.State = environment.State
	observation.StateMessage = environment.StateMessage
	observation.SubaccountGUID = environment.SubaccountGUID
	observation.TenantID = environment.TenantId
	observation.Type = environment.Type
	observation.Managers = managers

	return observation
}

func GetConnectionDetails(instance *provisioningclient.BusinessEnvironmentInstanceResponseObject) (managed.ConnectionDetails, error) {
	if instance == nil {
		return managed.ConnectionDetails{}, nil
	}
	var cflabels cfEnvironmentLabels
	var label string
	if instance.Labels != nil {
		label = *instance.Labels
	}
	if err := json.Unmarshal([]byte(label), &cflabels); err != nil {
		return managed.ConnectionDetails{}, err
	}
	details := managed.ConnectionDetails{
		v1alpha1.ResourceRaw: []byte(label),
	}

	if cflabels.OrgName != nil {
		details[v1alpha1.ResourceOrgName] = []byte(*cflabels.OrgName)
	}
	if cflabels.OrgId != nil {
		details[v1alpha1.ResourceOrgId] = []byte(*cflabels.OrgId)
	}
	if cflabels.ApiEndpoint != nil {
		details[v1alpha1.ResourceAPIEndpoint] = []byte(*cflabels.ApiEndpoint)
	}

	return details, nil
}

type cfEnvironmentLabels struct {
	ApiEndpoint *string `json:"API Endpoint,omitempty"`
	OrgName     *string `json:"Org Name,omitempty"`
	OrgId       *string `json:"Org ID,omitempty"`
}
