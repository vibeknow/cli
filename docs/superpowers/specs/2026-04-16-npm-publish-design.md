# vibeknow-cli npm 发布方案设计

## 概述

将 vibeknow-cli 发布到 npm，采用飞书 CLI（@larksuite/cli）相同的架构：npm 包只含 JS 脚本，postinstall 时从 GitHub Releases 下载对应平台的 Go 二进制文件，国内回退 npmmirror。

## 决策记录

| 决策项 | 结论 |
|--------|------|
| npm 包名 | `vibeknow-cli`（主包）+ `vibeknow`（占位转发包） |
| 版本号 | `0.1.0` |
| 二进制托管 | GitHub Releases（主源）+ npmmirror（国内回退） |
| 产物格式 | tar.gz（macOS/Linux）+ zip（Windows） |
| npx 支持 | 支持 — postinstall 跳过，launcher 按需下载 |
| bin 命令名 | `vibeknow` |
| 参考实现 | @larksuite/cli（飞书 CLI） |

## 架构

```
npm install -g vibeknow-cli
  │
  ├─ npm 下载主包（仅 JS 脚本，~10KB）
  │
  └─ postinstall → scripts/install.js
       ├─ 检测平台 + 架构
       ├─ npx 环境 → 跳过下载（首次运行时按需下载）
       ├─ 下载: GitHub Releases → 失败回退 npmmirror
       ├─ 解压 tar.gz/zip → bin/vibeknow
       └─ chmod 755

vibeknow create ...
  │
  └─ scripts/run.js
       ├─ 查找 bin/vibeknow
       ├─ 不存在 → 按需下载（npx 场景）
       └─ execFileSync(bin, args, {stdio: 'inherit'})
```

## 文件命名与 URL 规则

### GitHub Releases 产物

| 平台 | 文件名 |
|------|--------|
| macOS Intel | `vibeknow-cli-{VERSION}-darwin-amd64.tar.gz` |
| macOS ARM | `vibeknow-cli-{VERSION}-darwin-arm64.tar.gz` |
| Linux x64 | `vibeknow-cli-{VERSION}-linux-amd64.tar.gz` |
| Linux ARM | `vibeknow-cli-{VERSION}-linux-arm64.tar.gz` |
| Windows x64 | `vibeknow-cli-{VERSION}-windows-amd64.zip` |

### 压缩包内部结构

Unix (tar.gz):
```
vibeknow-cli-{VERSION}-{os}-{arch}/
  └─ vibeknow
```

Windows (zip):
```
vibeknow-cli-{VERSION}-windows-amd64/
  └─ vibeknow.exe
```

### URL 模板

```
主源:  https://github.com/vibeknow/cli/releases/download/v{VERSION}/{ARCHIVE}
回退:  https://registry.npmmirror.com/-/binary/vibeknow-cli/v{VERSION}/{ARCHIVE}
```

### 平台映射（Node.js → Go）

| process.platform | process.arch | Go OS | Go ARCH |
|---|---|---|---|
| darwin | arm64 | darwin | arm64 |
| darwin | x64 | darwin | amd64 |
| linux | x64 | linux | amd64 |
| linux | arm64 | linux | arm64 |
| win32 | x64 | windows | amd64 |

不在映射表中的组合报错退出。

## scripts/install.js（postinstall 下载器）

零依赖纯 Node.js 脚本。流程：

1. 检测 npx 环境（`npm_config_prefix` 包含临时路径）→ 跳过下载，打印提示
2. 从 `package.json` 读取 version
3. 检测 `process.platform` + `process.arch` → 查映射表，不支持则 exit(1)
4. 构造 archive 文件名和下载 URL
5. 用 `child_process.execSync` 调 curl 下载（`-fSL --retry 3`）
   - 先尝试 GitHub Releases
   - 失败 → 尝试 npmmirror
   - 都失败 → 打印错误（含手动下载 URL），exit(1)
6. 解压：tar.gz 用 `tar -xzf`，zip 用 PowerShell `Expand-Archive`
7. 移动二进制到 `bin/vibeknow`（Windows: `bin/vibeknow.exe`）
8. `chmod 755`（Unix）
9. 清理临时下载文件

设计要点：
- 用 curl 而非 Node.js https 模块 — 简单，自动处理 302 重定向，macOS/Linux/Windows 均预装
- 零 npm 依赖 — 避免循环依赖问题
- 静默模式 — 正常只打印一行下载进度

