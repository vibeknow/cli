# vibeknow-cli 设计文档

- **日期**：2026-04-15
- **状态**：Design, pending implementation plan
- **作者**：nullkey（与 Claude 协作 brainstorm）
- **参考实现**：[lark-cli](https://github.com/larksuite/cli)（MIT 许可，可直接借鉴代码）

## 1. 背景与目标

vibeknow 产品将文档/链接经 pipeline 自动生成视频。现有后端服务：

| 服务 | 仓库 | 职责 |
|---|---|---|
| go-vibeknow | `~/laoshen/go-vibeknow` | 业务系统 / 对外网关 |
| go-figlens | `~/laoshen/go-figlens` | 视频生成 pipeline |
| vectoria | `~/project/vectoria` | 文档解析 + RAG |
| go-account | `~/laoshen/go-account` | 用户注册登录 / Token 签发 |
| go-speech | `~/laoshen/go-speech` | 声音克隆 / TTS（内部） |

**目标**：构建一个 CLI 工具 `vibeknow`，同时服务三类用户：
- **终端用户 / Power User**：一条命令从素材生成视频。
- **AI Agent**（Claude Code、Cursor 等）：通过结构化命令 + Skills 自动化完成任务。
- **开发者 / 内部运维**：调试 pipeline、多环境切换、raw API 兜底。

**非目标**：
- 不做 Web UI，不做 GUI 客户端。
- 不做视频编辑 / 时间线调整，这些由 Web 端承担。

## 2. 关键决策概览

| 决策 | 选择 | 备选 | 理由 |
|---|---|---|---|
| 用户定位 | 三者兼顾（人 / Agent / 开发者） | 只服务其中一类 | 对齐 lark-cli 成功模式 |
| 技术栈 | Go + cobra + viper，npm 分发 | Python / TS / Rust | 与后端同语言、生态成熟、可大量复用 lark-cli 基础设施 |
| 命令架构 | 三层（Shortcuts / API / Raw） | 单层封装 | 同时满足"一条命令跑通"与"未封装 API 兜底" |
| Domain 切分 | E 架构骨架，A 阶段只填主链路 | 只做 A（无扩展位） | 保留扩展但控制初期工作量 |
| 认证 | 统一 token（go-account 签发），通过 go-vibeknow 网关转发至各后端 | 多服务直连 + 多 token | CLI 感知面最小；符合人/Agent 定位 |
| 多环境 | lark-cli 风格 profile（`~/.config/vibeknow/profiles.yaml`） | 环境变量 | 支持 dev/staging/prod 无缝切换 |
| 长耗时任务 | 默认同步 + 进度流；`--async` 切异步；`--output json` 时输出 NDJSON 事件流 | 纯同步 / 纯异步 | TTY 人类友好 + Agent 可流式解析 |
| Hero 命令 | `vibeknow create --from <source>`，复杂配置沉到 `project` | 参数全部平铺 | 单参数命令符合 Agent-Native；配置沉淀到后端可跨端共享 |
| AI Skills | 每个 domain 一个 Skill 包 | 一个大 Skill | Agent 按需加载，避免上下文爆炸 |

## 3. 整体架构

### 3.1 项目信息

- **仓库**：`~/laoshen/vibeknow-cli`
- **Go module**：`github.com/<org>/vibeknow-cli`（实际组织名待定）
- **npm 包**：`@vibeknow/cli`
- **二进制**：`vibeknow`

### 3.2 命令三层

1. **Shortcuts 层** — 面向人与 Agent 的"黄金路径"。封装多次后端调用 + 合理默认值。示例：`vibeknow create --from x.pdf`。
2. **API Commands 层** — 一一映射后端服务 API，参数贴近 proto/OpenAPI。示例：`vibeknow video.task.get --id xxx`。
3. **Raw 层** — 未封装 API 的兜底通道。示例：
   ```
   vibeknow api call --service figlens --method POST --path /v1/pipeline/run --body @x.json
   ```

### 3.3 Domain 切分

| Domain | 说明 | A 阶段填充 |
|---|---|---|
| `video` | 视频生成主流程（create / status / wait / cancel / download） | ✅ 全填 |
| `doc` | 素材文档（上传 / 解析 / 查看） | ✅ upload + get |
| `rag` | vectoria 知识库（query / ingest / collection 管理） | ✅ 只填 query |
| `voice` | go-speech：list / synth / clone / preview | 🔲 A 只填 `list` |
| `pipeline` | figlens pipeline 调试（stage 状态 / 中间产物 / 重跑） | 🔲 骨架占位 |
| `asset` | 模板、BGM、图片素材库 | 🔲 骨架占位 |
| `project` | 作品/项目配置（绑定默认模板、音色等） | ✅ create + use + list |
| `auth` | 对接 go-account | ✅ 全填 |
| `config` / `profile` / `doctor` / `update` / `completion` | 基础命令（借鉴 lark-cli） | ✅ 全填 |

### 3.4 Repo 目录结构

```
~/laoshen/vibeknow-cli/
├── cmd/                     # cobra 根 + 基础命令（auth/config/profile/doctor/update/completion）
├── internal/                # keychain/credential/output/client/selfupdate/...（vendor 或 fork 自 lark-cli）
├── shortcuts/               # 每 domain 一个包，export Shortcuts()
│   ├── register.go          # 根聚合，init() 里 append 所有 domain
│   ├── video/
│   ├── doc/
│   ├── rag/
│   ├── voice/
│   ├── project/
│   ├── pipeline/            # A: 空 Shortcuts()
│   └── asset/               # A: 空 Shortcuts()
├── api/                     # 每 domain 一个包，映射后端 API
├── raw/                     # `api call` 通用实现
├── client/                  # 后端 client SDK
│   ├── vibeknow/
│   ├── figlens/
│   ├── vectoria/
│   ├── account/
│   └── speech/
├── skills/                  # AI Agent Skills（每 domain 一个）
│   ├── vibeknow-core/
│   ├── vibeknow-create/
│   ├── vibeknow-doc/
│   ├── vibeknow-rag/
│   └── vibeknow-voice/
├── scripts/
├── tests/
├── main.go
├── go.mod
├── package.json
├── build.sh
├── Makefile
├── README.md
├── README.zh.md
├── CHANGELOG.md
└── LICENSE
```

**A 阶段最小命令集**（人话清单）：

```
vibeknow auth login / logout / whoami
vibeknow profile use / list / add
vibeknow config ...
vibeknow doc upload <file>  / doc get <id>
vibeknow rag query --collection <c> --q "..."
vibeknow voice list
vibeknow project create / use / list
vibeknow create --from <file|url> [--project x] [--voice x] [--async] [--output json]
vibeknow video status <id> / wait <id> / cancel <id> / download <id>
vibeknow api call ...                 # raw 兜底
vibeknow doctor / update / completion
```

### 3.5 与 lark-cli 的架构差异

- lark-cli 的 **API Commands 层**集中在 `cmd/api/` 下，由 `schema/` 生成（对应单一的飞书开放平台 schema）。
- vibeknow-cli 的 API Commands 层按 **domain 切**（`api/<domain>/`），因下游是 5 个独立服务，无统一 schema。
- 若未来做统一 OpenAPI 网关，可回归 lark-cli 的 `cmd/api/ + schema/` 模式，届时代码重构可控。

## 4. 认证与 Profile

### 4.1 认证流程

1. `vibeknow auth login` 调 go-account 签发 token。
   - A 阶段先实现其中一种方式（推荐 username/password 或 API key，二选一留给实现阶段决定）。
   - OAuth / SSO 留扩展位。
2. Token 存 OS keychain，复用 lark-cli 的 `internal/keychain` + `internal/credential`。
3. **所有后端服务都信任 go-account 签发的这一个 token**；CLI 每次请求携带。
4. 请求实际入口为 **go-vibeknow 网关**，由其反向代理到 figlens/vectoria/speech；CLI 只感知 `api_endpoint` 一个地址。
5. `vibeknow auth whoami` 调 go-account 验证并返回用户信息；`logout` 清 keychain。

### 4.2 Profile 模型

- 配置文件位置：`~/.config/vibeknow/profiles.yaml`（Windows 对应 `%AppData%\vibeknow\profiles.yaml`）。
- 每个 profile 字段：
  - `name`：profile 名（`dev` / `staging` / `prod` / 自定义）
  - `api_endpoint`：默认指向 go-vibeknow 网关
  - `credential_ref`：keychain 条目引用
  - `default_project`：可选，Hero 命令默认使用的 project
  - `service_overrides`（可选，仅开发者）：允许为特定服务指定直连 endpoint，用于"本地 figlens + 远端其它"这种调试场景。仅当 `VIBEKNOW_ALLOW_OVERRIDES=1` 或 profile 标记 `trust: dev` 时生效。
- 命令：`vibeknow profile use / add / list / remove / show`。

### 4.3 实施前置依赖（Prerequisites）

1. **go-vibeknow 充当聚合网关**：若当前形态不是这样，则需：
   - 方案 A：在 go-vibeknow 中补网关层转发 figlens/vectoria/speech。
   - 方案 B（fallback）：CLI 各服务直连，维护多个 endpoint，但仍统一使用 go-account 签发的 token。
2. **go-account 签发的 token 被所有下游服务识别**：若当前各服务各自鉴权，需要对齐统一身份。
3. **go-figlens 暴露阶段级 pipeline 状态**（见 §5.3）。

这三项在 implementation plan 阶段需要与各服务 owner 确认现状；若任何一项未就绪，CLI 对应能力需要降级。

## 5. 长耗时任务模型

### 5.1 任务生命周期

```
submit → queued → running(stage_a) → running(stage_b) → ... → succeeded | failed | cancelled
```

视频生成、voice clone、rag ingest 均复用此模型。

### 5.2 CLI 交互模式

默认：`vibeknow create --from x.pdf`

- **TTY + 默认输出**：同步挂起，彩色进度条展示当前 stage / 已耗时 / 预估剩余，结束打印视频 URL。
- **非 TTY 或 `--output json`**：逐行输出 NDJSON 事件：

  ```json
  {"ts":"...","event":"task.submitted","task_id":"t_123"}
  {"ts":"...","event":"stage.started","stage":"parse","task_id":"t_123"}
  {"ts":"...","event":"stage.progress","stage":"parse","percent":40,"task_id":"t_123"}
  {"ts":"...","event":"stage.succeeded","stage":"parse","task_id":"t_123"}
  {"ts":"...","event":"stage.started","stage":"outline","task_id":"t_123"}
  ...
  {"ts":"...","event":"task.succeeded","task_id":"t_123","video_url":"..."}
  ```

异步：`vibeknow create --from x.pdf --async`

- 立即返回 `task_id`。
- 配套：`vibeknow video status <id>` / `wait <id>` / `cancel <id>` / `download <id>`。
- `wait` 与同步模式共用同一段 streaming 代码。

### 5.3 后端传输协议

- 首选 **SSE**（HTTP 长连接、语义简单、跨网关友好、易断线重连）。
- Fallback **轮询**：SSE 不可用时 fallback 到 `GET /tasks/{id}`，可配置间隔。
- **对后端的要求**（依赖）：
  - `GET /tasks/{id}/events`：SSE 流，逐 stage 推送事件。
  - `GET /tasks/{id}`：一次性返回任务当前快照（含所有已发生 stage）。
  - 状态粒度至少到 stage（parse / outline / storyboard / tts / render 等）。

### 5.4 错误与退出码

`task.failed` event 必须包含：

```json
{"event":"task.failed","task_id":"...","failed_stage":"render","error_code":"...","error_message":"...","retryable":true}
```

Exit code 约定：

| Code | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 通用错误 |
| 2 | 参数错误 |
| 3 | 认证错误 |
| 4 | 任务失败但可重试 |
| 5 | 任务失败不可重试 |
| 130 | 用户中断（SIGINT） |

## 6. Hero 命令与 Project 模型

### 6.1 Hero Shortcut：`vibeknow create`

```
vibeknow create --from <file|url|doc_id>
vibeknow create --from x.pdf --project news-daily
vibeknow create --from x.pdf --voice v_abc --template news --duration 60s --aspect 9:16
vibeknow create --from x.pdf --async
vibeknow create --from x.pdf --output json
```

**`--from` 的解析逻辑**：

1. 若是本地路径（存在的文件）→ 自动 `doc upload` 后提交 pipeline。
2. 若是 URL（http/https 开头）→ 直接提交 pipeline（由 figlens 负责抓取）。
3. 若是 `doc_<id>` 格式 → 直接作为已存在的 doc_id 提交。

### 6.2 Project 模型

- `vibeknow project create news-daily --template news --voice v_abc --aspect 9:16 --duration 60s`
- `vibeknow project use news-daily`（写入 `default_project` 到当前 profile）
- `vibeknow project list / show / update / delete`
- **Project 数据存储在后端 go-vibeknow**，不是本地 config，从而跨设备一致并与 Web 端共享。本地只缓存 default_project 名。
- 优先级：命令行 flag（如 `--voice`）> `--project` 指定的 project > profile 里的 default_project > 全局默认值；只覆盖显式传入的字段，未传字段回退到上一级。

**不做**：交互式向导（与 Agent-Native 定位冲突，真要做放到 Web 端）。

## 7. AI Agent Skills

对齐 lark-cli 的 `skills/lark-<domain>/SKILL.md` 模式。

| Skill 包 | 覆盖命令 | A 阶段 |
|---|---|---|
| `vibeknow-core` | `auth`, `profile`, `config`, `doctor`（onboarding 指引） | ✅ |
| `vibeknow-create` | Hero 命令 + `video status/wait/download` + `project` | ✅ |
| `vibeknow-doc` | `doc upload/get` | ✅ |
| `vibeknow-rag` | `rag query` | ✅ |
| `vibeknow-voice` | `voice list`（后续补 clone） | 🔲 A 只填 list |
| `vibeknow-pipeline` | stage 级调试（给开发者 Agent） | 🔲 骨架 |

**SKILL.md 写作要点**：

- 明确 **TRIGGER**（何时调用）和 **SKIP**（何时不要调用）。
- 提供 **常见任务食谱**：自然语言任务 → 对应 CLI 命令序列。
- 标注 **NDJSON event schema**，便于 Agent 流式解析进度。

## 8. 测试、发布、可观测性

### 8.1 测试

- 每个 shortcut 单元测试（mock client）+ 关键链路集成测试（打到 staging profile）。
- 复用 lark-cli 的 `internal/httpmock`。
- CI 运行：`go test ./... -race`。

### 8.2 发布

- GitHub Actions 交叉编译：darwin-arm64/amd64、linux-arm64/amd64、windows-amd64。
- 打包为 npm 包 `@vibeknow/cli`，版本号语义化。
- Release 流程与 lark-cli 对齐（从源码可参考）。

### 8.3 可观测性

- `--verbose`：打印 request/response 摘要。
- `VIBEKNOW_TRACE=1`：在请求头携带 trace id，在后端 Jaeger/Tempo 能串联。
- `vibeknow doctor`：环境自检（endpoint 可达性、token 有效性、版本、网络诊断）。

### 8.4 自更新

- 复用 lark-cli 的 `internal/selfupdate`。
- `vibeknow update` 检查并升级 npm 包。

## 9. 开放问题 / 待实现阶段确认

1. **Go module / organization 名**：占位 `github.com/<org>/vibeknow-cli`，待定。
2. **登录方式**：A 阶段先选 username/password 还是 API key？
3. **go-vibeknow 网关现状**：是否已聚合下游？若未聚合，补网关工作量多大？
4. **go-figlens 状态流现状**：是否已暴露 stage 级状态？SSE or 仅轮询？
5. **go-account 签发 token 的识别范围**：各下游服务是否都认？
6. **vectoria collection 的约定**：RAG query 的 collection 命名、租户隔离策略。
7. **lark-cli 代码复用策略**：直接 vendor？fork？抽成独立 `cli-toolkit` 包？

这些问题在 implementation plan 阶段需要逐一消解，部分需要与服务 owner 对齐后决定。

## 10. 附录：lark-cli 可直接借鉴的模块

（MIT 许可，源路径 `~/project/cli`）

| lark-cli 模块 | 用途 |
|---|---|
| `internal/keychain` | OS keychain 封装 |
| `internal/credential` | Token 存取 |
| `internal/output` | 结构化输出（human / json / ndjson） |
| `internal/selfupdate` | 自更新 |
| `internal/httpmock` | 测试用 HTTP mock |
| `internal/lockfile` | 防止并发写 |
| `internal/cmdutil` | cobra 命令工具函数 |
| `internal/registry` | 命令注册聚合 |
| `shortcuts/common` | Shortcut 接口定义 + 通用 helper |
| `cmd/bootstrap.go` / `cmd/root.go` | 根命令 & 全局 flag 布局 |
| `shortcuts/register.go` | Domain 聚合模式参考 |
