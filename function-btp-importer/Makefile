# function-btp-importer Makefile
EXAMPLE ?= auto
# Crossplane engine the CLI runs for `crossplane render` (crossplane CLI >= 2.3
# renders through a real Crossplane image, not in-process). Tracks the control
# plane's Crossplane version so renders reflect what dev actually reconciles;
# the engine only exists for core >= 2.3, so while the control plane runs an
# older release this is the newest 2.3.x.
CROSSPLANE_VERSION ?= v2.3.5

EXAMPLE_DIR  := example/$(EXAMPLE)
OUTPUT_DIR   := $(EXAMPLE_DIR)/output
OBSERVED_FILE := $(EXAMPLE_DIR)/observed.yaml
REQUIRED_FILE := $(EXAMPLE_DIR)/required.yaml

# Pass observed resources when observed.yaml exists, otherwise use /dev/null so
# --observed-resources always works (same pattern as render.sh).
ifneq ($(wildcard $(OBSERVED_FILE)),)
OBSERVED_FLAG := --observed-resources=$(OBSERVED_FILE)
else
OBSERVED_FLAG := --observed-resources=/dev/null
endif

# Pass required resources (e.g. the CIS secret) when required.yaml exists.
# This lets crossplane render resolve the two-pass flow locally without a cluster.
ifneq ($(wildcard $(REQUIRED_FILE)),)
REQUIRED_FLAG := --required-resources=$(REQUIRED_FILE)
else
REQUIRED_FLAG :=
endif

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'


# Registry to push the function package to, e.g. make image REGISTRY=ghcr.io/my-org
REGISTRY ?=
IMAGE    := $(REGISTRY)/function-btp-importer:v0.0.0-$(shell git rev-parse --short HEAD)

.PHONY: image
image: ## Build and push the function package to REGISTRY (linux/amd64 + linux/arm64)
	@if [ -z "$(REGISTRY)" ]; then echo "Error: set REGISTRY, e.g. make image REGISTRY=ghcr.io/my-org"; exit 1; fi
	docker buildx build --platform=linux/amd64 --output="type=docker,dest=runtime-amd64.tar" .
	docker buildx build --platform=linux/arm64 --output="type=docker,dest=runtime-arm64.tar" .
	crossplane xpkg build \
		--package-root=./package \
		--embed-runtime-image-tarball=runtime-amd64.tar \
		--package-file=amd64.xpkg
	crossplane xpkg build \
		--package-root=./package \
		--embed-runtime-image-tarball=runtime-arm64.tar \
		--package-file=arm64.xpkg
	crossplane xpkg push --package-files=amd64.xpkg,arm64.xpkg "$(IMAGE)"
	@echo ""
	@echo "Pushed: $(IMAGE)"
	@rm -f runtime-amd64.tar runtime-arm64.tar amd64.xpkg arm64.xpkg

generate: ## Run go generate
	go generate ./...

.PHONY: build
build: ## Build the function
	go build ./...

.PHONY: test
test: ## Run unit tests
	go test ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run --config .golangci.yml ./...

.PHONY: render
render: ## Render an example (EXAMPLE=auto|explicit|filters). Requires the function running: go run . --insecure --debug
	@if ! nc -z localhost 9443 2>/dev/null; then \
		echo "Error: function is not running on localhost:9443"; \
		echo "Start it first with: go run . --insecure --debug"; \
		exit 1; \
	fi
	@echo "Rendering example: $(EXAMPLE)"
	@mkdir -p $(OUTPUT_DIR)
	crossplane render \
		$(EXAMPLE_DIR)/xr.yaml \
		$(EXAMPLE_DIR)/composition.yaml \
		$(EXAMPLE_DIR)/functions.yaml \
		--crossplane-version=$(CROSSPLANE_VERSION) \
		$(OBSERVED_FLAG) \
		$(REQUIRED_FLAG) \
		--include-full-xr \
		| tee $(OUTPUT_DIR)/rendered.yaml
	@echo ""
	@echo "Output written to $(OUTPUT_DIR)/rendered.yaml"
