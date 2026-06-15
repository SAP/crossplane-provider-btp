package v1alpha1

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	clusterv1alpha1 "github.com/sap/crossplane-provider-btp/apis/cluster/account/v1alpha1"
)

// GlobalAccountUuid Global Account UUID extractor function
func GlobalAccountUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*clusterv1alpha1.GlobalAccount)
		if !ok {
			return ""
		}
		return sg.Status.AtProvider.Guid
	}
}

// DirectoryUuid Directory Account UUID extractor function
func DirectoryUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		d, ok := mg.(*clusterv1alpha1.Directory)
		if !ok {
			return ""
		}
		if d.Status.AtProvider.Guid == nil {
			return ""
		}
		return *d.Status.AtProvider.Guid
	}
}

// SubaccountUuid Subaccount UUID extractor function
func SubaccountUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*clusterv1alpha1.Subaccount)
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

// CloudManagementSecretNamespace extracts the Reference of a cis instance to the namespace of secret
func CloudManagementSecretNamespace() reference.ExtractValueFn {
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

// ServiceInstanceUuid the ServiceInstanceID for the binding
func ServiceInstanceUuid() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*ServiceInstance)
		if !ok {
			return ""
		}
		return sg.Status.AtProvider.ID
	}
}
