package servicemanager

import (
	"context"
	"fmt"

	"github.com/sap/crossplane-provider-btp/internal"
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

		id, err := resolver.PlanIDByName(ctx, ServiceManagerOfferingName, servicePlanName, "", "sapbtp")
		if err != nil {
			return "", err
		}
		return id, err
	}
}

func (t ServiceManagerInstanceProxyClient) describeAdminBinding(ctx context.Context, subaccountGuid string) (*BindingCredentials, error) {
	response, raw, err := t.GetServiceManagementBinding(ctx, subaccountGuid).Execute()

	if raw != nil && raw.StatusCode == 404 {
		return nil, nil
	}

	return mapBindingCredentialTypes(response), specifyAccountsAPIError(err)
}

func (t ServiceManagerInstanceProxyClient) createAdminBinding(ctx context.Context, subaccountGuid string) (*BindingCredentials, error) {
	result, _, err := t.CreateServiceManagementBinding(ctx, subaccountGuid).Execute()
	if err != nil {
		return nil, specifyAccountsAPIError(err)
	}
	return mapBindingCredentialTypes(result), nil
}

func (t ServiceManagerInstanceProxyClient) deleteAdminBinding(ctx context.Context, subaccountGuid string) error {
	_, err := t.DeleteServiceManagementBindingOfSubaccount(ctx, subaccountGuid).Execute()
	return specifyAccountsAPIError(err)
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

// specifyAccountsAPIError surfaces the BTP accounts-service error body when present.
func specifyAccountsAPIError(err error) error {
	if genericErr, ok := err.(*accountsserviceclient.GenericOpenAPIError); ok {
		if accountError, ok := genericErr.Model().(accountsserviceclient.ApiExceptionResponseObject); ok {
			return fmt.Errorf("API Error: %v, Code %v", internal.Val(accountError.Error.Message), internal.Val(accountError.Error.Code))
		}
		if genericErr.Body() != nil {
			return fmt.Errorf("API Error: %s", string(genericErr.Body()))
		}
	}
	return err
}
