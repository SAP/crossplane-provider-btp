package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	pkgv1 "github.com/crossplane/crossplane/apis/v2/pkg/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const runtimeContainer = "package-runtime"

// ProviderRuntime checks the actual controller, not just package Healthy status.
// It only reads Kubernetes objects and logs; it never changes cleanup policies.
// Diagnostics exclude Secrets, pod specs/environment, and termination messages.
// Raw controller logs can contain sensitive data: retain them locally, do not
// commit or print them, and review/redact them before sharing.
type ProviderRuntime struct {
	kube client.Client
	logs func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error)
}

func NewProviderRuntime(config *rest.Config) (*ProviderRuntime, error) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, pkgv1.AddToScheme} {
		if err := add(scheme); err != nil {
			return nil, err
		}
	}
	config = rest.CopyConfig(config)
	config.Timeout = 15 * time.Second
	kube, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	core, err := coreclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &ProviderRuntime{
		kube: kube,
		logs: func(ctx context.Context, ns, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
			return core.Pods(ns).GetLogs(pod, opts).Stream(ctx)
		},
	}, nil
}

// Wait requires the expected package's current revision, a complete deployment
// rollout, and ready runtime pods continuously for 10s. A crashing container is
// an immediate failure, rather than a 30-minute managed-resource health timeout.
func (r *ProviderRuntime) Wait(ctx context.Context, provider, packageImage, controllerImage string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return r.wait(ctx, provider, packageImage, controllerImage, time.Second, 10*time.Second)
}

func (r *ProviderRuntime) wait(ctx context.Context, provider, packageImage, controllerImage string, interval, stable time.Duration) error {
	var readySince time.Time
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ready, detail, err := r.ready(ctx, provider, packageImage, controllerImage)
		if err != nil {
			return err
		}
		if ready {
			if readySince.IsZero() {
				readySince = time.Now()
			}
			if time.Since(readySince) >= stable {
				return nil
			}
		} else {
			readySince = time.Time{}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("provider %s runtime not ready (%s): %w", provider, detail, ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (r *ProviderRuntime) ready(ctx context.Context, name, packageImage, controllerImage string) (bool, string, error) {
	provider := &pkgv1.Provider{}
	if err := r.kube.Get(ctx, types.NamespacedName{Name: name}, provider); err != nil {
		if apierrors.IsNotFound(err) {
			return false, "Provider not found", nil
		}
		return false, "", err
	}
	if provider.Spec.Package != packageImage || provider.Status.CurrentRevision == "" {
		return false, "waiting for expected package revision", nil
	}
	revision := &pkgv1.ProviderRevision{}
	if err := r.kube.Get(ctx, types.NamespacedName{Name: provider.Status.CurrentRevision}, revision); err != nil {
		if apierrors.IsNotFound(err) {
			return false, "current revision not found", nil
		}
		return false, "", err
	}
	if revision.Spec.Package != packageImage || !metav1.IsControlledBy(revision, provider) {
		return false, "current revision does not belong to expected package", nil
	}
	deployments := &appsv1.DeploymentList{}
	if err := r.kube.List(ctx, deployments); err != nil {
		return false, "", err
	}
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if !metav1.IsControlledBy(deployment, revision) || deployment.DeletionTimestamp != nil || !hasRuntimeImage(deployment.Spec.Template.Spec, controllerImage) {
			continue
		}
		pods, err := r.deploymentPods(ctx, deployment)
		if err != nil {
			return false, "", err
		}
		readyPods := int32(0)
		for _, pod := range pods {
			if pod.DeletionTimestamp != nil || !hasRuntimeImage(pod.Spec, controllerImage) {
				continue
			}
			for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
				if waiting := status.State.Waiting; waiting != nil {
					switch waiting.Reason {
					case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "RunContainerError":
						return false, "", fmt.Errorf("provider %s pod %s/%s container %s: %s", name, pod.Namespace, pod.Name, status.Name, waiting.Reason)
					}
				}
				if terminated := status.State.Terminated; terminated != nil && terminated.ExitCode != 0 {
					return false, "", fmt.Errorf("provider %s pod %s/%s container %s terminated: reason=%s exitCode=%d", name, pod.Namespace, pod.Name, status.Name, terminated.Reason, terminated.ExitCode)
				}
			}
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == runtimeContainer && status.Ready && status.State.Running != nil {
					readyPods++
				}
			}
		}
		replicas := int32(1)
		if deployment.Spec.Replicas != nil {
			replicas = *deployment.Spec.Replicas
		}
		if replicas > 0 && deployment.Status.ObservedGeneration >= deployment.Generation &&
			deployment.Status.UpdatedReplicas == replicas && deployment.Status.Replicas == replicas &&
			deployment.Status.ReadyReplicas == replicas && deployment.Status.AvailableReplicas == replicas && readyPods == replicas {
			return true, "controller rollout and runtime pods ready", nil
		}
		return false, fmt.Sprintf("deployment %s/%s rollout incomplete", deployment.Namespace, deployment.Name), nil
	}
	return false, "expected revision's controller deployment not found", nil
}

