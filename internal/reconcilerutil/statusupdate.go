package reconcilerutil

import (
	"context"
	"time"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateStatusWithRetry attempts to update the status of a managed resource with exponential
// backoff retry logic. On conflict errors it re-fetches the object and re-applies the mutate
// function before retrying. The mutate function is expected to modify the object's status based
// on its current state.
func UpdateStatusWithRetry[T client.Object](ctx context.Context, kube client.Client, cr T, maxRetries int, mutate func(T)) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			select {
			case <-time.After(time.Duration(100*(1<<uint(i-1))) * time.Millisecond):
			case <-ctx.Done():
				return errors.Wrap(ctx.Err(), "status update cancelled")
			}

			// Re-fetch to resolve resource version conflicts before retrying
			if err := kube.Get(ctx, types.NamespacedName{Name: cr.GetName(), Namespace: cr.GetNamespace()}, cr); err != nil {
				return errors.Wrap(err, "re-fetch before status retry failed")
			}
		}

		mutate(cr)

		lastErr = kube.Status().Update(ctx, cr)
		if lastErr == nil {
			return nil
		}
		if !kerrors.IsConflict(lastErr) {
			return errors.Wrap(lastErr, "status update failed")
		}
	}

	return errors.Wrap(lastErr, "status update failed after retries")
}
