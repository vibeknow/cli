#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-X github.com/vibeknow/cli/cmd.version=${VERSION}"
DIST="${DIST:-./dist}"
rm -rf "$DIST"
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

  bin_name="vibeknow"
  [[ "$OS" == "windows" ]] && bin_name="vibeknow.exe"

  # Directory that will live inside the archive
  archive_dir="vibeknow-cli-${VERSION}-${OS}-${ARCH}"
  mkdir -p "$DIST/$archive_dir"

  out="$DIST/$archive_dir/$bin_name"
  echo "Building $out"
  GOOS="$OS" GOARCH="$ARCH" CGO_ENABLED=0 \
    go build -ldflags "$LDFLAGS" -o "$out" .

  # Package
  if [[ "$OS" == "windows" ]]; then
    (cd "$DIST" && zip -r "${archive_dir}.zip" "$archive_dir")
  else
    tar -czf "$DIST/${archive_dir}.tar.gz" -C "$DIST" "$archive_dir"
  fi

  echo "Packaged $DIST/${archive_dir}${OS:+$([ "$OS" = windows ] && echo .zip || echo .tar.gz)}"
done

echo "Done. Archives in $DIST/"
ls -lh "$DIST"/*.tar.gz "$DIST"/*.zip 2>/dev/null
