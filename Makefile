# ====================================================================================
# Setup Project
PROJECT_NAME := crossplane-provider-btp
PROJECT_REPO := github.com/sap/$(PROJECT_NAME)

# Terraform Related variables
export TERRAFORM_VERSION ?= 1.3.9

export TERRAFORM_PROVIDER_SOURCE ?= SAP/btp
export TERRAFORM_PROVIDER_REPO ?= https://github.com/SAP/terraform-provider-btp
export TERRAFORM_PROVIDER_VERSION ?= 1.18.0
export TERRAFORM_PROVIDER_DOWNLOAD_NAME ?= terraform-provider-btp
export TERRAFORM_PROVIDER_DOWNLOAD_URL_PREFIX ?= https://releases.hashicorp.com/$(TERRAFORM_PROVIDER_DOWNLOAD_NAME)/$(TERRAFORM_PROVIDER_VERSION)
export TERRAFORM_NATIVE_PROVIDER_BINARY ?= terraform-provider-btp_v1.18.0_x5
export TERRAFORM_DOCS_PATH ?= docs/resources

# set BUILD_ID if its not running in an action
BUILD_ID ?= $(shell date +"%H%M%S")

export TEST_CRS_PATH ?= test/e2e/testdata/crs

PLATFORMS ?= linux_amd64
#get version from current git release tag
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "v0.0.0-$$(git rev-parse HEAD)")
$(info VERSION is $(VERSION))

-include build/makelib/common.mk

# Setup Output
-include build/makelib/output.mk

# Setup Versions
GO_REQUIRED_VERSION=1.25
GOLANGCILINT_VERSION ?= 2.12.2

NPROCS ?= 1
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))
GO_STATIC_PACKAGES = $(GO_PROJECT)/cmd/provider $(GO_PROJECT)/cmd/exporter
GO_LDFLAGS += -X $(GO_PROJECT)/internal/version.Version=$(VERSION)
# this version will eventually be passed to the terraform provider
GO_LDFLAGS += -X $(GO_PROJECT)/internal/version.ProviderVersion=$(VERSION)

GO_SUBDIRS += cmd internal apis
GO111MODULE = on
GOTOOLCHAIN = local
-include build/makelib/golang.mk

# Override the GO_LINT_ARGS from golang.mk to use updated golangci-lint parameters
# this can potentially be removed when we update to a newer version of the build
GO_LINT_ARGS = --output.checkstyle.path=$(GO_LINT_OUTPUT)/checkstyle.xml --output.text.path=stdout

# kind-related versions
KIND_VERSION ?= v0.23.0
KIND_NODE_IMAGE_TAG ?= v1.30.2

# Setup Kubernetes tools
-include build/makelib/k8s_tools.mk

# Setup Images
IMAGES = provider-btp
-include build/makelib/imagelight.mk

export UUT_CONFIG = $(BUILD_REGISTRY)/provider-btp-$(ARCH):latest
export UUT_XPKG = $(BUILD_REGISTRY)/provider-btp-xpkg:latest
export UUT_IMAGES = {"crossplane/provider-btp":"$(UUT_XPKG)"}
testFilter ?= .*

.PHONY: local-build
local-build: build xpkg.build.provider-btp
	$(INFO) "Loading xpkg into docker as $(UUT_XPKG)"; \
	XPKG_FILE=$(XPKG_OUTPUT_DIR)/$(PLATFORM)/provider-btp-$(VERSION).xpkg && \
	XPKG_SHA=$$(docker load -i $$XPKG_FILE | sed -n 's/.*ID: //p') && \
	docker tag $$XPKG_SHA $(UUT_XPKG); \
	$(OK) "Built local images: $(UUT_CONFIG) $(UUT_XPKG)"; \

# ====================================================================================
# Setup XPKG

XPKGS ?= provider-btp
XPKG_REG_ORGS ?= ghcr.io/sap/crossplane-provider-btp/crossplane
-include build/makelib/xpkg.mk

# NOTE(hasheddan): we force image building to happen prior to xpkg build so that
# we ensure image is present in daemon.
xpkg.build.provider-btp: do.build.images

# NOTE(hasheddan): we ensure up is installed prior to running platform-specific
# build steps in parallel to avoid encountering an installation race condition.
build.init: $(UP)

