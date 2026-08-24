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

Edit `swagger-patched.json` before regenerating if the raw spec needs fixes.
The raw spec is kept as `swagger.json` (do not edit).

### Key generated types

- `DestinationsOnSubaccountLevelAPI` — interface with all subaccount-level destination operations
- `Destination` — property bag struct: `Name string`, `Type string`, `AdditionalProperties map[string]interface{}`
- Key methods:
  - `V1SubaccountDestinationsDestinationNameGet` — read one destination
  - `V1SubaccountDestinationsPost` — create
  - `V1SubaccountDestinationsPut` — update
  - `V1SubaccountDestinationsDestinationNameDelete` — delete
