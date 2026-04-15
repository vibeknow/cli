#!/usr/bin/env node
const fs = require('fs');
const path = require('path');

const platformMap = {
  'darwin-x64':   'vibeknow-darwin-amd64',
  'darwin-arm64': 'vibeknow-darwin-arm64',
  'linux-x64':    'vibeknow-linux-amd64',
  'linux-arm64':  'vibeknow-linux-arm64',
  'win32-x64':    'vibeknow-windows-amd64.exe',
};

const key = `${process.platform}-${process.arch}`;
const fname = platformMap[key];
if (!fname) {
  console.error(`[vibeknow] unsupported platform: ${key}`);
  process.exit(1);
}
const src = path.join(__dirname, '..', 'dist', fname);
const dst = path.join(__dirname, '..', 'dist', 'vibeknow' + (process.platform === 'win32' ? '.exe' : ''));
if (!fs.existsSync(src)) {
  console.error(`[vibeknow] missing binary: ${src}`);
  process.exit(1);
}
fs.copyFileSync(src, dst);
fs.chmodSync(dst, 0o755);