# ====================================================================================
# Fallthrough

# run `make help` to see the targets and options

# We want submodules to be set up the first time `make` is run.
# We manage the build/ folder and its Makefiles as a submodule.
# The first time `make` is run, the includes of build/*.mk files will
# all fail, and this target will be run. The next time, the default as defined
# by the includes will be run instead.
fallthrough: submodules
	@echo Initial setup complete. Running make again . . .
	@make

# ====================================================================================
# Setup Terraform for fetching provider schema
TERRAFORM := $(TOOLS_HOST_DIR)/terraform-$(TERRAFORM_VERSION)
TERRAFORM_WORKDIR := $(WORK_DIR)/terraform
TERRAFORM_PROVIDER_SCHEMA := config/schema.json
terraform.buildvars: common.buildvars
	@echo TERRAFORM_VERSION=$(TERRAFORM_VERSION)
	@echo TERRAFORM_PROVIDER_SOURCE=$(TERRAFORM_PROVIDER_SOURCE)
	@echo TERRAFORM_PROVIDER_REPO=$(TERRAFORM_PROVIDER_REPO)
	@echo TERRAFORM_PROVIDER_VERSION=$(TERRAFORM_PROVIDER_VERSION)
	@echo TERRAFORM_PROVIDER_DOWNLOAD_NAME=$(TERRAFORM_PROVIDER_DOWNLOAD_NAME)
	@echo TERRAFORM_NATIVE_PROVIDER_BINARY=$(TERRAFORM_NATIVE_PROVIDER_BINARY)
	@echo TERRAFORM_DOCS_PATH=$(TERRAFORM_DOCS_PATH)
	@echo TERRAFORM=$(TERRAFORM)
	@echo TERRAFORM_WORKDIR=$(TERRAFORM_WORKDIR)
	@echo TERRAFORM_PROVIDER_SCHEMA=$(TERRAFORM_PROVIDER_SCHEMA)

$(TERRAFORM_PROVIDER_SCHEMA): $(TERRAFORM)
	@$(INFO) generating provider schema for $(TERRAFORM_PROVIDER_SOURCE) $(TERRAFORM_PROVIDER_VERSION)
	@mkdir -p $(TERRAFORM_WORKDIR)
	@echo '{"terraform":[{"required_providers":[{"provider":{"source":"'"$(TERRAFORM_PROVIDER_SOURCE)"'","version":"'"$(TERRAFORM_PROVIDER_VERSION)"'"}}],"required_version":"'"$(TERRAFORM_VERSION)"'"}]}' > $(TERRAFORM_WORKDIR)/main.tf.json
	@echo $(TERRAFORM_PROVIDER_VERSION)
	@$(TERRAFORM) -chdir=$(TERRAFORM_WORKDIR) init > $(TERRAFORM_WORKDIR)/terraform-logs.txt 2>&1
	@echo $(TERRAFORM_WORKDIR)
	@$(TERRAFORM) -chdir=$(TERRAFORM_WORKDIR) providers schema -json=true > $(TERRAFORM_PROVIDER_SCHEMA) 2>> $(TERRAFORM_WORKDIR)/terraform-logs.txt
	@echo $(TERRAFORM)
	@echo $(TERRAFORM_PROVIDER_SOURCE)
	@$(OK) generating provider schema for $(TERRAFORM_PROVIDER_SOURCE) $(TERRAFORM_PROVIDER_VERSION)

$(TERRAFORM):
	@$(INFO) installing terraform $(HOSTOS)-$(HOSTARCH)
	@mkdir -p $(TOOLS_HOST_DIR)/tmp-terraform
	@curl -fsSL https://releases.hashicorp.com/terraform/$(TERRAFORM_VERSION)/terraform_$(TERRAFORM_VERSION)_$(SAFEHOST_PLATFORM).zip -o $(TOOLS_HOST_DIR)/tmp-terraform/terraform.zip
	@unzip $(TOOLS_HOST_DIR)/tmp-terraform/terraform.zip -d $(TOOLS_HOST_DIR)/tmp-terraform
	@mv $(TOOLS_HOST_DIR)/tmp-terraform/terraform $(TERRAFORM)
	@rm -fr $(TOOLS_HOST_DIR)/tmp-terraform
	@$(OK) installing terraform $(HOSTOS)-$(HOSTARCH)
