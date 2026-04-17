#!/usr/bin/env node
'use strict';

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

const PLATFORM_MAP = {
  'darwin-arm64':  { os: 'darwin',  arch: 'arm64', ext: '.tar.gz' },
  'darwin-x64':    { os: 'darwin',  arch: 'amd64', ext: '.tar.gz' },
  'linux-x64':     { os: 'linux',   arch: 'amd64', ext: '.tar.gz' },
  'linux-arm64':   { os: 'linux',   arch: 'arm64', ext: '.tar.gz' },
  'win32-x64':     { os: 'windows', arch: 'amd64', ext: '.zip' },
};

const GITHUB_REPO = 'vibeknow/cli';

function getVersion() {
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'package.json'), 'utf8'));
  return pkg.version;
}

function isNpx() {
  const prefix = process.env.npm_config_prefix || '';
  return prefix.includes('_npx');
}

function archiveName(version, platform) {
  return `vibeknow-cli-${version}-${platform.os}-${platform.arch}${platform.ext}`;
}

function downloadUrls(version, archive) {
  return [
    `https://github.com/${GITHUB_REPO}/releases/download/v${version}/${archive}`,
    `https://registry.npmmirror.com/-/binary/vibeknow-cli/v${version}/${archive}`,
  ];
}

function download(url, dest) {
  try {
    execSync(`curl -fSL --retry 3 -o "${dest}" "${url}"`, {
      stdio: ['ignore', 'ignore', 'pipe'],
      timeout: 120000,
    });
    return true;
  } catch {
    return false;
  }
}

function extractTarGz(archive, destDir) {
  execSync(`tar -xzf "${archive}" -C "${destDir}"`, { stdio: 'ignore' });
}

function extractZip(archive, destDir) {
  if (process.platform === 'win32') {
    execSync(
      `powershell -Command "Expand-Archive -Force -Path '${archive}' -DestinationPath '${destDir}'"`,
      { stdio: 'ignore' }
    );
  } else {
    execSync(`unzip -o "${archive}" -d "${destDir}"`, { stdio: 'ignore' });
  }
}

function install() {
  if (isNpx()) {
    console.log('[vibeknow] npx detected — binary will be downloaded on first run.');
    return;
  }

  const key = `${process.platform}-${process.arch}`;
  const platform = PLATFORM_MAP[key];
  if (!platform) {
    console.error(`[vibeknow] unsupported platform: ${key}`);
    process.exit(1);
  }

  const version = getVersion();
  const archive = archiveName(version, platform);
  const urls = downloadUrls(version, archive);

  const binDir = path.join(__dirname, '..', 'bin');
  fs.mkdirSync(binDir, { recursive: true });

  const binName = 'vibeknow' + (process.platform === 'win32' ? '.exe' : '');
  const binPath = path.join(binDir, binName);

  // Check if existing binary matches current version — skip download if so.
  if (fs.existsSync(binPath)) {
    try {
      const out = execSync(`"${binPath}" version`, { encoding: 'utf8', timeout: 5000 }).trim();
      if (out === version) {
        return; // already up to date
      }
      console.log(`[vibeknow] upgrading from ${out} to ${version}...`);
    } catch {
      // can't determine version, re-download
    }
    fs.unlinkSync(binPath);
  }

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'vibeknow-'));
  const tmpArchive = path.join(tmpDir, archive);

  let downloaded = false;
  for (const url of urls) {
    console.log(`[vibeknow] downloading from ${url}...`);
    if (download(url, tmpArchive)) {
      downloaded = true;
      break;
    }
    console.log(`[vibeknow] failed, trying next source...`);
  }

  if (!downloaded) {
    console.error(`[vibeknow] failed to download binary.`);
    console.error(`[vibeknow] you can download manually from:`);
    urls.forEach(u => console.error(`  ${u}`));
    fs.rmSync(tmpDir, { recursive: true, force: true });
    process.exit(1);
  }

  if (platform.ext === '.tar.gz') {
    extractTarGz(tmpArchive, tmpDir);
  } else {
    extractZip(tmpArchive, tmpDir);
  }

  const extractedDir = path.join(tmpDir, `vibeknow-cli-${version}-${platform.os}-${platform.arch}`);
  const extractedBin = path.join(extractedDir, binName);

  if (!fs.existsSync(extractedBin)) {
    console.error(`[vibeknow] binary not found in archive: ${extractedBin}`);
    fs.rmSync(tmpDir, { recursive: true, force: true });
    process.exit(1);
  }

  fs.copyFileSync(extractedBin, binPath);
  if (process.platform !== 'win32') {
    fs.chmodSync(binPath, 0o755);
  }

  fs.rmSync(tmpDir, { recursive: true, force: true });
  console.log(`[vibeknow] installed successfully.`);
}

module.exports = { install, PLATFORM_MAP, getVersion, archiveName, downloadUrls, download, extractTarGz, extractZip };

if (require.main === module) {
  install();
}
