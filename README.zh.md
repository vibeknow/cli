# vibeknow-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.25-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/vibeknow-cli.svg)](https://www.npmjs.com/package/vibeknow-cli)

[中文版](./README.zh.md) | [English](./README.md)

[VibeKnow](https://vibeknow.com) 官方命令行工具 —— 为人类和 AI Agent 打造。在命令行中将文档、链接、文件一键转为专业视频。一条命令，零剪辑技能。

[安装](#安装与快速开始) · [AI Agent 技能](#agent-技能) · [认证](#认证) · [命令](#命令参考) · [进阶](#进阶用法) · [贡献](#贡献)

## 为什么选 vibeknow-cli？

- **一条命令，完整视频** —— `vibeknow create --from report.pdf` 自动完成文档解析、脚本生成、配音、画面设计、渲染和打包
- **Agent 原生设计** —— 内置 3 个结构化 [Skills](./skills/)，兼容 Claude Code、Cursor 等 AI 工具，Agent 无需额外配置即可生成视频
- **实时阶段进度** —— SSE 流式推送 6 阶段进度（解析 → 大纲 → 配音 → 渲染 → 发布 → 建议），人类看进度条、机器读 NDJSON
- **多服务架构** —— 连接多个后端服务，支持按服务配置 endpoint 和独立认证
- **开源，零门槛** —— MIT 许可，`npm install` 即可使用
- **安全可控** —— OS 原生 keychain 存储凭证、ANSI 转义字符清理、verbose 日志自动脱敏、非生产 endpoint 信任边界保护

## 功能一览

| 分类 | 能力 |
|------|------|
| **创建** | 从文件、URL 或 doc_id 一键生成视频；自定义 prompt；选择音色；异步模式 |
| **文档** | 上传文件/URL、轮询解析状态、获取文档详情 |
| **视频** | 查看任务状态、流式实时进度（SSE）、下载已导出视频 |
| **音色** | 列出可用音色模板（分类、标签、预览地址） |
| **认证** | 基于浏览器的 Device Flow 登录（`vibeknow init` / `vibeknow auth login`）；双阶段 Agent 流程（`--no-wait` / `--device-code`）；通过 stdin 提供 PAT；CI 使用 `VIBEKNOW_TOKEN` 环境变量 |
| **Profile** | 多环境 profile（prod/staging/dev）、按服务覆盖 endpoint、信任边界 |
| **配置** | 全局 key-value 配置存储，跨会话持久化 |
| **诊断** | 环境自检：配置目录、keychain、locale、endpoint 可达性检查 |
| **Raw API** | 逃生舱：`vibeknow api call` 直调任意后端接口 |

## 安装与快速开始

### 环境要求

- Node.js（`npm`/`npx`）用于分发
- Go `v1.25`+（仅从源码构建时需要）

### 快速开始（人类用户）

> **AI 助手请注意：** 如果你是在帮用户安装，请直接跳到 [快速开始（AI Agent）](#快速开始ai-agent)。

```bash
# 1. 安装
npm install -g vibeknow-cli

# 2. 登录 —— 自动创建默认 profile 并引导你完成浏览器认证
vibeknow init

# 3. 生成视频
vibeknow create --from https://example.com/article
```

就这么简单。`vibeknow init` 负责创建 profile、打开浏览器完成 Device Flow 认证，并把 token 存入系统密钥链。

**从源码安装**（只有需要自己构建 Go 二进制时才用）：

```bash
git clone https://github.com/vibeknow/cli.git
cd vibeknow-cli
make install
```

### 快速开始（AI Agent）

`vibeknow init` 需要 TTY，Agent 改用双阶段 Device Flow：由人类在浏览器中点击一次验证链接，之后 Agent 可全程无人值守。

```bash
# 1. 安装
npm install -g vibeknow-cli

# 2. 非阻塞地发起 device-code 流程 —— 输出 JSON，包含 verification_uri
#    和 device_code。Agent 提取这两个字段。
vibeknow auth login --no-wait
# 示例输出：
# {
#   "device_code":      "dc_2913bcc...",
#   "user_code":        "UWWA-R8KS",
#   "verification_uri": "https://beta.lab.shiliu.chat/account/device",
#   "expires_in":       900,
#   "hint":             "请访问 https://... 并输入验证码 UWWA-R8KS"
# }

# 3. Agent 把 verification_uri 展示给人类，由对方在浏览器中打开并授权。
#    （一个 token 一次人工交互，不是每次调用都要交互。）

# 4. 用第 2 步得到的 device_code 恢复轮询 —— 阻塞直到被授权。
vibeknow auth login --device-code dc_2913bcc...

# 5. Token 已写入系统密钥链，后续所有命令自动携带认证。
vibeknow auth whoami
vibeknow auth status --output json   # 可解析的登录态
vibeknow create --from report.pdf
```

CI / 容器环境如果已经持有 JWT，可以跳过 Device Flow —— 见下方 [环境变量](#环境变量)。

## Agent 技能

| 技能 | 描述 |
|------|------|
| `vibeknow-core` | Profile 配置、认证管理、环境诊断、凭证配置 |
| `vibeknow-create` | 端到端视频生成：`create` 命令、`video status/wait/download`、音色选择、异步工作流 |
| `vibeknow-doc` | 文档上传（文件 + URL）、解析状态轮询、文档检索 |

技能文件位于 [`./skills/`](./skills/)，采用 `SKILL.md` + `references/` 结构。每个技能包含触发/跳过条件、命令配方和错误处理指南。

## 认证

vibeknow-cli 提供三条认证路径，分别适配人类、AI Agent 和 CI 场景：

| 方式 | 使用场景 | 存储 |
|------|---------|------|
| `vibeknow init` / `vibeknow auth login` | 人类交互式首次设置（浏览器 Device Flow） | 系统密钥链 |
| `vibeknow auth login --no-wait` 配合 `--device-code <code>` | AI Agent —— 非阻塞发起，人类授权后恢复轮询 | 系统密钥链 |
| `VIBEKNOW_TOKEN=<jwt>` 环境变量 | CI 流水线、容器、短期脚本（绕过密钥链） | 无（单次调用） |
| `vibeknow auth login --with-token`（从 stdin 读取 PAT） | 脚本化安装 + 预签发 token | 系统密钥链 |

```bash
# 当前登录身份
vibeknow auth whoami

# Token 来源、profile、过期时间 —— Agent 可加 --output json
vibeknow auth status
vibeknow auth status --output json

# 清除已存凭证
vibeknow auth logout
```

## 命令参考

### 核心命令

```bash
# 从 URL 生成视频
vibeknow create --from https://example.com/article

# 先列出可用的 voice ID，再通过 --voice 传入
vibeknow voice list
vibeknow create --from report.pdf --voice <上面列出的 voice-id>

# 自定义 prompt
vibeknow create --from data.csv --prompt "制作一个两分钟的讲解视频"

# 异步模式 —— 立即获取 task ID，稍后查看
vibeknow create --from doc.pdf --async
vibeknow video wait <task_id> --session-id <session_id>
```

### 文档管理

```bash
# 上传文件（自动创建 KB、上传、轮询到完成）
vibeknow doc upload report.pdf

# 上传 URL
vibeknow doc upload --url https://example.com/page

# 查看文档状态
vibeknow doc get --kb-id <kb_id> --doc-id <doc_id>
```

### 视频任务

```bash
# 查看任务状态
vibeknow video status <task_id>

# 流式进度（阻塞直到完成）
vibeknow video wait <task_id> --session-id <session_id>

# 下载已渲染的视频
vibeknow video download <task_id> --session-id <session_id>
```

### 生成可分享的视频

```
$ vk create --from ./slides.pdf
…
share_url=https://vibeknow.com/share/tok_abc
hint: Render MP4 (several minutes, extra credits) — vk video export 42 --session-id sess_xxx
```

pipeline 跑完后进入**预览阶段**：`share_url` 是一个可直接在浏览器里
播放最终视频的分享页，拿到就能发给任何人。

### （可选）导出可下载的 MP4

```
$ vk video export 42 --session-id sess_xxx --yes
exporting: 72% — rendering frames
export complete

$ vk video download 42 --session-id sess_xxx
output=sess_xxx.mp4
```

或者一键搞定：`vk create --from ... --export --yes`。

### 选择视频模式

```bash
vk create --from deck.pdf  --mode replica   # PPT/PDF 逐页还原
vk create --from talk.docx --mode script    # 讲稿模式（用文档原文做旁白）
vk create --from <src>     --aspect vertical --bgm
```

### 音色模板

```bash
# 列出所有可用音色
vibeknow voice list
```

### Profile 管理

```bash
# 添加开发 profile，覆盖本地 endpoint
vibeknow profile add dev \
  --endpoint-figlens http://localhost:<port> \
  --credential-ref vibeknow.dev \
  --trust dev --is-production=false

# 切换 profile
vibeknow profile use prod

# 查看 profile 详情
vibeknow profile show

# 列出所有 profile
vibeknow profile list
```

### Raw API 访问

```bash
# 直接调用任意后端接口
vibeknow api call --service <service> --method GET --path /v1/<resource>

# POST + JSON body
vibeknow api call --service <service> --method POST --path /v1/<resource> --body '{"key":"value"}'

# POST + body 从文件读取
vibeknow api call --service <service> --method POST --path /v1/<resource> --body @request.json
```

## 进阶用法

### 输出格式

```bash
# 默认：人类友好的文本（TTY 下自动选择）
vibeknow voice list

# JSON 输出
vibeknow voice list --output json

# 管道友好（非 TTY 自动选择 json）
vibeknow voice list | jq '.list[0].name'
```

### 环境变量

| 变量 | 用途 |
|------|------|
| `VIBEKNOW_TOKEN` | 所有服务共用的 JWT token（最高优先级凭证来源） |
| `VIBEKNOW_CONFIG_HOME` | 覆盖配置目录（默认：`~/.config/vibeknow`） |
| `VIBEKNOW_TRACE` | 设为 `1` 显示 trace ID 用于调试 |
| `VIBEKNOW_DEBUG` | 设为 `1` 打印详细日志（谨慎使用） |

### 环境诊断

```bash
# 完整环境检查（配置、凭证、endpoint 可达性）
vibeknow doctor
```

## 架构

vibeknow-cli 采用**多 endpoint** 架构 —— CLI 连接多个后端服务，每个服务负责特定领域（认证、文档、视频 pipeline 等）。服务通过 profile 配置，支持按环境覆盖 endpoint。

`create` 命令编排完整流程：文档上传 → 视频生成（SSE）→ 导出 & 下载。

## 贡献

欢迎在 [github.com/vibeknow/cli](https://github.com/vibeknow/cli) 提 issue 和 PR。

```bash
# 开发环境
git clone https://github.com/vibeknow/cli.git
cd vibeknow-cli
make build    # 编译
make test     # 运行全部测试（含 race detector）
make lint     # go vet
```

## 许可证

[MIT](./LICENSE)