pull-docs:
	@$(INFO) pull-docs called
	@if [ ! -d "$(WORK_DIR)/$(TERRAFORM_PROVIDER_SOURCE)" ]; then \
  		mkdir -p "$(WORK_DIR)/$(TERRAFORM_PROVIDER_SOURCE)" && \
		git clone -c advice.detachedHead=false --depth 1 --filter=blob:none --branch "v$(TERRAFORM_PROVIDER_VERSION)" --sparse "$(TERRAFORM_PROVIDER_REPO)" "$(WORK_DIR)/$(TERRAFORM_PROVIDER_SOURCE)"; \
	fi
	@git -C "$(WORK_DIR)/$(TERRAFORM_PROVIDER_SOURCE)" sparse-checkout set "$(TERRAFORM_DOCS_PATH)"

# cleans the .work directory used for upjet/tf provider. Necessary after tf provider version upgrades so we just do this every time to have a clean dir
.PHONY: clean-work
clean-work:
	@$(INFO) cleaning work directory
	@rm -rf .work

generate.init: clean-work $(TERRAFORM_PROVIDER_SCHEMA) pull-docs

.PHONY: $(TERRAFORM_PROVIDER_SCHEMA) pull-docs terraform.buildvars

# Update the submodules, such as the common build scripts.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

# NOTE(hasheddan): the build submodule currently overrides XDG_CACHE_HOME in
# order to force the Helm 3 to use the .work/helm directory. This causes Go on
# Linux machines to use that directory as the build cache as well. We should
# adjust this behavior in the build submodule because it is also causing Linux
# users to duplicate their build cache, but for now we just make it easier to
# identify its location in CI so that we cache between builds.
go.cachedir:
	@go env GOCACHE

# This is for running out-of-cluster locally, and is for convenience. Running
# this make target will print out the command which was used. For more control,
# try running the binary directly with different arguments.
run: go.build
	@$(INFO) Running Crossplane locally out-of-cluster . . .
	@# To see other arguments that can be provided, run the command with --help instead
	$(GO_OUT_DIR)/provider --debug


debug: go.build
	dlv debug ./cmd/provider --headless --listen=:2345 --log --api-version=2 --accept-multiclient -- --debug

dev-debug: $(KIND) $(KUBECTL)
	@$(INFO) Creating kind cluster
	@$(KIND) create cluster --name=$(PROJECT_NAME)-dev
	@$(KUBECTL) cluster-info --context kind-$(PROJECT_NAME)-dev
	@$(INFO) Installing Provider Template CRDs
	@$(KUBECTL) apply -R -f package/crds
	@$(INFO) Creating crossplane-system namespace
	@$(KUBECTL) create ns crossplane-system
	@$(INFO) Creating provider config and secret
	@$(KUBECTL) apply -R -f examples/provider
	@$(INFO) Now you can debug the provider with the IDE...

dev: $(KIND) $(KUBECTL)
	@$(INFO) Creating kind cluster
	@$(KIND) create cluster --name=$(PROJECT_NAME)-dev
	@$(KUBECTL) cluster-info --context kind-$(PROJECT_NAME)-dev
	@$(INFO) Installing Provider Template CRDs
	@$(KUBECTL) apply -R -f package/crds
	@$(INFO) Starting Provider Template controllers
	@$(GO) run cmd/provider/main.go --debug

dev-clean: $(KIND) $(KUBECTL)
	@$(INFO) Deleting kind cluster
	@$(KIND) delete cluster --name=$(PROJECT_NAME)-dev

.PHONY: submodules fallthrough test-integration run dev dev-clean e2e.run-final

# ====================================================================================
# Special Targets

# Install gomplate
GOMPLATE_VERSION := 3.10.0
GOMPLATE := $(TOOLS_HOST_DIR)/gomplate-$(GOMPLATE_VERSION)

