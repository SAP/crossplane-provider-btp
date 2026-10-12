package serviceinstanceclient

import (
	"encoding/json"
	"testing"

	smopenapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

// Real-shaped Service Manager service_plan (trimmed from a live GET
// /v1/service_plans on the dev control plane: objectstore/standard). It carries
// the live-only fields SAP's published OpenAPI omits — schemas, plan_updateable,
// maximum_polling_duration — and exercises the create-vs-update distinction
// (create has preventDeletion; update adds versioning).
const livePlanWithSchemas = `{
  "id": "plan-1",
  "name": "standard",
  "catalog_name": "standard",
  "ready": true,
  "bindable": true,
  "plan_updateable": true,
  "maximum_polling_duration": 10800,
  "service_offering_id": "off-1",
  "schemas": {
    "service_instance": {
      "create": {
        "parameters": {
          "$schema": "http://json-schema.org/draft-04/schema#",
          "type": "object",
          "properties": { "preventDeletion": { "type": "boolean" } }
        }
      },
      "update": {
        "parameters": {
          "$schema": "http://json-schema.org/draft-04/schema#",
          "type": "object",
          "properties": {
            "versioning": { "type": "boolean" },
            "preventDeletion": { "type": "boolean" }
          }
        }
      }
    }
  }
}`

// Proves the patched/regenerated ServicePlanResponseObject binds the live JSON
// shape into the typed schema structs, and the live-only scalar fields decode.
func TestServicePlanSchemasDeserialize(t *testing.T) {
	var plan smopenapi.ServicePlanResponseObject
	if err := json.Unmarshal([]byte(livePlanWithSchemas), &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	if !plan.HasPlanUpdateable() || !plan.GetPlanUpdateable() {
		t.Error("plan_updateable did not decode to true")
	}
	if got := plan.GetMaximumPollingDuration(); got != 10800 {
		t.Errorf("maximum_polling_duration = %d, want 10800", got)
	}

	if !plan.HasSchemas() {
		t.Fatal("schemas did not decode")
	}
	schemas := plan.GetSchemas()
	si := schemas.GetServiceInstance()

	create := si.GetCreate()
	createProps := props(t, &create)
	if _, ok := createProps["preventDeletion"]; !ok {
		t.Errorf("create schema missing preventDeletion; got keys %v", keys(createProps))
	}
	if _, ok := createProps["versioning"]; ok {
		t.Error("create schema must NOT expose versioning (update-only key)")
	}

	update := si.GetUpdate()
	updateProps := props(t, &update)
	if _, ok := updateProps["versioning"]; !ok {
		t.Errorf("update schema missing versioning; got keys %v", keys(updateProps))
	}
	if _, ok := updateProps["preventDeletion"]; !ok {
		t.Error("update schema missing preventDeletion")
	}
}

// props extracts the JSON Schema "properties" object from a SchemaParameters leaf.
func props(t *testing.T, sp *smopenapi.SchemaParameters) map[string]interface{} {
	t.Helper()
	p := sp.GetParameters() // map[string]interface{} = the JSON Schema document
	pr, ok := p["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters.properties not an object: %T", p["properties"])
	}
	return pr
}

func keys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
