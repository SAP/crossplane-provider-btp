package servicemanager

import (
	"context"
	"fmt"

	"github.com/sap/crossplane-provider-btp/internal"
	accountsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	ctrl "sigs.k8s.io/controller-runtime"
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

// SemanticLookuper returns a SemanticLookuper backed by the subaccount's
// existing service-manager admin binding, used by the orphaned-external-name
// adoption heal path for the ServiceManager resource. It returns (nil, nil)
// when no admin binding exists yet (i.e. the service-manager instance has not
// been created in BTP, so there is nothing to adopt).
func (t ServiceManagerInstanceProxyClient) SemanticLookuper(ctx context.Context, subaccountGuid string) (SemanticLookuper, error) {
	binding, err := t.describeAdminBinding(ctx, subaccountGuid)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, nil
	}
	return NewServiceManagerClient(ctx, binding)
}

// EnsureSemanticLookuper returns a SemanticLookuper with full subaccount
// visibility, backed by the subaccount-admin service-manager binding. Unlike
// SemanticLookuper it MINTS a temporary admin binding via the accounts-service
// when none exists yet, and returns a cleanup function that removes that
// temporary binding again (no-op when an existing binding was reused).
//
// This is the credential source the SI/SB/CM adoption heal must use: the
// per-resource serviceManagerSecret bindings are platform-scoped and do not
// list instances created via the btp terraform provider, whereas the
// subaccount-admin binding sees the whole subaccount.
func (t ServiceManagerInstanceProxyClient) EnsureSemanticLookuper(ctx context.Context, subaccountGuid string) (SemanticLookuper, func(), error) {
	noop := func() {}

	binding, err := t.describeAdminBinding(ctx, subaccountGuid)
	if err != nil {
		return nil, noop, err
	}
	cleanup := noop
	if binding == nil {
		// mint a temporary admin binding; caller must call cleanup to remove it.
		binding, err = t.createAdminBinding(ctx, subaccountGuid)
		if err != nil {
			return nil, noop, err
		}
		// Detach the cleanup delete from ctx so that a reconcile timeout /
		// cancellation — the common case that motivates cleanup in the first
		// place — does not silently orphan the temporary admin binding when the
		// caller defers cleanup(). Also log the error instead of dropping it on
		// the floor so a persistent failure is at least visible.
		cleanup = func() {
			delCtx := context.WithoutCancel(ctx)
			if dErr := t.deleteAdminBinding(delCtx, subaccountGuid); dErr != nil {
				ctrl.Log.Info("EnsureSemanticLookuper cleanup: failed to delete temporary admin binding",
					"subaccountGuid", subaccountGuid, "error", dErr.Error())
			}
		}
	}

	cl, err := NewServiceManagerClient(ctx, binding)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	return cl, cleanup, nil
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
