package providerconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errGetPC              = "cannot get ProviderConfig"
	errGetCISCreds        = "cannot get CIS credentials"
	errGetSACreds         = "cannot get Service Account credentials"
	errTrackRUsage        = "cannot track ResourceUsage"
	errTrackPCUsage       = "cannot track ProviderConfig usage"
	errNewClient          = "cannot create new Service"
	errCisSecretEmpty     = "CIS Secret is empty or nil, please check config & secrets referenced in provider config"
	errSaSecretEmpty      = "Service Account Secret is empty or nil, please check config & secrets referenced in provider config"
	errSecretKeyNotFound  = "%s: %v key not found in secret data"
	errCisSecretCorrupted = "CIS Secret does not match expected format"
)

// Setup adds a controller that reconciles ProviderConfigs by accounting for
// their current usage.
func Setup(mgr ctrl.Manager, o internalopts.UpjetOptions) error {
	name := providerconfig.ControllerName(v1alpha1.ProviderConfigGroupKind)

	of := resource.ProviderConfigKinds{
		Config:    v1alpha1.ProviderConfigGroupVersionKind,
		Usage:     v1alpha1.ProviderConfigUsageGroupVersionKind,
		UsageList: v1alpha1.ProviderConfigUsageListGroupVersionKind,
	}

	r := providerconfig.NewReconciler(
		mgr, of,
		providerconfig.WithLogger(o.Logger.WithValues("controller", name)),
		providerconfig.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntimeWithBackoff()).
		For(&v1alpha1.ProviderConfig{}).
		Watches(&v1alpha1.ProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
		WithEventFilter(resource.DesiredStateChanged()).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SetupGated is equivalent to Setup for providerconfig (no feature gating needed).
func SetupGated(mgr ctrl.Manager, o internalopts.UpjetOptions) error {
	return Setup(mgr, o)
}

func CreateClient(
	ctx context.Context,
	mg resource.Managed,
	kube client.Client,
	track LegacyTracker,
	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error),
	resourcetracker tracking.ReferenceResolverTracker,
) (*btp.Client, error) {

	lm, ok := mg.(resource.LegacyManaged)
	if !ok {
		return nil, errors.New("managed resource does not implement LegacyManaged")
	}

	pc, err := ResolveProviderConfig(ctx, lm, kube)
	if err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	if err = track.Track(ctx, lm); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err = resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	CISSecretData, cisErr := loadCisCredentials(ctx, kube, pc)
	if cisErr != nil {
		return nil, cisErr
	}

	ServiceAccountSecretData, saErr := loadSaCredentials(ctx, kube, pc)
	if saErr != nil {
		return nil, saErr
	}

	pcName := mg.GetProviderConfigReference().Name
	svc, err := DefaultClientCache.GetOrCreate(pcName, CISSecretData, ServiceAccountSecretData, func() (*btp.Client, error) {
		return newServiceFn(CISSecretData, ServiceAccountSecretData)
	})
	return svc, errors.Wrap(err, errNewClient)
}

func ResolveProviderConfig(ctx context.Context, mg resource.LegacyManaged, kube client.Client) (*v1alpha1.ProviderConfig, error) {
	pc := &v1alpha1.ProviderConfig{}
	ref := mg.GetProviderConfigReference()
	if ref == nil {
		return nil, errors.New("managed resource has no ProviderConfigReference")
	}
	err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, pc)
	return pc, err
}

// Resolves CIS credential secret to unified json string format
// Supports two formats:
//   - our own format:
//     data:
//     {"endpoints": {...}, "uaa": {...}, "grant_type": "client_credentials", ...
//     ....
//   - btp service operator generated:
//     endpoints: // json as string
//     uaa:	   // json as string
//     grant_type: client_credentials
func loadCisCredentials(ctx context.Context, kube client.Client, pc *v1alpha1.ProviderConfig) ([]byte, error) {
	cd := pc.Spec.CISSecret
	var secret corev1.Secret

	if findErr := kube.Get(ctx,
		types.NamespacedName{
			Namespace: cd.SecretRef.Namespace,
			Name:      cd.SecretRef.Name,
		}, &secret); findErr != nil {
		return nil, errors.Wrap(findErr, errGetCISCreds)
	}
	// Custom format with stringified json as data attribute
	if stringEncodedData, ok := secret.Data[cd.SecretRef.Key]; ok {
		return stringEncodedData, nil
	} else { // btp service operator generated format
		toBytes, err := decodedBtpOperatorSecret(secret.Data)
		if err != nil {
			return nil, errors.Wrap(err, errCisSecretCorrupted)
		}
		// basic validation to avoid empty secrets, defaults to "{}" containing 2 bytes
		if len(toBytes) == 2 {
			return nil, errors.New(errCisSecretEmpty)
		}
		return toBytes, nil
	}
}

// loadSaCredentials loads Service Account credentials from secret
func loadSaCredentials(ctx context.Context, kube client.Client, pc *v1alpha1.ProviderConfig) ([]byte, error) {
	cd := pc.Spec.ServiceAccountSecret

	ServiceAccountSecretData, err := resource.CommonCredentialExtractor(
		ctx,
		cd.Source,
		kube,
		cd.CommonCredentialSelectors,
	)
	if err != nil {
		return nil, errors.Wrap(err, errGetSACreds)
	}
	if ServiceAccountSecretData == nil {
		return nil, fmt.Errorf(errSecretKeyNotFound, errSaSecretEmpty, cd.SecretRef.Key)
	}

	return ServiceAccountSecretData, nil
}

// decodes btp service operator generated format from map of byte slices to stringified json
func decodedBtpOperatorSecret(data map[string][]byte) ([]byte, error) {
	var unpackedData = map[string]interface{}{}
	for _, k := range mapKeys(data) {
		if json.Valid(data[k]) {
			// any attribute that contains json as string, needs treated as rawvalues to avoid escaped quotes
			unpackedData[k] = json.RawMessage(data[k])
		} else {
			// others need to be handled as strings, otherwise byte slices will be base64 encoded during marshal
			unpackedData[k] = string(data[k])
		}

	}
	return json.Marshal(unpackedData)

}

func mapKeys(data map[string][]byte) []string {
	keys := make([]string, len(data))
	i := 0
	for k := range data {
		keys[i] = k
		i++
	}
	return keys
}
