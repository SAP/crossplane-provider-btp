FROM alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
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
