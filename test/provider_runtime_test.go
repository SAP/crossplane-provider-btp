package test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgv1 "github.com/crossplane/crossplane/apis/v2/pkg/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const runtimeTestImage = "example.invalid/provider:new"

func runtimeTestFixture(t *testing.T) (*ProviderRuntime, *pkgv1.Provider, *pkgv1.ProviderRevision, *appsv1.Deployment, *appsv1.ReplicaSet, *corev1.Pod) {
	t.Helper()
	provider := &pkgv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "provider", UID: "provider-uid"}}
	provider.Spec.Package = runtimeTestImage
	provider.Status.CurrentRevision = "new-revision"
	revision := &pkgv1.ProviderRevision{ObjectMeta: metav1.ObjectMeta{Name: "new-revision", UID: "revision-uid"}}
	revision.Spec.Package = runtimeTestImage
	revision.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(provider, pkgv1.SchemeGroupVersion.WithKind("Provider"))}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: "crossplane-system", UID: "deployment-uid", Generation: 2}}
	deployment.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(revision, pkgv1.SchemeGroupVersion.WithKind("ProviderRevision"))}
	deployment.Spec.Replicas = ptr.To(int32(1))
	deployment.Spec.Template.Spec.Containers = []corev1.Container{{Name: runtimeContainer, Image: runtimeTestImage}}
	deployment.Status = appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}
	set := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "set", Namespace: deployment.Namespace, UID: "set-uid"}}
	set.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(deployment, appsv1.SchemeGroupVersion.WithKind("Deployment"))}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: deployment.Namespace, UID: "pod-uid"}}
	pod.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(set, appsv1.SchemeGroupVersion.WithKind("ReplicaSet"))}
	pod.Spec = *deployment.Spec.Template.Spec.DeepCopy()
	pod.Status.Phase = corev1.PodRunning
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: runtimeContainer, Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}
	return &ProviderRuntime{}, provider, revision, deployment, set, pod
}

func runtimeTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, pkgv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestProviderRuntimeReadiness(t *testing.T) {
	for _, scenario := range []string{
		"ready", "missing-provider", "old-current-revision", "foreign-revision", "foreign-deployment", "old-image", "scaled-zero", "unobserved-generation", "incomplete-rollout", "no-pods", "unready-pod", "foreign-pod", "deleting-pod", "crash-loop", "image-pull", "terminated", "init-failure",
	} {
		t.Run(scenario, func(t *testing.T) {
			r, provider, revision, deployment, set, pod := runtimeTestFixture(t)
			wantError := false
			switch scenario {
			case "old-current-revision":
				revision.Spec.Package = "example.invalid/provider:old"
			case "foreign-revision":
				revision.OwnerReferences = nil
			case "foreign-deployment":
				deployment.OwnerReferences = nil
			case "old-image":
				deployment.Spec.Template.Spec.Containers[0].Image = "example.invalid/provider:old"
			case "scaled-zero":
				deployment.Spec.Replicas = ptr.To(int32(0))
			case "unobserved-generation":
				deployment.Status.ObservedGeneration = 1
			case "incomplete-rollout":
				deployment.Status.UpdatedReplicas = 0
			case "unready-pod":
				pod.Status.ContainerStatuses[0].Ready = false
			case "foreign-pod":
				pod.OwnerReferences = nil
			case "deleting-pod":
				pod.DeletionTimestamp = ptr.To(metav1.Now())
				pod.Finalizers = []string{"test.invalid/finalizer"}
			case "crash-loop", "image-pull":
				reason := "CrashLoopBackOff"
				if scenario == "image-pull" {
					reason = "ImagePullBackOff"
				}
				pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}
				wantError = true
			case "terminated":
				pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 2, Reason: "Error"}}
				wantError = true
			case "init-failure":
				pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "init", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}}}
				wantError = true
			}
			objects := []client.Object{provider, revision, deployment, set, pod}
			if scenario == "missing-provider" {
				objects = objects[1:]
			}
			if scenario == "no-pods" {
				objects = objects[:4]
			}
			r.kube = runtimeTestClient(t, objects...)
			ready, _, err := r.ready(context.Background(), provider.Name, runtimeTestImage, runtimeTestImage)
			if (err != nil) != wantError || ready != (scenario == "ready") {
				t.Fatalf("ready=%t err=%v, expected ready=%t error=%t", ready, err, scenario == "ready", wantError)
			}
		})
	}
}

