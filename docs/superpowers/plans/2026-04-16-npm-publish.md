# vibeknow-cli npm 发布 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 vibeknow-cli 以飞书 CLI 模式发布到 npm — npm 包只含 JS 脚本，postinstall 从 GitHub Releases 下载平台二进制，国内回退 npmmirror。

**Architecture:** 单 npm 包 `vibeknow-cli` 内含 `scripts/install.js`（postinstall 下载器）和 `scripts/run.js`（launcher shim）。CI 通过 GitHub Actions 在 push tag 时交叉编译 Go 二进制、打包 tar.gz/zip、创建 GitHub Release、发布 npm 包。另有 `vibeknow` 占位包转发到主包。

**Tech Stack:** Node.js (scripts), Go (binary), GitHub Actions (CI), curl (download)

**Spec:** `docs/superpowers/specs/2026-04-16-npm-publish-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `package.json` | Modify | npm 包元数据，bin 入口，postinstall 脚本 |
| `scripts/install.js` | Create | postinstall 下载器：检测平台→下载→解压→chmod |
| `scripts/run.js` | Rewrite | launcher shim：查找二进制→按需下载→exec |
| `scripts/npm-postinstall.js` | Delete | 被 install.js 取代 |
| `scripts/npm-launcher.js` | Delete | 被 run.js 取代 |
| `build.sh` | Modify | 增加 tar.gz/zip 打包步骤 |
| `.github/workflows/release.yml` | Create | tag 触发：交叉编译→GitHub Release→npm publish |
| `npm/vibeknow/package.json` | Create | 占位包，依赖转发到 vibeknow-cli |
| `.gitignore` | Modify | 加 `bin/`、`test.pdf` |

---

### Task 1: 更新 package.json

**Files:**
- Modify: `package.json`

- [ ] **Step 1: 更新 package.json**

将 `package.json` 的全部内容替换为：

```json
{
  "name": "vibeknow-cli",
  "version": "0.1.0",
  "description": "VibeKnow CLI — turn docs into videos",
  "license": "MIT",
  "bin": {
    "vibeknow": "scripts/run.js"
  },
  "scripts": {
    "postinstall": "node scripts/install.js"
  },
  "files": [
    "scripts/",
    "README.md",
    "LICENSE"
  ],
  "os": [
    "darwin",
    "linux",
    "win32"
  ],
  "cpu": [
    "x64",
    "arm64"
  ],
  "engines": {
    "node": ">=16"
  },
  "repository": {
    "type": "git",
    "url": "https://github.com/vibeknow/cli.git"
  },
  "homepage": "https://github.com/vibeknow/cli",
  "bugs": {
    "url": "https://github.com/vibeknow/cli/issues"
  },
  "keywords": [
    "cli",
    "video",
    "ai",
    "document",
    "tts"
  ]
}
```

- [ ] **Step 2: 验证 JSON 合法**

Run: `cat package.json | python3 -m json.tool > /dev/null && echo "OK"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add package.json
git commit -m "chore: update package.json for npm publish as vibeknow-cli"
```

---

### Task 2: 创建 scripts/install.js（postinstall 下载器）

**Files:**
- Create: `scripts/install.js`

- [ ] **Step 1: 创建 install.js**

创建 `scripts/install.js`，内容如下：

```javascript
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
  // npx sets npm_config_prefix to a temp directory
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

  // Already installed
  if (fs.existsSync(binPath)) {
    return;
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

  // Extract
  if (platform.ext === '.tar.gz') {
    extractTarGz(tmpArchive, tmpDir);
  } else {
    extractZip(tmpArchive, tmpDir);
  }

  // Find the binary inside extracted directory
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

  // Cleanup
  fs.rmSync(tmpDir, { recursive: true, force: true });
  console.log(`[vibeknow] installed successfully.`);
}

// Export for reuse in run.js
module.exports = { install, PLATFORM_MAP, getVersion, archiveName, downloadUrls, download, extractTarGz, extractZip };

