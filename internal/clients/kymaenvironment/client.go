package environments

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

type Client interface {
	DescribeInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (
		*provisioningclient.BusinessEnvironmentInstanceResponseObject,
		error,
	)
	CreateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error
	UpdateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error
	DeleteInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error
}

func GenerateObservation(
	environment *provisioningclient.BusinessEnvironmentInstanceResponseObject,
) v1alpha1.KymaEnvironmentObservation {
	observation := v1alpha1.KymaEnvironmentObservation{}

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

	return observation
}

func GetConnectionDetails(instance *provisioningclient.BusinessEnvironmentInstanceResponseObject, httpClient *http.Client) (managed.ConnectionDetails, error) {
	labelMap := map[string]string{}

	var iLabel string
	if instance.Labels != nil {
		iLabel = *instance.Labels
	}
	jsonErr := json.Unmarshal([]byte(iLabel), &labelMap)
	if jsonErr != nil {
		return nil, jsonErr
	}

	fileContent, err := readKubeconfigFromUrl(labelMap[v1alpha1.KubeConfigLabelKey], httpClient)
	if err != nil {
		return nil, err
	}

	details := managed.ConnectionDetails{}
	details[v1alpha1.KubeConfigSecretKey] = fileContent
	for k, v := range labelMap { // convert map to right format
		details[k] = []byte(v)
	}

	serverUrl, caData := internal.ParseConnectionDetailsFromKubeYaml(fileContent)
	details["server"] = []byte(serverUrl)
	details["certificate-authority-data"] = []byte(caData)

	return details, nil
}

func readKubeconfigFromUrl(url string, httpClient *http.Client) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("couldn't load kubeconfig file: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("couldn't load kubeconfig file: %v", err)
	}

	return data, nil
}
