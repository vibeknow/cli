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

// Per-source wall clock. The whole install runs inside a host budget — a
// WorkBuddy connector gets 300s for `npm install -g` including this script —
// so a source that hangs has to be abandoned early enough to leave the next
// one a chance. 120s per source spent that budget on the first host that went
// quiet, which is the failure mode this is most likely to meet: not a refused
// connection but a reachable-yet-stalled CDN.
const DOWNLOAD_TIMEOUT_MS = 45000;

function downloadUrls(version, archive) {
  const urls = [];

  // A mirror supplied by the environment wins. This exists so the source list
  // can change without publishing a package: WorkBuddy passes it through
  // cli.json's `env` field, which reaches this script because npm hands the
  // install command's environment to postinstall. Moving to a new bucket is
  // then a connector update, not a release.
  const base = (process.env.VIBEKNOW_BINARY_BASE_URL || '').trim().replace(/\/+$/, '');
  if (base) {
    urls.push(`${base}/v${version}/${archive}`);
  }

  urls.push(`https://github.com/${GITHUB_REPO}/releases/download/v${version}/${archive}`);
  // Kept last, and known to 404 until the package is registered for binary
  // sync on npmmirror. A miss costs one round trip rather than the timeout
  // above, so leaving it in place is cheap and it starts working the moment
  // the sync is configured — but nothing should be relying on it today.
  urls.push(`https://registry.npmmirror.com/-/binary/vibeknow-cli/v${version}/${archive}`);
  return urls;
}

function download(url, dest) {
  try {
    execSync(`curl -fSL --retry 2 --connect-timeout 10 -o "${dest}" "${url}"`, {
      stdio: ['ignore', 'ignore', 'pipe'],
      timeout: DOWNLOAD_TIMEOUT_MS,
    });
    return true;
  } catch (e) {
    // Reported rather than swallowed: with several sources tried in turn, the
    // last message is the only thing separating "this host is blocked here"
    // from "this version was never published", and those need different fixes.
    const detail = (e.stderr ? e.stderr.toString() : '').trim().split('\n').pop();
    if (detail) {
      console.log(`[vibeknow] ${detail}`);
    }
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
