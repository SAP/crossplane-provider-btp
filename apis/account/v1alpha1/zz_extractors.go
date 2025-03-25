package v1alpha1

import (
	"github.com/crossplane/crossplane-runtime/pkg/reference"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// GlobalAccountUuid Global Account UUID extractor function
func GlobalAccountUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*GlobalAccount)
		if !ok {
			return ""
		}
		return sg.Status.AtProvider.Guid

	}
}

// DirectoryUuid Directory Account UUID extractor function
func DirectoryUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		d, ok := mg.(*Directory)
		if !ok {
			return ""
		}
		if d.Status.AtProvider.Guid == nil {
			return ""
		}
		return *d.Status.AtProvider.Guid

	}
}

// SubaccountUuid Global Account UUID extractor function
func SubaccountUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*Subaccount)
		if !ok {
			return ""
		}
		if sg.Status.AtProvider.SubaccountGuid == nil {
			return ""
		}
		return *sg.Status.AtProvider.SubaccountGuid
	}
}

// ServiceManagerSecret extracts the Reference of a service manager instance to a secret name
func ServiceManagerSecret() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*ServiceManager)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Name
	}
}

// ServiceManagerSecretNamespace extracts the Reference of a service manager instance to the namespace of secret
func ServiceManagerSecretNamespace() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*ServiceManager)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Namespace
	}
}

// CloudManagementSecret extracts the Reference of a cis instance to a secret name
func CloudManagementSecret() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*CloudManagement)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Name
	}
}

// CloudManagementSecretSecretNamespace extracts the Reference of a cis instance to the namespace of secret
func CloudManagementSecretSecretNamespace() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*CloudManagement)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Namespace
	}
}

// CloudManagementSubaccountUuid extracts the Reference of a Subaccount to the namespace of secret
func CloudManagementSubaccountUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*CloudManagement)
		if !ok {
			return ""
		}
		return sg.Spec.ForProvider.SubaccountGuid
	}
}

// ServicePlanId extracts ID of a service plan in its read-only CR
func ServicePlanId() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sp, ok := mg.(*ServicePlan)
		if !ok {
			return ""
		}
		return sp.Status.AtProvider.ServicePlanId
	}
}

// SubaccountServiceInstanceId extracts ID of SubaccountServiceInstance
func SubaccountServiceInstanceId() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		ssi, ok := mg.(*SubaccountServiceInstance)
		if !ok {
			return ""
		}
		if ssi.Status.AtProvider.ID == nil {
			return ""
		}
		return *ssi.Status.AtProvider.ID
	}
}
