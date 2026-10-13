# syntax=docker/dockerfile:1
# Home Assistant add-on image. Build from the REPOSITORY ROOT:
#   docker build -f loxone2shelly/Dockerfile --build-arg BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.20 .
# The GitHub Actions workflow (.github/workflows/addon.yml) publishes the
# per-architecture images referenced by config.yaml.
ARG BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.20

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS go-builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY VERSION ./VERSION
RUN VERSION=$(tr -d '[:space:]' < VERSION) && \
    GOARM=$(echo "${TARGETVARIANT}" | tr -d v) \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o gateway ./cmd/gateway

FROM --platform=$BUILDPLATFORM node:20-alpine AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
COPY VERSION /tmp/VERSION
RUN export VITE_APP_VERSION=$(tr -d '[:space:]' < /tmp/VERSION) && \
    export VITE_BUILD_DATE=$(date -u +%Y-%m-%d) && \
    npm run build

FROM ${BUILD_FROM}
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /app/gateway .
COPY --from=web-builder /app/web/dist ./web/dist
COPY loxone2shelly/rootfs /
RUN chmod a+x /run.sh
EXPOSE 8080
CMD ["/run.sh"]
