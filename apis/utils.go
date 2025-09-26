package apis

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
)

var addToSchemesLock sync.Mutex

func AddToSchemeConcurrent(s *runtime.Scheme) error {
	addToSchemesLock.Lock()
	defer addToSchemesLock.Unlock()
	return AddToScheme(s)
}
