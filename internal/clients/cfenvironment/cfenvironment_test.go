package environments

import (
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

// crWithManagers returns a CloudFoundryEnvironment CR with the given managers.
func _(specManagers []string, statusManagers []string) v1alpha1.CloudFoundryEnvironment {
	return v1alpha1.CloudFoundryEnvironment{
		Spec: v1alpha1.CfEnvironmentSpec{
			ForProvider: v1alpha1.CfEnvironmentParameters{
				Managers: specManagers,
			},
		},
		Status: v1alpha1.EnvironmentStatus{
			AtProvider: v1alpha1.CfEnvironmentObservation{
				Managers: toUserSlice(statusManagers),
			},
		},
	}
}

// toUserSlice converts a slice of strings to a slice of v1alpha1.User.
func toUserSlice(ss []string) []v1alpha1.User {
	us := make([]v1alpha1.User, 0)
	for _, s := range ss {
		us = append(us, v1alpha1.User{Username: s, Origin: "sap.ids"})
	}
	return us
}
