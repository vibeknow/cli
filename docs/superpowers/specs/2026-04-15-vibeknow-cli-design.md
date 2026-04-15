# vibeknow-cli 设计文档

- **日期**：2026-04-15
- **状态**：Design, pending implementation plan
- **作者**：nullkey（与 Claude 协作 brainstorm）
- **参考实现**：[lark-cli](https://github.com/larksuite/cli)（MIT 许可，可直接借鉴代码）
- **版本**：v2.2（v2.1 基础上补齐 lark-cli 最佳实践：AGENTS.md、终端输出清理、`--output` 枚举、Skill references 结构）

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
| 认证 | 统一 token（go-account 签发），通过 go-vibeknow 网关转发（**前提：§4.1 G1 + G2 成立**） | 多服务直连 + 多 token（见 §4.4 Fallback B） | CLI 感知面最小；符合人/Agent 定位 |
| 多环境 | lark-cli 风格 profile（`~/.config/vibeknow/profiles.yaml`） | 环境变量 | 支持 dev/staging/prod 无缝切换 |
| 长耗时任务 | 默认同步 + 进度流；`--async` 切异步；`--output json` 时输出 NDJSON 事件流 | 纯同步 / 纯异步 | TTY 人类友好 + Agent 可流式解析 |
| Hero 命令 | `vibeknow create --from <source>`，复杂配置沉到 `project` | 参数全部平铺 | 单参数命令符合 Agent-Native；配置沉淀到后端可跨端共享 |
| AI Skills | 每个 domain 一个 Skill 包；**仅在命令真正落地的 domain 发布 Skill** | 一个大 Skill / 骨架 Skill 预占位 | Agent 按需加载；空 Skill 比不存在更糟 |

## 3. 整体架构

### 3.1 项目信息

- **仓库**：`~/laoshen/vibeknow-cli`
- **Go module**：`github.com/<org>/vibeknow-cli`（实际组织名待定，见 §10 开放问题）
- **npm 包**：`@vibeknow/cli`
- **二进制**：`vibeknow`

### 3.2 命令三层

1. **Shortcuts 层** — 面向人与 Agent 的"黄金路径"。封装多次后端调用 + 合理默认值 + 业务侧默认值合成（如 `create` 会自动按需 upload → submit → wait）。**特征**：动词在前（create / download），对人/Agent 语义自然。
2. **API Commands 层** — 一一映射后端服务 API，参数贴近 proto/OpenAPI。**特征**：命名对齐后端 RPC/REST（`video.task.get --id xxx`），单次请求。
3. **Raw 层** — 未封装 API 的兜底通道。所有 raw 请求**仍通过 go-vibeknow 网关**，`--service` 只是路由提示，不是直连：
   ```
   vibeknow api call --service figlens --method POST --path /v1/pipeline/run --body @x.json
   ```
   等价于 `POST {gateway}/proxy/figlens/v1/pipeline/run`，网关负责鉴权 + 转发。仅当 profile 启用 `service_overrides` 时才可能直连（见 §4.3）。

**分层判定规则**（两个工程师应该做出一致的选择）：

| 情况 | 放在哪层 |
|---|---|
| 单次后端调用，参数直映 | API Commands（`api/<domain>/`） |
| 多次调用 / 含本地 I/O（读文件、写下载） / 含默认值推导 | Shortcuts（`shortcuts/<domain>/`） |
| 后端尚未封装成 client，或一次性 / 调试用 | Raw（`raw/`） |

**例**：`doc upload` 是 shortcut（要读文件、算 hash、走分片或预签名），但它**内部调用**的单次 `POST /v1/docs` 同时也在 `api/doc/` 里提供（供其它 shortcut 和高级用户组合调用）。

### 3.3 Domain 切分

| Domain | 说明 | A 阶段填充 | A 阶段 Skill |
|---|---|---|---|
| `video` | 视频生成主流程（create / status / wait / cancel / download） | ✅ 全填 | ✅ 合入 `vibeknow-create` |
| `doc` | 素材文档（上传 / 解析 / 查看） | ✅ upload + get | ✅ `vibeknow-doc` |
| `rag` | vectoria 知识库（query / ingest / collection 管理） | ✅ 只填 query | ✅ `vibeknow-rag` |
| `voice` | go-speech：list / synth / clone / preview | 🔲 A 只填 `list` | ❌ 不发布 Skill（命令太少） |
| `pipeline` | figlens pipeline 调试 | 🔲 目录占位，空 `Shortcuts()` | ❌ 不发布 Skill |
| `asset` | 模板、BGM、图片素材库 | 🔲 目录占位 | ❌ 不发布 Skill |
| `project` | 作品/项目配置 | ✅ create + use + list | ✅ 合入 `vibeknow-create` |
| `auth` | 对接 go-account | ✅ 全填 | ✅ 合入 `vibeknow-core` |
| `config` / `profile` / `doctor` / `update` / `completion` | 基础命令 | ✅ 全填 | ✅ 合入 `vibeknow-core` |

**原则**：Skill 只在命令集足够形成自洽工作流时发布。骨架 Skill 会让 Agent 加载后"发现空"，体验比缺失更差。

### 3.4 Repo 目录结构

```
~/laoshen/vibeknow-cli/
├── cmd/                     # cobra 根 + 基础命令（auth/config/profile/doctor/update/completion）
├── internal/                # keychain/credential/output/client/selfupdate/...（vendor 或 fork 自 lark-cli）
├── shortcuts/               # 每 domain 一个包，export Shortcuts()
│   ├── register.go
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
├── skills/                  # A 阶段只发布 4 个 Skill；每个 Skill 见 §7 目录结构
│   ├── vibeknow-core/       # SKILL.md + references/{commands,events,errors,recipes}.md
│   ├── vibeknow-create/
│   ├── vibeknow-doc/
│   └── vibeknow-rag/
├── scripts/
├── tests/
├── main.go
├── go.mod
├── package.json
├── build.sh
├── Makefile
├── README.md
├── README.zh.md
├── AGENTS.md                # 仓库级 Agent 指引（目录速览、命令约定、常见陷阱）
├── CHANGELOG.md
└── LICENSE
```

**A 阶段最小命令集**：

```
vibeknow auth login / logout / whoami
vibeknow profile use / list / add / remove / show
vibeknow config get / set / list
vibeknow doc upload <file>  / doc get <id>
vibeknow rag query --collection <c> --q "..."
vibeknow voice list
vibeknow project create / use / list / show
vibeknow create --from <file|url|doc_id> [--project x] [--voice x] [--async] [--output json]
vibeknow video status <id> / wait <id> / cancel <id> / download <id>
vibeknow api call ...                 # raw 兜底
vibeknow doctor / update / completion
```

### 3.5 与 lark-cli 的架构差异

- lark-cli 的 **API Commands 层**集中在 `cmd/api/` 下，由 `schema/` 生成（对应单一的飞书开放平台 schema）。
- vibeknow-cli 的 API Commands 层按 **domain 切**（`api/<domain>/`），因下游是 5 个独立服务，无统一 schema。
- 若未来做统一 OpenAPI 网关，可回归 lark-cli 模式；届时重构可控。

## 4. 认证、Profile 与网关姿态

### 4.1 Go / No-Go Gates（实施前必须确认）

以下三项是**架构的前提假设**，任一项不成立都会让设计形态发生根本变化。在 implementation plan 阶段必须先与服务 owner 对齐；未就绪则设计需要改版而非局部打补丁。

| Gate | 内容 | 若不成立 |
|---|---|---|
| **G1** | go-vibeknow 已是（或愿意补成）对外网关，聚合 figlens/vectoria/speech | 走 §4.4 Fallback B：CLI 直连多服务 |
| **G2** | go-account 签发的 token 被所有下游服务信任（或可通过网关统一注入） | 改造成多 token 模型，需要重新设计 `client/` 各包的鉴权与 credential 存储 |
| **G3** | go-figlens 暴露（或愿意补成）stage 级状态（SSE 或可轮询的 snapshot） | §5 长耗时任务模型降级为"只有 submitted / succeeded / failed 三态"，进度条消失 |

**本 spec 的其余内容均建立在 G1 + G2 + G3 成立的假设之上。**

### 4.2 主路径认证流程（假设 G1 + G2 成立）

1. `vibeknow auth login` 调 go-account 签发 token。
   - A 阶段先实现一种登录方式：**推荐 username/password**（最低依赖），OAuth / SSO 留扩展位。
   - CI / 无 TTY 场景下，支持 `VIBEKNOW_TOKEN` 环境变量直接注入，跳过 keychain。
2. Token 存 OS keychain，复用 lark-cli 的 `internal/keychain` + `internal/credential`。
   - **Keychain 不可用 fallback**（Linux headless）：加密后存 `~/.config/vibeknow/credentials.enc`（AES-GCM，密钥派生自 machine id + 用户显式 passphrase）。由 `vibeknow auth login --storage file` 显式触发，不自动降级。
3. 所有请求携带 token；go-vibeknow 网关反向代理到 figlens/vectoria/speech；CLI 只感知 `api_endpoint` 一个地址。
4. `vibeknow auth whoami` 调 go-account 验证；`logout` 清 credential。

### 4.3 Profile 模型

配置文件：`~/.config/vibeknow/profiles.yaml`（Windows：`%AppData%\vibeknow\profiles.yaml`）。

**Schema**（canonical，见 §11.3 yaml 示例）：
```
profiles:
  - name: string (必填，唯一)
    api_endpoint: string (必填，网关地址)
    credential_ref: string (必填，keychain 条目名或 file:// 路径)
    default_project: string (可选)
    trust: "user" | "dev" (默认 user)
    is_production: bool (默认 true)
    service_overrides: map[string]string (可选，仅 trust=dev 且 is_production=false 生效)
current: string (当前激活 profile 名)
```

**`service_overrides`**：允许开发者为特定后端服务指定直连 endpoint（调试场景）。生效条件（全部满足）：
1. `profile.trust == "dev"`
2. `profile.is_production == false`（**必须显式声明**；默认 `true` 保护普通用户）
3. 环境变量 `VIBEKNOW_ALLOW_OVERRIDES=1`

三重开关全部满足时，raw `api call --service X` 和 API Commands 层走直连而非网关，并在 stderr 打印显著警告（包含被覆盖的服务名与目标地址）。任一条件未满足：`service_overrides` 字段读入即忽略并打 warning，不影响其它字段。

**不采用启发式保护**（如检查 `api_endpoint` 是否含 `prod` 子串）—— 启发式会被域名形态绕过，不如显式声明可靠。

命令：`vibeknow profile use / add / list / remove / show`。

### 4.4 Fallback B：多服务直连（若 G1 不成立）

**仅在实施阶段确认 G1 不成立时启用，作为备选设计。** 不作为主路径实现。

- Profile schema 扩展为 `endpoints: map[service]url`，每个服务一个 endpoint。
- `client/` 各包独立发起请求。
- 鉴权仍基于 G2：token 相同，但每次请求需附加 `X-Vibeknow-Service` 之类 header 让下游自行识别。
- 若 G2 也不成立，则进入"多服务 + 多 token"最坏情况，此时应回到 brainstorm 阶段重谈。

## 5. 长耗时任务模型

### 5.1 任务生命周期

```
submit → queued → running(stage_a) → running(stage_b) → ... → succeeded | failed | cancelled
```

视频生成、voice clone（未来）、rag ingest（未来）均复用此模型。

### 5.2 CLI 交互模式

默认：`vibeknow create --from x.pdf`

- **TTY + 默认输出**：同步挂起，彩色进度条展示当前 stage / 已耗时 / 预估剩余，结束打印视频 URL。
- **非 TTY 或 `--output json`**：逐行输出 NDJSON 事件流（完整 schema 见 §11.1）。

异步：`vibeknow create --from x.pdf --async`

| 模式 | 输出 | 退出 |
|---|---|---|
| `--async` + 默认输出 | 打印一行 `Task submitted: t_123`（stderr 附提示如何 `wait`） | 立即 exit 0 |
| `--async` + `--output json` | **只输出一行** `{"event":"task.submitted","task_id":"...","schema_version":"1"}` 然后 exit 0 | 立即 exit 0 |

`wait <id>` 与同步模式复用同一段 streaming 代码。

### 5.3 后端传输协议

- 首选 **SSE**：`GET /tasks/{id}/events`，逐 stage 推送事件。
- Fallback **轮询**：`GET /tasks/{id}`（任务快照，含所有已发生 stage），间隔默认 2s，可通过 `--poll-interval` 覆盖。
- 协议选择由 `doctor` 探测网关能力后自动决定；也可通过 `VIBEKNOW_STREAM_MODE=sse|poll` 强制。
- 断线重连：SSE 带 `Last-Event-ID` header 从上次断点续传，最多重连 3 次，间隔指数退避。

### 5.4 错误与退出码

`task.failed` event schema 见 §11.1。

| Exit Code | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 通用错误 |
| 2 | 参数错误 |
| 3 | 认证错误 |
| 4 | 任务失败但可重试（`retryable: true`） |
| 5 | 任务失败不可重试 |
| 6 | 流中断 / 网络错误（**任务状态未知**，Agent 应 `wait <id>` 重试获取结果而非重新 submit） |
| 130 | 用户中断（SIGINT） |

**CLI 对 `stage.failed` 的行为**：
- `fatal=false`：视为进度信息。TTY 下进度条标红该 stage 但继续；NDJSON 模式透传事件。**不**设置非 0 exit code，等待后续终态事件。
- `fatal=true`：作为 `task.failed` 的前奏，CLI 保留该事件的 `error_code` / `error_message`，最终随 `task.failed` 到来时用上面的 exit code 规则退出。
- 若 SSE 在 `fatal=true` 的 `stage.failed` 之后断流（未收到 `task.failed`）：按 exit code 6 处理。

**Agent 决策指南**（会写入 `vibeknow-create` Skill）：
- code 4 → 同一 task_id 调 `vibeknow video retry <id>`（A 阶段内未实现则回退到重新 `create`）。
- code 5 → 不重试，向用户报告 `error_message`。
- code 6 → 对同一 task_id 调 `vibeknow video wait <id>`，不要重新 create。

## 6. Hero 命令与 Project 模型

### 6.1 Hero Shortcut：`vibeknow create`

```
vibeknow create --from <file|url|doc_id>
vibeknow create --from x.pdf --project news-daily
vibeknow create --from x.pdf --voice v_abc --template news --duration 60s --aspect 9:16
vibeknow create --from x.pdf --async
vibeknow create --from x.pdf --output json
```

**`--from` 解析规则**（按顺序尝试，首个匹配即采用）：

| 优先级 | 规则 | 处理 |
|---|---|---|
| 1 | 字面前缀 `doc_` 且符合 doc_id 格式（`^doc_[a-zA-Z0-9]{8,}$`） | 视为已存在 doc_id，直接提交 pipeline |
| 2 | `file://` 或绝对路径 | 视为本地文件；不存在则 exit 2 + 明确错误信息 |
| 3 | `http://` / `https://` | 视为远程 URL，交由 figlens 抓取 |
| 4 | 非绝对路径 | 先按相对路径解析（相对 cwd）；存在则视为文件，不存在则 exit 2（**不隐式视为 URL**，避免歧义） |

上传本地文件时自动 `doc upload`，然后用返回的 doc_id 提交 pipeline。

### 6.2 Project 模型

- `vibeknow project create news-daily --template news --voice v_abc --aspect 9:16 --duration 60s [--tags a,b]`
- `vibeknow project use news-daily`（写入 `default_project` 到当前 profile）
- `vibeknow project list / show / update / delete`
- **Project 数据存储在后端 go-vibeknow**，跨设备一致，与 Web 端共享。本地只缓存 default_project 名。

**字段覆盖规则**（解决 reviewer 指出的 list/map 歧义）：

| 字段类型 | `--project X` + 命令行 flag 同时存在时的行为 |
|---|---|
| 标量（`--voice`, `--template`, `--duration`, `--aspect`） | flag 覆盖 project 字段 |
| 列表（`--tags a,b`） | **替换**，不合并（即 flag 传入的列表完全覆盖 project 的） |
| 对象（未来可能的 `--render-options`） | **替换**，不深合并 |

未显式传入的字段一律回退到 project 值 → profile 默认 → 全局默认。

**不做**：交互式向导（与 Agent-Native 定位冲突，真要做放到 Web 端）。

## 7. AI Agent Skills

对齐 lark-cli 的 `skills/lark-<domain>/SKILL.md` 模式，但 A 阶段**只发布命令集够自洽的 4 个 Skill**：

| Skill 包 | 覆盖命令 |
|---|---|
| `vibeknow-core` | `auth`, `profile`, `config`, `doctor`（onboarding + 环境诊断指引） |
| `vibeknow-create` | Hero 命令 + `video status/wait/download/cancel` + `project` + `voice list`（Agent 最常用的"一键生成+跟进"） |
| `vibeknow-doc` | `doc upload/get` |
| `vibeknow-rag` | `rag query` |

**Skill 目录结构**（对齐 lark-cli）：

```
skills/<skill-name>/
├── SKILL.md           # 主文档：TRIGGER / SKIP / 常见任务食谱 / 核心命令速查
└── references/
    ├── commands.md    # 完整命令与 flag 参考（长内容）
    ├── events.md      # NDJSON event schema 节录
    ├── errors.md      # error code 清单与 exit code 处理指南
    └── recipes.md     # 进阶场景 / 组合调用示例
```

**SKILL.md 本身保持精简**（目标 <200 行），长文档全部放 `references/`，由 Agent 按需加载，避免一次性灌入拉高上下文成本。

**SKILL.md 写作要点**：
- 明确 **TRIGGER**（何时调用）和 **SKIP**（何时不要调用）。
- 提供 **常见任务食谱**：自然语言任务 → 对应 CLI 命令序列。
- 标注 **NDJSON event schema 版本**（引用 §11.1），便于 Agent 流式解析进度。
- 包含 **exit code 处理指南**（见 §5.4）。
- 在需要深度信息时**指向 `references/*.md`** 而非把内容内联。

## 8. 非功能需求

### 8.1 测试

- 每个 shortcut 单元测试（mock client）+ 关键链路集成测试。
- 复用 lark-cli 的 `internal/httpmock`。
- CI：`go test ./... -race`。
- **集成测试对 staging 环境的依赖**：作为 prereq 列在 §10 开放问题；若 staging 不稳定，集成测试改为本地 docker-compose 模拟后端（后续 plan 决定）。

### 8.2 发布

- GitHub Actions 交叉编译：darwin-arm64/amd64、linux-arm64/amd64、windows-amd64。
- 打包为 npm 包 `@vibeknow/cli`，版本号语义化。
- npm `postinstall` 钩子根据平台选择对应 Go 二进制放置到 `node_modules/.bin/vibeknow`。

### 8.3 自更新

- npm 分发场景下，`vibeknow update` 实际执行 `npm update -g @vibeknow/cli`（子进程），而非二进制自我替换。
- 如 CLI 是手动下载二进制（非 npm 场景），fallback 走 lark-cli 的 `internal/selfupdate` 二进制替换路径。
- 启动时异步检查新版本（每日至多一次），提示用户（不强制）。

### 8.4 可观测性与遥测

- `--verbose`：打印 request/response 摘要（method、URL、status、耗时）。**不打 body，不打 header 中的 Authorization**。
- `VIBEKNOW_DEBUG=1`：额外打印 request body / response body 前 4KB（用户显式开启，视为已知风险）。
- `VIBEKNOW_TRACE=1`：请求头携带 `X-Trace-Id`，在后端 Jaeger/Tempo 可串联。
- `vibeknow doctor`：环境自检（endpoint 可达性、token 有效性、版本、SSE/轮询模式探测）。
- **匿名使用遥测**：A 阶段**不做**。未来若做需补 opt-in 机制 + 独立遥测 endpoint + 清晰 privacy policy。

### 8.5 安全

**Credential 来源优先级**（三者共存时，按顺序取首个有值）：

| 优先级 | 来源 | 特征 | logout 行为 |
|---|---|---|---|
| 1 | `VIBEKNOW_TOKEN` 环境变量 | 不落盘；适合 CI | 不可清除（env 由调用方管理）；但 logout 会在 stderr 提示 |
| 2 | OS keychain（`credential_ref` 指向 keychain 条目） | 默认；适合人类用户 | 清除对应条目 |
| 3 | 加密文件（`credential_ref` 为 `file://` 路径，AES-GCM） | Linux headless fallback，由 `auth login --storage file` 显式触发 | 删除文件 |

同一 profile 下三者同时有值时：env 覆盖一切；env 缺省则按 `credential_ref` 形态选 keychain 或 file。logout 只影响与当前 profile 绑定的持久化来源（2 或 3），不会"清"掉 env。

- **`service_overrides` 信任边界**：规则见 §4.3（需 `trust: dev` + `is_production: false` + `VIBEKNOW_ALLOW_OVERRIDES=1` 三重开关）。
- **本地文件读取**：`--from` 路径校验为 regular file（非 symlink-to-socket / device），大小上限默认 500MB（可配置）。
- **下载磁盘空间预检**：`video download` 前若 `Content-Length` 已知，检查目标盘剩余空间是否 ≥ `size × 1.1`；不足则 exit 2 并提示。未知 size 时不预检，但写入失败（ENOSPC）时删除半成品并 exit 1。
- **日志脱敏**：所有日志路径统一经 `internal/redact`，自动抹掉 Authorization / Cookie / token-like 字符串。
- **终端输出清理**：对齐 lark-cli。所有面向 stdout / stderr 的后端数据统一经 `internal/charcheck` 剥离 ANSI escape、回车覆盖、其它 C0 / C1 控制字符，防止恶意响应伪造终端显示。`--output json|ndjson` 模式下输出由 CLI 自行生成的 JSON，该规则对数据值仍然生效（JSON 字符串字段转义后不含裸控制字符）。

### 8.6 本地缓存与下载

- **`doc upload`**：默认**不**缓存 doc_id 映射。`--cache-id` flag 可选写入 `~/.cache/vibeknow/doc-ids.json`（仅 path + hash + doc_id + ts，无内容），便于重复提交时秒级复用。
- **`video download`**：
  - 默认路径 `./<task_id>.mp4`，存在时 exit 2 报错；`--overwrite` 覆盖，`--output <path>` 指定。
  - 支持断点续传（HTTP Range），默认开启；`--no-resume` 强制重下。
  - 下载过程展示进度条（TTY）或 NDJSON `download.progress` event（非 TTY）。

### 8.7 并发与锁

- `lockfile`（复用 lark-cli）用于两个场景：
  1. **Profile 写操作**（`profile add/remove/use`、`config set`）：文件级锁，避免并发写 yaml 冲突。
  2. **自更新检查**：避免多个 CLI 进程同时尝试下载升级。
- **业务命令不加锁**。多个 `vibeknow create` 并发允许；后端自己限流。

### 8.8 版本兼容性

- **API 版本**：所有 client 请求固定 `/v1/` 前缀。服务端切 `/v2` 时 CLI 需同步发版。
- **CLI ↔ 后端 skew 策略**：
  - 启动时 `doctor` 或首次请求时校验后端 `X-Vibeknow-Api-Version` header；主版本不匹配时拒绝执行并提示升级。
  - 次版本向后兼容（新增字段 CLI 忽略；缺失字段按默认值处理）。
- **Raw 层 `--path`**：视为逃生舱，不保证稳定性；路径由用户自担风险。
- **Deprecation**：命令/flag 废弃时先一个 minor 版本打 warning，再下一个 major 版本移除。CHANGELOG 显式列出。

### 8.9 国际化（i18n）

- **A 阶段作用域**：CLI 输出英文 + 中文（与 lark-cli 对齐的 zh/en 双语 README）。
- **实现方式**：`internal/i18n` + 简单的 key-based string table，按 `LANG` / `VIBEKNOW_LANG` 选择语言；key 缺失时 fallback 英文。
- **SKILL.md**：A 阶段只出英文版本（Agent 生态 lingua franca），中文版后续补。
- **错误消息**：所有用户可见 error 必须有对应 key，禁止硬编码字符串。
- **日志 / 调试输出**：英文硬编码可接受（面向开发者）。

### 8.10 输出格式（`--output` 取值）

A 阶段支持 3 种取值，默认由 TTY 探测决定：

| 值 | 场景 | 内容 |
|---|---|---|
| `text`（TTY 下默认） | 人类交互 | 彩色 / 表格 / 进度条；不稳定契约，可随版本调整 |
| `json`（非 TTY 下默认；`--async` 单次返回） | 结构化一次性输出 | 单个 JSON 对象；带 `schema_version` |
| `ndjson`（事件流场景，含 `create`、`wait`、`video download` 进度） | 结构化流式输出 | 每行一个 JSON 对象；按 §11.1 / §11.2 契约 |

**规则**：
- `--output` 未显式指定时，**TTY → `text`；非 TTY → 事件流命令用 `ndjson`，其它用 `json`**。
- `--output text` 的字段命名 / 顺序不承诺稳定；Agent 严禁 regex 解析 `text` 输出。
- `table` / `yaml` 等其它格式**不在 A 阶段实现**，留 flag 预占。遇到未支持值时 exit 2 并列出支持清单。

## 9. 附录：lark-cli 可直接借鉴的模块

（MIT 许可，源路径 `~/project/cli`）

| lark-cli 模块 | 用途 |
|---|---|
| `internal/keychain` | OS keychain 封装 |
| `internal/credential` | Token 存取 |
| `internal/output` | 结构化输出（human / json / ndjson） |
| `internal/selfupdate` | 自更新（二进制场景，npm 场景另写） |
| `internal/httpmock` | 测试用 HTTP mock |
| `internal/lockfile` | 防止并发写 |
| `internal/cmdutil` | cobra 命令工具函数 |
| `internal/registry` | 命令注册聚合 |
| `shortcuts/common` | Shortcut 接口定义 + 通用 helper |
| `cmd/bootstrap.go` / `cmd/root.go` | 根命令 & 全局 flag 布局 |
| `shortcuts/register.go` | Domain 聚合模式参考 |

**复用策略（开放问题）**：直接 vendor、fork 到本仓库、还是抽出独立 `cli-toolkit` 共享包？取舍见 §10。

## 10. 开放问题 / 待 plan 阶段确认

1. **Go module / organization 名**：占位 `github.com/<org>/vibeknow-cli`，待定。
2. **G1 网关现状**：go-vibeknow 是否已聚合下游？补网关工作量？
3. **G2 Token 信任范围**：各下游服务是否都认 account token？
4. **G3 figlens 状态流现状**：是否已暴露 stage 级状态？SSE 还是仅轮询？
5. **登录方式二选一**：A 阶段先做 username/password 还是 API key？
6. **vectoria collection 约定**：RAG query 的 collection 命名、租户隔离策略。
7. **lark-cli 代码复用策略**：vendor / fork / 抽独立包？
8. **Staging 环境稳定性**：集成测试是否依赖，还是改本地 docker-compose？
9. **Doc_id 格式规范**：`^doc_[a-zA-Z0-9]{8,}$` 是否匹配后端实际实现？
10. **Task_id 命名**：`t_` 前缀由谁签发？后端是否已有格式约定？
11. **后端 SSE endpoint 是否已存在**：路径形态、事件格式是否对齐 §11.1 schema？
12. **网关 proxy 路径形态**：§3.2 示例用 `/proxy/figlens/v1/...`，实际由 go-vibeknow 采用 path-based 还是 header-based 路由需对齐。
13. **Submit 失败的输出形态**：submit 阶段本身失败（auth / 参数 / 网关不可达），客户端尚无 task_id，一律走 §11.2 Error Object 而非 task event —— 需在 plan 阶段写死此契约。
14. **`doctor` 探测是懒加载还是启动前置**：首次 `create` 是否同步跑能力探测并缓存？缓存 TTL？
15. **`Error Object.message` 的语言**：跟随 `VIBEKNOW_LANG` 还是固定英文？Agent 用 `code` 不受影响，但人类读 `message`。
16. **`task_id` 格式**：示例 `t_123` 是否反映后端真实格式？Spec 应与后端一次锁定后不再假设前缀。

## 11. Canonical Schemas

**本节是实施契约**。所有 schema 变更需要更新 `schema_version`，Agent 按 version 容错解析。

### 11.1 Task Event（NDJSON）

所有事件共同字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `schema_version` | string | ✅ | 当前为 `"1"` |
| `ts` | string (RFC3339) | ✅ | 事件发生时间（服务端） |
| `event` | string (enum) | ✅ | 见下表 |
| `task_id` | string | ✅ | 任务 id |

事件类型与额外字段：

| `event` | 额外字段 | 说明 |
|---|---|---|
| `task.submitted` | — | 任务已提交 |
| `task.queued` | `queue_position?: int` | 排队中 |
| `stage.started` | `stage: string` | stage 开始，stage 取值：`parse` \| `outline` \| `storyboard` \| `tts` \| `render` \| `publish`（A 阶段至少这 6 个） |
| `stage.progress` | `stage: string`, `percent: int (0-100)`, `message?: string` | stage 进度，推送频率 ≥1 次/5s |
| `stage.succeeded` | `stage: string`, `duration_ms: int` | stage 完成 |
| `stage.failed` | `stage: string`, `error_code: string`, `error_message: string`, `retryable: bool`, `fatal: bool` | stage 失败。`fatal=true` 时后端必须紧随 `task.failed`；`fatal=false` 时 pipeline 内部重试或跳过，任务继续（可能后续仍有 `stage.started` / `task.succeeded`） |
| `task.succeeded` | `video_url: string`, `thumbnail_url?: string`, `duration_ms: int` | 任务成功 |
| `task.failed` | `failed_stage: string`, `error_code: string`, `error_message: string`, `retryable: bool` | 任务失败（终态） |
| `task.cancelled` | `cancelled_by: string` | 任务被取消（终态） |

**终态事件**（`task.succeeded` / `task.failed` / `task.cancelled`）后 CLI 关闭流并退出。

### 11.2 Error Object

所有 CLI 错误输出（`--output json` 场景）的统一形态：

```json
{
  "schema_version": "1",
  "error": {
    "code": "string (enum，见下)",
    "message": "string (人可读)",
    "details": { "任意键值对，按 code 不同而不同" },
    "retryable": false,
    "trace_id": "string (若 VIBEKNOW_TRACE=1)"
  }
}
```

Error code 枚举（A 阶段）：`auth_required` | `auth_expired` | `invalid_args` | `not_found` | `permission_denied` | `network_error` | `stream_interrupted` | `task_failed` | `rate_limited` | `version_mismatch` | `internal_error` | `unknown`。

### 11.3 Profile YAML

```yaml
# ~/.config/vibeknow/profiles.yaml
schema_version: "1"
current: dev
profiles:
  - name: prod
    api_endpoint: https://api.vibeknow.example.com
    credential_ref: vibeknow.prod
    default_project: news-daily
    trust: user
    is_production: true
  - name: dev
    api_endpoint: https://staging-api.vibeknow.example.com
    credential_ref: vibeknow.dev
    trust: dev
    is_production: false
    service_overrides:
      figlens: http://localhost:8081
```

校验规则：
- `name` 唯一；`current` 必须指向存在的 profile 名。
- `trust` 只接受 `user` | `dev`；缺省 `user`。
- `is_production` 只接受 `true` | `false`；缺省 `true`（保护语义：未明说就按生产处理）。
- `service_overrides` 仅在 `trust == "dev"` **且** `is_production == false` **且** `VIBEKNOW_ALLOW_OVERRIDES=1` 三重开关全部满足时被 CLI 读取；任一不满足则读入即忽略并打 warning。
- `api_endpoint` 必须是绝对 URL（scheme + host）。

### 11.4 Project Object

后端存储的 Project（CLI 侧透传，此处定义 CLI 感知的必要字段；完整字段以后端为准）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | ✅ | 后端生成 |
| `name` | string | ✅ | 用户指定，租户内唯一 |
| `template` | string | ✅ | 模板 id |
| `voice` | string | ✅ | 音色 id |
| `aspect` | string (enum) | ✅ | `16:9` \| `9:16` \| `1:1` |
| `duration_sec` | int | ✅ | 目标时长（秒） |
| `tags` | []string | — | 可选 |
| `created_at` / `updated_at` | string (RFC3339) | ✅ | — |

`vibeknow project show --output json` 返回 `{ "schema_version": "1", "project": { ... } }`。

---

## 附：v2.1 → v2.2 变更摘要

- 仓库根部新增 `AGENTS.md`（仓库级 Agent 指引），对齐 lark-cli。
- §8.5 新增"终端输出清理"（C0/C1 控制字符剥离），对齐 lark-cli 安全姿态。
- §8.10 新增 `--output` 取值枚举（`text` / `json` / `ndjson`）与 TTY 自动选择规则。
- §7 新增 Skill 目录结构（`SKILL.md` + `references/`），SKILL.md 精简到 <200 行。

## 附：v2 → v2.1 变更摘要

- 新增 `is_production` profile 字段（默认 `true`），取代脆弱的字符串启发式作为 `service_overrides` 保护条件（§4.3 / §8.5 / §11.3）。
- §8.5 新增 credential 来源优先级表（env / keychain / file），澄清三者共存行为。
- §11.1 `stage.failed` 新增 `fatal` 字段，明确"stage 失败是否终止任务"的语义；§5.4 对应补 CLI 行为说明。
- §8.5 新增下载磁盘空间预检规则。
- §2 决策表"认证"行注明依赖 G1/G2 前提。
- §10 开放问题新增 5 项（网关路由形态、submit 失败输出、doctor 加载时机、error message i18n、task_id 格式）。

## 附：v1 → v2 变更摘要

- 将 go/no-go gates 独立成 §4.1，并对 G1 不成立场景补 §4.4 Fallback B。
- 新增 §3.2 "分层判定规则"，解决 `shortcuts/` vs `api/` 的放置歧义。
- 新增 §8.5 Security、§8.6 Cache/Download、§8.7 Concurrency、§8.8 Versioning、§8.9 i18n。
- 新增 §11 Canonical Schemas（event / error / profile yaml / project）。
- 明确 `--async` + `--output json` 行为（§5.2）。
- 新增 exit code 6（流中断，任务状态未知）。
- `--from` 解析规则表格化，非绝对路径不隐式视为 URL（§6.1）。
- Project 字段覆盖对 list/map 定义为"替换而非合并"（§6.2）。
- A 阶段只发布 4 个 Skill，去掉骨架 Skill（§3.3 / §7）。
- npm 场景的 `update` 路径澄清为 `npm update -g`（§8.3）。
