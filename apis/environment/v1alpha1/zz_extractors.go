package v1alpha1

import (
	"github.com/crossplane/crossplane-runtime/pkg/reference"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// KymaEnvironmentBindingSecret extracts the Reference of some Kyma credentials to a secret name
func KymaEnvironmentBindingSecret() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*KymaEnvironmentBinding)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Name
	}
}

// KymaEnvironmentBindingSecretNamespace extracts the Reference of some Kyma credentials to the namespace of secret
func KymaEnvironmentBindingSecretNamespace() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*KymaEnvironmentBinding)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Namespace
	}
}
