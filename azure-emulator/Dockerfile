# syntax=docker/dockerfile:1
# CloudTyke Azure emulator - multi-stage build producing a static, non-root image.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=0.1.0-dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/bwalia/cloudtyke/azure-emulator/internal/version.Version=${VERSION} -X github.com/bwalia/cloudtyke/azure-emulator/internal/version.Commit=${COMMIT} -X github.com/bwalia/cloudtyke/azure-emulator/internal/version.Date=${DATE}" \
      -o /out/emulator ./cmd/emulator \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/tyke ./cmd/tyke \
 && mkdir -p /out/data /out/certs

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="CloudTyke Azure emulator" \
      org.opencontainers.image.description="A local emulator for Azure APIs (ARM, Entra ID, Microsoft Graph subset). Not affiliated with or endorsed by Microsoft." \
      org.opencontainers.image.source="https://github.com/bwalia/cloudtyke" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY --from=build /out/emulator /emulator
COPY --from=build /out/tyke /tyke
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build --chown=65532:65532 /out/certs /certs
USER 65532:65532
ENV CLOUDTYKE_DATA_DIR=/data CLOUDTYKE_CERT_EXPORT_DIR=/certs CLOUDTYKE_LISTEN=:443
EXPOSE 443
VOLUME ["/data"]
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=10 CMD ["/emulator", "healthcheck"]
ENTRYPOINT ["/emulator"]
CMD ["serve"]
