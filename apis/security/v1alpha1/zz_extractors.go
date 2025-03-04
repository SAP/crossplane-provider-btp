package v1alpha1

import (
	"github.com/crossplane/crossplane-runtime/pkg/reference"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// SubaccountAPICredentialSecret extracts the Reference of a cis instance to a secret name
func SubaccountAPICredentialSecret() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*SubaccountApiCredential)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Name
	}
}

// SubaccountAPICredentialSecretSecretNamespace extracts the Reference of a cis instance to the namespace of secret
func SubaccountAPICredentialSecretSecretNamespace() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		sg, ok := mg.(*SubaccountApiCredential)
		if !ok {
			return ""
		}
		if sg.Spec.WriteConnectionSecretToReference == nil {
			return ""
		}
		return sg.Spec.WriteConnectionSecretToReference.Namespace
	}
}
