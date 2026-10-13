# syntax=docker/dockerfile:1

# ---- Go backend ----
FROM golang:1.24-alpine AS go-builder
WORKDIR /app

# Dependencies first for layer caching
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY VERSION ./VERSION

RUN VERSION=$(tr -d '[:space:]' < VERSION) && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o gateway ./cmd/gateway

# ---- React frontend ----
FROM node:20-alpine AS web-builder
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY web/ ./
COPY VERSION /tmp/VERSION

RUN export VITE_APP_VERSION=$(tr -d '[:space:]' < /tmp/VERSION) && \
    export VITE_BUILD_DATE=$(date -u +%Y-%m-%d) && \
    npm run build

# ---- Runtime ----
FROM alpine:3.20
WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=go-builder /app/gateway .
COPY --from=web-builder /app/web/dist ./web/dist
COPY configs/config.example.yaml ./config.example.yaml

# Config (and backups) live in /app/configs; mount a volume there.
RUN mkdir -p /app/configs

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

ENTRYPOINT ["./gateway"]
CMD ["-config", "/app/configs/config.yaml"]
