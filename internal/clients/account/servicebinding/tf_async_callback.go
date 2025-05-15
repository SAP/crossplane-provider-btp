package servicebindingclient

import (
	"context"
	"fmt"

	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/crossplane/upjet/pkg/terraform"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
)

var errUpdateStatusFmt = "cannot update status of the resource %s after an async %s"

type APICallbacks struct {
	kube client.Client

	saveCallbackFn SaveConditionsFn
}

// Create makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Create(name string) terraform.CallbackFn {
	return func(err error, ctx context.Context) error {
		fmt.Println("CREATE CALLBACK FOR ServiceBinding " + name)

		uErr := ac.saveCallbackFn(ctx, ac.kube, name, ujresource.LastAsyncOperationCondition(err), ujresource.AsyncOperationFinishedCondition())

		return errors.Wrapf(uErr, errUpdateStatusFmt, name, "create")
	}
}

// Update makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Update(name string) terraform.CallbackFn {
	return func(error, context.Context) error {
		return nil
	}
}

// Destroy makes sure the error is saved in async operation condition.
func (ac *APICallbacks) Destroy(name string) terraform.CallbackFn {
	return func(error, context.Context) error {
		return nil
	}
}
