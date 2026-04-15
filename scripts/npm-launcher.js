#!/usr/bin/env node
const { spawnSync } = require('child_process');
const path = require('path');
const bin = path.join(__dirname, '..', 'dist', 'vibeknow' + (process.platform === 'win32' ? '.exe' : ''));
const r = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
process.exit(r.status ?? 1);
