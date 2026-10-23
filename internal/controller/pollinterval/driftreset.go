package pollinterval

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WithDriftReset wraps c so that a successful Update restarts the
// stable-resource backoff. Drift corrected by Update leaves Synced True,
// so its lastTransitionTime would not move on its own.
func WithDriftReset(c managed.ExternalConnector) managed.ExternalConnector {
	return managed.ExternalConnectorFn(func(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
		ec, err := c.Connect(ctx, mg)
		if err != nil {
			return nil, err
		}
		return driftResetClient{ExternalClient: ec}, nil
	})
}

// driftResetClient refreshes Synced's lastTransitionTime after a successful Update.
type driftResetClient struct {
	managed.ExternalClient
}

func (c driftResetClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	u, err := c.ExternalClient.Update(ctx, mg)
	if err != nil {
		return u, err
	}
	synced := mg.GetCondition(xpv2.TypeSynced)
	if synced.Status != corev1.ConditionTrue {
		return u, nil
	}
	synced.LastTransitionTime = metav1.Now()
	// SetConditions ignores a condition equal apart from its
	// lastTransitionTime, so replace Synced with a placeholder first.
	mg.SetConditions(xpv2.Condition{Type: xpv2.TypeSynced})
	mg.SetConditions(synced)
	return u, nil
}
