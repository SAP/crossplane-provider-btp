package test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

const (
	credentialAdoptionCleanupBudget           = 8 * time.Minute
	credentialAdoptionCleanupReservePerAction = 5 * time.Second
)

// CredentialAdoptionCleanupAction is one named, independently bounded cleanup attempt.
type CredentialAdoptionCleanupAction struct {
	Name    string
	Timeout time.Duration
	Run     func(context.Context) error
}

// RegisterCredentialAdoptionCleanup registers ordered, bounded cleanup on the
// testing scope that owns an e2e-framework feature. Unlike feature Teardown,
// testing cleanup still runs after FailNow and framework fail-fast.
func RegisterCredentialAdoptionCleanup(t *testing.T, parent context.Context, actions ...CredentialAdoptionCleanupAction) {
	t.Helper()
	var once sync.Once
	t.Cleanup(func() {
		var result error
		once.Do(func() {
			result = runCredentialAdoptionCleanup(parent, actions, credentialAdoptionCleanupBudget)
		})
		if result != nil {
			t.Errorf("credential adoption cleanup incomplete: %v", result)
		}
	})
}

func runCredentialAdoptionCleanup(parent context.Context, actions []CredentialAdoptionCleanupAction, budget time.Duration) error {
	if parent == nil {
		parent = context.Background()
	}
	base := context.WithoutCancel(parent)
	budgetCtx, cancelBudget := context.WithTimeout(base, budget)
	defer cancelBudget()

	var failures []error
	for i, action := range actions {
		if action.Run == nil {
			failures = append(failures, fmt.Errorf("%s: no cleanup function", action.Name))
			continue
		}
		if err := budgetCtx.Err(); err != nil {
			failures = append(failures, fmt.Errorf("%s: total cleanup budget exhausted: %w", action.Name, err))
			continue
		}
		remaining := time.Until(deadlineOf(budgetCtx))
		reserve := time.Duration(len(actions)-i-1) * credentialAdoptionCleanupReservePerAction
		available := remaining - reserve
		if available <= 0 {
			failures = append(failures, fmt.Errorf("%s: no time remains after reserving budget for later actions", action.Name))
			continue
		}
		timeout := action.Timeout
		if timeout <= 0 || timeout > available {
			timeout = available
		}
		ctx, cancel := context.WithTimeout(budgetCtx, timeout)
		err := action.Run(ctx)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", action.Name, err))
		}
	}
	return errors.Join(failures...)
}

func deadlineOf(ctx context.Context) time.Time {
	deadline, ok := ctx.Deadline()
	if !ok {
		return time.Now()
	}
	return deadline
}
