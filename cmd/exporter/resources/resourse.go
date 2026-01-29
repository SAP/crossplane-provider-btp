package resources

const UNDEFINED_NAME = "UNDEFINED-NAME"

type BtpResource interface {
	GetID() string
	GetDisplayName() string
	GetExternalName() string
	GenerateK8sResourceName() string
}
