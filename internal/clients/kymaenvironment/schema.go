package environments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// SchemaCache is the long-lived, client-independent schema cache. It holds only
// parsed schemas keyed by (environmentType, planName); it has no dependency on
// any BTP client, so a single instance can be shared across reconciles (and
// across ProviderConfigs). The BTP client used for a cache-miss fetch is
// supplied per call. Safe for concurrent use.
type SchemaCache struct {
	ttl time.Duration
	now func() time.Time // injectable for tests

	mu    sync.RWMutex
	cache map[string]cachedSchema // key: environmentType + "|" + planName
}

// NewSchemaCache returns an empty schema cache with the default TTL. Share one
// instance across reconciles so cached schemas survive; the cache is
// process-local, so each controller pod maintains its own.
func NewSchemaCache() *SchemaCache {
	return &SchemaCache{
		ttl:   defaultTTL,
		now:   time.Now,
		cache: map[string]cachedSchema{},
	}
}

// schemaFetcher pairs the shared cache with a single reconcile's BTP client. It
// satisfies SchemaFetcher; the client is consulted only on a cache miss, so it
// is never held longer than the reconcile that created it. scope is the
// per-region cache-key discriminator, computed once from the client at
// construction rather than on every GetUpdateSchema call (see clientScope).
type schemaFetcher struct {
	cache *SchemaCache
	btp   btp.Client
	scope string
}

// NewSchemaFetcher returns a SchemaFetcher backed by its own private cache.
// Retained for callers that don't share a cache; prefer
// NewSchemaFetcherWithCache to persist schemas across reconciles.
func NewSchemaFetcher(client btp.Client) SchemaFetcher {
	return &schemaFetcher{cache: NewSchemaCache(), btp: client, scope: clientScope(client)}
}

// NewSchemaFetcherWithCache pairs a shared, long-lived cache with the current
// reconcile's BTP client. A nil cache is tolerated (a private cache is used),
// so the returned fetcher never panics on GetUpdateSchema.
func NewSchemaFetcherWithCache(cache *SchemaCache, client btp.Client) SchemaFetcher {
	if cache == nil {
		cache = NewSchemaCache()
	}
	return &schemaFetcher{cache: cache, btp: client, scope: clientScope(client)}
}

func cacheKey(environmentType, planName string) string {
	return environmentType + "|" + planName
}

// clientScope returns a stable, opaque per-region discriminator for the cache
// key. availableEnvironments is fetched with the CIS credentials from each CR's
// Cloud Management secret; the updateSchema structure is constant across
// subaccounts within a region (a regional-catalog blueprint) but is tied to the
// region. A single provider reconciles CRs across regions, so we scope the
// cache by the provisioning endpoint to avoid serving one region's schema for
// another. The endpoint URL is not secret (it's a public service URL); we hash
// it only to keep the key opaque and fixed-length. Computed once per fetcher,
// so the full digest is used (no truncation). Returns "" when no endpoint is
// available (e.g. in tests), which degrades to a plan-global key.
func clientScope(client btp.Client) string {
	if client.Credential != nil && client.Credential.CISCredential != nil {
		if url := client.Credential.CISCredential.Endpoints.ProvisioningServiceUrl; url != "" {
			sum := sha256.Sum256([]byte(url))
			return hex.EncodeToString(sum[:])
		}
	}
	return ""
}

func (f *schemaFetcher) GetUpdateSchema(ctx context.Context, environmentType, planName string) (*Schema, error) {
	return f.cache.get(ctx, f.btp, f.scope, environmentType, planName)
}

// get returns the cached schema for the given key, fetching via the supplied
// client on a cold or stale entry. scope is the precomputed per-region
// discriminator (see clientScope). See SchemaFetcher.GetUpdateSchema for the
// caching/fail-closed contract.
func (c *SchemaCache) get(ctx context.Context, client btp.Client, scope, environmentType, planName string) (*Schema, error) {
	key := scope + "#" + cacheKey(environmentType, planName)

	// Fast path: fresh cache entry.
	c.mu.RLock()
	entry, hit := c.cache[key]
	c.mu.RUnlock()
	if hit && c.now().Sub(entry.fetchedAt) < c.ttl {
		return entry.schema, nil
	}

	// Cold or stale — refetch. Keep the previous entry available as a
	// fallback if the fetch fails.
	fresh, err := fetch(ctx, client, environmentType, planName)
	if err != nil {
		if hit {
			// Warm cache override: transient BTP failure shouldn't take
			// drift detection offline.
			return entry.schema, nil
		}
		return nil, err
	}

	c.mu.Lock()
	c.cache[key] = cachedSchema{schema: fresh, fetchedAt: c.now()}
	c.mu.Unlock()

	return fresh, nil
}

// fetch pulls the matching availableEnvironments entry and parses its
// updateSchema JSON string into our internal Schema type.
func fetch(ctx context.Context, client btp.Client, environmentType, planName string) (*Schema, error) {
	req := client.ProvisioningServiceClient.GetAvailableEnvironments(ctx)
	resp, _, err := client.ProvisioningServiceClient.GetAvailableEnvironmentsExecute(req)
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
