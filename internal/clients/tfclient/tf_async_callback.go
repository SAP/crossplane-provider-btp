package tfclient

import (
	"context"

	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/crossplane/upjet/pkg/terraform"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
)

var errUpdateStatusFmt = "cannot update status of the resource %s after an async %s"

func NewAPICallbacks(kube client.Client, saveConditionsFn SaveConditionsFn) *APICallbacks {
	return &APICallbacks{
		kube:           kube,
		saveCallbackFn: saveConditionsFn,
	}
}

type APICallbacks struct {
	kube client.Client

	saveCallbackFn SaveConditionsFn
}

// Create makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Create(name string) terraform.CallbackFn {
	return func(err error, ctx context.Context) error {
		uErr := ac.saveCallbackFn(ctx, ac.kube, name, ujresource.LastAsyncOperationCondition(err), ujresource.AsyncOperationFinishedCondition())
		return errors.Wrapf(uErr, errUpdateStatusFmt, name, "create")
	}
}

// Update makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Update(name string) terraform.CallbackFn {
	return func(err error, ctx context.Context) error {
		uErr := ac.saveCallbackFn(ctx, ac.kube, name, ujresource.LastAsyncOperationCondition(err), ujresource.AsyncOperationFinishedCondition())
		return errors.Wrapf(uErr, errUpdateStatusFmt, name, "update")
	}
}

// Destroy makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Destroy(name string) terraform.CallbackFn {
	return func(err error, ctx context.Context) error {
		uErr := ac.saveCallbackFn(ctx, ac.kube, name, ujresource.LastAsyncOperationCondition(err), ujresource.AsyncOperationFinishedCondition())
		return errors.Wrapf(uErr, errUpdateStatusFmt, name, "destroy")
	}
}
