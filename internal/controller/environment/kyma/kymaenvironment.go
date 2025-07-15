package kyma

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/internal"
	environments "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironment"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	kymaenv "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironment"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotKymaEnvironment   = "managed resource is not a KymaEnvironment custom resource"
	errExtractSecretKey     = "No Cloud Management Secret Found"
	errGetCredentialsSecret = "Could not get secret of local cloud management"
	errTrackPCUsage         = "cannot track ProviderConfig usage"
	errGetPC                = "cannot get ProviderConfig"
	errGetCreds             = "cannot get credentials"
	errTrackRUsage          = "cannot track ResourceUsage"
	errCheckUpdate          = "Could not check for needsUpdate"
	errParameterParsing     = ".Spec.ForProvider.Parameters seem to be corrupted"
	errServiceParsing       = "Parameters from service response seem to be corrupted"
	errCantDescribe         = "Could not describe kyma instance"
	errCircutBreak          = "circuit breaker is on; check retry status, update parameters or set annotation " + v1alpha1.IgnoreCircuitBreaker + " to any value"
	maxRetriesDefault       = 3
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
	log          logr.Logger
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client     kymaenv.Client
	tracker    tracking.ReferenceResolverTracker
	kube       client.Client
	httpClient *http.Client
	log        logr.Logger
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KymaEnvironment)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKymaEnvironment)
	}

	instance, hasUpdate, err := c.client.DescribeInstance(ctx, *cr)

	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errCantDescribe)
	}

	lastModified := cr.Status.AtProvider.ModifiedDate
	cr.Status.AtProvider = kymaenv.GenerateObservation(instance)

	if cr.Status.AtProvider.State == nil {
		cr.Status.SetConditions(xpv1.Unavailable())
	} else if *cr.Status.AtProvider.State == v1alpha1.InstanceStateOk {
		cr.Status.SetConditions(xpv1.Available())
	} else if *cr.Status.AtProvider.State == v1alpha1.InstanceStateCreating {
		cr.Status.SetConditions(xpv1.Creating())
	} else if *cr.Status.AtProvider.State == v1alpha1.InstanceStateDeleting {
		cr.Status.SetConditions(xpv1.Deleting())
	} else if *cr.Status.AtProvider.State == v1alpha1.InstanceStateUpdating {
		cr.Status.SetConditions(xpv1.Available())
	} else {
		cr.Status.SetConditions(xpv1.Unavailable())
	}

	if needsCreation := c.needsCreation(cr); needsCreation {
		return managed.ExternalObservation{
			ResourceExists: !needsCreation,
		}, nil
	}

	needsUpdate, diff, err := c.needsUpdateWithDiff(cr)
	if needsUpdate || err != nil {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: !needsUpdate,
			Diff:             diff,
		}, errors.Wrap(err, errCheckUpdate)
	}

	if connectionDetailsNeedUpdate(lastModified, cr) {
		details, readErr := environments.GetConnectionDetails(instance, c.httpClient)
		if readErr != nil {
			return managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			}, errors.Wrap(readErr, "can not obtain kubeConfig")
		}
		return managed.ExternalObservation{
			ResourceExists:          true,
			ResourceUpToDate:        true,
			ConnectionDetails:       details,
			ResourceLateInitialized: hasUpdate,
		}, nil
	}

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        true,
		ResourceLateInitialized: hasUpdate,
	}, nil
}

