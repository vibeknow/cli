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
- **实时阶段进度** —— SSE 流式推送 4 阶段进度（大纲 → 配音 → 渲染 → 发布），人类看进度条、机器读 NDJSON
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

> **二进制怎么来的**：`vibeknow-cli` 把各平台的 Go 二进制打成独立的 npm 包（`vibeknow-cli-darwin-arm64` 等）放在 `optionalDependencies` 里，npm 按 `os`/`cpu` 只装匹配当前机器的那一个。所以二进制走的是**你已经在用的那个 registry**——公共镜像也好、公司内网代理也好——不需要额外一个主机可达。
>
> 如果用了 `--no-optional` 之类的开关跳过了它，`postinstall` 会退回从 GitHub Releases 下载。内网环境下那一步大概率不通，重装时带上 `--include=optional` 即可。

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
#   "verification_uri": "https://vibeknow.com/account/device",
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

**由宿主拉起 CLI 的场景**（连接器平台、IDE 集成）想要的是一条阻塞命令，而不是上面的两段式：

```bash
vibeknow auth login --headless
# 立刻把 {"user_code", "verification_uri", "expires_in", "hint"} 打到 stdout，
# 然后原地轮询直到授权完成。不需要 TTY、不需要按回车，也不会自己开浏览器
# —— 由宿主读到 URL 后去打开。
```

待授权的设备码同时会落盘到配置目录，因此宿主在用户授权完成前把进程杀掉**也不会丢掉这次登录**：下一次 `vibeknow auth status` 会把 token 兑换完成。在等待期间，`auth status --output json` 会在 `"authenticated": false` 之外返回 `"pending_authorization": true`，用于区分"正在等用户授权"和"从没登录过"。`auth logout` 会清掉它。

## Agent 技能

