// Package test provides helpers for controller unit tests.
package test

import (
	"fmt"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Manager captures registered controllers for direct reconciliation without
// starting watches, servers, or background workers.
type Manager struct {
	fake.Manager
	Controllers []controller.Controller
}

func (m *Manager) Add(r manager.Runnable) error {
	c, ok := r.(controller.Controller)
	if !ok {
		return fmt.Errorf("expected a controller, got %T", r)
	}
	m.Controllers = append(m.Controllers, c)
	return nil
}

func (m *Manager) GetControllerOptions() config.Controller {
	// Tests build the same controllers in isolated managers within one process.
	return config.Controller{SkipNameValidation: ptr.To(true)}
}

// GetEventRecorderFor provides the legacy recorder expected by the setups.
func (m *Manager) GetEventRecorderFor(string) record.EventRecorder {
	return &record.FakeRecorder{}
}
