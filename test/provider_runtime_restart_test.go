package test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type runtimeRestartClient struct {
	client.Client
	deletePod func(context.Context, client.Object, ...client.DeleteOption) error
}

func (c *runtimeRestartClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.deletePod(ctx, obj, opts...)
}

func TestProviderRuntimeRestart(t *testing.T) {
	for _, scenario := range []string{"replacement-ready", "multiple-replicas", "same-name-new-uid", "old-process-remains", "no-replacement", "replacement-crashes", "delete-fails", "wrong-package", "ambiguous-deployments", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			r, provider, revision, deployment, set, pod := runtimeTestFixture(t)
			foreign := pod.DeepCopy()
			foreign.Name, foreign.UID, foreign.OwnerReferences = "unrelated", "unrelated-uid", nil
			objects := []client.Object{provider, revision, deployment, set, pod, foreign}
			wantDeletes := 1
			if scenario == "multiple-replicas" {
				deployment.Spec.Replicas = ptr.To(int32(2))
				deployment.Status.Replicas, deployment.Status.UpdatedReplicas = 2, 2
				deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas = 2, 2
				second := pod.DeepCopy()
				second.Name, second.UID = "pod-two", "pod-two-uid"
				objects = append(objects, second)
				wantDeletes = 2
			}
			if scenario == "ambiguous-deployments" {
				second := deployment.DeepCopy()
				second.Name, second.UID = "second-controller", "second-controller-uid"
				objects = append(objects, second)
			}
			base := runtimeTestClient(t, objects...)
			deletes := 0
			r.kube = &runtimeRestartClient{Client: base, deletePod: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
				original, ok := obj.(*corev1.Pod)
				if !ok || original.Name == foreign.Name {
					t.Fatal("restart targeted an unrelated object")
				}
				options := (&client.DeleteOptions{}).ApplyOptions(opts)
				if options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != original.UID {
					t.Fatal("restart omitted the original Pod UID precondition")
				}
				deletes++
				if scenario == "delete-fails" {
					return errors.New("synthetic delete failure")
				}
				if scenario == "old-process-remains" {
					return nil // Simulate a still-terminating Pod; stale health must not pass.
				}
				if err := base.Delete(ctx, original, opts...); err != nil {
					return err
				}
				if scenario == "no-replacement" {
					return nil
				}
				replacement := original.DeepCopy()
				replacement.UID += "-replacement"
				replacement.ResourceVersion = ""
				if scenario != "same-name-new-uid" {
					replacement.Name += "-replacement"
				}
				if scenario == "replacement-crashes" {
					replacement.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
				}
				return base.Create(ctx, replacement)
			}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			image := runtimeTestImage
			if scenario == "wrong-package" {
				image = "not-installed"
			}
			if scenario == "old-process-remains" || scenario == "no-replacement" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
				defer cancel()
			}
			if scenario == "cancelled" {
				cancel()
			}
			err := r.restart(ctx, provider.Name, image, runtimeTestImage, time.Millisecond, 0)
			switch scenario {
			case "replacement-ready", "multiple-replicas", "same-name-new-uid":
				if err != nil || deletes != wantDeletes {
					t.Fatalf("restart deletes=%d err=%v, want %d deletes and ready replacements", deletes, err, wantDeletes)
				}
			case "old-process-remains", "no-replacement":
				if !errors.Is(err, context.DeadlineExceeded) || deletes != 1 {
					t.Fatalf("restart accepted stale or absent runtime: deletes=%d err=%v", deletes, err)
				}
			case "replacement-crashes":
				if err == nil || !strings.Contains(err.Error(), "CrashLoopBackOff") {
					t.Fatalf("restart did not reject crashing replacement: %v", err)
				}
			case "delete-fails":
				if err == nil || !strings.Contains(err.Error(), "synthetic delete failure") {
					t.Fatalf("restart swallowed deletion failure: %v", err)
				}
			default:
				if err == nil || deletes != 0 {
					t.Fatalf("unsafe restart proceeded: deletes=%d err=%v", deletes, err)
				}
			}
			if err := base.Get(context.Background(), client.ObjectKeyFromObject(foreign), &corev1.Pod{}); err != nil {
				t.Fatalf("unrelated Pod was not preserved: %v", err)
			}
			gotDeployment := &appsv1.Deployment{}
			if err := base.Get(context.Background(), client.ObjectKeyFromObject(deployment), gotDeployment); err != nil || gotDeployment.Generation != deployment.Generation {
				t.Fatalf("restart changed the Deployment: %v", err)
			}
		})
	}
}
