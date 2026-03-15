.PHONY: all build test lint clean docker-build

BINARY   := loxone2shelly
CMD_PKG  := ./cmd/gateway
GO_FLAGS := -a -installsuffix cgo

all: build

## build: Compile the Go binary
build:
	go build $(GO_FLAGS) -o $(BINARY) $(CMD_PKG)

## test: Run all Go unit tests
test:
	go test ./... -count=1

## test-v: Run all Go unit tests with verbose output
test-v:
	go test ./... -count=1 -v

## lint: Run golangci-lint (requires golangci-lint to be installed)
lint:
	golangci-lint run ./...

## vet: Run go vet
vet:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY)

## docker-build: Build the Docker image
docker-build:
	docker build -t loxone2shelly:latest .

## help: Display this help message
help:
	@grep -E '^## ' Makefile | sed 's/^## //'
