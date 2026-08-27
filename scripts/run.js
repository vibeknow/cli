#!/usr/bin/env node
'use strict';

const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const { resolvePlatformBinary } = require('./platform-binary.js');

const binName = 'vibeknow' + (process.platform === 'win32' ? '.exe' : '');
const binPath = path.join(__dirname, '..', 'bin', binName);
const oldBinPath = binPath + '.old';

// The binary that shipped as an optional dependency wins, and wins before
// anything else runs: it is already on disk, at the right version, and needed
// no network at all. Everything below this point — the .old recovery, the
// on-demand download — exists for installs that did not get one.
const packaged = resolvePlatformBinary();
if (packaged) {
  try {
    execFileSync(packaged, process.argv.slice(2), { stdio: 'inherit' });
    process.exit(0);
  } catch (e) {
    process.exit(typeof e.status === 'number' ? e.status : 1);
  }
}

// Windows self-update can leave the previous binary renamed to <name>.old
// when the replace step is interrupted. Recover it so the CLI keeps working.
function restoreOldBinary() {
  try {
    if (fs.existsSync(binPath)) {
      fs.rmSync(binPath, { force: true });
    }
    fs.renameSync(oldBinPath, binPath);
    return true;
  } catch (_) {
    return false;
  }
}

if (process.platform === 'win32' && fs.existsSync(oldBinPath)) {
  if (!fs.existsSync(binPath)) {
    restoreOldBinary();
  } else {
    // Both present: health-check the new binary. If it responds, the update
    // completed — drop the stale .old. Otherwise roll back.
    try {
      execFileSync(binPath, ['--version'], { stdio: 'ignore', timeout: 10000 });
      try {
        fs.rmSync(oldBinPath, { force: true });
      } catch (_) { /* best-effort cleanup */ }
    } catch (_) {
      restoreOldBinary();
    }
  }
}

// If binary is missing (npx or --ignore-scripts), download on demand.
if (!fs.existsSync(binPath)) {
  console.log('[vibeknow] binary not found, downloading...');
  try {
    const { install } = require('./install.js');
    install();
  } catch (e) {
    console.error('[vibeknow] on-demand download failed:', e.message || e);
  }
}

if (!fs.existsSync(binPath)) {
  console.error(
    `Error: vibeknow binary not found at ${binPath}\n\n` +
    `The binary normally arrives as an optional dependency, and is downloaded\n` +
    `only when that was skipped. Both paths missed, which usually means:\n` +
    `  - npm ran with --no-optional / --ignore-optional, and the download\n` +
    `    then failed too (proxy / firewall / release unavailable)\n` +
    `  - npm is configured with ignore-scripts=true, which skips the download\n` +
    `  - the registry in use carries vibeknow-cli but not its platform packages\n\n` +
    `To fix, reinstall and let the optional dependency through:\n` +
    `  npm install -g vibeknow-cli --include=optional\n` +
    `Or fetch the binary directly:\n` +
    `  node "${path.join(__dirname, 'install.js')}"\n`
  );
  process.exit(1);
}

try {
  execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(typeof e.status === 'number' ? e.status : 1);
}
