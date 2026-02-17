package resources

import (
	"fmt"

	"github.com/SAP/xp-clifford/parsan"
)

const (
	UndefinedName          = "UNDEFINED-NAME"
	UndefinedExternalName  = "UNDEFINED-EXTERNAL-NAME"
	DefaultSecretNamespace = "default"
)

const (
	WarnMissingServiceName         = "WARNING: service name is missing"
	WarnMissingServicePlanName     = "WARNING: service plan name is missing"
	WarnMissingSubaccountGuid      = "WARNING: subaccount ID is missing"
	WarnMissingBindingId           = "WARNING: binding ID is missing"
	WarnMissingInstanceId          = "WARNING: service instance ID is missing"
	WarnMissingExternalName        = "WARNING: external name is missing"
	WarnUndefinedResourceName      = "WARNING: could not generate a valid resource name"
	WarnUndefinedExternalName      = "WARNING: could not generate a valid external name"
	WarnUnsupportedEntityType      = "WARNING: only 'SUBACCOUNT' entity type is supported for Entitlement resources"
	WarnCannotResolveSubaccount    = "WARNING: cannot resolve subaccount ID to a resource name"
	WarnServiceInstanceNotUsable   = "WARNING: service instance is not in a usable state"
	WarnNotServiceManager          = "WARNING: the service instance is not a service manager instance"
	WarnNotCloudManagement         = "WARNING: the service instance is not a cloud management instance"
	WarnMissingServiceManagerName  = "WARNING: service manager reference is missing"
	WarnMissingLandscapeLabel      = "WARNING: landscape name is missing"
	WarnMissingOrgName             = "WARNING: CF org name is missing"
	WarnMissingEnvironmentName     = "WARNING: environment name is missing"
	WarnMissingCloudManagementName = "WARNING: cloud management reference is missing"
)

type BtpResource interface {
	GetID() string
	GetDisplayName() string
	GetExternalName() string
	GenerateK8sResourceName() string
}

func GenerateK8sResourceName(id, name, kind string) (string, error) {
	resourceName := UndefinedName
	hasId := id != ""
	hasName := name != ""
	hasKind := kind != ""

	switch {
	case hasName:
		names := parsan.ParseAndSanitize(name, parsan.RFC1035LowerSubdomain)
		if len(names) == 0 {
			return UndefinedName, fmt.Errorf("cannot sanitize name: %s", name)
		} else {
			resourceName = names[0]
		}
	case hasId && hasKind:
		resourceName = kind + "-" + id
	}

	return resourceName, nil
}
