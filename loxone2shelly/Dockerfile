ARG BUILD_FROM
FROM golang:1.22-alpine AS go-builder

WORKDIR /app
RUN apk add --no-cache git
COPY src/ .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gateway ./cmd/gateway

FROM node:20-alpine AS web-builder
WORKDIR /app/web
COPY src/web/package.json ./
RUN npm install
COPY src/web/ ./
COPY src/VERSION /tmp/VERSION
RUN export VITE_APP_VERSION=$(cat /tmp/VERSION | tr -d '[:space:]') && \
    export VITE_BUILD_DATE=$(date -u +%Y-%m-%d) && \
    npm run build

FROM ${BUILD_FROM}

RUN apk add --no-cache ca-certificates tzdata bash

WORKDIR /app
COPY --from=go-builder /app/gateway .
COPY --from=web-builder /app/web/dist ./web/dist
COPY rootfs /

RUN chmod a+x /run.sh

EXPOSE 8080
CMD ["/run.sh"]
