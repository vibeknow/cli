#!/usr/bin/env node
'use strict';

const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const binName = 'vibeknow' + (process.platform === 'win32' ? '.exe' : '');
const binPath = path.join(__dirname, '..', 'bin', binName);
const oldBinPath = binPath + '.old';

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
    `This usually means the postinstall script was skipped.\n` +
    `Common causes:\n` +
    `  - npm is configured with ignore-scripts=true\n` +
    `  - The postinstall download failed (proxy / firewall / release unavailable)\n\n` +
    `To fix, run the install script manually:\n` +
    `  node "${path.join(__dirname, 'install.js')}"\n` +
    `Or reinstall globally:\n` +
    `  npm install -g vibeknow-cli\n`
  );
  process.exit(1);
}

try {
  execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(typeof e.status === 'number' ? e.status : 1);
}
