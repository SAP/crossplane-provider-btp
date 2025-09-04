package servicebindingnative

import (
	"context"
	"fmt"
	"maps"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	serviceinstanceclient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	servicemanagerclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

const errMisUse = "can not request API without GUID"

// ServiceBindingClientI acts as clear interface between controller and buisness logic
type ServiceBindingClientI interface {
	CreateServiceBinding(ctx context.Context) (*v1alpha1.ServiceBinding, map[string][]byte, error)
	UpdateServiceBinding(ctx context.Context) (*v1alpha1.ServiceBinding, error)
	DeleteServiceBinding(ctx context.Context) error
	NeedsCreation(ctx context.Context) (bool, error)
	NeedsUpdate(ctx context.Context) (bool, error)
	SyncStatus(ctx context.Context) (map[string][]byte, error)
	IsAvailable() bool
}

func NewServiceBindingClient(client *servicemanagerclient.APIClient, cr *v1alpha1.ServiceBinding, kube client.Client) *ServiceBindingClient {
	return &ServiceBindingClient{
		client: client,
		cr:     cr,
		kube:   kube,
	}
}

type ServiceBindingClient struct {
	client *servicemanagerclient.APIClient
	cr     *v1alpha1.ServiceBinding
	kube   client.Client

	cachedApi *servicemanagerclient.ServiceBindingResponseObject
}

func (d *ServiceBindingClient) UpdateServiceBinding(ctx context.Context) (*v1alpha1.ServiceBinding, error) {
	// without an externalID we can't connect to the API
	if d.externalID() == "" {
		return d.cr, errors.New(errMisUse)
	}

	// nothing to update here

	return d.cr, nil
}

func (d *ServiceBindingClient) DeleteServiceBinding(ctx context.Context) error {
	// without an externalID we can't connect to the API
	if eid := d.externalID(); eid == "" {
		return errors.New(errMisUse)
	} else {
		_, _, err := d.client.ServiceBindingsAPI.DeleteServiceBinding(ctx, eid).Execute()
		if genericErr, ok := err.(*servicemanagerclient.GenericOpenAPIError); ok &&
			strings.Contains(genericErr.Error(), "404") {
			return nil
		}

		return err
	}
}

func (d *ServiceBindingClient) NeedsCreation(ctx context.Context) (bool, error) {
	if d.externalID() == "" {
		return true, nil
	}
	var err error
	d.cachedApi, err = d.getServiceBinding(ctx)

	return d.cachedApi == nil, err
}

func (d *ServiceBindingClient) getServiceBinding(ctx context.Context) (*servicemanagerclient.ServiceBindingResponseObject, error) {
	extID := d.externalID()
	// without an externalID we can't connect to the API
	if extID == "" {
		return nil, errors.New(errMisUse)
	}

	ServiceBinding, raw, err := d.client.ServiceBindingsAPI.GetServiceBindingById(ctx, extID).Execute()
	if raw.StatusCode == 404 {
		// Unfortunately the API has no error type for 404 errors, so we can only extract that from raw status
		return nil, nil
	}
	if err != nil {
		return nil, specifyAPIError(err)
	}
	return ServiceBinding, nil
}

func (d *ServiceBindingClient) NeedsUpdate(ctx context.Context) (bool, error) {
	if d.cachedApi == nil {
		var err error
		d.cachedApi, err = d.getServiceBinding(ctx)
		if err != nil {
			return false, err
		}
	}
	return !isSynced(d.cr, d.cachedApi), nil
}

func (d *ServiceBindingClient) CreateServiceBinding(ctx context.Context) (*v1alpha1.ServiceBinding, map[string][]byte, error) {
	payload, err := d.toCreateApiPayload(ctx)
	if err != nil {
		return nil, nil, err
	}

	if d.cr.Spec.ForProvider.Rotation != nil {
		payload.Name = randomName(d.cr.Spec.ForProvider.Name)
	}

	sb, _, err := d.client.ServiceBindingsAPI.
		CreateServiceBinding(ctx).
		CreateServiceBindingRequestPayload(payload).
		Execute()

	if err != nil {
		return d.cr, nil, specifyAPIError(err)
	}
	meta.SetExternalName(d.cr, sb.GetId())
	d.cr.Status.AtProvider.ID = sb.GetId()
	d.cr.Status.AtProvider.Name = sb.GetName()
	d.cr.Status.AtProvider.CreatedAt = internal.Ptr(v1.NewTime(sb.GetCreatedAt()))

	creds, err := mapByte(sb.GetCredentials())
	if err != nil {
		return nil, nil, err
	}
	return d.cr, creds, nil
}

func (d *ServiceBindingClient) SyncStatus(ctx context.Context) (map[string][]byte, error) {
	if d.cachedApi == nil {
		var err error
		d.cachedApi, err = d.getServiceBinding(ctx)
		if err != nil {
			return nil, err
		}
	}

	d.cr.Status.AtProvider.ID = d.cachedApi.GetId()
	d.cr.Status.AtProvider.Name = d.cachedApi.GetName()
	d.cr.Status.AtProvider.CreatedAt = internal.Ptr(v1.NewTime(*d.cachedApi.CreatedAt))
	d.cr.Status.AtProvider.LastOperation.Description = d.cachedApi.LastOperation.GetDescription()
	d.cr.Status.AtProvider.LastOperation.Id = d.cachedApi.LastOperation.GetId()
	d.cr.Status.AtProvider.LastOperation.Ready = d.cachedApi.LastOperation.GetReady()
	d.cr.Status.AtProvider.LastOperation.Type = v1alpha1.ServiceBindingOperationType(d.cachedApi.LastOperation.GetType())
	d.cr.Status.AtProvider.LastOperation.State = v1alpha1.ServiceBindingOperationState(d.cachedApi.LastOperation.GetState())

	return mapByte(d.cachedApi.GetCredentials())
}

func (d *ServiceBindingClient) IsAvailable() bool {
	for _, c := range d.cr.Status.ConditionedStatus.Conditions {
		if c.Type == xpv1.TypeReady {
			return c.Status == corev1.ConditionTrue
		}
	}

	return false
}

func (d *ServiceBindingClient) externalID() string {
	extName := meta.GetExternalName(d.cr)

	if _, err := uuid.Parse(extName); err != nil {
		return ""
	}
	return extName
}

func isSynced(cr *v1alpha1.ServiceBinding, api *servicemanagerclient.ServiceBindingResponseObject) bool {
	return internal.Val(cr.Spec.ForProvider.ServiceInstanceID) == api.GetServiceInstanceId()
}

func (d *ServiceBindingClient) toCreateApiPayload(ctx context.Context) (servicemanagerclient.CreateServiceBindingRequestPayload, error) {
	paramters, err := buildComplexParameterMap(ctx, d.kube, d.cr.Spec.ForProvider.ParameterSecretRefs, d.cr.Spec.ForProvider.Parameters.Raw)
	if err != nil {
		return servicemanagerclient.CreateServiceBindingRequestPayload{}, errors.Wrap(err, "cannot create api payload")
	}

	payload := servicemanagerclient.CreateServiceBindingRequestPayload{
		Name:              d.cr.Spec.ForProvider.Name,
		ServiceInstanceId: *d.cr.Spec.ForProvider.ServiceInstanceID,
		Parameters:        internal.Ptr(paramters),
	}
	return payload, nil
}

var _ ServiceBindingClientI = &ServiceBindingClient{}

func specifyAPIError(err error) error {
	if genericErr, ok := err.(*servicemanagerclient.GenericOpenAPIError); ok {
		if genericErr.Body() != nil {
			return fmt.Errorf("API Error: %s, Status: %s", string(genericErr.Body()), genericErr.Error())
		}
	}
	return err
}

func buildComplexParameterMap(ctx context.Context, kube client.Client, secretRefs []xpv1.SecretKeySelector, specParams []byte) (map[string]string, error) {
	// resolve all parameter secret references and merge them into a single map
	parameterData, err := serviceinstanceclient.LookupSecrets(ctx, kube, secretRefs)
	if err != nil {
		return nil, err
	}

	// merge the plain parameters with the secret parameters
	specParamsMap, err := internal.UnmarshalRawParameters(specParams)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec parameters: %w", err)
	}

	maps.Copy(parameterData, specParamsMap)

	return mapString(parameterData)
}

func mapString(d map[string]any) (map[string]string, error) {
	m2 := make(map[string]string)

	for key, value := range d {
		switch value := value.(type) {
		case string:
			m2[key] = value
		default:
			m2[key] = fmt.Sprintf("%v", value)
		}
	}

	return m2, nil
}

func mapByte(d map[string]any) (map[string][]byte, error) {
	m2 := make(map[string][]byte)

	for key, value := range d {
		switch value := value.(type) {
		case string:
			m2[key] = []byte(value)
		case []byte:
			m2[key] = value
		default:
			m2[key] = fmt.Appendf(nil, "%v", value)
		}
	}

	return m2, nil
}
