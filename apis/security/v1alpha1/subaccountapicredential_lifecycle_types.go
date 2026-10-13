package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SubaccountApiCredentialLifecyclePhase records acquisition state independently
// of Terraform observation state. It is deliberately not a Terraform attribute.
type SubaccountApiCredentialLifecyclePhase string

const (
	SubaccountApiCredentialPhaseEligible      SubaccountApiCredentialLifecyclePhase = "Eligible"
	SubaccountApiCredentialPhaseCreatePending SubaccountApiCredentialLifecyclePhase = "CreatePending"
	SubaccountApiCredentialPhaseOwned         SubaccountApiCredentialLifecyclePhase = "Owned"
	SubaccountApiCredentialPhaseRejected      SubaccountApiCredentialLifecyclePhase = "Rejected"
	SubaccountApiCredentialPhaseUncertain     SubaccountApiCredentialLifecyclePhase = "Uncertain"
)

// SubaccountApiCredentialLifecycleStatus is a UID-bound acquisition receipt.
// It contains no credential material and is kept outside status.atProvider,
// which Upjet replaces during observation.
type SubaccountApiCredentialLifecycleStatus struct {
	// ManagedResourceUID binds this receipt to one Kubernetes object incarnation.
	ManagedResourceUID string `json:"managedResourceUID"`
	// SubaccountID and Name identify the established or attempted BTP credential.
	SubaccountID string `json:"subaccountID,omitempty"`
	Name         string `json:"name,omitempty"`
	// +kubebuilder:validation:Enum=Eligible;CreatePending;Owned;Rejected;Uncertain
	Phase       SubaccountApiCredentialLifecyclePhase `json:"phase"`
	AttemptID   string                                `json:"attemptID,omitempty"`
	AttemptedAt *metav1.Time                          `json:"attemptedAt,omitempty"`
	Reason      string                                `json:"reason,omitempty"`
	// Resolution fields are reserved for the restricted, audited legacy-resolution procedure.
	ResolvedBy         string       `json:"resolvedBy,omitempty"`
	ResolvedAt         *metav1.Time `json:"resolvedAt,omitempty"`
	ResolutionEvidence string       `json:"resolutionEvidence,omitempty"`
}
