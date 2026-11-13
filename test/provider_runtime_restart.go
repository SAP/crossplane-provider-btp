package test

import (
	"context"
	"fmt"
	"time"

	pkgv1 "github.com/crossplane/crossplane/apis/v2/pkg/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Restart replaces all Pods belonging to the expected provider's current runtime.
// This deliberately mutates Kubernetes and is only for disposable test clusters.
// It waits for every original Pod UID to disappear before accepting a stable,
// ready replacement runtime; package health alone cannot prove a cold process.
func (r *ProviderRuntime) Restart(ctx context.Context, provider, packageImage, controllerImage string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return r.restart(ctx, provider, packageImage, controllerImage, time.Second, 10*time.Second)
}

func (r *ProviderRuntime) restart(ctx context.Context, name, packageImage, controllerImage string, interval, stable time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ready, detail, err := r.ready(ctx, name, packageImage, controllerImage)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("refusing to restart provider %s: runtime is not ready (%s)", name, detail)
	}
	provider := &pkgv1.Provider{}
	if err := r.kube.Get(ctx, types.NamespacedName{Name: name}, provider); err != nil {
		return err
	}
	revision := &pkgv1.ProviderRevision{}
	if err := r.kube.Get(ctx, types.NamespacedName{Name: provider.Status.CurrentRevision}, revision); err != nil {
		return err
	}
	if provider.Spec.Package != packageImage || revision.Spec.Package != packageImage || !metav1.IsControlledBy(revision, provider) {
		return fmt.Errorf("provider %s revision changed before restart", name)
	}
	deployments := &appsv1.DeploymentList{}
	if err := r.kube.List(ctx, deployments); err != nil {
		return err
	}
	var target *appsv1.Deployment
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if deployment.DeletionTimestamp == nil && metav1.IsControlledBy(deployment, revision) && hasRuntimeImage(deployment.Spec.Template.Spec, controllerImage) {
			if target != nil {
				return fmt.Errorf("provider %s has multiple current runtime deployments; refusing ambiguous restart", name)
			}
			target = deployment
		}
	}
	if target == nil {
		return fmt.Errorf("provider %s runtime deployment disappeared before restart", name)
	}
	pods, err := r.deploymentPods(ctx, target)
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		return fmt.Errorf("provider %s has no runtime Pods to restart", name)
	}
	for i := range pods {
		pod := &pods[i]
		if pod.UID == "" {
			return fmt.Errorf("runtime Pod %s/%s has no UID", pod.Namespace, pod.Name)
		}
	}
	for i := range pods {
		pod := &pods[i]
		if err := r.kube.Delete(ctx, pod, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pod.UID}}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("restart provider Pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}
	// Do not accept replacement readiness while an old process can still hold
	// the operation tracker (e.g. a terminating or finalizer-blocked Pod).
	for _, original := range pods {
		for {
			current := &corev1.Pod{}
			err := r.kube.Get(ctx, client.ObjectKeyFromObject(&original), current)
			if apierrors.IsNotFound(err) || (err == nil && current.UID != original.UID) {
				break
			}
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("original provider Pod %s/%s did not terminate: %w", original.Namespace, original.Name, ctx.Err())
			case <-time.After(interval):
			}
		}
	}
	return r.wait(ctx, name, packageImage, controllerImage, interval, stable)
}
