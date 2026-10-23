//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"

	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/wait"
)

const unsupportedCredentialAdoptionMessage = "adopting an existing subaccountapicredential is not supported"

// A clean, unannotated probe may now be rejected during the guarded Read before
// it reaches Create. Either this exact rejection or a provider duplicate-name
// error is evidence that the target existed; arbitrary reconcile errors are not.
func credentialExistenceProof(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, unsupportedCredentialAdoptionMessage) || strings.Contains(message, "already exists")
}

// Only the annotated adoption attempt must report the resource-specific rejection.
func credentialRejectionError(cr *v1alpha1.SubaccountApiCredential, suppliedExternalName string, requireAdoptionError bool) error {
	var errs []error
	if cr.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue {
		errs = append(errs, errors.New("existing credential became Ready"))
	}
	synced := cr.GetCondition(xpv1.TypeSynced)
	if synced.Status != corev1.ConditionFalse || synced.ObservedGeneration < cr.Generation {
		errs = append(errs, fmt.Errorf("expected current-generation reconcile error, got Synced=%s/%s observedGeneration=%d generation=%d", synced.Status, synced.Reason, synced.ObservedGeneration, cr.Generation))
	} else if requireAdoptionError && !strings.Contains(strings.ToLower(synced.Message), unsupportedCredentialAdoptionMessage) {
		errs = append(errs, fmt.Errorf("expected explicit adoption rejection, got %q", synced.Message))
	}
	if got := xpmeta.GetExternalName(cr); got != "" && got != suppliedExternalName {
		errs = append(errs, fmt.Errorf("rejected resource retargeted external-name to %q (supplied %q)", got, suppliedExternalName))
	}
	return errors.Join(errs...)
}

// Cleanup requests only normal Kubernetes deletion. In particular, it never
// removes finalizers or resolves uncertain external ownership to force cleanup.
// Every object is attempted even when an earlier resource cannot finalize.
func deleteCredentialAdoptionTestResources(ctx context.Context, kube client.Client, objects []client.Object, logf func(string, ...any), opts ...wait.Option) error {
	var errs []error
	for _, object := range objects {
		key := client.ObjectKeyFromObject(object)
		if err := kube.Get(ctx, key, object); err != nil {
			if !apierrors.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("get %T %q for cleanup: %w", object, key.Name, err))
			}
			continue
		}
		logf("Cleanup %T %q: external-name=%q finalizers=%v", object, key.Name, xpmeta.GetExternalName(object), object.GetFinalizers())
		uid := object.GetUID()
		err := kube.Delete(ctx, object, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err == nil {
			err = wait.For(func(ctx context.Context) (bool, error) {
				getErr := kube.Get(ctx, key, object)
				if apierrors.IsNotFound(getErr) {
					return true, nil
				}
				return false, getErr
			}, append([]wait.Option{wait.WithContext(ctx)}, opts...)...)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("cleanup %T %q incomplete (external-name=%q finalizers=%v): %w", object, key.Name, xpmeta.GetExternalName(object), object.GetFinalizers(), err))
		}
	}
	return errors.Join(errs...)
}
