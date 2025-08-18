package kymamodule

import (
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KymaCr is the Schema for the KymaCR inside the Kyma Cluster.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/types.go#L97
type KymaCr struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KymaSpec   `json:"spec,omitempty"`
	Status KymaStatus `json:"status,omitempty"`
}

// KymaSpec defines the desired state of Kyma.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/types.go#L106
type KymaSpec struct {
	Channel string   `json:"channel"`
	Modules []Module `json:"modules,omitempty"`
}

// KymaStatus defines the observed state of Kyma
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/types.go#L121
type KymaStatus struct {
	Modules []v1alpha1.ModuleStatus `json:"modules,omitempty"`
}

// Module defines the components to be installed.
// ref https://github.com/kyma-project/cli/blob/838d9b9e8506489da336bf790e4814fbe1caba0b/internal/kube/kyma/types.go#L112
type Module struct {
	Name                 string `json:"name"`
	ControllerName       string `json:"controller,omitempty"`
	Channel              string `json:"channel,omitempty"`
	CustomResourcePolicy string `json:"customResourcePolicy,omitempty"`
	Managed              *bool  `json:"managed,omitempty"`
}
