## Btp Provisioning Service API Go

### Prerequisites
Install openapi-generator-cli (using mac)
```bash
brew install openapi-generator
```
for other OS see [official guide](https://openapi-generator.tech/docs/installation)

### How to regenerate
```bash
openapi-generator generate -i swagger-patched.json -g go -o pkg/
go fmt ./...
go mod tidy -v
```

### Apply patches
Sometimes api specs need to be patched prior to generating code out of them.
For that the widely accepted json-patch standard can be used. To not introduce any more dependencies into the project
we do not include an opinionated json-patch library but rather leave it up to the contributor to choose one.
You can find a list here: https://jsonpatch.com
