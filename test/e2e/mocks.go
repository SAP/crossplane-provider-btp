package e2e

import (
	"encoding/json"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/k8s"
)

type MockList struct {
	client.ObjectList

	Items []k8s.Object
}
type FakeManaged struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	resource.ProviderConfigReferencer
	resource.ConnectionSecretWriterTo
	resource.Orphanable
	resource.Manageable
	xpv1.ConditionedStatus
}

// DeepCopyObject returns a copy of the object as runtime.Object
func (m *FakeManaged) DeepCopyObject() runtime.Object {
	out := &FakeManaged{}
	j, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}
