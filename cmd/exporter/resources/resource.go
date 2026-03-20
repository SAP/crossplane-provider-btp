package resources

import (
	"fmt"
	"unicode"

	"github.com/SAP/xp-clifford/parsan"
)

const (
	UndefinedName          = "UNDEFINED-NAME"
	UndefinedExternalName  = "UNDEFINED-EXTERNAL-NAME"
	DefaultSecretNamespace = "default"
)

const (
	WarnMissingServiceName            = "WARNING: service name is missing"
	WarnMissingServicePlanName        = "WARNING: service plan name is missing"
	WarnMissingSubaccountGuid         = "WARNING: subaccount ID is missing"
	WarnMissingBindingId              = "WARNING: binding ID is missing"
	WarnTooManyBindingIDs             = "WARNING: one binding ID expected, got: %d"
	WarnMissingInstanceId             = "WARNING: service instance ID is missing"
	WarnMissingInstanceName           = "WARNING: service instance name is missing"
	WarnMissingBindingName            = "WARNING: service binding name is missing"
	WarnMissingExternalName           = "WARNING: external name is missing"
	WarnUndefinedResourceName         = "WARNING: could not generate a valid resource name"
	WarnUndefinedExternalName         = "WARNING: could not generate a valid external name"
	WarnUnsupportedEntityType         = "WARNING: only 'SUBACCOUNT' entity type is supported for Entitlement resources"
	WarnCannotResolveSubaccount       = "WARNING: cannot resolve subaccount ID to a resource name"
	WarnServiceInstanceNotUsable      = "WARNING: service instance is not in a usable state"
	WarnNotServiceManager             = "WARNING: the service instance is not a service manager instance"
	WarnNotCloudManagement            = "WARNING: the service instance is not a cloud management instance"
	WarnMissingServiceManagerName     = "WARNING: service manager reference is missing"
	WarnMissingLandscapeLabel         = "WARNING: landscape name is missing"
	WarnMissingOrgName                = "WARNING: CF org name is missing"
	WarnMissingEnvironmentName        = "WARNING: environment name is missing"
	WarnMissingCloudManagementName    = "WARNING: cloud management reference is missing"
	WarnDefaultEntitlementEnableFalse = "WARNING: entitlement does not have 'enable' field set to true"
)

type BtpResource interface {
	GetID() string
	GetDisplayName() string
	GetExternalName() string
	GenerateK8sResourceName() string
}

func GenerateK8sResourceName(guid, name string) (string, error) {
	if name == "" {
		return UndefinedName, nil
	}

	candidate := name
	if guid != "" {
		// Resource name is a DNS subdomain name: subdomain := label("." label)*
		// RFC 1123 label names must start with an alphabetic character
		// See also https://kubernetes.io/docs/concepts/overview/working-with-objects/names/
		candidate += "."
		if unicode.IsDigit(rune(guid[0])) {
			candidate += "x"
		}
		candidate += guid
	}

	names := parsan.ParseAndSanitize(candidate, parsan.RFC1035LowerSubdomain)
	if len(names) == 0 {
		return UndefinedName, fmt.Errorf("cannot sanitize name: %s", name)
	}

	return names[0], nil
}
