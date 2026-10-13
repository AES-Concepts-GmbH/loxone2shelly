#!/bin/bash
# Builds the Home Assistant add-on images locally (repository root as build context).
# Usage: ./loxone2shelly/build-addon.sh [arch ...]   (default: amd64 aarch64 armv7)
set -euo pipefail

ADDON_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$ADDON_DIR")"
VERSION=$(tr -d '[:space:]' < "$PROJECT_ROOT/VERSION")
ARCHS=("$@")
if [ ${#ARCHS[@]} -eq 0 ]; then
    ARCHS=(amd64 aarch64 armv7)
fi

platform_for() {
    case "$1" in
        amd64) echo "linux/amd64" ;;
        aarch64) echo "linux/arm64" ;;
        armv7) echo "linux/arm/v7" ;;
        *) echo "unsupported arch: $1" >&2; exit 1 ;;
    esac
}

echo "Building Loxone2Shelly add-on v${VERSION}"
for arch in "${ARCHS[@]}"; do
    base=$(grep "^  ${arch}:" "$ADDON_DIR/build.yaml" | sed -E 's/.*: *"?([^"]+)"?/\1/')
    echo "==> ${arch} (base: ${base})"
    docker buildx build \
        --platform "$(platform_for "$arch")" \
        --build-arg BUILD_FROM="${base}" \
        -f "$ADDON_DIR/Dockerfile" \
        -t "ghcr.io/aes-concepts-gmbh/loxone2shelly-${arch}:${VERSION}" \
        -t "ghcr.io/aes-concepts-gmbh/loxone2shelly-${arch}:latest" \
        --load \
        "$PROJECT_ROOT"
done
echo "Build complete!"
