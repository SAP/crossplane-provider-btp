ARG BASE_NAME=alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
FROM ${BASE_NAME}
RUN apk --no-cache add ca-certificates

ARG TARGETOS
ARG TARGETARCH
ARG COMMIT_HASH
ARG VERSION
ARG BASE_NAME

ADD bin/${TARGETOS}_${TARGETARCH}/provider /usr/local/bin/crossplane-provider-btp

ENV USER_ID=65532

USER ${USER_ID}
EXPOSE 8080

ENTRYPOINT ["crossplane-provider-btp"]

LABEL org.opencontainers.image.title="provider-btp" \
      org.opencontainers.image.description="Crossplane Provider for SAP BTP" \
      org.opencontainers.image.documentation="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.url="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.source="https://github.com/sap/crossplane-provider-btp.git" \
      org.opencontainers.image.vendor="SAP SE" \
      org.opencontainers.image.base.name="${BASE_NAME}" \
      org.opencontainers.image.revision="${COMMIT_HASH}" \
      org.opencontainers.image.version="${VERSION}"
