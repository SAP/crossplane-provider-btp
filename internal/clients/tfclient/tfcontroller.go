package tfclient

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TfProxyConnectorI[NATIVE resource.Managed] interface {
	Connect(context.Context, NATIVE) (TfProxyControllerI, error)
}

type TfProxyControllerI interface {
	Observe(ctx context.Context) (bool, error)
	Create(ctx context.Context) error
	Delete(ctx context.Context) error
	// QueryUpdatedData returns the relevant status data once the async creation is done
	QueryAsyncData(ctx context.Context) *ObservationData
}

type SaveConditionsFn func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error

type ObservationData struct {
	ExternalName string `json:"externalName"`
	ID           string `json:"id"`
	Conditions   []xpv1.Condition
}

// TfMapper is a generic interface to map a native resource to an upjet resource that will be used for applying to terraform
type TfMapper[NATIVE resource.Managed, UPJETTED ujresource.Terraformed] interface {
	TfResource(NATIVE, client.Client) (UPJETTED, error)
}

type TfProxyConnector[NATIVE resource.Managed, UPJETTED ujresource.Terraformed] struct {
	tfMapper  TfMapper[NATIVE, UPJETTED]
	connector managed.ExternalConnecter
	kube      client.Client
}

func NewTfProxyConnector[NATIVE resource.Managed, UPJETTED ujresource.Terraformed](tfConnector managed.ExternalConnecter, tfMapper TfMapper[NATIVE, UPJETTED], kube client.Client) TfProxyConnector[NATIVE, UPJETTED] {
	return TfProxyConnector[NATIVE, UPJETTED]{
		connector: tfConnector,
		tfMapper:  tfMapper,
		kube:      kube,
	}
}

func (t *TfProxyConnector[NATIVE, UPJETTED]) Connect(ctx context.Context, cr NATIVE) (TfProxyControllerI, error) {
	ssi, err := t.tfMapper.TfResource(cr, t.kube)

	if err != nil {
		return nil, err
	}

	ctrl, err := t.connector.Connect(ctx, ssi)
	if err != nil {
		return nil, err
	}

	return &TfProxyController[UPJETTED]{
		tfClient:   ctrl,
		tfResource: ssi,
	}, nil
}

var _ TfProxyControllerI = &TfProxyController[*v1alpha1.SubaccountServiceInstance]{}

// TODO: rethink naming
// TfProxyController is a client that provides lifecycle management for a resource by internally delegating to a terraform based resource
type TfProxyController[UPJETTED ujresource.Terraformed] struct {
	tfClient   managed.ExternalClient
	tfResource UPJETTED
}

// QueryUpdatedData returns the relevant status data once the async creation is done
func (t *TfProxyController[UPJETTED]) QueryAsyncData(ctx context.Context) *ObservationData {
	// only query the async data if the operation is finished
	if t.tfResource.GetCondition(ujresource.TypeAsyncOperation).Reason == ujresource.ReasonFinished {
		sid := &ObservationData{}
		bla := &v1alpha1.SubaccountServiceInstance{}
		bla.GetID()
		sid.ID = t.tfResource.GetID()
		sid.ExternalName = meta.GetExternalName(t.tfResource)
		sid.Conditions = []xpv1.Condition{xpv1.Available(), ujresource.AsyncOperationFinishedCondition()}
		return sid
	}
	return nil
}

// Create implements TfProxyControllerI.
func (t *TfProxyController[UPJETTED]) Create(ctx context.Context) error {
	_, err := t.tfClient.Create(ctx, t.tfResource)
	return err
}

// Delete implements TfProxyControllerI.
func (t *TfProxyController[UPJETTED]) Delete(ctx context.Context) error {
	err := t.tfClient.Delete(ctx, t.tfResource)
	return err
}

// Observe implements TfProxyControllerI.
func (t *TfProxyController[UPJETTED]) Observe(ctx context.Context) (bool, error) {
	// will return true, true, in case of in memory running async operations
	obs, err := t.tfClient.Observe(ctx, t.tfResource)
	if err != nil {
		return false, err
	}
	return obs.ResourceExists, nil
}
