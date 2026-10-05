FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=1.0.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /loggerbin .

FROM scratch
ARG VERSION=1.0.0
ARG REVISION=unknown
LABEL org.opencontainers.image.title="Loggerbin" \
      org.opencontainers.image.description="Browser-encrypted paste sharing" \
      org.opencontainers.image.source="https://github.com/LoggeL/loggerbin" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$REVISION
COPY --from=build /loggerbin /loggerbin
COPY --chown=65532:65532 deploy/data /data
COPY LICENSE THIRD_PARTY_NOTICES /licenses/
USER 65532:65532
ENV LOGGERBIN_HOST=0.0.0.0 LOGGERBIN_PORT=8080 LOGGERBIN_DATA_DIR=/data
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=4s --start-period=5s --retries=3 CMD ["/loggerbin", "healthcheck"]
ENTRYPOINT ["/loggerbin"]
