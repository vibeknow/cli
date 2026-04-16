#!/usr/bin/env node
'use strict';

const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const binName = 'vibeknow' + (process.platform === 'win32' ? '.exe' : '');
const binPath = path.join(__dirname, '..', 'bin', binName);

// If binary is missing (npx or --ignore-scripts), download on demand
if (!fs.existsSync(binPath)) {
  console.log('[vibeknow] binary not found, downloading...');
  const { install } = require('./install.js');
  install();
}

if (!fs.existsSync(binPath)) {
  console.error('[vibeknow] failed to install binary. Please try: npm install -g vibeknow-cli');
  process.exit(1);
}

try {
  const result = execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(e.status ?? 1);
}
