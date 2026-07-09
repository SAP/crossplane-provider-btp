package environments

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sap/crossplane-provider-btp/btp"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

// kymaAzureUpdateSchema is a trimmed but faithful copy of the real Kyma
// (azure plan) updateSchema returned by
// GET /provisioning/v1/availableEnvironments. Sourced from issue #682's
// linked comment; only the fields the drift detector cares about are kept
// here, plus one non-relevant field (autoScalerMax) to prove we handle
// integer defaults too.
const kymaAzureUpdateSchema = `{
  "parameters": {
    "type": "object",
    "properties": {
      "ingressFiltering": {
        "type": "boolean",
        "default": false
      },
      "gvisor": {
        "type": "object",
        "properties": {
          "enabled": {
            "type": "boolean",
            "default": false
          }
        }
      },
      "accessControlList": {
        "type": "object",
        "properties": {
          "allowedCIDRs": {
            "type": "array"
          }
        }
      },
      "autoScalerMax": {
        "type": "integer",
        "default": 20
      }
    }
  }
}`

// fakeEnvironmentsAPI implements the minimum surface of the OpenAPI-generated
// EnvironmentsAPI interface that the schema fetcher touches. Only two methods
// matter here — everything else panics if reached, which surfaces accidental
// widening.
type fakeEnvironmentsAPI struct {
	provisioningclient.EnvironmentsAPI
	calls    int
	response *provisioningclient.AvailableEnvironmentResponseCollection
	err      error
}

func (f *fakeEnvironmentsAPI) GetAvailableEnvironments(ctx context.Context) provisioningclient.ApiGetAvailableEnvironmentsRequest {
	return provisioningclient.ApiGetAvailableEnvironmentsRequest{} // opaque; not consulted by Execute
}

func (f *fakeEnvironmentsAPI) GetAvailableEnvironmentsExecute(_ provisioningclient.ApiGetAvailableEnvironmentsRequest) (*provisioningclient.AvailableEnvironmentResponseCollection, *http.Response, error) {
	f.calls++
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.response, &http.Response{StatusCode: 200}, nil
}

func fakeAvailableEnvironments(entries ...provisioningclient.AvailableEnvironmentResponseObject) *provisioningclient.AvailableEnvironmentResponseCollection {
	c := &provisioningclient.AvailableEnvironmentResponseCollection{}
	c.SetAvailableEnvironments(entries)
	return c
}

func kymaAzureEntry(schema string) provisioningclient.AvailableEnvironmentResponseObject {
	e := provisioningclient.AvailableEnvironmentResponseObject{}
	envType := "kyma"
	plan := "azure"
	e.EnvironmentType = &envType
	e.PlanName = &plan
	e.UpdateSchema = &schema
	return e
}

func newFetcherWithFake(t *testing.T, api *fakeEnvironmentsAPI) *schemaFetcher {
	t.Helper()
	return &schemaFetcher{
		btp:   btp.Client{ProvisioningServiceClient: api},
		ttl:   defaultTTL,
		now:   time.Now,
		cache: map[string]cachedSchema{},
	}
}

func TestParseSchema_ExtractsKymaAzureShape(t *testing.T) {
	got, err := parseSchema(kymaAzureUpdateSchema)
	if err != nil {
		t.Fatalf("parseSchema returned unexpected error: %v", err)
	}

	// ingressFiltering: boolean, default false
	if p, ok := got.Properties["ingressFiltering"]; !ok {
		t.Errorf("missing property ingressFiltering")
	} else {
		if p.Type != "boolean" {
			t.Errorf("ingressFiltering.Type = %q, want boolean", p.Type)
		}
		if p.Default != false {
			t.Errorf("ingressFiltering.Default = %v, want false", p.Default)
		}
	}

	// gvisor: object, no top-level default, nested enabled=boolean/default=false
	if p, ok := got.Properties["gvisor"]; !ok {
		t.Errorf("missing property gvisor")
	} else {
		if p.Type != "object" {
			t.Errorf("gvisor.Type = %q, want object", p.Type)
		}
		if p.Default != nil {
			t.Errorf("gvisor.Default = %v, want nil", p.Default)
		}
		if inner, ok := p.Properties["enabled"]; !ok {
			t.Errorf("missing nested property gvisor.enabled")
		} else if inner.Default != false {
			t.Errorf("gvisor.enabled.Default = %v, want false", inner.Default)
		}
	}

	// accessControlList: object, no defaults anywhere in this fixture
	if p, ok := got.Properties["accessControlList"]; !ok {
		t.Errorf("missing property accessControlList")
	} else {
		if p.Type != "object" {
			t.Errorf("accessControlList.Type = %q, want object", p.Type)
		}
		if p.Default != nil {
			t.Errorf("accessControlList.Default = %v, want nil", p.Default)
		}
	}

	// autoScalerMax: integer, default 20 (JSON unmarshals numbers as float64)
	if p, ok := got.Properties["autoScalerMax"]; !ok {
		t.Errorf("missing property autoScalerMax")
	} else {
		if p.Type != "integer" {
			t.Errorf("autoScalerMax.Type = %q, want integer", p.Type)
		}
		if p.Default != float64(20) {
			t.Errorf("autoScalerMax.Default = %v, want 20", p.Default)
		}
	}
}

