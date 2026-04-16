# vibeknow-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.25-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/@vibeknow/cli.svg)](https://www.npmjs.com/package/@vibeknow/cli)

[中文版](./README.zh.md) | [English](./README.md)

[VibeKnow](https://vibeknow.com) 官方命令行工具 —— 为人类和 AI Agent 打造。在命令行中将文档、链接、文件一键转为专业视频。一条命令，零剪辑技能。

[安装](#安装与快速开始) · [AI Agent 技能](#agent-技能) · [认证](#认证) · [命令](#命令参考) · [进阶](#进阶用法) · [贡献](#贡献)

## 为什么选 vibeknow-cli？

- **一条命令，完整视频** —— `vibeknow create --from report.pdf` 自动完成文档解析、脚本生成、配音、画面设计、渲染和打包
- **Agent 原生设计** —— 内置 3 个结构化 [Skills](./skills/)，兼容 Claude Code、Cursor 等 AI 工具，Agent 无需额外配置即可生成视频
- **实时阶段进度** —— SSE 流式推送 6 阶段进度（解析 → 大纲 → 配音 → 渲染 → 发布 → 建议），人类看进度条、机器读 NDJSON
- **多服务架构** —— 直连 4 个后端服务（vectoria / figlens / vibeknow / account），支持按服务配置 endpoint 和独立认证
- **开源，零门槛** —— MIT 许可，`npm install` 即可使用
- **安全可控** —— OS 原生 keychain 存储凭证、ANSI 转义字符清理、verbose 日志自动脱敏、非生产 endpoint 信任边界保护

## 功能一览

| 分类 | 能力 |
|------|------|
| **创建** | 从文件、URL 或 doc_id 一键生成视频；自定义 prompt；选择音色；异步模式 |
| **文档** | 上传文件/URL 到 vectoria、轮询解析状态、获取文档详情 |
| **视频** | 查看任务状态、流式实时进度（SSE）、下载已导出视频（签名 URL） |
| **音色** | 列出可用音色模板（分类、标签、预览地址） |
| **认证** | Token 认证（环境变量）、whoami、凭证状态查看、登出；Device Flow 计划在 v1 支持 |
| **Profile** | 多环境 profile（prod/staging/dev）、按服务覆盖 endpoint、信任边界 |
| **配置** | 全局 key-value 配置存储，跨会话持久化 |
| **诊断** | 环境自检：配置目录、keychain、locale、4 个 endpoint 并发可达性 + API 版本探测 |
| **Raw API** | 逃生舱：`vibeknow api call --service figlens --path /v1/tasks` 直调任意后端接口 |

## 安装与快速开始

### 环境要求

- Node.js（`npm`/`npx`）用于分发
- Go `v1.25`+（仅从源码构建时需要）

### 快速开始（人类用户）

> **AI 助手请注意：** 如果你是在帮用户安装，请直接跳到 [快速开始（AI Agent）](#快速开始ai-agent)。

#### 安装

**方式 1 —— npm 安装（推荐）：**

```bash
npm install -g @vibeknow/cli
```

**方式 2 —— 从源码安装：**

```bash
git clone https://github.com/shiliu-ai/vibeknow-cli.git
cd vibeknow-cli
make install
```

#### 配置

```bash
# 1. 添加 profile
vibeknow profile add prod \
  --endpoint-account https://account.vibeknow.com \
  --endpoint-vectoria https://vectoria.vibeknow.com \
  --endpoint-figlens https://figlens.vibeknow.com \
  --endpoint-vibeknow https://api.vibeknow.com \
  --credential-ref vibeknow.prod

# 2. 设置 token（从 Web 端登录后获取）
export VIBEKNOW_TOKEN="your-jwt-token-here"

# 3. 设置 vectoria API key
export VECTORIA_API_KEY="your-vectoria-api-key"

# 4. 验证
vibeknow auth whoami
vibeknow doctor
```

#### 生成第一个视频

```bash
vibeknow create --from https://example.com/article --voice t260312180132IV37e611
```

### 快速开始（AI Agent）

> 每一步执行后请验证再继续。

**第 1 步 —— 安装**

```bash
npm install -g @vibeknow/cli
```

**第 2 步 —— 配置 profile**（根据你的环境调整 endpoint）

```bash
vibeknow profile add prod \
  --endpoint-account https://account.vibeknow.com \
  --endpoint-vectoria https://vectoria.vibeknow.com \
  --endpoint-figlens https://figlens.vibeknow.com \
  --endpoint-vibeknow https://api.vibeknow.com \
  --credential-ref vibeknow.prod
```

**第 3 步 —— 设置凭证**（从 Web 端或 CI secrets 获取）

```bash
export VIBEKNOW_TOKEN="<jwt>"
export VECTORIA_API_KEY="<key>"
```

**第 4 步 —— 验证**

```bash
vibeknow auth whoami
vibeknow voice list
```

## Agent 技能

| 技能 | 描述 |
|------|------|
| `vibeknow-core` | Profile 配置、认证管理、环境诊断、凭证配置 |
| `vibeknow-create` | 端到端视频生成：`create` 命令、`video status/wait/download`、音色选择、异步工作流 |
| `vibeknow-doc` | 文档上传（文件 + URL）、解析状态轮询、文档查询 |

技能文件位于 [`./skills/`](./skills/)，采用 `SKILL.md` + `references/` 结构。每个技能包含触发/跳过条件、命令配方和错误处理指南。

## 认证

vibeknow-cli 目前支持基于 Token 的环境变量认证：

| 方式 | 用法 |
|------|------|
| `VIBEKNOW_TOKEN` 环境变量 | account / figlens / vibeknow 服务的 JWT token |
| `VECTORIA_API_KEY` 环境变量 | vectoria 文档服务的 API key |
| Keychain 存储 | 通过 profile 中的 `credential_ref` 将 token 持久化到 OS keychain |

```bash
# 查看当前登录身份
vibeknow auth whoami

# 查看凭证来源（env / keychain / 无）
vibeknow auth status

# 清除存储的凭证
vibeknow auth logout
```

> **v1 计划：** 交互式 `auth login`，支持 OAuth Device Flow + Personal Access Token（PAT）。详见[设计文档](./docs/superpowers/specs/2026-04-15-vibeknow-cli-design.md) §4.2。

## 命令参考

### 核心命令

```bash
# 从 URL 生成视频
vibeknow create --from https://example.com/article

# 从本地文件生成，指定音色
vibeknow create --from report.pdf --voice t260312180132IV37e611

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

### 音色模板

```bash
# 列出所有可用音色
vibeknow voice list
```

### Profile 管理

```bash
# 添加开发 profile，覆盖本地 endpoint
vibeknow profile add dev \
  --endpoint-figlens http://localhost:20067 \
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
vibeknow api call --service figlens --method GET --path /v1/tasks?offset=0&limit=10

# POST + JSON body
vibeknow api call --service vectoria --method POST --path /v1/knowledgebases --body '{"name":"my-kb"}'

# POST + body 从文件读取
vibeknow api call --service figlens --method POST --path /v1/tasks/init --body @request.json
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
| `VIBEKNOW_TOKEN` | JWT token（最高优先级凭证来源） |
| `VECTORIA_API_KEY` | vectoria 服务 API key |
| `VIBEKNOW_CONFIG_HOME` | 覆盖配置目录（默认：`~/.config/vibeknow`） |
| `VIBEKNOW_TRACE` | 设为 `1` 显示 trace ID 用于后端日志关联 |
| `VIBEKNOW_DEBUG` | 设为 `1` 打印请求/响应 body（谨慎使用） |

### 环境诊断

```bash
# 完整环境检查（含 4 个服务的并发健康探测 + API 版本检查）
vibeknow doctor
```

## 架构

vibeknow-cli 采用**多 endpoint 直连**架构 —— CLI 独立对接 4 个后端服务：

| 服务 | 用途 | 认证 |
|------|------|------|
| **account** | 用户认证、个人信息 | JWT（`X-Authorization-Token`） |
| **vectoria** | 文档解析、RAG | API Key（`X-API-Key`） |
| **figlens** | 视频 pipeline、SSE 流 | JWT（`X-Authorization-Token`） |
| **vibeknow** | 计费、音色管理 | JWT（`X-Authorization-Token`） |

`create` 命令跨服务编排：vectoria（上传）→ figlens（SSE 生成）→ figlens（导出 + 下载）。

完整设计见[设计文档](./docs/superpowers/specs/2026-04-15-vibeknow-cli-design.md)。

## 贡献

欢迎在 [github.com/shiliu-ai/vibeknow-cli](https://github.com/shiliu-ai/vibeknow-cli) 提 issue 和 PR。

```bash
# 开发环境
git clone https://github.com/shiliu-ai/vibeknow-cli.git
cd vibeknow-cli
make build    # 编译
make test     # 运行全部测试（含 race detector）
make lint     # go vet
```

## 许可证

[MIT](./LICENSE)
