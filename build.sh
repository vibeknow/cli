#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-X github.com/vibeknow/cli/cmd.version=${VERSION}"
DIST="${DIST:-./dist}"
NPM_DIST="$DIST/npm"
rm -rf "$DIST"
mkdir -p "$DIST" "$NPM_DIST"

platforms=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

# Go and Node disagree on two of these names, and the npm packages have to use
# Node's: npm matches the `os`/`cpu` fields against process.platform /
# process.arch, so a package that says "amd64" or "windows" would never be
# selected on any machine.
node_os() { case "$1" in windows) echo win32 ;; *) echo "$1" ;; esac; }
node_arch() { case "$1" in amd64) echo x64 ;; *) echo "$1" ;; esac; }

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

  # Stage the npm platform package carrying the same binary.
  #
  # This is the path that makes the CLI installable where GitHub is not
  # reachable. The binary rides inside an npm package, so it arrives over
  # whatever registry already works — the public mirrors, or a company's own
  # proxy — instead of over a second, separate host that has to be reachable
  # too. `scripts/install.js` keeps its download as the fallback for when
  # these were skipped (`--no-optional`, or a registry that has the main
  # package but not these).
  NOS="$(node_os "$OS")"
  NARCH="$(node_arch "$ARCH")"
  # Scoped, and it has to be. Publishing five unscoped names that all begin
  # `vibeknow-cli-` reads to npm's abuse heuristics as typosquatting the
  # package they sit next to, and it refuses them:
  #   403 Package name triggered spam detection
  # A scope is what tells the registry the family has a single owner, which is
  # why every project doing this — @esbuild/*, @swc/*, @rollup/* — is scoped.
  pkg_name="@vectorfunc/vibeknow-cli-${NOS}-${NARCH}"
  pkg_dir="$NPM_DIST/vibeknow-cli-${NOS}-${NARCH}"
  mkdir -p "$pkg_dir"
  cp "$out" "$pkg_dir/$bin_name"
  chmod 755 "$pkg_dir/$bin_name"

  # No `exports` field, deliberately: the main package resolves the binary
  # with require.resolve("<pkg>/<bin>"), and an `exports` map would have to
  # enumerate it to keep that working.
  cat >"$pkg_dir/package.json" <<JSON
{
  "name": "$pkg_name",
  "version": "$VERSION",
  "description": "vibeknow CLI binary for ${NOS}-${NARCH}. Installed automatically as an optional dependency of vibeknow-cli; not meant to be depended on directly.",
  "license": "MIT",
  "os": ["$NOS"],
  "cpu": ["$NARCH"],
  "files": ["$bin_name"],
  "repository": { "type": "git", "url": "https://github.com/vibeknow/cli.git" },
  "homepage": "https://github.com/vibeknow/cli",
  "preferUnplugged": true
}
JSON

  cat >"$pkg_dir/README.md" <<MD
# ${pkg_name}

The \`vibeknow\` binary for ${NOS}-${NARCH}.

This package exists so \`vibeknow-cli\` can ship its binary over the npm
registry rather than fetching it from a separate host. Install
[\`vibeknow-cli\`](https://www.npmjs.com/package/vibeknow-cli) instead — npm
picks the right platform package on its own.
MD

  echo "Staged $pkg_dir"
done

echo "Done. Archives in $DIST/, npm platform packages in $NPM_DIST/"
ls -lh "$DIST"/*.tar.gz "$DIST"/*.zip 2>/dev/null
ls -d "$NPM_DIST"/*/ 2>/dev/null
