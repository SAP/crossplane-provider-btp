# ====================================================================================
# Setup Project

PLATFORMS := linux_amd64 linux_arm64
include ../../../build/makelib/common.mk

# ====================================================================================
#  Options
DOCKER_REGISTRY ?= crossplane
IMAGE = $(BUILD_REGISTRY)/crossplane/provider-btp
OSBASEIMAGE = scratch
include ../../../build/makelib/imagelight.mk

# ====================================================================================
# Targets

img.build:
	@$(INFO) docker build $(IMAGE)
	@cp Dockerfile $(IMAGE_TEMP_DIR) || $(FAIL)
	@cp -R ../../../package $(IMAGE_TEMP_DIR) || $(FAIL)
	@cd $(IMAGE_TEMP_DIR) && $(SED_CMD) 's|BASEIMAGE|$(OSBASEIMAGE)|g' Dockerfile || $(FAIL)
	@cd $(IMAGE_TEMP_DIR) && $(SED_CMD) 's|VERSION|$(VERSION)|g' package/crossplane.yaml || $(FAIL)
	@cd $(IMAGE_TEMP_DIR) && $(SED_CMD) 's|DOCKER_REGISTRY|$(BUILD_REGISTRY)|g' package/crossplane.yaml || $(FAIL)
	@cd $(IMAGE_TEMP_DIR) && find package -type f -name '*.yaml' -exec cat {} >> 'package.yaml' \; -exec printf '\n---\n' \; || $(FAIL)
	@docker build $(BUILD_ARGS) \
		--build-arg GIT_COMMIT="${COMMIT_HASH}" \
		--build-arg BUILD_TIMESTAMP="$(TIME_LONG)" \
		-t $(IMAGE) \
		-t $(IMAGE):$(VERSION) \
		$(IMAGE_TEMP_DIR) || $(FAIL)
	@$(OK) docker build $(IMAGE)
