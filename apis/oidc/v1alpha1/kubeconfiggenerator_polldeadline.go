package v1alpha1

import "time"

// NextPollDeadline always returns a past deadline, pinning this resource at
// --poll: it picks up renewed OIDC tokens only by polling, and its Observe
// reads only Kubernetes secrets, so this costs no BTP traffic.
func (*KubeConfigGenerator) NextPollDeadline() (time.Time, bool) {
	return time.Time{}, true
}