func TestProviderRuntimeWait(t *testing.T) {
	r, provider, revision, deployment, set, pod := runtimeTestFixture(t)
	r.kube = runtimeTestClient(t, provider, revision, deployment, set, pod)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if err := r.wait(ctx, provider.Name, runtimeTestImage, runtimeTestImage, time.Millisecond, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("accepted transient ready state without stabilization")
	}

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := r.wait(ctx, provider.Name, runtimeTestImage, runtimeTestImage, time.Millisecond, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := r.wait(ctx, provider.Name, "not-installed", runtimeTestImage, time.Millisecond, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded timeout for stale revision, got %v", err)
	}

	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
	r.kube = runtimeTestClient(t, provider, revision, deployment, set, pod)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.wait(ctx, provider.Name, runtimeTestImage, runtimeTestImage, time.Minute, 10*time.Second); err == nil || !strings.Contains(err.Error(), "CrashLoopBackOff") || ctx.Err() != nil {
		t.Fatalf("expected immediate crash error without waiting for timeout, got %v", err)
	}
}

func TestProviderRuntimeCapture(t *testing.T) {
	r, provider, revision, deployment, set, pod := runtimeTestFixture(t)
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "SENSITIVE", Value: "do-not-dump-env"}}
	pod.Status.ContainerStatuses[0].RestartCount = 3
	pod.Status.ContainerStatuses[0].ImageID = "containerd://image-id"
	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "do-not-dump-wait-message"}}
	pod.Status.ContainerStatuses[0].LastTerminationState.Terminated = &corev1.ContainerStateTerminated{ExitCode: 2, Reason: "Error", Message: "do-not-dump-termination-message", StartedAt: metav1.NewTime(time.Unix(10, 0)), FinishedAt: metav1.NewTime(time.Unix(11, 0))}
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady", Message: "do-not-dump-condition-message"}}
	foreign := pod.DeepCopy()
	foreign.Name = "unrelated"
	foreign.OwnerReferences = nil
	r.kube = runtimeTestClient(t, provider, revision, deployment, set, pod, foreign)
	calls := 0
	r.logs = func(_ context.Context, ns, name string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
		calls++
		if ns != pod.Namespace || name != pod.Name || opts.Container != runtimeContainer || opts.Follow || opts.LimitBytes == nil || *opts.LimitBytes != 1024*1024 {
			t.Fatal("unbounded, unscoped or unexpected log request")
		}
		if opts.TailLines != nil && *opts.TailLines != 1000 {
			t.Fatal("unexpected tail log line limit")
		}
		body := "current startup error"
		if opts.TailLines != nil {
			body = strings.Repeat("x", 2*1024*1024)
		}
		return io.NopCloser(strings.NewReader(body)), nil
	}
	dir := filepath.Join(t.TempDir(), "diagnostics")
	if err := r.Capture(context.Background(), provider.Name, dir); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("got %d log requests, want head/tail current and previous only for owned pod", calls)
	}
	for _, end := range []string{"head", "tail"} {
		if _, err := os.Stat(filepath.Join(dir, pod.Namespace+"_"+pod.Name+"_"+runtimeContainer+"_previous-false_"+end+".log")); err != nil {
			t.Fatalf("missing current %s log capture: %v", end, err)
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range files {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 || info.Size() > 1024*1024 {
			t.Fatalf("unsafe artifact permissions or unbounded size: %s", entry.Name())
		}
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("diagnostic directory must be private: %v", err)
	}
	status, err := os.ReadFile(filepath.Join(dir, pod.Namespace+"_"+pod.Name+"_status.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"do-not-dump", "SENSITIVE", "Message"} {
		if strings.Contains(string(status), forbidden) {
			t.Fatalf("status leaked forbidden field/content %q", forbidden)
		}
	}
	for _, expected := range []string{"CrashLoopBackOff", "previousExitCode\": 2", "imageID", "containerd://image-id", "previousFinishedAt", "previousStartedAt", "ContainersNotReady"} {
		if !strings.Contains(string(status), expected) {
			t.Fatalf("status missing diagnostic field %q", expected)
		}
	}

	// One unavailable stream must not prevent the other stream/status capture.
	r.logs = func(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
		if !opts.Previous {
			return nil, fmt.Errorf("current log unavailable")
		}
		return io.NopCloser(strings.NewReader("previous startup error")), nil
	}
	dir = filepath.Join(t.TempDir(), "partial")
	if err := r.Capture(context.Background(), provider.Name, dir); err == nil {
		t.Fatal("missing log error was swallowed")
	}
	if _, err := os.Stat(filepath.Join(dir, pod.Namespace+"_"+pod.Name+"_"+runtimeContainer+"_previous-true_tail.log")); err != nil {
		t.Fatal("failed to preserve previous log after current log error")
	}

	// A never-restarted container has no previous stream; do not report that
	// normal Kubernetes response as an incomplete diagnostic capture.
	r, provider, revision, deployment, set, pod = runtimeTestFixture(t)
	r.kube = runtimeTestClient(t, provider, revision, deployment, set, pod)
	r.logs = func(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
		if opts.Previous {
			t.Fatal("requested previous log for a container with zero restarts")
		}
		return io.NopCloser(strings.NewReader("current log")), nil
	}
	if err := r.Capture(context.Background(), provider.Name, filepath.Join(t.TempDir(), "never-restarted")); err != nil {
		t.Fatalf("unexpected diagnostic error for zero-restart container: %v", err)
	}
}

func TestProviderRuntimeLogTransport(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if req.URL.Path != "/api/v1/namespaces/crossplane-system/pods/controller/log" || req.URL.Query().Get("previous") != "true" || req.URL.Query().Get("container") != runtimeContainer {
			t.Errorf("unexpected log request: %s", req.URL.Path)
		}
		_, _ = w.Write([]byte("synthetic startup failure"))
	}))
	defer server.Close()
	r, err := NewProviderRuntime(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := r.logs(context.Background(), "crossplane-system", "controller", &corev1.PodLogOptions{Container: runtimeContainer, Previous: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stream.Close(); err != nil {
			t.Errorf("close synthetic log stream: %v", err)
		}
	})
	body, err := io.ReadAll(stream)
	if err != nil || string(body) != "synthetic startup failure" || requests != 1 {
		t.Fatalf("stream failed: requests=%d err=%v", requests, err)
	}
}
