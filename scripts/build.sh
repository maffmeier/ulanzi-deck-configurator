#!/bin/sh
# Cross-compiles release binaries into dist/. Run inside the dev container:
#   docker compose run --rm dev sh scripts/build.sh [version]
set -eu
VERSION=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
LDFLAGS="-s -w -X main.version=$VERSION"
mkdir -p dist

build() {
  goos=$1 goarch=$2 out=$3 extra=${4:-}
  echo "-> $out"
  GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS $extra" -o "dist/$out" ./cmd/ulanzi-deck
}

build linux   amd64 ulanzi-deck-linux-amd64
build linux   arm64 ulanzi-deck-linux-arm64
# GUI subsystem: no console window when started via autostart.
build windows amd64 ulanzi-deck-windows-amd64.exe "-H windowsgui"
build windows arm64 ulanzi-deck-windows-arm64.exe "-H windowsgui"
# Without cgo the macOS build has no tray icon (see internal/interfaces/tray).
build darwin  amd64 ulanzi-deck-darwin-amd64
build darwin  arm64 ulanzi-deck-darwin-arm64
