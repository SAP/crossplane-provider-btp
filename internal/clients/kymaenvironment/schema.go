package environments

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/btp"
)

// Schema is the parsed, minimal representation of a BTP updateSchema for a
// given (environmentType, planName). It captures only the fields the drift
// detector needs: the set of top-level properties, their declared JSON Schema
// defaults, and any nested structure required to recognise "all defaults"
// object values.
//
// Fields that Kyma's schema declares but the drift detector does not need
// (enums, minimum/maximum, patterns, descriptions, controlsOrder, etc.) are
// intentionally dropped. If a future rule needs them, extend this type.
type Schema struct {
	// Properties are the top-level property names of the update contract.
	// A key absent here is outside the contract (e.g. create-only fields
	// like region, networking, modules, colocateControlPlane on Kyma).
	Properties map[string]Property
}

// Property captures the pieces of a JSON Schema property node that the drift
// detector needs.
type Property struct {
	// Type is the JSON Schema `type`. May be empty when the schema omits it
	// (e.g. oneOf nodes); the diff helper then treats the value as opaque
	// and falls back to strict equality.
	Type string

	// Default is the schema-declared default, or nil if none. For objects
	// this may be nil even when the effective default is {}; the diff helper
	// handles that using Properties below.
	Default any

	// Properties holds nested property schemas for `type: object` nodes,
	// used to recognise the "all defaults" state recursively.
	Properties map[string]Property
}

// SchemaFetcher fetches and caches BTP updateSchemas per environment/plan.
//
// The zero value is not usable; construct via NewSchemaFetcher.
type SchemaFetcher interface {
	// GetUpdateSchema returns the parsed updateSchema for the given
	// (environmentType, planName). Results are cached in memory with a 24h
	// TTL. On BTP fetch failure with a warm cache entry, returns the cached
	// copy. On BTP fetch failure with no cache entry, returns an error
	// (fail-closed).
	GetUpdateSchema(ctx context.Context, environmentType, planName string) (*Schema, error)
}

// defaultTTL is the cache expiry for a fetched schema. Kyma product metadata
// is static; 24h balances staleness protection against unnecessary BTP calls.
const defaultTTL = 24 * time.Hour

type cachedSchema struct {
	schema    *Schema
	fetchedAt time.Time
}

type schemaFetcher struct {
	btp btp.Client
	ttl time.Duration
	now func() time.Time // injectable for tests

	mu    sync.RWMutex
	cache map[string]cachedSchema // key: environmentType + "|" + planName
}

// NewSchemaFetcher returns a SchemaFetcher backed by the given BTP client.
// Cache is process-local; each controller pod maintains its own.
func NewSchemaFetcher(client btp.Client) SchemaFetcher {
	return &schemaFetcher{
		btp:   client,
		ttl:   defaultTTL,
		now:   time.Now,
		cache: map[string]cachedSchema{},
	}
}

func cacheKey(environmentType, planName string) string {
	return environmentType + "|" + planName
}

func (f *schemaFetcher) GetUpdateSchema(ctx context.Context, environmentType, planName string) (*Schema, error) {
	key := cacheKey(environmentType, planName)

	// Fast path: fresh cache entry.
	f.mu.RLock()
	entry, hit := f.cache[key]
	f.mu.RUnlock()
	if hit && f.now().Sub(entry.fetchedAt) < f.ttl {
		return entry.schema, nil
	}

	// Cold or stale — refetch. Keep the previous entry available as a
	// fallback if the fetch fails.
	fresh, err := f.fetch(ctx, environmentType, planName)
	if err != nil {
		if hit {
			// Warm cache override: transient BTP failure shouldn't take
			// drift detection offline.
			return entry.schema, nil
		}
		return nil, err
	}

	f.mu.Lock()
	f.cache[key] = cachedSchema{schema: fresh, fetchedAt: f.now()}
	f.mu.Unlock()

	return fresh, nil
}

// fetch pulls the matching availableEnvironments entry and parses its
// updateSchema JSON string into our internal Schema type.
func (f *schemaFetcher) fetch(ctx context.Context, environmentType, planName string) (*Schema, error) {
	req := f.btp.ProvisioningServiceClient.GetAvailableEnvironments(ctx)
	resp, _, err := f.btp.ProvisioningServiceClient.GetAvailableEnvironmentsExecute(req)
	if err != nil {
		return nil, errors.Wrap(err, "fetching availableEnvironments from BTP")
	}
	if resp == nil {
		return nil, errors.New("availableEnvironments response was nil")
	}

	for _, e := range resp.GetAvailableEnvironments() {
		if e.GetEnvironmentType() != environmentType || e.GetPlanName() != planName {
			continue
		}
		if !e.HasUpdateSchema() {
			return nil, errors.Errorf(
				"availableEnvironments returned no updateSchema for %s/%s",
				environmentType, planName,
			)
		}
		return parseSchema(e.GetUpdateSchema())
	}
	return nil, errors.Errorf(
		"no availableEnvironments entry for %s/%s",
		environmentType, planName,
	)
}

// parseSchema converts BTP's raw JSON Schema string into our internal Schema.
// It extracts only the fields the drift detector uses; unknown fields are
// silently ignored.
func parseSchema(raw string) (*Schema, error) {
	// BTP wraps the schema under `parameters` at the top level.
	var envelope struct {
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, errors.Wrap(err, "parsing schema envelope")
	}
	if len(envelope.Parameters) == 0 {
		return &Schema{Properties: map[string]Property{}}, nil
	}
	props, err := parseProperties(envelope.Parameters)
	if err != nil {
		return nil, err
	}
	return &Schema{Properties: props}, nil
}

// parseProperties reads a JSON Schema `{"type": "object", "properties": {...}}`
// node and returns the parsed properties map. Non-object schema roots yield
// an empty map.
func parseProperties(raw json.RawMessage) (map[string]Property, error) {
	var node struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, errors.Wrap(err, "parsing schema properties")
	}
	out := map[string]Property{}
	for name, propRaw := range node.Properties {
		prop, err := parseProperty(propRaw)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing property %q", name)
		}
		out[name] = prop
	}
	return out, nil
}

// parseProperty parses a single JSON Schema property node into our Property
// type. Recurses into nested `properties` for object-typed nodes.
func parseProperty(raw json.RawMessage) (Property, error) {
	var node struct {
		Type       string          `json:"type"`
		Default    json.RawMessage `json:"default"`
		Properties json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		return Property{}, err
	}
	p := Property{Type: node.Type}

	if len(node.Default) > 0 {
		var d any
		if err := json.Unmarshal(node.Default, &d); err != nil {
			return Property{}, errors.Wrap(err, "parsing default")
		}
		p.Default = d
	}

	if node.Type == "object" && len(node.Properties) > 0 {
		// Reuse parseProperties by wrapping the inner properties map in a
		// synthetic object envelope so shapes align.
		nested, err := parseProperties(
			json.RawMessage(`{"type":"object","properties":` + string(node.Properties) + `}`),
		)
		if err != nil {
			return Property{}, err
		}
		p.Properties = nested
	}

	return p, nil
}
