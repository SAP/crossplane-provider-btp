package apis

import "github.com/crossplane/crossplane-runtime/v2/pkg/resource"

type ManagedTested interface {
	resource.Managed
	SetExternalID(newID string)
	GetExternalID() string
}
