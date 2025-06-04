package v1alpha1

func (md *Directory) SetExternalID(newID string) {
	md.Spec.ForProvider.DisplayName = &newID
}

func (md *Directory) GetExternalID() string {
	return *md.Spec.ForProvider.DisplayName
}

func (ms *Subaccount) SetExternalID(newID string) {
	ms.Spec.ForProvider.DisplayName = newID
	ms.Spec.ForProvider.Subdomain = newID
}
func (ms *Subaccount) GetExternalID() string {
	return ms.Spec.ForProvider.DisplayName
}
