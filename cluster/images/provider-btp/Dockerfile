# renovate: datasource=docker depName=gcr.io/distroless/static
ARG DISTROLESS_DIGEST="sha256:1a64f46364620465ef9e9a8f5413757dfdf22695e050bbfd7303eeaf17a59383"

FROM gcr.io/distroless/static@${DISTROLESS_DIGEST}
ARG TARGETOS
ARG TARGETARCH

COPY bin/${TARGETOS}_${TARGETARCH}/provider /usr/local/bin/crossplane-provider-btp

USER 65532
EXPOSE 8080

ENTRYPOINT ["crossplane-provider-btp"]

ARG DISTROLESS_DIGEST
ARG COMMIT_HASH
ARG VERSION

LABEL org.opencontainers.image.title="provider-btp" \
      org.opencontainers.image.description="Crossplane Provider for SAP BTP" \
      org.opencontainers.image.documentation="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.url="https://github.com/sap/crossplane-provider-btp" \
      org.opencontainers.image.source="https://github.com/sap/crossplane-provider-btp.git" \
      org.opencontainers.image.vendor="SAP SE" \
      org.opencontainers.image.base.name="gcr.io/distroless/static" \
      org.opencontainers.image.base.digest="${DISTROLESS_DIGEST}" \
      org.opencontainers.image.revision="${COMMIT_HASH}" \
      org.opencontainers.image.version="${VERSION}"
