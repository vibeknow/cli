#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-X github.com/vibeknow/cli/cmd.version=${VERSION}"
DIST="${DIST:-./dist}"
mkdir -p "$DIST"

platforms=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

for platform in "${platforms[@]}"; do
  IFS='/' read -r OS ARCH <<<"$platform"
  out="$DIST/vibeknow-${OS}-${ARCH}"
  [[ "$OS" == "windows" ]] && out="${out}.exe"
  echo "Building $out"
  GOOS="$OS" GOARCH="$ARCH" CGO_ENABLED=0 \
    go build -ldflags "$LDFLAGS" -o "$out" .
done
