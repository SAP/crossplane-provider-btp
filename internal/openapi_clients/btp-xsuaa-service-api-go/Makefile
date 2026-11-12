.DEFAULT_GOAL := ci
#.SILENT:

.PHONY: help
help: ## Show this help screen
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

ci: apply generate

.PHONY: generate
generate:
	openapi-generator generate -i swagger.json -g go -o pkg/ --additional-properties=generateInterfaces=true,structPrefix=true,disallowAdditionalPropertiesIfNotPresent=false,enumUnknownDefaultCase=true --skip-validate-spec
	go mod tidy -v

#.PHONY: download
#download:
#	@echo "Downloading Spec from $(SPEC_URL)"
#	@curl -sL $(SPEC_URL)| jq -S '.' > swagger.json