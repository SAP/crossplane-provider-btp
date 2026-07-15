FROM alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
RUN apk --no-cache add ca-certificates bash

ARG TARGETOS
ARG TARGETARCH

ADD bin/${TARGETOS}_${TARGETARCH}/provider /usr/local/bin/crossplane-provider-btp

ENV USER_ID=65532

# Setup Terraform environment

## Provider-dependent configuration
ARG TERRAFORM_VERSION
ARG TERRAFORM_PROVIDER_SOURCE
ARG TERRAFORM_PROVIDER_VERSION
ARG TERRAFORM_PROVIDER_DOWNLOAD_NAME
ARG TERRAFORM_NATIVE_PROVIDER_BINARY
ARG CROSSPLANE_PROVIDER_VERSION

## End of - Provider-dependent configuration

# OpenTofu (MPL-2.0) normalizes provider source addresses to lowercase and
# uses `registry.opentofu.org` (not `registry.terraform.io`) as the implicit
# hostname when `main.tf.json` says e.g. `"source":"SAP/btp"`. It also
# refuses symlinks inside the mirror tree (only the mirror root may be a
# symlink). So we lay the mirror down at the path OpenTofu actually searches:
# `/terraform/provider-mirror/registry.opentofu.org/<lowercase-source>/<ver>/<os>_<arch>`.
# `TERRAFORM_PROVIDER_SOURCE` stays `SAP/btp` for the rest of the codebase
# (main.tf.json, host-side schema generation, etc.); only the on-disk plugin
# path is lowercased.
ENV PLUGIN_DIR=/terraform/provider-mirror/registry.opentofu.org/sap/btp/${TERRAFORM_PROVIDER_VERSION}/${TARGETOS}_${TARGETARCH}
ENV TF_CLI_CONFIG_FILE=/terraform/.terraformrc
ENV TF_FORK=0

RUN mkdir -p ${PLUGIN_DIR}

ADD https://github.com/opentofu/opentofu/releases/download/v${TERRAFORM_VERSION}/tofu_${TERRAFORM_VERSION}_${TARGETOS}_${TARGETARCH}.zip /tmp/opentofu.zip
ADD https://github.com/SAP/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}/releases/download/v${TERRAFORM_PROVIDER_VERSION}/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}_${TERRAFORM_PROVIDER_VERSION}_${TARGETOS}_${TARGETARCH}.zip /tmp
COPY terraformrc.hcl ${TF_CLI_CONFIG_FILE}

# OpenTofu (MPL-2.0) is installed under the name `terraform`. Upjet's executor
# hardcodes the binary name "terraform" (workspace.go:413); OpenTofu is a
# drop-in replacement at the CLI level, so we just rename on install.
# Required for the identity-injector workaround (#521): identity forwarding
# lands in CLI 1.12+ (Terraform CLI ≥ 1.12 is BSL; OpenTofu 1.12+ is MPL-2.0).
RUN unzip /tmp/opentofu.zip -d /tmp/opentofu \
  && mv /tmp/opentofu/tofu /usr/local/bin/terraform \
  && chmod +x /usr/local/bin/terraform \
  && rm -rf /tmp/opentofu /tmp/opentofu.zip \
  && unzip /tmp/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}_${TERRAFORM_PROVIDER_VERSION}_${TARGETOS}_${TARGETARCH}.zip -d ${PLUGIN_DIR} \
  && chmod +x ${PLUGIN_DIR}/* \
  && rm /tmp/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}_${TERRAFORM_PROVIDER_VERSION}_${TARGETOS}_${TARGETARCH}.zip \
  && chown -R ${USER_ID}:${USER_ID} /terraform

# Provider controller needs these environment variables at runtime
ENV TERRAFORM_VERSION=${TERRAFORM_VERSION}
ENV TERRAFORM_PROVIDER_SOURCE=${TERRAFORM_PROVIDER_SOURCE}
ENV TERRAFORM_PROVIDER_VERSION=${TERRAFORM_PROVIDER_VERSION}
ENV TERRAFORM_NATIVE_PROVIDER_PATH=${PLUGIN_DIR}/${TERRAFORM_NATIVE_PROVIDER_BINARY}
ENV TF_APPEND_USER_AGENT="crossplane-provider-btp/${CROSSPLANE_PROVIDER_VERSION}"

USER ${USER_ID}
EXPOSE 8080

ENTRYPOINT ["crossplane-provider-btp"]

LABEL org.opencontainers.image.title="provider-btp" \
      org.opencontainers.image.description="Crossplane Provider for SAP BTP" \
      org.opencontainers.image.documentation="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.url="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.source="https://github.com/sap/crossplane-provider-btp.git" \
      org.opencontainers.image.vendor="SAP SE" \
      org.opencontainers.image.base.name="docker.io/library/alpine:3.23.3"
