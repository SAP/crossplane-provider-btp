package servicemanager

import (
	"context"

	accountsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

const ServiceManagerOfferingName = "service-manager"

func NewServiceManagerInstanceProxyClient(apiClient *accountsserviceclient.APIClient) ServiceManagerInstanceProxyClient {
	return ServiceManagerInstanceProxyClient{
		SubaccountOperationsAPI: apiClient.SubaccountOperationsAPI,
		smServiceFn: func(ctx context.Context, credentials *BindingCredentials) (PlanIdResolver, error) {
			return NewServiceManagerClient(ctx, credentials)
		},
	}
}

// ServiceManagerInstanceProxyClient is a throw-away implementation, which retrieves a servicePlanID by
// - creating an intermediate subaccount-admin servicemanager instance via the accountsapi
// - looksup the servicePLanID via those created credentials binding
// - deletes this intermediate servicemanager instance
// -> THIS NEEDS TO BE REPLACED VIA TF DATASOURCES AS SOON AS THOSE ARE AVAILABLE IN UPJET
type ServiceManagerInstanceProxyClient struct {
	accountsserviceclient.SubaccountOperationsAPI

	// serviceManager API Client needs to be configured with a secret thats not known during initialization
	smServiceFn func(ctx context.Context, credentials *BindingCredentials) (PlanIdResolver, error)
}

func (t ServiceManagerInstanceProxyClient) ServiceManagerPlanIDByName(ctx context.Context, subaccountId string, servicePlanName string) (string, error) {
	// if binding exists we use it to resolve serviceplan
	binding, err := t.describeAdminBinding(ctx, subaccountId)
	if err != nil {
		return "", err
	}
	if binding != nil {
		return t.resolveServicePlan(ctx, servicePlanName)(binding)
	}
	// otherwise we dynamically create and delete an instance and resolve the serviceplan using its credentials
	return t.dynamicServiceInstance(ctx, subaccountId, t.resolveServicePlan(ctx, servicePlanName))
}

func (t ServiceManagerInstanceProxyClient) dynamicServiceInstance(ctx context.Context, subaccountId string, resolvalFn func(binding *BindingCredentials) (string, error)) (string, error) {
	binding, err := t.createAdminBinding(ctx, subaccountId)
	if err != nil {
		return "", err
	}

	id, err := resolvalFn(binding)
	if err != nil {
		return "", err
	}

	err = t.deleteAdminBinding(ctx, subaccountId)
	if err != nil {
		return "", err
	}

	return id, nil
}

func (t ServiceManagerInstanceProxyClient) resolveServicePlan(ctx context.Context, servicePlanName string) func(binding *BindingCredentials) (string, error) {
	return func(binding *BindingCredentials) (string, error) {
		resolver, err := t.smServiceFn(ctx, binding)

		if err != nil {
			return "", err
		}

		id, err := resolver.PlanIDByName(ctx, ServiceManagerOfferingName, servicePlanName, "")
		if err != nil {
			return "", err
		}
		return id, err
	}
}

func (t ServiceManagerInstanceProxyClient) describeAdminBinding(ctx context.Context, subaccountGuid string) (*BindingCredentials, error) {
	response, raw, err := t.GetServiceManagementBinding(ctx, subaccountGuid).Execute()

	if raw.StatusCode == 404 {
		return nil, nil
	}

	return mapBindingCredentialTypes(response), err
}

func (t ServiceManagerInstanceProxyClient) createAdminBinding(ctx context.Context, subaccountGuid string) (*BindingCredentials, error) {
	result, _, err := t.CreateServiceManagementBinding(ctx, subaccountGuid).Execute()
	if err != nil {
		return nil, err
	}
	return mapBindingCredentialTypes(result), err
}

func (t ServiceManagerInstanceProxyClient) deleteAdminBinding(ctx context.Context, subaccountGuid string) error {
	_, err := t.DeleteServiceManagementBindingOfSubaccount(ctx, subaccountGuid).Execute()
	return err
}

// mapBindingCredentialTypes is a helper function to convert ServiceManagerBindingResponseObject to BindingCredentials by mapping each value individually
func mapBindingCredentialTypes(in *accountsserviceclient.ServiceManagerBindingResponseObject) *BindingCredentials {
	if in == nil {
		return nil
	}
	out := new(BindingCredentials)
	out.Clientid = in.Clientid
	out.Clientsecret = in.Clientsecret
	out.Url = in.Url
	out.SmUrl = in.SmUrl
	out.Xsappname = in.Xsappname
	return out
}

// FindManagedSMResources looks up the IDs of the managed-service-manager
// service instance and (optionally) its binding pair under the given
// subaccount by name, via the SAP Service Manager API. It uses the existing
// admin binding for that subaccount (without creating a new one) -- if no
// admin binding exists yet, returns ("", "", nil) so the caller can fall back
// to the normal Create path.
//
// Used by the ServiceManager / CloudManagement controllers to recover from
// upjet workspace state loss: when upjet reports the underlying SI/binding
// as not existing despite a prior Create having succeeded on BTP, this
// returns the real IDs so the controller can adopt them into the public CR's
// external-name instead of dropping the finalizer (deletion path) or
// re-creating and hitting 409 Conflict (creation path).
func (t ServiceManagerInstanceProxyClient) FindManagedSMResources(
	ctx context.Context, subaccountID, instanceName, bindingName string,
) (siID string, sbID string, err error) {
	if subaccountID == "" || instanceName == "" {
		return "", "", nil
	}
	binding, err := t.describeAdminBinding(ctx, subaccountID)
	if err != nil || binding == nil {
		return "", "", err
	}
	smClient, err := NewServiceManagerClient(ctx, binding)
	if err != nil {
		return "", "", err
	}
	siID, err = smClient.FindServiceInstanceIDByName(ctx, subaccountID, instanceName)
	if err != nil || siID == "" {
		return "", "", err
	}
	if bindingName == "" {
		return siID, "", nil
	}
	sbID, err = smClient.FindServiceBindingIDByName(ctx, siID, bindingName)
	if err != nil {
		return siID, "", err
	}
	return siID, sbID, nil
}
