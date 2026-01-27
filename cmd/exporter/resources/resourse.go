package resources

type BtpResource interface {
	GetID() string
	GetDisplayName() string
	GetExternalName() string
	GenerateK8sResourceName() string
}
