## BTP Destination Service API Go

### Prerequisites

Install openapi-generator-cli (Mac):
```bash
brew install openapi-generator
```

### How to regenerate

```bash
openapi-generator generate \
  -i swagger-patched.json \
  -g go \
  -o pkg/ \
  --additional-properties=generateInterfaces=true,disallowAdditionalPropertiesIfNotPresent=false,packageName=openapi

# Remove generated files we don't need
rm -rf pkg/test pkg/go.mod pkg/go.sum pkg/git_push.sh pkg/.travis.yml

go mod tidy
```

### Apply patches

Sometimes api specs need to be patched prior to generating code out of them.
For that the widely accepted json-patch standard can be used. To not introduce any more dependencies into the project
we do not include an opinionated json-patch library but rather leave it up to the contributor to choose one.
You can find a list here: https://jsonpatch.com

The patches applied to `swagger.json` to produce `swagger-patched.json` are recorded in `swagger-patch.json`.
To re-apply them before regenerating:

```bash
# Example using the npx json-patch-cli tool (one of many options from jsonpatch.com):
npx json-patch-cli swagger.json swagger-patch.json > swagger-patched.json
```

Current patches:
- **If-Match header on PUT `/v1/subaccountDestinations`** — adds optional `If-Match` header for ETag-based optimistic concurrency control on destination updates. The original SAP spec omits this parameter despite the API supporting it.

### Key generated types

- `DestinationsOnSubaccountLevelAPI` — interface with all subaccount-level destination operations
- `Destination` — property bag struct: `Name string`, `Type string`, `AdditionalProperties map[string]interface{}`
- Key methods:
  - `V1SubaccountDestinationsDestinationNameGet` — read one destination
  - `V1SubaccountDestinationsPost` — create
  - `V1SubaccountDestinationsPut` — update
  - `V1SubaccountDestinationsDestinationNameDelete` — delete
