'use strict';

// Locating the binary that shipped as an optional dependency.
//
// `vibeknow-cli` declares one `@vectorfunc/vibeknow-cli-<platform>-<arch>` package per
// platform in optionalDependencies, each carrying that platform's binary and
// nothing else. npm matches their `os`/`cpu` fields against the machine and
// installs exactly one; the other four are skipped, which is why they are
// optional rather than regular dependencies — a skip has to be a normal
// outcome, not an install failure.
//
// The scope is load-bearing. Five unscoped names all starting `vibeknow-cli-`
// read to npm's abuse heuristics as typosquatting the package beside them, and
// get refused with "Package name triggered spam detection". A scope tells the
// registry the family has one owner, which is why @esbuild/*, @swc/* and
// @rollup/* all look like this.
//
// The point of the arrangement is reachability. A binary fetched from GitHub
// Releases needs a second host to be reachable on top of the registry, and on
// mainland or corporate networks that is the host that is not. A binary that
// rides inside an npm package arrives over whatever registry already works —
// including a company's internal proxy, which has to work or nothing installs
// at all.
//
// The download in install.js stays as the fallback: optional dependencies can
// legitimately be absent (`npm install --no-optional`, `--ignore-optional`, a
// registry carrying the main package but not these, or an npm old enough to
// have had bugs here).

const fs = require('fs');

/**
 * Name of the platform package for the machine we are running on, or null if
 * this platform never had one built.
 */
function platformPackageName() {
  const key = `${process.platform}-${process.arch}`;
  const supported = [
    'darwin-arm64',
    'darwin-x64',
    'linux-x64',
    'linux-arm64',
    'win32-x64',
  ];
  return supported.includes(key) ? `@vectorfunc/vibeknow-cli-${key}` : null;
}

function binName() {
  return 'vibeknow' + (process.platform === 'win32' ? '.exe' : '');
}

/**
 * Absolute path to the binary provided by the optional platform package, or
 * null when it is not installed.
 *
 * Resolution is left to require.resolve rather than being built by hand:
 * global installs, local installs, pnpm's non-flat layout and npm workspaces
 * all put the package somewhere different, and Node already knows where it
 * looked. The platform packages deliberately carry no `exports` field so this
 * subpath resolves.
 */
function resolvePlatformBinary() {
  const pkg = platformPackageName();
  if (!pkg) return null;
  let resolved;
  try {
    resolved = require.resolve(`${pkg}/${binName()}`);
  } catch (_) {
    return null;
  }
  if (!fs.existsSync(resolved)) return null;

  // npm preserves the executable bit through the tarball, but not every
  // client and not every filesystem does — a binary that is present and
  // unreadable-as-executable would otherwise fail with EACCES, which reads
  // like a broken install rather than a fixable permission.
  if (process.platform !== 'win32') {
    try {
      fs.accessSync(resolved, fs.constants.X_OK);
    } catch (_) {
      try {
        fs.chmodSync(resolved, 0o755);
      } catch (_) {
        return null;
      }
    }
  }
  return resolved;
}

module.exports = { platformPackageName, resolvePlatformBinary, binName };