## scripts/run.js（launcher shim）

注册为 `bin.vibeknow`，用户执行 `vibeknow` 时调用此脚本。

1. 构造二进制路径 `path.join(__dirname, '..', 'bin', 'vibeknow')`（Windows 加 `.exe`）
2. 检查二进制是否存在
   - 不存在 → 调用与 install.js 相同的下载逻辑（覆盖 npx 首次运行场景）
3. `execFileSync(bin, process.argv.slice(2), { stdio: 'inherit' })`
4. `process.exit(r.status ?? 1)`

## build.sh 改造

在现有交叉编译基础上，增加打包步骤：

```bash
# 现有：编译裸二进制到 dist/
GOOS=$OS GOARCH=$ARCH go build -ldflags "$LDFLAGS" -o "$out" .

# 新增：打包
# Unix → tar.gz
tar -czf "dist/vibeknow-cli-${VERSION}-${OS}-${ARCH}.tar.gz" \
    -C dist "vibeknow-cli-${VERSION}-${OS}-${ARCH}"

# Windows → zip
zip -j "dist/vibeknow-cli-${VERSION}-windows-amd64.zip" \
    "dist/vibeknow-cli-${VERSION}-windows-amd64/vibeknow.exe"
```

## GitHub Actions Release Workflow

文件：`.github/workflows/release.yml`

触发条件：push tag `v*`

全部在 ubuntu-latest 上交叉编译（`CGO_ENABLED=0` 时 Go 交叉编译无需目标平台），节省 CI 分钟数。

```
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - checkout
      - setup-go 1.25
      - 运行 build.sh，编译全部 5 个平台 + 打包 tar.gz/zip
      - upload-artifact（所有压缩包）

  release:
    needs: build
    steps:
      - 下载所有 artifact
      - gh release create v{VERSION}，上传所有压缩包

  publish-npm:
    needs: release
    steps:
      - checkout
      - setup-node 20
      - npm publish（vibeknow-cli）
      - npm publish（vibeknow 占位包）
    env:
      NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

## package.json 更新

```json
{
  "name": "vibeknow-cli",
  "version": "0.1.0",
  "description": "VibeKnow CLI — turn docs into videos",
  "license": "MIT",
  "bin": { "vibeknow": "scripts/run.js" },
  "scripts": { "postinstall": "node scripts/install.js" },
  "files": ["scripts/", "README.md", "LICENSE"],
  "os": ["darwin", "linux", "win32"],
  "cpu": ["x64", "arm64"],
  "engines": { "node": ">=16" },
  "repository": { "type": "git", "url": "https://github.com/vibeknow/cli.git" },
  "homepage": "https://github.com/vibeknow/cli",
  "bugs": { "url": "https://github.com/vibeknow/cli/issues" },
  "keywords": ["cli", "video", "ai", "document", "tts"]
}
```

关键变化：
- name: `@vibeknow/cli` → `vibeknow-cli`
- version: `0.1.0-p0` → `0.1.0`
- files: 移除 `dist/`
- 新增: repository, homepage, bugs, keywords, os, cpu

## vibeknow 占位包

目录：`npm/vibeknow/package.json`

```json
{
  "name": "vibeknow",
  "version": "0.0.1",
  "description": "This package has moved to vibeknow-cli. Install with: npm i -g vibeknow-cli",
  "license": "MIT",
  "dependencies": { "vibeknow-cli": "*" },
  "bin": { "vibeknow": "./node_modules/vibeknow-cli/scripts/run.js" },
  "repository": { "type": "git", "url": "https://github.com/vibeknow/cli.git" },
  "keywords": ["cli", "video", "ai"]
}
```

## .gitignore 更新

新增：
```
bin/
test.pdf
```

## 改动文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `package.json` | 修改 | 更新名称、版本、字段、移除 dist |
| `scripts/install.js` | 新建 | postinstall 远程下载器 |
| `scripts/run.js` | 重写 | 加按需下载逻辑 |
| `scripts/npm-postinstall.js` | 删除 | 被 install.js 取代 |
| `scripts/npm-launcher.js` | 删除 | 被 run.js 取代 |
| `build.sh` | 修改 | 增加 tar.gz/zip 打包步骤 |
| `.github/workflows/release.yml` | 新建 | CI 自动构建+发布 |
| `npm/vibeknow/package.json` | 新建 | 占位包 |
| `.gitignore` | 修改 | 加 bin/、test.pdf |
