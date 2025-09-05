package providerconfig

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/upjet/pkg/controller"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errGetPC              = "cannot get ProviderConfig"
	errGetCISCreds        = "cannot get CIS credentials"
	errGetCFCreds         = "cannot get Service Account credentials"
	errTrackRUsage        = "cannot track ResourceUsage"
	errTrackPCUsage       = "cannot track ProviderConfig usage"
	errNewClient          = "cannot create new Service"
	errCisSecretEmpty     = "CIS Secret is empty or nil, please check config & secrets referenced in provider config"
	errCisSecretCorrupted = "CIS Secret does not match expected format"
)

// Setup adds a controller that reconciles ProviderConfigs by accounting for
// their current usage.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := providerconfig.ControllerName(v1alpha1.ProviderConfigGroupKind)

	of := resource.ProviderConfigKinds{
		Config:    v1alpha1.ProviderConfigGroupVersionKind,
		UsageList: v1alpha1.ProviderConfigUsageListGroupVersionKind,
	}

	r := providerconfig.NewReconciler(
		mgr, of,
		providerconfig.WithLogger(o.Logger.WithValues("controller", name)),
		providerconfig.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.ProviderConfig{}).
		Watches(&v1alpha1.ProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
		WithEventFilter(resource.DesiredStateChanged()).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

func CreateClient(
	ctx context.Context,
	mg resource.Managed,
	kube client.Client,
	track resource.Tracker,
	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error),
	resourcetracker tracking.ReferenceResolverTracker,
) (*btp.Client, error) {

	pc, err := ResolveProviderConfig(ctx, mg, kube)
	if err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	if err = track.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err = resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	CISSecretData, cisErr := loadCisCredentials(ctx, kube, pc)
	if cisErr != nil {
		return nil, cisErr
	}

	cd := pc.Spec.ServiceAccountSecret
	ServiceAccountSecretData, err := resource.CommonCredentialExtractor(
		ctx,
		cd.Source,
		kube,
		cd.CommonCredentialSelectors,
	)
	if err != nil {
		return nil, errors.Wrap(err, errGetCFCreds)
	}
	if ServiceAccountSecretData == nil {
		return nil, errors.New(errCisSecretEmpty)

	}

	svc, err := newServiceFn(CISSecretData, ServiceAccountSecretData)
	return svc, errors.Wrap(err, errNewClient)
}

func ResolveProviderConfig(ctx context.Context, mg resource.Managed, kube client.Client) (*v1alpha1.ProviderConfig, error) {
	pc := &v1alpha1.ProviderConfig{}
	err := kube.Get(ctx, types.NamespacedName{Name: mg.GetProviderConfigReference().Name}, pc)
	return pc, err
}

// Resolves CIS credential secret to unified json string format
// Supports two formats:
//   - our own format:
//     data:
//     endpoints: // json as string
//     uaa:	   // json as string
//     grant_type: client_credentials
//     ....
//   - btp service operator generated:
//     data:
//     data: //(key as defined in providerconfig ref)
//     {"endpoints": {...}, "uaa": {...}, "grant_type": "client_credentials", ...
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
		return toBytes, nil
	}
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