$(GOMPLATE):
	@$(INFO) installing gomplate $(SAFEHOSTPLATFORM)
	@mkdir -p $(TOOLS_HOST_DIR)
	@curl -fsSLo $(GOMPLATE) https://github.com/hairyhenderson/gomplate/releases/download/v$(GOMPLATE_VERSION)/gomplate_$(SAFEHOSTPLATFORM) || $(FAIL)
	@chmod +x $(GOMPLATE)
	@$(OK) installing gomplate $(SAFEHOSTPLATFORM)

export GOMPLATE

# This target adds a new api type and its controller.
# You would still need to register new api in "apis/<provider>.go" and
# controller in "internal/controller/<provider>.go".
# Arguments:
#   provider: Camel case name of your provider, e.g. GitHub, PlanetScale
#   group: API group for the type you want to add.
#   kind: Kind of the type you want to add
#	apiversion: API version of the type you want to add. Optional and defaults to "v1alpha1"
provider.addtype: $(GOMPLATE)
	@[ "${provider}" ] || ( echo "argument \"provider\" is not set"; exit 1 )
	@[ "${group}" ] || ( echo "argument \"group\" is not set"; exit 1 )
	@[ "${kind}" ] || ( echo "argument \"kind\" is not set"; exit 1 )
	@PROVIDER=$(provider) GROUP=$(group) KIND=$(kind) APIVERSION=$(apiversion) ./hack/helpers/addtype.sh

define CROSSPLANE_MAKE_HELP
Crossplane Targets:
    submodules            Update the submodules, such as the common build scripts.
    run                   Run crossplane locally, out-of-cluster. Useful for development.

endef
# The reason CROSSPLANE_MAKE_HELP is used instead of CROSSPLANE_HELP is because the crossplane
# binary will try to use CROSSPLANE_HELP if it is set, and this is for something different.
export CROSSPLANE_MAKE_HELP

crossplane.help:
	@echo "$$CROSSPLANE_MAKE_HELP"

help-special: crossplane.help

.PHONY: crossplane.help help-special

######## Our Targets ###########
# run unit tests
test.run: go.test.unit

# e2e tests
e2e.run: test-acceptance

#run single test-e2e-long test with <make e2e testFilter=functionNameOfTest>
test-e2e-long: local-build $(KIND) $(HELM3) generate-test-crs
	@$(INFO) running integration tests
	go test -v $(PROJECT_REPO)/test/... -tags=e2e -count=1 -test.v -run '^$(testFilter)$$' -timeout 240m 2>&1 | tee test-output.log
	@$(OK) integration tests passed
	@echo "===========Test Summary==========="
	@grep -E "PASS|FAIL" test-output.log
	@case `tail -n 1 test-output.log` in \
     		*FAIL*) echo "❌ Error: Test failed"; exit 1 ;; \
     		*) echo "✅ All tests passed"; $(OK) integration tests passed ;; \
     esac

#run single e2e test with <make e2e testFilter=functionNameOfTest>
.PHONY: test-acceptance
test-acceptance: local-build $(KIND) $(HELM3) generate-test-crs
	@$(INFO) running end-to-end tests
	@$(INFO) Skipping long running tests
	go test -v  $(PROJECT_REPO)/test/e2e -tags=e2e -short -count=1 -test.v -run '^$(testFilter)$$' -timeout 120m 2>&1 | tee test-output.log
	@echo "===========Test Summary==========="
	@grep -E "PASS|FAIL" test-output.log
	@case `tail -n 1 test-output.log` in \
     		*FAIL*) echo "❌ Error: Test failed"; exit 1 ;; \
     		*) echo "✅ All tests passed"; $(OK) integration tests passed ;; \
     esac

.PHONY: test-acceptance-debug
test-acceptance-debug: local-build $(KIND) $(HELM3) generate-test-crs
	@$(INFO) running end-to-end tests
	@$(INFO) Skipping long running tests
	go test -gcflags="all=-N -l" -c -v  $(PROJECT_REPO)/test/e2e/ -tags=e2e -o ./test/e2e/test-acceptance-debug.test -timeout 30m
	dlv exec ./test/e2e/test-acceptance-debug.test --wd ./test/e2e/ --headless --listen=:2345 --log --api-version=2 --accept-multiclient -- -test.short -test.count=1 -test.v -test.run '^$(testFilter)$$'; EXIT_CODE=$$?; rm ./test/e2e/test-acceptance-debug.test; exit $$EXIT_CODE
	@$(OK) end-to-end tests passed

