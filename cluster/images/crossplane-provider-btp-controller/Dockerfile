ARG BUILDENV=upbound

FROM scratch AS upbound
ADD provider /provider
ARG BASEDIR

FROM scratch AS piper
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG BASEDIR=cluster/images/crossplane-provider-btp-controller/

ARG TERRAFORM_VERSION=1.3.9
ARG TERRAFORM_PROVIDER_SOURCE=SAP/btp
ARG TERRAFORM_PROVIDER_VERSION=1.0.0
ARG TERRAFORM_PROVIDER_DOWNLOAD_NAME=terraform-provider-btp
ARG TERRAFORM_NATIVE_PROVIDER_BINARY=${TERRAFORM_PROVIDER_DOWNLOAD_NAME}_${TERRAFORM_VERSION}_linux_amd64

ADD provider-$TARGETOS.$TARGETARCH /provider

FROM ${BUILDENV} AS buildenv

FROM alpine:3.20.3@sha256:1e42bbe2508154c9126d48c2b8a75420c3544343bf86fd041fb7527e017a4b4a
RUN apk --no-cache add ca-certificates bash
ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY --from=buildenv /provider /usr/local/bin/crossplane-provider-btp

ENV USER_ID=65532

# Setup Terraform environment

## Provider-dependent configuration
ARG TERRAFORM_VERSION
ARG TERRAFORM_PROVIDER_SOURCE
ARG TERRAFORM_PROVIDER_VERSION
ARG TERRAFORM_PROVIDER_DOWNLOAD_NAME
ARG TERRAFORM_NATIVE_PROVIDER_BINARY

## End of - Provider-dependent configuration

ARG BUILD_TIMESTAMP
ARG GIT_COMMIT
ENV PLUGIN_DIR /terraform/provider-mirror/registry.terraform.io/${TERRAFORM_PROVIDER_SOURCE}/${TERRAFORM_PROVIDER_VERSION}/${TARGETOS}_${TARGETARCH}
ENV TF_CLI_CONFIG_FILE /terraform/.terraformrc
ENV TF_FORK 0

RUN mkdir -p ${PLUGIN_DIR}

ADD https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_${TARGETOS}_${TARGETARCH}.zip /tmp
ADD https://github.com/SAP/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}/releases/download/v${TERRAFORM_PROVIDER_VERSION}/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}_${TERRAFORM_PROVIDER_VERSION}_${TARGETOS}_${TARGETARCH}.zip /tmp
ADD ${BASEDIR}terraformrc.hcl ${TF_CLI_CONFIG_FILE}

RUN unzip /tmp/terraform_${TERRAFORM_VERSION}_${TARGETOS}_${TARGETARCH}.zip -d /usr/local/bin \
  && chmod +x /usr/local/bin/terraform \
  && rm /tmp/terraform_${TERRAFORM_VERSION}_${TARGETOS}_${TARGETARCH}.zip \
  && unzip /tmp/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}_${TERRAFORM_PROVIDER_VERSION}_${TARGETOS}_${TARGETARCH}.zip -d ${PLUGIN_DIR} \
  && chmod +x ${PLUGIN_DIR}/* \
  && rm /tmp/${TERRAFORM_PROVIDER_DOWNLOAD_NAME}_${TERRAFORM_PROVIDER_VERSION}_${TARGETOS}_${TARGETARCH}.zip \
  && chown -R ${USER_ID}:${USER_ID} /terraform
# End of - Setup Terraform environment

# Provider controller needs these environment variable at runtime
ENV TERRAFORM_VERSION ${TERRAFORM_VERSION}
ENV TERRAFORM_PROVIDER_SOURCE ${TERRAFORM_PROVIDER_SOURCE}
ENV TERRAFORM_PROVIDER_VERSION ${TERRAFORM_PROVIDER_VERSION}
ENV TERRAFORM_NATIVE_PROVIDER_PATH ${PLUGIN_DIR}/${TERRAFORM_NATIVE_PROVIDER_BINARY}
ENV TF_APPEND_USER_AGENT "crossplane-provider-btp/${CROSSPLANE_PROVIDER_VERSION}"

USER ${USER_ID}
EXPOSE 8080

ENTRYPOINT ["crossplane-provider-btp"]

ARG BUILD_TIMESTAMP
ARG GIT_COMMIT

LABEL org.opencontainers.image.title="crossplane-provider-btp-controller" \
      org.opencontainers.image.description="Crossplane controller for BTP accounts" \
      org.opencontainers.image.documentation="https://pages.github.tools.sap/cloud-orchestration/" \
      org.opencontainers.image.url="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.source="https://github.com/sap/crossplane-provider-btp.git" \
      org.opencontainers.image.vendor="SAP SE" \
      org.opencontainers.image.base.name="docker.io/library/alpine:3.20.3" \
      org.opencontainers.image.base.digest="sha256:beefdbd8a1da6d2915566fde36db9db0b524eb737fc57cd1367effd16dc0d06d" \
      org.opencontainers.image.revision="$GIT_COMMIT" \
      org.opencontainers.image.created="$BUILD_TIMESTAMP"
