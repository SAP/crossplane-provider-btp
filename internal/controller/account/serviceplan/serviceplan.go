/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package serviceplan

import (
	"context"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

const (
	errNotServicePlan   = "managed resource is not a ServicePlan custom resource"
	errTrackPCUsage     = "cannot track ProviderConfig usage"
	errTrackRUsage      = "cannot track ResourceUsage"
	errExtractSecretKey = "No Service Manager Secret Found"
	errGetPC            = "cannot get ProviderConfig"
	errGetCreds         = "cannot get servicemanager credentials"

	errNewClient = "cannot create new Service"
	errApiGet    = "cannot retrieve ServicePlanId from API"
)

type connector struct {
	kube  client.Client
	usage resource.Tracker

	resourcetracker     tracking.ReferenceResolverTracker
	newPlanIdResolverFn func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error)
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.ServicePlan)
	if !ok {
		return nil, errors.New(errNotServicePlan)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	if cr.Spec.ForProvider.ServiceManagerSecret == "" || cr.Spec.ForProvider.ServiceManagerSecretNamespace == "" {
		return nil, errors.New(errExtractSecretKey)
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(
		ctx, types.NamespacedName{
			Namespace: cr.Spec.ForProvider.ServiceManagerSecretNamespace,
			Name:      cr.Spec.ForProvider.ServiceManagerSecret,
		}, secret,
	); err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	sm, err := c.newPlanIdResolverFn(ctx, secret.Data)
	if err != nil {
		return nil, err
	}

	return &external{
		client:  sm,
		tracker: c.resourcetracker,
	}, nil
}

type external struct {
	// we can reuse the existing interface and impl from the servicemanager package
	client servicemanager.PlanIdResolver

	tracker tracking.ReferenceResolverTracker
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServicePlan)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServicePlan)
	}

	// gracefully acknowledge deletion by returning not found
	if meta.WasDeleted(cr) {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// otherwise lookup and store the PlanId in observation
	id, err := c.client.PlanIDByName(ctx, cr.Spec.ForProvider.OfferingName, cr.Spec.ForProvider.PlanName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errApiGet)
	}
	setObservation(cr, id)
	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	if _, ok := mg.(*v1alpha1.ServicePlan); !ok {
		return managed.ExternalCreation{}, errors.New(errNotServicePlan)
	}

	return managed.ExternalCreation{}, errors.New("Create() is not part of the ServicePlan controller")
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	if _, ok := mg.(*v1alpha1.ServicePlan); !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServicePlan)
	}

	return managed.ExternalUpdate{}, errors.New("Update() is not part of the ServicePlan controller")
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.ServicePlan)
	if !ok {
		return errors.New(errNotServicePlan)
	}

	// just gracefully acknowledge the deletion, since its READ-ONLY
	cr.Status.SetConditions(xpv1.Deleting())
	return nil
}

// setObservation sets the observed state, which is the ServicePlanId in this case
func setObservation(cr *v1alpha1.ServicePlan, id string) {
	cr.Status.AtProvider.ServicePlanId = id
}
