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

// Code generated by angryjet. DO NOT EDIT.

package v1alpha1

import (
	"context"
	reference "github.com/crossplane/crossplane-runtime/pkg/reference"
	errors "github.com/pkg/errors"
	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveReferences of this CloudFoundryEnvironment.
func (mg *CloudFoundryEnvironment) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPIResolver(c, mg)

	var rsp reference.ResolutionResponse
	var err error

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.SubaccountGuid,
		Extract:      v1alpha1.SubaccountUuid(),
		Reference:    mg.Spec.SubaccountRef,
		Selector:     mg.Spec.SubaccountSelector,
		To: reference.To{
			List:    &v1alpha1.SubaccountList{},
			Managed: &v1alpha1.Subaccount{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.SubaccountGuid")
	}
	mg.Spec.SubaccountGuid = rsp.ResolvedValue
	mg.Spec.SubaccountRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSecret,
		Extract:      v1alpha1.CloudManagementSecret(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSecret")
	}
	mg.Spec.CloudManagementSecret = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSecretNamespace,
		Extract:      v1alpha1.CloudManagementSecretSecretNamespace(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSecretNamespace")
	}
	mg.Spec.CloudManagementSecretNamespace = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSubaccountGuid,
		Extract:      v1alpha1.CloudManagementSubaccountUuid(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSubaccountGuid")
	}
	mg.Spec.CloudManagementSubaccountGuid = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	return nil
}

// ResolveReferences of this KymaEnvironment.
func (mg *KymaEnvironment) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPIResolver(c, mg)

	var rsp reference.ResolutionResponse
	var err error

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.SubaccountGuid,
		Extract:      v1alpha1.SubaccountUuid(),
		Reference:    mg.Spec.SubaccountRef,
		Selector:     mg.Spec.SubaccountSelector,
		To: reference.To{
			List:    &v1alpha1.SubaccountList{},
			Managed: &v1alpha1.Subaccount{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.SubaccountGuid")
	}
	mg.Spec.SubaccountGuid = rsp.ResolvedValue
	mg.Spec.SubaccountRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSecret,
		Extract:      v1alpha1.CloudManagementSecret(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSecret")
	}
	mg.Spec.CloudManagementSecret = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSecretNamespace,
		Extract:      v1alpha1.CloudManagementSecretSecretNamespace(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSecretNamespace")
	}
	mg.Spec.CloudManagementSecretNamespace = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSubaccountGuid,
		Extract:      v1alpha1.CloudManagementSubaccountUuid(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSubaccountGuid")
	}
	mg.Spec.CloudManagementSubaccountGuid = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	return nil
}

// ResolveReferences of this KymaEnvironmentBinding.
func (mg *KymaEnvironmentBinding) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPIResolver(c, mg)

	var rsp reference.ResolutionResponse
	var err error

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.KymaEnvironmentId,
		Extract:      KymaInstanceId(),
		Reference:    mg.Spec.KymaEnvironmentRef,
		Selector:     mg.Spec.KymaEnvironmentSelector,
		To: reference.To{
			List:    &KymaEnvironmentList{},
			Managed: &KymaEnvironment{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.KymaEnvironmentId")
	}
	mg.Spec.KymaEnvironmentId = rsp.ResolvedValue
	mg.Spec.KymaEnvironmentRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSecret,
		Extract:      v1alpha1.CloudManagementSecret(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSecret")
	}
	mg.Spec.CloudManagementSecret = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSecretNamespace,
		Extract:      v1alpha1.CloudManagementSecretSecretNamespace(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSecretNamespace")
	}
	mg.Spec.CloudManagementSecretNamespace = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	rsp, err = r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.CloudManagementSubaccountGuid,
		Extract:      v1alpha1.CloudManagementSubaccountUuid(),
		Reference:    mg.Spec.CloudManagementRef,
		Selector:     mg.Spec.CloudManagementSelector,
		To: reference.To{
			List:    &v1alpha1.CloudManagementList{},
			Managed: &v1alpha1.CloudManagement{},
		},
	})
	if err != nil {
		return errors.Wrap(err, "mg.Spec.CloudManagementSubaccountGuid")
	}
	mg.Spec.CloudManagementSubaccountGuid = rsp.ResolvedValue
	mg.Spec.CloudManagementRef = rsp.ResolvedReference

	return nil
}
