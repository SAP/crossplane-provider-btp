FROM scratch

COPY package.yaml .

ARG BUILD_TIMESTAMP
ARG GIT_COMMIT

LABEL org.opencontainers.image.title="crossplane-provider-btp" \
      org.opencontainers.image.description="Configuration for btp-account provider" \
      org.opencontainers.image.documentation="https://pages.github.tools.sap/cloud-orchestration/" \
      org.opencontainers.image.url="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.source="https://github.com/sap/crossplane-provider-btp.git" \
      org.opencontainers.image.vendor="SAP SE" \
      org.opencontainers.image.base.name="scratch" \
      org.opencontainers.image.revision="$GIT_COMMIT" \
      org.opencontainers.image.created="$BUILD_TIMESTAMP"