if (require.main === module) {
  install();
}
```

- [ ] **Step 2: 验证语法**

Run: `node -c scripts/install.js`
Expected: 无输出（语法正确）

- [ ] **Step 3: Commit**

```bash
git add scripts/install.js
git commit -m "feat: add install.js — postinstall binary downloader from GitHub Releases"
```

---

### Task 3: 重写 scripts/run.js（launcher shim）

**Files:**
- Rewrite: `scripts/run.js`

- [ ] **Step 1: 重写 run.js**

将 `scripts/run.js` 的全部内容替换为：

```javascript
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
```

- [ ] **Step 2: 验证语法**

Run: `node -c scripts/run.js`
Expected: 无输出（语法正确）

- [ ] **Step 3: Commit**

```bash
git add scripts/run.js
git commit -m "feat: rewrite run.js — launcher with on-demand binary download for npx"
```

---

### Task 4: 删除旧脚本

**Files:**
- Delete: `scripts/npm-postinstall.js`
- Delete: `scripts/npm-launcher.js`

- [ ] **Step 1: 删除旧文件**

```bash
git rm scripts/npm-postinstall.js scripts/npm-launcher.js
```

- [ ] **Step 2: Commit**

```bash
git commit -m "chore: remove old npm-postinstall.js and npm-launcher.js"
```

---

### Task 5: 改造 build.sh（增加打包步骤）

**Files:**
- Modify: `build.sh`

- [ ] **Step 1: 重写 build.sh**

将 `build.sh` 的全部内容替换为：

```bash
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
```

- [ ] **Step 2: 本地测试构建（仅当前平台验证脚本能运行）**

Run: `VERSION=0.1.0 bash build.sh`
Expected: `dist/` 下产生 5 个目录和 5 个压缩包（`.tar.gz` × 4 + `.zip` × 1）

- [ ] **Step 3: 验证压缩包内部结构**

Run: `tar -tzf dist/vibeknow-cli-0.1.0-darwin-arm64.tar.gz`
Expected:
```
vibeknow-cli-0.1.0-darwin-arm64/
vibeknow-cli-0.1.0-darwin-arm64/vibeknow
```

- [ ] **Step 4: 清理并 commit**

```bash
rm -rf dist/
git add build.sh
git commit -m "feat: build.sh — add tar.gz/zip packaging for GitHub Releases"
```

---

### Task 6: 更新 .gitignore

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: 追加新条目**

在 `.gitignore` 末尾追加：

```
bin/
test.pdf
```

- [ ] **Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: gitignore bin/ and test.pdf"
```

---

### Task 7: 创建 GitHub Actions release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: 创建 release.yml**

创建 `.github/workflows/release.yml`，内容如下：

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Build all platforms
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          VERSION="$VERSION" bash build.sh

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: dist
          path: |
            dist/*.tar.gz
            dist/*.zip

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Download artifacts
        uses: actions/download-artifact@v4
        with:
          name: dist
          path: dist

      - name: Create GitHub Release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release create "$GITHUB_REF_NAME" \
            --title "$GITHUB_REF_NAME" \
            --generate-notes \
            dist/*.tar.gz dist/*.zip

  publish-npm:
    needs: release
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          registry-url: 'https://registry.npmjs.org'

      - name: Publish vibeknow-cli
        run: npm publish --access public
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}

      - name: Publish vibeknow (redirect package)
        working-directory: npm/vibeknow
        run: npm publish --access public
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

- [ ] **Step 2: 验证 YAML 语法**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "OK"`
Expected: `OK`（如果没有 pyyaml，跳过此步）

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow — build, GitHub Release, npm publish on tag"
```

---

### Task 8: 创建 vibeknow 占位包

**Files:**
- Create: `npm/vibeknow/package.json`

- [ ] **Step 1: 创建目录和 package.json**

创建 `npm/vibeknow/package.json`，内容如下：

```json
{
  "name": "vibeknow",
  "version": "0.0.1",
  "description": "This package has moved to vibeknow-cli. Install with: npm i -g vibeknow-cli",
  "license": "MIT",
  "dependencies": {
    "vibeknow-cli": "*"
  },
  "bin": {
    "vibeknow": "./node_modules/vibeknow-cli/scripts/run.js"
  },
  "repository": {
    "type": "git",
    "url": "https://github.com/vibeknow/cli.git"
  },
  "keywords": [
    "cli",
    "video",
    "ai"
  ]
}
```

- [ ] **Step 2: Commit**

```bash
git add npm/vibeknow/package.json
git commit -m "chore: add vibeknow redirect package"
```

---

### Task 9: 端到端验证

- [ ] **Step 1: 验证 npm pack 只包含预期文件**

Run: `npm pack --dry-run 2>&1`
Expected: 只包含 `package.json`、`scripts/install.js`、`scripts/run.js`、`README.md`、`LICENSE`。不应包含 `dist/`、`.go` 文件、`tests/` 等。

- [ ] **Step 2: 验证 build.sh 产生正确的产物**

Run: `VERSION=0.1.0 bash build.sh && ls dist/*.tar.gz dist/*.zip`
Expected:
```
dist/vibeknow-cli-0.1.0-darwin-amd64.tar.gz
dist/vibeknow-cli-0.1.0-darwin-arm64.tar.gz
dist/vibeknow-cli-0.1.0-linux-amd64.tar.gz
dist/vibeknow-cli-0.1.0-linux-arm64.tar.gz
dist/vibeknow-cli-0.1.0-windows-amd64.zip
```

- [ ] **Step 3: 验证 install.js 语法和导出**

Run: `node -e "const m = require('./scripts/install.js'); console.log(typeof m.install, typeof m.PLATFORM_MAP)"`
Expected: `function object`

- [ ] **Step 4: 验证 run.js 语法**

Run: `node -c scripts/run.js`
Expected: 无输出

- [ ] **Step 5: 清理 dist**

Run: `rm -rf dist/`

---

## 发布前人工检查清单

以下步骤需要手动完成，不在自动化范围内：

1. **npm 登录**: `npm login`（使用 vectorfunc 账号）
2. **设置 GitHub Secret**: 在 repo Settings → Secrets 中添加 `NPM_TOKEN`
3. **npmmirror 收录**: 首次发布后在 npmmirror 申请 binary mirror 收录（可延后）
4. **首次发布**: `git tag v0.1.0 && git push origin v0.1.0` 触发 CI
5. **验证安装**: `npm install -g vibeknow-cli && vibeknow --version`