.PHONY: generate-test-crs
generate-test-crs:
	@$(INFO) Generating CRS in $(TEST_CRS_PATH)
	@find $(TEST_CRS_PATH) -type f -name "*.yaml" -exec sh -c '\
    	for template; do \
    		envsubst < "$$template" > "$${template}.tmp" && mv "$${template}.tmp" "$$template"; \
    	done' sh {} +
	@$(OK) CRS generated



# Generate external-name documentation from *_types.go files
.PHONY: docs.generate-external-name
docs.generate-external-name:
	@$(INFO) Generating external-name documentation from *_types.go files
	@$(GO) run ./scripts/generate-external-name-docs.go
	@$(OK) External-name documentation generated

# ====================================================================================
# Upgrade Tests
# ====================================================================================

# If UPGRADE_TEST_CRS_TAG is not set, use UPGRADE_TEST_FROM_TAG as default
UPGRADE_TEST_CRS_TAG ?= $(UPGRADE_TEST_FROM_TAG)

.PHONY: generate-upgrade-test-crs
generate-upgrade-test-crs: TEST_CRS_PATH := test/upgrade/testdata # Should also generate for custom CRs
generate-upgrade-test-crs: generate-test-crs

.PHONY: check-upgrade-test-vars
check-upgrade-test-vars:
	@for var in UPGRADE_TEST_FROM_TAG UPGRADE_TEST_TO_TAG; do \
		if [ -z "$${!var}" ]; then \
			echo "❌ Error: $$var is not set"; exit 1; \
		fi; \
	done

.PHONY: pull-upgrade-test-version-crs
pull-upgrade-test-version-crs:
	@if [ -z "$(UPGRADE_TEST_CRS_TAG)" ]; then \
		echo "❌ Error: UPGRADE_TEST_CRS_TAG or UPGRADE_TEST_FROM_TAG is not set"; exit 1; \
	fi
	@$(INFO) "Pulling CRs from $(UPGRADE_TEST_CRS_TAG)"
	@if [ "$(UPGRADE_TEST_CRS_TAG)" = "local" ]; then \
		$(OK) "Using local CRs from test/upgrade/testdata/baseCRs/ (UPGRADE_TEST_CRS_TAG is \"local\")"; \
	else \
		$(INFO) "Attempting to check out CRs from tag $(UPGRADE_TEST_CRS_TAG)"; \
		if git ls-tree -r $(UPGRADE_TEST_CRS_TAG) --name-only | grep -q "^test/upgrade/testdata/baseCRs/"; then \
			$(INFO) "Checking out CRs to test/upgrade/testdata/baseCRs from $(UPGRADE_TEST_CRS_TAG)"; \
			rm -r test/upgrade/testdata/baseCRs; \
			mkdir -p test/upgrade/testdata/baseCRs; \
			git archive $(UPGRADE_TEST_CRS_TAG) test/upgrade/testdata/baseCRs | tar -x --strip-components=2 -C test/upgrade; \
			$(OK) "Checked out CRs to test/upgrade/testdata/baseCRs from $(UPGRADE_TEST_CRS_TAG)"; \
		else \
			$(WARN) "No CRs found for tag $(UPGRADE_TEST_CRS_TAG) at path test/upgrade/testdata/baseCRs/. Defaulting to local CRs"; \
		fi; \
	fi

.PHONY: build-upgrade-test-images
build-upgrade-test-images:
	@if [ "$(UPGRADE_TEST_FROM_TAG)" == "local" ] || [ "$(UPGRADE_TEST_TO_TAG)" == "local" ]; then \
		$(INFO) "Building local images (UPGRADE_TEST_FROM_TAG or UPGRADE_TEST_TO_TAG is \"local\")"; \
		$(MAKE) local-build; \
		$(OK) "Built local images for upgrade tests"; \
	fi

