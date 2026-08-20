package servicemanager

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	smclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

// ParameterClient reads and writes a service instance's parameters directly via
// the Service Manager, used for parameter-drift detection and repair (separate
// from the recovery lookups in SemanticLookuper).
type ParameterClient interface {
	// GetInstanceParameters returns the parameters currently in effect for the
	// instance. found is false when the offering returns no parameters; callers
	// treat !found as "no drift signal" and skip the comparison.
	GetInstanceParameters(ctx context.Context, serviceInstanceID string) (params map[string]any, found bool, err error)

	// UpdateInstanceParameters PATCHes the instance parameters synchronously,
	// bypassing terraform apply (a no-op for parameters-only changes).
	UpdateInstanceParameters(ctx context.Context, serviceInstanceID string, desiredParamsJSON []byte) error
}

var _ ParameterClient = &ServiceManagerClient{}

// GetInstanceParameters implements ParameterClient.
func (sm *ServiceManagerClient) GetInstanceParameters(ctx context.Context, serviceInstanceID string) (map[string]any, bool, error) {
	// The generated Execute() decodes into map[string]string and fails on
	// non-string values. On that error it returns a GenericOpenAPIError whose
	// Body() still holds the raw 200 payload, which we decode ourselves.
	_, resp, err := sm.ServiceInstancesAPI.
		GetServiceInstanceParameters(ctx, serviceInstanceID).Execute()
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	var body []byte
	if err != nil {
		// 404/400 (e.g. instances_retrievable=false) means no drift signal.
		if resp != nil && (resp.StatusCode == 404 || resp.StatusCode == 400) {
			return nil, false, nil
		}
		// A 200 decode error carries the raw body; recover it.
		var apiErr *smclient.GenericOpenAPIError
		if resp != nil && resp.StatusCode == 200 && errors.As(err, &apiErr) && len(apiErr.Body()) > 0 {
			body = apiErr.Body()
		} else {
			return nil, false, specifyAPIError(err)
		}
	} else {
		if resp == nil {
			return nil, false, nil
		}
		var rerr error
		body, rerr = io.ReadAll(resp.Body)
		if rerr != nil {
			return nil, false, rerr
		}
	}

	body = bytes.TrimSpace(body)
	if len(body) == 0 || string(body) == "null" || string(body) == "{}" {
		return nil, false, nil
	}

	params := map[string]any{}
	if jerr := json.Unmarshal(body, &params); jerr != nil {
		// A non-object body is treated as "no parameters", not an error.
		return nil, false, nil
	}
	if len(params) == 0 {
		return nil, false, nil
	}
	return params, true, nil
}

// UpdateInstanceParameters implements ParameterClient. Uses a raw HTTP request
// because the generated client models parameters as map[string]string and
// cannot carry nested objects.
func (sm *ServiceManagerClient) UpdateInstanceParameters(ctx context.Context, serviceInstanceID string, desiredParamsJSON []byte) error {
	if sm.httpClient == nil || sm.smBaseURL == nil {
		return errors.New("service manager client not configured for raw updates")
	}
	if len(bytes.TrimSpace(desiredParamsJSON)) == 0 {
		return errors.New("empty desired parameters")
	}

	payload := struct {
		Parameters json.RawMessage `json:"parameters"`
	}{Parameters: json.RawMessage(desiredParamsJSON)}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "cannot marshal update payload")
	}

	u := *sm.smBaseURL
	u.Path = "/v1/service_instances/" + serviceInstanceID
	q := u.Query()
	q.Set("async", "false")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "cannot build update request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "service manager update request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return errors.Errorf("service manager update returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}