[`./skills/`](./skills/) 目录包含三个采用开放 [Agent Skills](https://agentskills.io)
规范的技能，兼容 55+ AI Agent 运行时（Claude Code、Cursor、OpenCode、
GitHub Copilot、Gemini CLI 等）。

| 技能 | 描述 |
|------|------|
| `vibeknow-core` | Profile 配置、认证管理、环境诊断、凭证配置 |
| `vibeknow-create` | 端到端视频生成：`create` 命令、`video status/wait/download`、音色选择、异步工作流 |
| `vibeknow-doc` | 文档上传（文件 + URL）、解析状态轮询、文档检索 |

### 安装

```bash
npx skills add vibeknow/cli             # 全部三个，安装到当前项目
npx skills add vibeknow/cli -g          # 全局安装（所有项目可用）
npx skills add vibeknow/cli --skill vibeknow-create   # 单独安装某一个
```

自动识别本机已装的 Agent 运行时并把技能 symlink 到对应目录。详见
[skills.sh](https://skills.sh)。

每个技能采用 `SKILL.md` + `references/` 结构，包含触发/跳过条件、命令配方
和按 exit code 驱动的错误处理。

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

# 先列出音色，两列都可以传给 --voice（# 或 SPEECH_VOICE_ID）
vibeknow voice list
vibeknow create --from report.pdf --voice 1

# 复用已上传的文档（doc_id 必须配上它的 --kb-id）
vibeknow create --from <doc_id> --kb-id <kb_id>

# 手上是一段文字而不是文件——对话里粘贴的正文用这个
vibeknow create --text "知识管理的三个常见误区…"
vibeknow create --from - --script-lock <<'EOF'          # 照着原文念
大家好。今天我只讲一件事……
EOF

# 自定义 prompt
vibeknow create --from data.csv --prompt "制作一个两分钟的讲解视频"

# 套用已存好的风格预设（~/.config/vibeknow/presets/brand.yaml）
vibeknow create --from deck.pdf --preset brand
vibeknow create --from deck.pdf --preset brand --aspect vertical   # 命令行上给的优先

# 异步模式 —— 提交、确认任务已起跑，然后断开
vibeknow create --from doc.pdf --async

# 分段跟进，每段都短到不会被调用方自己的超时掐断。
# 退出码 6 且 reason 为 "wait_budget_expired" = 继续等；退出码 0 = 真的完成了。
vibeknow video wait --for 90s --output json
vibeknow video wait          # 自动接上最近一次的任务
```

**预设**是一个 YAML 文件，装着你反复用的那组风格参数——模式、画幅、主题、
音色、语言、背景音乐、数字人位置。它只提供默认值：命令行上同时给出的参数
一律优先。它不能携带 `--export` / `--yes` / `--confirm`，所以打开别人给的
预设文件永远不会替你批准一次扣费。完整约定见 [AGENTS.md](AGENTS.md)。

`--async` 在后端确认任务已起跑后返回（秒级），而不是等视频渲染完（分钟级）；
CLI 断开后渲染继续在服务端进行。参数不合法、积分不足这类当场被拒的情况，
`--async` 会自己报错并以非 0 退出，所以只要打印出了 `task_id`，任务就是真的跑起来了。

### 找回一个任务

每次生成都由 `(task_id, session_id)` 这一对标识，`create` 会把它记在本地，
所以你不必自己保存：

```bash
vk jobs list                  # 本机发起过的所有任务，最新在前
vk jobs list --active         # 只看还没跑完的
vk video wait                 # 接上最近一次
vk video wait 42              # 或指定 task，session 由本机记录补齐
vk jobs prune --terminal      # 清掉已完成的记录
```

显式传 `--session-id` 依然可用，且优先级高于本地记录。

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
vibeknow video wait <task_id>

# 下载已渲染的视频（文件路径用 --dest，--output 统一表示输出格式）
vibeknow video download <task_id> --dest ./final.mp4
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

$ vk video download 42 --session-id sess_xxx --dest sess_xxx.mp4
output=sess_xxx.mp4
```

或者一键搞定：`vk create --from ... --export --yes`。

### 只改一句话，不用重做整支视频

```
$ vk video script 42                     # 免费：逐幕读出讲了什么
[3] 结论  (4.5s)
增长主要来自海外市场。

$ vk video edit 42 --scene 3 --script "增长几乎全部来自海外市场。"
```

以前一句话说错，只能重跑一次 `create`，全价。`video edit` 只替换一幕的讲稿
并重生成这一幕；加 `--script-only` 则只重生配音，更便宜。

它计费，所以走与 `video export` 相同的确认闸门——并且会同时给出现有措辞和
拟改措辞，因为你要同意的是这个差异。**不可撤销**；已渲染的 MP4 也不会被撤下，
`video download` 在你重新导出之前仍返回旧讲稿的版本。

### 把字幕调到能看清

```
$ vk subtitle presets
#  NAME   LOOK
1  白字·黑底  text #ffffff · plate rgba(8,8,12,0.68) · no outline
2  白字·黑边  text #ffffff · no plate · outline 3px rgba(0,0,0,0.92) · Noto Sans SC 600

$ vk video set 42 --subtitle-preset 2 --subtitle-size 52
```

字幕能不能看清，取决于几个字段的**组合**而不是单个设置——底板上的描边看不见，
描边下的底板发糊。预设是设计侧给的成套观感，每一套都带齐了它需要的全部字段。
单独的参数仍然叠加在预设之上，所以上面这条的意思是"就这套观感，但字大一点"。

`vk subtitle fonts` 列出允许的字体家族——在此之前这份清单根本无从得知。

### 选择视频模式

```bash
vk create --from deck.pdf  --mode replica   # PPT 讲解（逐页还原）
vk create --from post.md   --mode image --pages 8   # 图解视频（讲稿逐页 AI 生图）
vk create --from notes.md  --mode handdraw  # 手绘动画（中段长时间无进度属正常）
vk create --from <src>     --aspect vertical --bgm
```

风格与成片语言可叠加在任意 pipeline 模式上：

```bash
vk theme list --mode image                   # 查看该模式的风格目录
vk create --from post.md --mode image --theme <theme_id>
vk create --from post.md --language en-US    # 讲稿 + 配音语言
```

### 加数字人主讲

```bash
vk avatar list                               # 公模 sys_<id> + 本人训练的 ua_<id>
vk create --from deck.pdf --avatar sys_7 --voice <它的 VOICE_ID>
vk create --from deck.pdf --avatar ua_12 --avatar-position bottom-right --avatar-size 300
```

`--mode handdraw` 与 `--engine agent` 下不可用。若 `video export` 因数字人
幕失败被拒，执行 `vk video avatar-retry`（不重复扣费），完成后再导出。

`--script-lock`（原稿锁定）直接用文档原文当讲稿、跳过写稿，且可叠加在任意模式上
（它取代了原来的 `--mode script`，旧写法仍可用但会告警）：

```bash
vk create --from talk.docx --script-lock                 # 用原文做旁白
vk create --from talk.docx --mode image --script-lock    # 用原文做旁白 + 逐页生图
```

### 选择生成引擎（可选）

```bash
vk create --from <src> --engine agent       # v=2 agent 引擎（与前端选项对齐）
vk create --from <src> --engine pipeline    # v=3 pipeline（默认）
```

### 清理累积的知识库

```bash
vk kb list --output json --size 5             # 看下都有啥
vk kb prune --pattern 'vibeknow-cli-*'        # 试运行（默认）
vk kb prune --pattern 'vibeknow-cli-*' --yes  # 真正删除
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
vibeknow voice list --output json | jq '.templates[0].name'
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
