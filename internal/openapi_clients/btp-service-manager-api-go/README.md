## Btp Service Manager Service API Go

### Prerequisites
Install openapi-generator-cli (using mac)
```bash
brew install openapi-generator
```
for other OS see [official guide](https://openapi-generator.tech/docs/installation)

### How to regenerate
```bash
openapi-generator generate -i swagger-patched.json -g go -o pkg/ --additional-properties=generateInterfaces=true
go mod tidy -v
```

### Apply patches
Sometimes api specs need to be patched prior to generating code out of them.
For that the widely accepted json-patch standard can be used. To not introduce any more dependencies into the project
we do not include an opinionated json-patch library but rather leave it up to the contributor to choose one.
You can find a list here: https://jsonpatch.com

> **Important: keep `swagger-patched.json` key-sorted and UTF-8.**
> `swagger-patched.json` is committed with all JSON object keys sorted alphabetically (recursively)
> and non-ASCII characters left as raw UTF-8. openapi-generator emits Go struct fields in the order
> the properties appear in the spec, so if you re-serialize the patched spec in a different key order
> the regenerated client churns across *every* model (field reordering) and the real change is buried.
> When applying the patch, serialize with sorted keys and without ASCII-escaping. For example, in Python:
> ```python
> json.dump(spec, f, indent=2, sort_keys=True, ensure_ascii=False)
> ```
> After regenerating, `git diff pkg/` should contain only your intended change.