func hasRuntimeImage(spec corev1.PodSpec, image string) bool {
	for _, container := range spec.Containers {
		if container.Name == runtimeContainer && container.Image == image {
			return true
		}
	}
	return false
}

// Follow ownership, not just a label selector: unrelated/old provider pods must
// neither satisfy readiness nor have their logs collected accidentally.
func (r *ProviderRuntime) deploymentPods(ctx context.Context, deployment *appsv1.Deployment) ([]corev1.Pod, error) {
	sets := &appsv1.ReplicaSetList{}
	if err := r.kube.List(ctx, sets, client.InNamespace(deployment.Namespace)); err != nil {
		return nil, err
	}
	pods := &corev1.PodList{}
	if err := r.kube.List(ctx, pods, client.InNamespace(deployment.Namespace)); err != nil {
		return nil, err
	}
	var owned []corev1.Pod
	for _, pod := range pods.Items {
		for i := range sets.Items {
			if metav1.IsControlledBy(&sets.Items[i], deployment) && metav1.IsControlledBy(&pod, &sets.Items[i]) {
				owned = append(owned, pod)
				break
			}
		}
	}
	return owned, nil
}

// Capture writes minimal pod status plus bounded current/previous container logs
// before teardown deletes the provider. The caller supplies a private directory.
// Each log head is limited to 1MiB and each tail to 1MiB/1000 lines; all I/O
// has a total 1-minute budget.
// Previous logs are requested only when Kubernetes reports a container restart.
func (r *ProviderRuntime) Capture(ctx context.Context, providerName, directory string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return err
	}
	provider := &pkgv1.Provider{}
	if err := r.kube.Get(ctx, types.NamespacedName{Name: providerName}, provider); err != nil {
		return err
	}
	revisions := &pkgv1.ProviderRevisionList{}
	if err := r.kube.List(ctx, revisions); err != nil {
		return err
	}
	deployments := &appsv1.DeploymentList{}
	if err := r.kube.List(ctx, deployments); err != nil {
		return err
	}
	var failures []error
	if err := writeRuntimeJSON(filepath.Join(directory, "provider.json"), map[string]any{
		"name": provider.Name, "package": provider.Spec.Package, "currentRevision": provider.Status.CurrentRevision,
		"conditions": summarizeConditions(provider.Status.Conditions),
	}); err != nil {
		failures = append(failures, err)
	}
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		var revision *pkgv1.ProviderRevision
		for j := range revisions.Items {
			candidate := &revisions.Items[j]
			if metav1.IsControlledBy(candidate, provider) && metav1.IsControlledBy(deployment, candidate) {
				revision = candidate
				break
			}
		}
		if revision == nil {
			continue
		}
		if err := writeRuntimeJSON(filepath.Join(directory, revision.Name+"_revision.json"), map[string]any{
			"name": revision.Name, "package": revision.Spec.Package, "desiredState": revision.Spec.DesiredState,
			"conditions": summarizeConditions(revision.Status.Conditions),
		}); err != nil {
			failures = append(failures, err)
		}
		if err := writeRuntimeJSON(filepath.Join(directory, deployment.Namespace+"_"+deployment.Name+"_deployment.json"), map[string]any{
			"generation": deployment.Generation, "observedGeneration": deployment.Status.ObservedGeneration,
			"desiredReplicas": deployment.Spec.Replicas, "replicas": deployment.Status.Replicas,
			"updatedReplicas": deployment.Status.UpdatedReplicas, "readyReplicas": deployment.Status.ReadyReplicas,
			"availableReplicas": deployment.Status.AvailableReplicas,
		}); err != nil {
			failures = append(failures, err)
		}
		pods, err := r.deploymentPods(ctx, deployment)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, pod := range pods {
			prefix := filepath.Join(directory, pod.Namespace+"_"+pod.Name)
			// Do not serialize the whole Pod or ContainerStatus: either may
			// contain credentials in env, args, or termination messages.
			summary := struct {
				Phase      corev1.PodPhase           `json:"phase"`
				Reason     string                    `json:"reason,omitempty"`
				NodeName   string                    `json:"nodeName,omitempty"`
				Conditions []podConditionSummary     `json:"conditions,omitempty"`
				Containers []runtimeContainerSummary `json:"containers,omitempty"`
			}{Phase: pod.Status.Phase, Reason: pod.Status.Reason, NodeName: pod.Spec.NodeName}
			for _, condition := range pod.Status.Conditions {
				summary.Conditions = append(summary.Conditions, podConditionSummary{Type: condition.Type, Status: condition.Status, Reason: condition.Reason})
			}
			for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
				summary.Containers = append(summary.Containers, summarizeContainer(status, pod.Spec))
			}
			if err := writeRuntimeJSON(prefix+"_status.json", summary); err != nil {
				failures = append(failures, err)
			}
			for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
				for _, previous := range []bool{false, true} {
					if previous && !containerHasRestarts(pod.Status, container.Name) {
						continue
					}
					for _, end := range []string{"head", "tail"} {
						limit := int64(1024 * 1024)
						var tail *int64
						if end == "tail" {
							tailLines := int64(1000)
							tail = &tailLines
						}
						reader, err := r.logs(ctx, pod.Namespace, pod.Name, &corev1.PodLogOptions{
							Container: container.Name, Previous: previous, TailLines: tail, LimitBytes: &limit,
						})
						path := fmt.Sprintf("%s_%s_previous-%t_%s.log", prefix, container.Name, previous, end)
						if err == nil {
							var body []byte
							body, err = io.ReadAll(io.LimitReader(reader, limit))
							closeErr := reader.Close()
							if err == nil {
								err = errors.Join(closeErr, os.WriteFile(path, body, 0600))
							}
						}
						if err != nil {
							failures = append(failures, fmt.Errorf("collect %s: %w", filepath.Base(path), err))
						}
					}
				}
			}
		}
	}
	return errors.Join(failures...)
}

func writeRuntimeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

type conditionSummary struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type podConditionSummary struct {
	Type   corev1.PodConditionType `json:"type"`
	Status corev1.ConditionStatus  `json:"status"`
	Reason string                  `json:"reason,omitempty"`
}

func summarizeConditions(conditions []xpv2.Condition) []conditionSummary {
	out := make([]conditionSummary, 0, len(conditions))
	for _, condition := range conditions {
		out = append(out, conditionSummary{Type: string(condition.Type), Status: string(condition.Status), Reason: string(condition.Reason)})
	}
	return out
}

func containerHasRestarts(status corev1.PodStatus, name string) bool {
	for _, container := range append(status.InitContainerStatuses, status.ContainerStatuses...) {
		if container.Name == name {
			return container.RestartCount > 0 || container.LastTerminationState.Terminated != nil
		}
	}
	return false
}

type runtimeContainerSummary struct {
	Name                      string     `json:"name"`
	Image                     string     `json:"image,omitempty"`
	ImageID                   string     `json:"imageID,omitempty"`
	Restarts                  int32      `json:"restarts"`
	WaitingReason             string     `json:"waitingReason,omitempty"`
	TerminationReason         string     `json:"terminationReason,omitempty"`
	ExitCode                  int32      `json:"exitCode,omitempty"`
	StartedAt                 *time.Time `json:"startedAt,omitempty"`
	FinishedAt                *time.Time `json:"finishedAt,omitempty"`
	PreviousTerminationReason string     `json:"previousTerminationReason,omitempty"`
	PreviousExitCode          int32      `json:"previousExitCode,omitempty"`
	PreviousStartedAt         *time.Time `json:"previousStartedAt,omitempty"`
	PreviousFinishedAt        *time.Time `json:"previousFinishedAt,omitempty"`
}

func summarizeContainer(status corev1.ContainerStatus, spec corev1.PodSpec) runtimeContainerSummary {
	summary := runtimeContainerSummary{Name: status.Name, ImageID: status.ImageID, Restarts: status.RestartCount}
	for _, container := range append(spec.InitContainers, spec.Containers...) {
		if container.Name == status.Name {
			summary.Image = container.Image
			break
		}
	}
	if waiting := status.State.Waiting; waiting != nil {
		summary.WaitingReason = waiting.Reason
	}
	if terminated := status.State.Terminated; terminated != nil {
		summary.TerminationReason, summary.ExitCode = terminated.Reason, terminated.ExitCode
		summary.StartedAt, summary.FinishedAt = timePointer(terminated.StartedAt.Time), timePointer(terminated.FinishedAt.Time)
	}
	if terminated := status.LastTerminationState.Terminated; terminated != nil {
		summary.PreviousTerminationReason, summary.PreviousExitCode = terminated.Reason, terminated.ExitCode
		summary.PreviousStartedAt, summary.PreviousFinishedAt = timePointer(terminated.StartedAt.Time), timePointer(terminated.FinishedAt.Time)
	}
	return summary
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