func connectionDetailsNeedUpdate(lastModified *string, cr *v1alpha1.KymaEnvironment) bool {
	return lastModified != nil && !reflect.DeepEqual(lastModified, cr.Status.AtProvider.ModifiedDate)
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KymaEnvironment)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKymaEnvironment)
	}

	guid, err := c.client.CreateInstance(ctx, *cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	meta.SetExternalName(cr, guid)

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.KymaEnvironment)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotKymaEnvironment)
	}
	if cr.Status.RetryStatus != nil && cr.Status.RetryStatus.CircuitBreaker && !metav1.HasAnnotation(cr.ObjectMeta, v1alpha1.IgnoreCircuitBreaker) {
		return managed.ExternalUpdate{}, errors.New(errCircutBreak)
	}

	err := c.client.UpdateInstance(ctx, *cr)

	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.KymaEnvironment)
	if !ok {
		return errors.New(errNotKymaEnvironment)
	}
	c.tracker.SetConditions(ctx, cr)
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return errors.New(providerv1alpha1.ErrResourceInUse)
	}

	if cr.Status.AtProvider.State != nil && *cr.Status.AtProvider.State == v1alpha1.InstanceStateDeleting {
		return nil
	}

	return c.client.DeleteInstance(ctx, *cr)
}

func (c *external) needsCreation(cr *v1alpha1.KymaEnvironment) bool {
	return cr.Status.AtProvider.State == nil
}

func (c *external) needsUpdateWithDiff(cr *v1alpha1.KymaEnvironment) (bool, string, error) {
	if *cr.Status.AtProvider.State != v1alpha1.InstanceStateOk {
		return false, "", nil
	}

	desired, err := internal.UnmarshalRawParameters(cr.Spec.ForProvider.Parameters.Raw)
	desired = kymaenv.AddKymaDefaultParameters(desired, cr.Name, string(cr.UID))
	if err != nil {
		return false, "", errors.Wrap(err, errParameterParsing)
	}

	current, err := internal.UnmarshalRawParameters([]byte(*cr.Status.AtProvider.Parameters))
	if err != nil {
		return false, "", errors.Wrap(err, errServiceParsing)
	}

	maxRetries, err := lookupMaxRetries(cr, maxRetriesDefault)
	if err != nil {
		return false, "", err
	}

	diff := cmp.Diff(desired, current)

	updateCircuitBreakerStatus(cr, desired, current, diff, maxRetries)

	return diff != "", diff, nil

}

func lookupMaxRetries(cr *v1alpha1.KymaEnvironment, defaultRetries int) (int, error) {
	if metav1.HasAnnotation(cr.ObjectMeta, v1alpha1.AnnotationMaxRetries) {
		maxRetries, err := strconv.Atoi(cr.GetAnnotations()[v1alpha1.AnnotationMaxRetries])
		return maxRetries, errors.Wrap(err, "could not parse max retries annotation")
	}
	return defaultRetries, nil
}

func updateCircuitBreakerStatus(cr *v1alpha1.KymaEnvironment, desired any, current any, diff string, maxRetries int) {
	desiredHash := hash(desired)
	currentHash := hash(current)
	if cr.Status.RetryStatus == nil {
		cr.Status.RetryStatus = &v1alpha1.RetryStatus{}
	}

	cr.Status.RetryStatus.Diff = diff
	if diff == "" || !hashesArePersistent(cr, desiredHash, currentHash) {
		// Reset retry status if hashes change
		cr.Status.RetryStatus.DesiredHash = desiredHash
		cr.Status.RetryStatus.CurrentHash = currentHash
		cr.Status.RetryStatus.Count = 1
		cr.Status.RetryStatus.CircuitBreaker = false
	} else {
		if !cr.Status.RetryStatus.CircuitBreaker {
			cr.Status.RetryStatus.Count++
			cr.Status.RetryStatus.CircuitBreaker = circuitBroken(cr, maxRetries)
		}
	}
}

func circuitBroken(cr *v1alpha1.KymaEnvironment, maxRetries int) bool {
	return cr.Status.RetryStatus.Count >= maxRetries
}

func hashesArePersistent(cr *v1alpha1.KymaEnvironment, desiredHash string, currentHash string) bool {
	return cr.Status.RetryStatus.DesiredHash == desiredHash && cr.Status.RetryStatus.CurrentHash == currentHash
}

func hash(params any) string {
	h := sha256.New()
	if err := json.NewEncoder(h).Encode(params); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
