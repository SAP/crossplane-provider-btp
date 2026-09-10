package subaccountapicredential

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	btpconfig "github.com/sap/crossplane-provider-btp/config/btp_subaccount_api_credential"
)

// NewConnectionSecretValidatingConnector wraps an external connector so that
// the external resource is observed before its connection Secret is checked.
// This ordering is important: a malformed Secret must not prevent recreation
// when the BTP credential is already absent, and must not block deletion.
func NewConnectionSecretValidatingConnector(delegate managed.ExternalConnector, kube client.Client) managed.ExternalConnector {
	return &connectionSecretValidatingConnector{delegate: delegate, kube: kube}
}

type connectionSecretValidatingConnector struct {
	delegate managed.ExternalConnector
	kube     client.Client
}

func (c *connectionSecretValidatingConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	external, err := c.delegate.Connect(ctx, mg)
	if err != nil {
		return nil, err
	}
	return &connectionSecretValidatingExternal{delegate: external, kube: c.kube}, nil
}

type connectionSecretValidatingExternal struct {
	delegate managed.ExternalClient
	kube     client.Client
}

func (e *connectionSecretValidatingExternal) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	observation, err := e.delegate.Observe(ctx, mg)
	if err != nil {
		return observation, err
	}

	if meta.WasDeleted(mg) || !observation.ResourceExists {
		return observation, nil
	}

	cr, ok := mg.(*v1alpha1.SubaccountApiCredential)
	if !ok {
		return observation, errors.New("managed resource is not of type SubaccountApiCredential")
	}
	if err := btpconfig.ValidateConnectionSecret(ctx, e.kube, cr); err != nil {
		return observation, err
	}
	return observation, nil
}

func (e *connectionSecretValidatingExternal) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	return e.delegate.Create(ctx, mg)
}

func (e *connectionSecretValidatingExternal) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	return e.delegate.Update(ctx, mg)
}

func (e *connectionSecretValidatingExternal) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	return e.delegate.Delete(ctx, mg)
}

func (e *connectionSecretValidatingExternal) Disconnect(ctx context.Context) error {
	return e.delegate.Disconnect(ctx)
}
