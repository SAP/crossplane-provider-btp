package tfclient

import (
	"context"

	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/crossplane/upjet/pkg/terraform"
	tferrors "github.com/crossplane/upjet/pkg/terraform/errors"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errUpdateStatusFmt = "cannot update status of the resource %s after an async %s"

func NewAPICallbacks(kube client.Client, saveConditionsFn SaveConditionsFn) *APICallbacks {
	return &APICallbacks{
		kube:           kube,
		saveCallbackFn: saveConditionsFn,
		log:            ctrl.Log.WithName("tf-async-callback"),
	}
}

type APICallbacks struct {
	kube client.Client

	saveCallbackFn SaveConditionsFn

	log logr.Logger
}

// Create makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Create(name string) terraform.CallbackFn {
	return ac.callback("create", name, tferrors.NewAsyncCreateFailed)
}

// Update makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Update(name string) terraform.CallbackFn {
	return ac.callback("update", name, tferrors.NewAsyncUpdateFailed)
}

// Destroy makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Destroy(name string) terraform.CallbackFn {
	return ac.callback("destroy", name, func(err error) error { return err })
}

// callback resolves the terraform resource name to the native managed
// resource via SaveConditionsFn. classify tags upjet v1's ApplyFailure with
// the operation, so the reason matches upjet v2's.
func (ac *APICallbacks) callback(op, name string, classify func(error) error) terraform.CallbackFn {
	return func(err error, ctx context.Context) error {
		uErr := ac.saveCallbackFn(ctx, ac.kube, name, ujresource.LastAsyncOperationCondition(classify(err)), ujresource.AsyncOperationFinishedCondition())
		if uErr == nil {
			return nil
		}
		// upjet logs a failing callback at info level only (#968).
		ac.log.Error(uErr, "async terraform callback could not persist its result",
			"operation", op, "terraformName", name)
		return errors.Wrapf(uErr, errUpdateStatusFmt, name, op)
	}
}
