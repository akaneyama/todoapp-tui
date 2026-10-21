#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-1.0.0}"
LDFLAGS="-s -w -X main.version=${VERSION}"
OUT="bin"

mkdir -p "${OUT}"

echo "==> Building host binaries (${VERSION})..."
go build -ldflags="${LDFLAGS}" -trimpath -o "${OUT}/server" ./cmd/server
go build -ldflags="${LDFLAGS}" -trimpath -o "${OUT}/client" ./cmd/client
echo "Build complete: ${OUT}/server, ${OUT}/client"
