// Package fake provides in-memory fakes of the BTP client interfaces the
// resolver consumes. They are data-backed: a test describes what each fake
// serves and reads its counters afterwards; the fakes never assert.
//
// The zero value of every fake serves nothing: lookups are clean no-matches
// and plan identities are unknown. Setting Err makes every call fail with it.
package fake

import (
	"context"

	"github.com/crossplane/function-sdk-go/errors"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btp"
)

// PlanIdentity is the offering/plan catalog-name pair served for one service
// plan ID.
type PlanIdentity struct {
	Offering string
	Plan     string
}

// SMClient fakes the Service Manager operations the resolver uses.
type SMClient struct {
	Instances map[string]*btp.SMResource // name → resource
	Bindings  map[string]*btp.SMResource // name → resource
	Plans     map[string]PlanIdentity    // plan ID → catalog names
	Err       error                      // when set, every call fails with it
	PlanGets  int                        // GetServicePlanIdentity call count
}

// FindServiceInstance returns the instance registered under name, or nil.
func (f *SMClient) FindServiceInstance(_ context.Context, name string) (*btp.SMResource, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Instances[name], nil
}

// FindServiceBinding returns the binding registered under name, or nil.
func (f *SMClient) FindServiceBinding(_ context.Context, name string) (*btp.SMResource, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Bindings[name], nil
}

// GetServicePlanIdentity returns the catalog names registered for planID and
// counts the call; an unregistered plan is an error, like a 404 from SM.
func (f *SMClient) GetServicePlanIdentity(_ context.Context, planID string) (string, string, error) {
	if f.Err != nil {
		return "", "", f.Err
	}
	f.PlanGets++
	identity, ok := f.Plans[planID]
	if !ok {
		return "", "", errors.Errorf("service plan %q not found", planID)
	}
	return identity.Offering, identity.Plan, nil
}

// Close is a no-op.
func (f *SMClient) Close() error { return nil }

// AccountsClient fakes the Accounts Service operations the resolver uses.
type AccountsClient struct {
	Subaccounts map[string]string // "subdomain/region" → guid
	Err         error             // when set, every call fails with it
}

// FindSubaccount returns the guid registered under subdomain/region, or "".
func (f *AccountsClient) FindSubaccount(_ context.Context, subdomain, region string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	return f.Subaccounts[subdomain+"/"+region], nil
}

// ProvisioningClient fakes the Provisioning Service operations the resolver
// uses.
type ProvisioningClient struct {
	Kyma         map[string]string // "name/plan" → id
	CloudFoundry map[string]string // "name" → id
	Err          error             // when set, every call fails with it
}

// FindKymaEnvironment returns the id registered under name/plan, or "".
func (f *ProvisioningClient) FindKymaEnvironment(_ context.Context, name, planName string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	return f.Kyma[name+"/"+planName], nil
}

// FindCloudFoundryEnvironment returns the id registered under name, or "".
func (f *ProvisioningClient) FindCloudFoundryEnvironment(_ context.Context, name string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	return f.CloudFoundry[name], nil
}
