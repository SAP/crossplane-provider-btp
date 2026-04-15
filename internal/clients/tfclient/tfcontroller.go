package tfclient

import (
	"context"
	"encoding/json"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Status string

const (
	Unknown     Status = "Unknown"
	NotExisting Status = "NotExisting"
	Drift       Status = "Drift"
	UpToDate    Status = "UpToDate"
)

const iso8601Date = "2006-01-02T15:04:05Z0700"

// TfProxyConnectorI is an generic interface that prepares a TfProxyController and returns it for the given native resource
type TfProxyConnectorI[NATIVE resource.Managed] interface {
	Connect(context.Context, NATIVE) (TfProxyControllerI, error)
}

// TfProxyControllerI is an interface that provides the lifecycle management for a resource by internally delegating to a terraform based resource
type TfProxyControllerI interface {
	Observe(ctx context.Context) (Status, map[string][]byte, error)
	Create(ctx context.Context) error
	Update(ctx context.Context) error
	Delete(ctx context.Context) error
	// QueryUpdatedData returns the relevant status data once the async creation is done
	QueryAsyncData(ctx context.Context) *ObservationData
}

type SaveConditionsFn func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error

// ObservationData is the bridge struct that carries data from the Terraform resource to the Crossplane CR.
// It is filled by QueryAsyncData() and then saved to the CR status by saveInstanceData().
type ObservationData struct {
	// ExternalName is the name used to identify the resource in the external system (BTP)
	ExternalName string `json:"externalName"`
	// ID is the unique identifier of the resource in BTP
	ID string `json:"id"`
	// DashboardURL is the URL of the web-based management UI for the resource
	DashboardURL string `json:"dashboardUrl"`
	// Conditions are the Crossplane conditions to set on the CR (e.g. Available, AsyncOperationFinished)
	Conditions []xpv1.Condition

	// Additional observation fields populated from the Terraform resource
	CreatedDate  *metav1.Time `json:"createdDate,omitempty"`
	LastModified *metav1.Time `json:"lastModified,omitempty"`
	State        string       `json:"state,omitempty"`
	Ready        *bool        `json:"ready,omitempty"`
	Usable       *bool        `json:"usable,omitempty"`
	PlatformID   string       `json:"platformId,omitempty"`
}

// TfMapper is a generic interface to map a native resource to an upjet resource that will be used for applying to terraform
type TfMapper[NATIVE resource.Managed, UPJETTED ujresource.Terraformed] interface {
	TfResource(context.Context, NATIVE, client.Client) (UPJETTED, error)
}

type TfProxyConnector[NATIVE resource.Managed, UPJETTED ujresource.Terraformed] struct {
	tfMapper  TfMapper[NATIVE, UPJETTED]
	connector managed.ExternalConnecter
	kube      client.Client
}

func NewTfProxyConnector[NATIVE resource.Managed, UPJETTED ujresource.Terraformed](tfConnector managed.ExternalConnecter, tfMapper TfMapper[NATIVE, UPJETTED], kube client.Client) TfProxyConnector[NATIVE, UPJETTED] {
	return TfProxyConnector[NATIVE, UPJETTED]{
		connector: tfConnector,
		tfMapper:  tfMapper,
		kube:      kube,
	}
}

// Connect prepares the TfProxyController for the given native resource and returns it, it uses an implementation of TfMapper to map the native resource to an upjet resource
func (t *TfProxyConnector[NATIVE, UPJETTED]) Connect(ctx context.Context, cr NATIVE) (TfProxyControllerI, error) {
	ssi, err := t.tfMapper.TfResource(ctx, cr, t.kube)

	if err != nil {
		return nil, err
	}

	ctrl, err := t.connector.Connect(ctx, ssi)
	if err != nil {
		return nil, err
	}

	return &TfProxyController[UPJETTED]{
		tfClient:   ctrl,
		tfResource: ssi,
	}, nil
}

var _ TfProxyControllerI = &TfProxyController[*v1alpha1.SubaccountServiceInstance]{}

// TfProxyController is a client that provides lifecycle management for a resource by internally delegating to a terraform based resource
type TfProxyController[UPJETTED ujresource.Terraformed] struct {
	tfClient   managed.ExternalClient
	tfResource UPJETTED
}

// QueryUpdatedData returns the relevant status data once the async creation is done
func (t *TfProxyController[UPJETTED]) QueryAsyncData(ctx context.Context) *ObservationData {
	// only query the async data if the operation is finished
	if t.tfResource.GetCondition(ujresource.TypeAsyncOperation).Reason == ujresource.ReasonFinished {
		sid := &ObservationData{}
		sid.ID = t.tfResource.GetID()
		sid.ExternalName = meta.GetExternalName(t.tfResource)
		sid.Conditions = []xpv1.Condition{xpv1.Available(), ujresource.AsyncOperationFinishedCondition()}

		// GetObservation() returns the raw key-value map from the Terraform state.
		// Each field is typed as "any", so we use type assertions e.g. .(string) to safely extract values.
		// The ", ok" pattern means: if the field is missing or the wrong type, skip it instead of crashing.
		if obs, err := t.tfResource.GetObservation(); err == nil {
			if dashboardURL, ok := obs["dashboard_url"].(string); ok {
				sid.DashboardURL = dashboardURL
			}
			// Dates come as RFC3339 strings e.g. "2026-01-01T10:00:00Z".
			// We parse them into Go time.Time and then wrap in metav1.Time for Kubernetes compatibility.
			if createdDate, ok := obs["created_date"].(string); ok {
				if t, err := time.Parse(iso8601Date, createdDate); err == nil {
					mt := metav1.NewTime(t.UTC())
					sid.CreatedDate = &mt
				}
			}
			if lastModified, ok := obs["last_modified"].(string); ok {
				if t, err := time.Parse(iso8601Date, lastModified); err == nil {
					mt := metav1.NewTime(t.UTC())
					sid.LastModified = &mt
				}
			}
			if state, ok := obs["state"].(string); ok {
				sid.State = state
			}
			if ready, ok := obs["ready"].(bool); ok {
				sid.Ready = &ready
			}
			if usable, ok := obs["usable"].(bool); ok {
				sid.Usable = &usable
			}
			if platformID, ok := obs["platform_id"].(string); ok {
				sid.PlatformID = platformID
			}
		}

		return sid
	}
	return nil
}

func (t *TfProxyController[UPJETTED]) Create(ctx context.Context) error {
	_, err := t.tfClient.Create(ctx, t.tfResource)
	return err
}

func (t *TfProxyController[UPJETTED]) Update(ctx context.Context) error {
	_, err := t.tfClient.Update(ctx, t.tfResource)
	return err
}

func (t *TfProxyController[UPJETTED]) Delete(ctx context.Context) error {
	_, err := t.tfClient.Delete(ctx, t.tfResource)
	return err
}

// Observe implements TfProxyControllerI.
func (t *TfProxyController[UPJETTED]) Observe(ctx context.Context) (Status, map[string][]byte, error) {
	// will return true, true, in case of in memory running async operations
	obs, err := t.tfClient.Observe(ctx, t.tfResource)
	if err != nil {
		return Unknown, nil, err
	}

	if !obs.ResourceExists {
		return NotExisting, map[string][]byte{}, nil
	}
	if !obs.ResourceUpToDate {
		return Drift, map[string][]byte{}, nil
	}

	flatDetails, err := flattenSecretData(obs.ConnectionDetails)
	if err != nil {
		return Unknown, nil, err
	}
	return UpToDate, flatDetails, nil
}

// flattenSecretData takes a map[string][]byte and flattens any JSON object values into the result map.
// For each key whose value is a JSON object, its keys/values are added to the result map as top-level entries.
// Non-JSON values are kept as-is.
func flattenSecretData(secretData map[string][]byte) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for k, v := range secretData {
		var jsonMap map[string]any
		if err := json.Unmarshal(v, &jsonMap); err == nil {
			for jk, jv := range jsonMap {
				switch val := jv.(type) {
				case string:
					result[jk] = []byte(val)
				default:
					b, err := json.Marshal(val)
					if err != nil {
						return nil, err
					}
					result[jk] = b
				}
			}
		} else {
			result[k] = v
		}
	}
	return result, nil
}
