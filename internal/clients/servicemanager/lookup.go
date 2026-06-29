package servicemanager

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrSecretNotFound is returned by LoadCredsFromSecret when the named secret
// could not be retrieved. Callers treat this as "credentials unavailable, skip
// the recovery path" rather than a hard reconcile failure.
var ErrSecretNotFound = errors.New("service manager credentials secret not found")

// LoadCredsFromSecret reads a Service Manager binding-credentials Kubernetes
// Secret (the one written by a ServiceManager / CloudManagement CR) and parses
// it into BindingCredentials usable by NewServiceManagerClient.
//
// Returns (nil, ErrSecretNotFound) when the Secret doesn't exist so callers
// can distinguish "no creds, can't look up" from "transient error, retry".
func LoadCredsFromSecret(ctx context.Context, kube client.Client, namespace, name string) (*BindingCredentials, error) {
	if name == "" {
		return nil, ErrSecretNotFound
	}
	if namespace == "" {
		namespace = "default"
	}
	sec := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sec); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrSecretNotFound
		}
		return nil, errors.Wrapf(err, "cannot load service manager secret %s/%s", namespace, name)
	}
	creds, err := NewCredsFromOperatorSecret(sec.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse service manager secret %s/%s", namespace, name)
	}
	return &creds, nil
}

// FindServiceInstanceIDByName returns the BTP UUID of the service instance with
// the given name in the given subaccount, querying the SAP Service Manager API
// directly. Returns ("", nil) when no matching instance exists.
//
// This is the durable-source-of-truth lookup that the SI / SM / CM controllers
// fall back to when upjet's workspace-state-based Observe returns NotExists
// despite a BTP-side resource still existing (e.g. after a pod restart wiped
// the workspace before external-name was persisted to the CR).
func (sm *ServiceManagerClient) FindServiceInstanceIDByName(ctx context.Context, subaccountID, name string) (string, error) {
	if subaccountID == "" || name == "" {
		return "", nil
	}
	q := fmt.Sprintf("name eq '%s' and subaccount_id eq '%s'", name, subaccountID)
	res, _, err := sm.GetAllServiceInstances(ctx).FieldQuery(q).Execute()
	if err != nil {
		return "", specifyAPIError(err)
	}
	if res == nil || len(res.Items) == 0 {
		return "", nil
	}
	id := res.Items[0].Id
	if id == nil {
		return "", nil
	}
	return *id, nil
}

// FindServiceBindingIDByName returns the BTP UUID of the service binding with
// the given name under the given service instance, querying the SAP Service
// Manager API directly. Returns ("", nil) when no matching binding exists.
func (sm *ServiceManagerClient) FindServiceBindingIDByName(ctx context.Context, serviceInstanceID, name string) (string, error) {
	if serviceInstanceID == "" || name == "" {
		return "", nil
	}
	q := fmt.Sprintf("name eq '%s' and service_instance_id eq '%s'", name, serviceInstanceID)
	res, _, err := sm.GetAllServiceBindings(ctx).FieldQuery(q).Execute()
	if err != nil {
		return "", specifyAPIError(err)
	}
	if res == nil || len(res.Items) == 0 {
		return "", nil
	}
	id := res.Items[0].Id
	if id == nil {
		return "", nil
	}
	return *id, nil
}