func TestSchemaFetcher_CacheHit(t *testing.T) {
	fake := &fakeEnvironmentsAPI{
		response: fakeAvailableEnvironments(kymaAzureEntry(kymaAzureUpdateSchema)),
	}
	f := newFetcherWithFake(t, fake)

	for i := 0; i < 3; i++ {
		if _, err := f.GetUpdateSchema(context.Background(), "kyma", "azure"); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
	if fake.calls != 1 {
		t.Errorf("fake.calls = %d, want 1 (cache should absorb repeat calls)", fake.calls)
	}
}

func TestSchemaFetcher_CacheMissOnDifferentKey(t *testing.T) {
	fake := &fakeEnvironmentsAPI{
		response: fakeAvailableEnvironments(
			kymaAzureEntry(kymaAzureUpdateSchema),
			func() provisioningclient.AvailableEnvironmentResponseObject {
				e := kymaAzureEntry(kymaAzureUpdateSchema)
				plan := "aws"
				e.PlanName = &plan
				return e
			}(),
		),
	}
	f := newFetcherWithFake(t, fake)

	if _, err := f.GetUpdateSchema(context.Background(), "kyma", "azure"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := f.GetUpdateSchema(context.Background(), "kyma", "aws"); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("fake.calls = %d, want 2 (different key must refetch)", fake.calls)
	}
}

func TestSchemaFetcher_FetchFailure_WarmCacheOverrides(t *testing.T) {
	fake := &fakeEnvironmentsAPI{
		response: fakeAvailableEnvironments(kymaAzureEntry(kymaAzureUpdateSchema)),
	}
	f := newFetcherWithFake(t, fake)
	// Shrink the TTL so the second call is a stale-cache path.
	f.ttl = time.Nanosecond

	if _, err := f.GetUpdateSchema(context.Background(), "kyma", "azure"); err != nil {
		t.Fatalf("prime call: %v", err)
	}

	// Force the next fetch to fail.
	fake.err = errors.New("btp is on fire")
	// Advance time past the TTL.
	f.now = func() time.Time { return time.Now().Add(time.Hour) }

	got, err := f.GetUpdateSchema(context.Background(), "kyma", "azure")
	if err != nil {
		t.Fatalf("stale-cache fallback returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("stale-cache fallback returned nil schema")
	}
	if _, ok := got.Properties["ingressFiltering"]; !ok {
		t.Errorf("stale-cache result missing ingressFiltering")
	}
}

func TestSchemaFetcher_FetchFailure_ColdCacheReturnsError(t *testing.T) {
	fake := &fakeEnvironmentsAPI{err: errors.New("btp unreachable")}
	f := newFetcherWithFake(t, fake)

	_, err := f.GetUpdateSchema(context.Background(), "kyma", "azure")
	if err == nil {
		t.Fatal("cold-cache fetch failure did not return error")
	}
}

func TestSchemaFetcher_NoMatchingEntry(t *testing.T) {
	fake := &fakeEnvironmentsAPI{
		response: fakeAvailableEnvironments(kymaAzureEntry(kymaAzureUpdateSchema)),
	}
	f := newFetcherWithFake(t, fake)

	_, err := f.GetUpdateSchema(context.Background(), "kyma", "gcp") // plan not in fixture
	if err == nil {
		t.Fatal("expected error for unknown plan, got nil")
	}
}

// Compilation sanity: verify our internal Property type matches what the
// diff helper (commit 2) will consume. If this test needs to change to keep
// building, the diff helper's contract with Schema is changing — call it out
// in review.
func TestSchema_InternalContract(t *testing.T) {
	s := &Schema{Properties: map[string]Property{
		"a": {Type: "boolean", Default: false},
	}}
	if diff := cmp.Diff(false, s.Properties["a"].Default); diff != "" {
		t.Errorf("unexpected Property representation: %s", diff)
	}
}
