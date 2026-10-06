package servicemanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sap/crossplane-provider-btp/internal"
	accountsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	ctrl "sigs.k8s.io/controller-runtime"
)

const ServiceManagerOfferingName = "service-manager"

const adminBindingCleanupTimeout = 30 * time.Second

// TempBindingName builds a deterministic, BTP-safe binding name for a CR.
// Format: "crossplane-tmp-<crName>-<namespace>", truncated to 63 chars.
func TempBindingName(crName, namespace string) string {
	full := "crossplane-tmp-" + crName + "-" + namespace
	if len(full) > 63 {
		return full[:63]
	}
	return full
}

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

func (t ServiceManagerInstanceProxyClient) ServiceManagerPlanIDByName(ctx context.Context, subaccountId string, servicePlanName string, bindingName string) (string, error) {
	// if a named binding already exists (e.g. orphan from previous reconcile) reuse it
	binding, err := t.describeAdminBindingV2(ctx, subaccountId, bindingName)
	if err != nil {
		return "", err
	}
	if binding != nil {
		return t.resolveServicePlan(ctx, servicePlanName)(binding)
	}
	// otherwise dynamically create, resolve, and defer-delete the named binding
	return t.dynamicServiceInstance(ctx, subaccountId, bindingName, t.resolveServicePlan(ctx, servicePlanName))
}

func (t ServiceManagerInstanceProxyClient) dynamicServiceInstance(ctx context.Context, subaccountId, bindingName string, resolvalFn func(binding *BindingCredentials) (string, error)) (id string, err error) {
	binding, err := t.createAdminBindingV2(ctx, subaccountId, bindingName)
	if err != nil {
		return "", err
	}

	// Always clean up the named binding after use. Lookup failures and
	// reconcile cancellation must not leave it behind.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), adminBindingCleanupTimeout)
		defer cancel()
		if cleanupErr := t.deleteAdminBindingV2(cleanupCtx, subaccountId, bindingName); cleanupErr != nil {
			cleanupErr = fmt.Errorf("delete temporary service-manager admin binding %q: %w", bindingName, cleanupErr)
			if err == nil {
				err = cleanupErr
			} else {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()

	id, err = resolvalFn(binding)
	if err != nil {
		id = ""
	}
	return id, err
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

	if raw != nil && raw.StatusCode == 404 {
		return nil, nil
	}

	return mapBindingCredentialTypes(response), specifyAccountsAPIError(err)
}

func (t ServiceManagerInstanceProxyClient) describeAdminBindingV2(ctx context.Context, subaccountGuid, bindingName string) (*BindingCredentials, error) {
	result, raw, err := t.GetServiceManagerBindingV2(ctx, subaccountGuid, bindingName).Execute()
	if raw != nil && raw.StatusCode == 404 {
		return nil, nil
	}
	return mapV2BindingCredentialTypes(result), specifyAccountsAPIError(err)
}

func (t ServiceManagerInstanceProxyClient) createAdminBindingV2(ctx context.Context, subaccountGuid, bindingName string) (*BindingCredentials, error) {
	payload := accountsserviceclient.NewCreateServiceManagerBindingRequestPayload(bindingName)
	result, _, err := t.CreateServiceManagerBindingV2(ctx, subaccountGuid).
		CreateServiceManagerBindingRequestPayload(*payload).Execute()
	if err != nil {
		return nil, specifyAccountsAPIError(err)
	}
	return mapV2BindingCredentialTypes(result), nil
}

func (t ServiceManagerInstanceProxyClient) deleteAdminBindingV2(ctx context.Context, subaccountGuid, bindingName string) error {
	_, err := t.DeleteServiceManagerBindingV2(ctx, subaccountGuid, bindingName).Execute()
	return specifyAccountsAPIError(err)
}

func mapV2BindingCredentialTypes(in *accountsserviceclient.ServiceManagerBindingExtendedResponseObject) *BindingCredentials {
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
// visibility, backed by a named temporary service-manager admin binding. Unlike
// SemanticLookuper it MINTS a temporary named binding via the accounts-service V2
// API when none exists yet, and returns a cleanup function that removes it.
// If the named binding already exists (orphan from a previous reconcile) it is
// reused directly — no-op when an existing binding was reused.
//
// This is the credential source the SI/SB/CM adoption heal must use: the
// per-resource serviceManagerSecret bindings are platform-scoped and do not
// list instances created via the btp terraform provider, whereas the
// subaccount-admin binding sees the whole subaccount.
func (t ServiceManagerInstanceProxyClient) EnsureSemanticLookuper(ctx context.Context, subaccountGuid, bindingName string) (SemanticLookuper, func(), error) {
	noop := func() {}

	binding, err := t.describeAdminBindingV2(ctx, subaccountGuid, bindingName)
	if err != nil {
		return nil, noop, err
	}
	cleanup := noop
	if binding == nil {
		// mint a temporary named binding; caller must call cleanup to remove it.
		binding, err = t.createAdminBindingV2(ctx, subaccountGuid, bindingName)
		if err != nil {
			return nil, noop, err
		}
		// Detach the cleanup delete from ctx so that a reconcile timeout /
		// cancellation does not silently orphan the temporary admin binding when
		// the caller defers cleanup().
		cleanup = func() {
			delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), adminBindingCleanupTimeout)
			defer cancel()
			if dErr := t.deleteAdminBindingV2(delCtx, subaccountGuid, bindingName); dErr != nil {
				ctrl.Log.Info("EnsureSemanticLookuper cleanup: failed to delete temporary admin binding",
					"subaccountGuid", subaccountGuid, "bindingName", bindingName, "error", dErr.Error())
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
