#!/bin/bash
set -e

ADDON_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$ADDON_DIR")"
SRC_DIR="$ADDON_DIR/src"
VERSION=$(cat "$PROJECT_ROOT/VERSION" | tr -d '[:space:]')

echo "Building Loxone2Shelly Add-on v${VERSION}"

# Clean and recreate src directory
rm -rf "$SRC_DIR"
mkdir -p "$SRC_DIR"

# Copy Go source
cp -r "$PROJECT_ROOT/cmd" "$SRC_DIR/"
cp -r "$PROJECT_ROOT/internal" "$SRC_DIR/"
cp "$PROJECT_ROOT/go.mod" "$SRC_DIR/"
cp "$PROJECT_ROOT/go.sum" "$SRC_DIR/" 2>/dev/null || true
cp "$PROJECT_ROOT/VERSION" "$SRC_DIR/"

# Copy Web source
cp -r "$PROJECT_ROOT/web" "$SRC_DIR/"
rm -rf "$SRC_DIR/web/node_modules" "$SRC_DIR/web/dist"

# Copy configs
mkdir -p "$SRC_DIR/configs"
cp "$PROJECT_ROOT/configs/config.example.yaml" "$SRC_DIR/configs/"

echo "Source prepared in $SRC_DIR"

# Build Docker images
ARCHITECTURES="aarch64 amd64 armv7"
for arch in $ARCHITECTURES; do
    echo "Building for $arch..."
    docker build \
        --build-arg BUILD_FROM="ghcr.io/home-assistant/${arch}-base:3.19" \
        -t "loxone2shelly-${arch}:${VERSION}" \
        -t "loxone2shelly-${arch}:latest" \
        "$ADDON_DIR"
done

echo "Build complete!"
