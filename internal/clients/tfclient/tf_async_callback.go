package tfclient

import (
	"context"

	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
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
func (ac *APICallbacks) Create(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("create", name)
}

// Update makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Update(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("update", name)
}

// Destroy makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Destroy(name types.NamespacedName, _ bool) terraform.CallbackFn {
	return ac.callback("destroy", name)
}

// callback resolves the terraform resource name to the native managed
// resource via SaveConditionsFn.
func (ac *APICallbacks) callback(op string, name types.NamespacedName) terraform.CallbackFn {
	return func(err error, ctx context.Context) error {
		uErr := ac.saveCallbackFn(ctx, ac.kube, name, ujresource.LastAsyncOperationCondition(err), ujresource.AsyncOperationFinishedCondition())
		if uErr == nil {
			return nil
		}
		// upjet logs a failing callback at info level only (#968).
		ac.log.Error(uErr, "async terraform callback could not persist its result",
			"operation", op, "terraformName", name.String())
		return errors.Wrapf(uErr, errUpdateStatusFmt, name, op)
	}
}