.PHONY: upgrade-test
upgrade-test: $(KIND) check-upgrade-test-vars build-upgrade-test-images pull-upgrade-test-version-crs generate-upgrade-test-crs
	@$(INFO) Running upgrade tests
	@go test -tags=upgrade ./test/upgrade -v -short -count=1 -run '^$(testFilter)$$' -timeout 120m 2>&1 | tee upgrade-test-output.log
	@echo "===========Test Summary==========="
	@grep -E "PASS|FAIL" upgrade-test-output.log
	@case `tail -n 1 upgrade-test-output.log` in \
			*FAIL*) echo "❌ Error: Test failed"; exit 1 ;; \
			*) echo "✅ All tests passed"; $(OK) Upgrade tests passed ;; \
	 esac

.PHONY: upgrade-test-debug
upgrade-test-debug: $(KIND) check-upgrade-test-vars build-upgrade-test-images pull-upgrade-test-version-crs generate-upgrade-test-crs
	@$(INFO) Running upgrade tests
	@cd test/upgrade && dlv test --listen=:2345 --headless=true --api-version=2 --build-flags="-tags=upgrade" -- -test.v -test.short -test.count=1 -test.timeout 120m -test.run '^$(testFilter)$$' 2>&1 | tee upgrade-test-output.log
	@echo "===========Test Summary==========="
	@grep -E "PASS|FAIL" upgrade-test-output.log
	@case `tail -n 1 upgrade-test-output.log` in \
			*FAIL*) echo "❌ Error: Test failed"; exit 1 ;; \
			*) echo "✅ All tests passed"; $(OK) Upgrade tests passed ;; \
	 esac

.PHONY: upgrade-test-restore-crs
upgrade-test-restore-crs:
	@$(INFO) Restoring test/upgrade/testdata/baseCRs
	@git restore test/upgrade/testdata/baseCRs
	@$(OK) CRs restored

.PHONY: upgrade-test-clean
upgrade-test-clean: upgrade-test-restore-crs
	@$(INFO) Cleaning upgrade test artifacts
	@rm -rf test/upgrade/logs
	@rm -f upgrade-test-output.log
	@$(OK) Upgrade test artifacts cleaned
	@$(INFO) Deleting kind clusters
	@$(KIND) get clusters 2>/dev/null | grep e2e | xargs -r -n1 $(KIND) delete cluster --name || true
	@$(OK) Kind clusters deleted
	@$(INFO) Cleaning BTP artifacts
	@$(GO) run .github/workflows/cleanup.go
	@$(OK) BTP artifacts cleaned

# ====================================================================================
# E2E Test Environment Setup
# ====================================================================================

# Interactively ask for the file paths containing each required e2e configuration value
# and write them into a .env file. Each variable's value is read from the specified file
# so that secrets never need to be typed on the command line.
.PHONY: e2e-tests-create-dot-env-from-files
e2e-tests-create-dot-env-from-files:
	@echo "This target creates a .env file for e2e tests by reading each required"
	@echo "configuration value from a file you specify."
	@echo "See https://github.com/SAP/crossplane-provider-btp#required-configuration"
	@echo ""
	@> .env
	@for entry in \
		"BTP_TECHNICAL_USER:JSON file with BTP technical user credentials (email/username/password)" \
		"CIS_CENTRAL_BINDING:JSON file with the cis-central service binding data" \
		"CLI_SERVER_URL:File containing the BTP CLI server URL (e.g. https://cli.btp.cloud.sap/)" \
		"GLOBAL_ACCOUNT:File containing the global account subdomain" \
		"IDP_URL:File containing the IDP URL connectable to the global account" \
		"SECOND_DIRECTORY_ADMIN_EMAIL:File containing a second admin email (different from technical user)" \
		"TECHNICAL_USER_EMAIL:File containing the email address of the BTP technical user" \
	; do \
		varname=$${entry%%:*}; \
		description=$${entry#*:}; \
		printf "%s\n  (%s)\n  Path: " "$$varname" "$$description" >&2; \
		read -r filepath; \
		if [ -z "$$filepath" ]; then \
			echo "⚠️  Skipping $$varname (no path provided)" >&2; \
			continue; \
		fi; \
		if [ ! -f "$$filepath" ]; then \
			echo "❌ Error: file not found: $$filepath" >&2; exit 1; \
		fi; \
		value=$$(cat "$$filepath"); \
		printf '%s=%s\n' "$$varname" "$$value" >> .env; \
		echo "✅ $$varname written" >&2; \
	done
	@echo ""
	@$(OK) .env file created
