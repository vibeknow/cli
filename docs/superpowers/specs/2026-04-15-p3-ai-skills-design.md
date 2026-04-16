# P3: AI Agent Skills 设计文档

- **日期**：2026-04-15
- **状态**：Design, pending implementation plan
- **作者**：nullkey（与 Claude 协作）
- **依赖**：vibeknow-cli v0.3.0-p2（P0/P1/P2 已完成）
- **参考**：主 spec `2026-04-15-vibeknow-cli-design.md` §7

## 1. 目标

为 vibeknow-cli 编写 AI Agent Skills，让 Claude Code / Cursor 等 Agent 能按需加载能力描述，正确调用 CLI 命令完成任务。

对齐 lark-cli 的 `skills/lark-<domain>/SKILL.md` + `references/` 模式。

## 2. 原则

1. **只发布已实现命令的 Skill** — spec 原话："仅在命令真正落地的 domain 发布 Skill"。空 Skill 比不存在更糟。
2. **SKILL.md < 200 行** — 长内容放 `references/`，Agent 按需加载，降低上下文成本。
3. **Minimal references** — 只在有实际内容时创建文件，不留空占位。
4. **Per-topic reference split**（`commands.md` / `events.md` / `errors.md` / `recipes.md`），而非 per-command split — vibeknow 每个 skill 命令数量少（3-6），per-command 过度碎片化。

## 3. 范围

### 3.1 发布的 Skills（3 个）

| Skill | 覆盖命令 | 理由 |
|---|---|---|
| `vibeknow-core` | `auth` (status/whoami/logout), `profile` (add/use/list/show/remove), `config` (get/set/list), `doctor` | 环境搭建 + 诊断，所有命令已实现 |
| `vibeknow-create` | `create`, `video` (status/wait/download), `voice list` | Agent 最核心的"生成+跟进"链路，所有命令已实现 |
| `vibeknow-doc` | `doc` (upload/get) | 文档管理，所有命令已实现 |

### 3.2 不发布的 Skills

| Skill | 原因 |
|---|---|
| `vibeknow-rag` | `rag query` 命令未实现，不发布空 Skill |

### 3.3 不覆盖的已实现命令

| 命令 | 说明 |
|---|---|
| `api call` | Raw escape hatch，面向开发者手动调试，不适合 Agent 调用 |
| `version` | 无需 Skill 指导 |
| `completion` | Shell 补全生成，与 Agent 无关 |
| `update` | 未实现（P0 stub） |

## 4. 目录结构

```
skills/
├── vibeknow-core/
│   ├── SKILL.md
│   └── references/
│       ├── commands.md
│       └── errors.md
├── vibeknow-create/
│   ├── SKILL.md
│   └── references/
│       ├── commands.md
│       ├── events.md
│       ├── errors.md
│       └── recipes.md
└── vibeknow-doc/
    ├── SKILL.md
    └── references/
        ├── commands.md
        └── errors.md
```

共 3 个 SKILL.md + 8 个 reference 文件 = 11 个文件。

## 5. SKILL.md 规范

### 5.1 Frontmatter

```yaml
---
name: vibeknow-<domain>
version: 0.3.0
description: "一句话描述，供 Agent 匹配 TRIGGER 场景"
metadata:
  requires:
    bins: ["vibeknow"]
  cliHelp: "vibeknow <domain> --help"
---
```

- `version` 与 CLI 版本对齐（当前 0.3.0）。
- `metadata.requires.bins` 声明依赖的二进制。
- `metadata.cliHelp` 告诉 Agent 如何获取最新帮助。

### 5.2 SKILL.md 结构（每个 Skill 统一）

```markdown
# <domain> (v0.3.0)

## TRIGGER
何时调用此 Skill。

## SKIP
何时不要调用此 Skill（指向其他 Skill）。

## Core Concepts
关键概念（简洁，3-5 条）。

## Quick Reference
命令速查表（name + 一句话 + 指向 references/commands.md）。

## Common Tasks
自然语言任务 → CLI 命令序列（食谱式）。

## Exit Code Handling
退出码速查（指向 references/errors.md 获取完整列表）。

## Output Formats
--output text|json|ndjson 行为说明（仅 vibeknow-create 需要 ndjson 部分）。

## References
指向 references/*.md 的链接列表。
```

### 5.3 行数预算

| Skill | SKILL.md 预算 | 说明 |
|---|---|---|
| vibeknow-core | ~120 行 | 命令多但概念简单 |
| vibeknow-create | ~180 行 | 核心 Skill，含 NDJSON 摘要和 exit code 指南 |
| vibeknow-doc | ~80 行 | 最精简 |

## 6. References 规范

### 6.1 commands.md

每个命令完整列出：
- Synopsis（`vibeknow <cmd> [flags]`）
- 所有 flags 及默认值
- 输出示例（text + json 各一个，简短）
- 注意事项

### 6.2 events.md（仅 vibeknow-create）

引用主 spec §11.1 的 NDJSON Task Event schema：
- 所有事件共同字段
- 各事件类型及额外字段
- 终态事件说明
- Agent 解析示例（伪代码）

### 6.3 errors.md

统一格式：
- Exit code 表（0-6, 130）
- Error code 枚举（`auth_required` | `auth_expired` | ... | `unknown`）
- §11.2 Error Object JSON schema
- 每个 exit code 的 Agent 应对策略

vibeknow-core 和 vibeknow-doc 共享同一套 exit code / error code 体系，但各自的 errors.md 只列出与该 domain 相关的场景和应对。

### 6.4 recipes.md（仅 vibeknow-create）

进阶场景：
- 异步提交 + 后台轮询
- 重试策略（exit 4 可重试 vs exit 5 不可重试）
- Exit 6（流中断）的恢复：`vibeknow video wait <task_id>`
- NDJSON 流式解析脚本示例
- 批量创建模式（循环 + `--async`）

## 7. 各 Skill 详细内容

### 7.1 vibeknow-core

**TRIGGER**: 首次配置 vibeknow、认证问题、切换环境/profile、诊断连接问题。

**SKIP**: 视频生成（→ vibeknow-create）、文档管理（→ vibeknow-doc）。

**Core Concepts**:
- **Profile**: 命名环境配置（endpoints + credential），支持 dev/staging/prod 切换
- **Credential 来源优先级**: `VIBEKNOW_TOKEN` env > OS keychain > 加密文件
- **Endpoint 信任边界**: localhost/非官方域名需 `trust: dev` + `is_production: false`

**Common Tasks**:

| 任务 | 命令 |
|---|---|
| 查看当前认证状态 | `vibeknow auth status` |
| 查看当前用户 | `vibeknow auth whoami` |
| 登出 | `vibeknow auth logout` |
| 添加开发环境 | `vibeknow profile add dev --endpoint-figlens http://localhost:20067 --trust dev --is-production=false --credential-ref vibeknow.dev` |
| 切换到 dev | `vibeknow profile use dev` |
| 查看 profile 详情 | `vibeknow profile show [name]` |
| 诊断环境 | `vibeknow doctor` |
| 设置配置项 | `vibeknow config set <key> <value>` |

### 7.2 vibeknow-create

**TRIGGER**: 用户想从文档/URL/文件生成视频、查看视频状态、下载视频、列出可用声音模板。

**SKIP**: 仅上传/查询文档（→ vibeknow-doc）、认证/环境配置（→ vibeknow-core）。

**Core Concepts**:
- **Hero 命令**: `vibeknow create --from <source>` 一条命令完成 "解析 → 生成 → 等待 → 返回 URL"
- **--from 接受 3 种输入**: doc_id（直接使用）、URL（自动上传 vectoria）、本地文件路径（自动上传）
- **同步 vs 异步**: 默认同步等待完成；`--async` 立即返回 task_id
- **NDJSON 事件流**: `--output ndjson` 输出结构化进度事件（schema_version: "1"）
- **6 个 Pipeline Stage**: parse → outline → storyboard → tts → render → publish

**Common Tasks**:

| 任务 | 命令 |
|---|---|
| 从文件生成视频（同步等待） | `vibeknow create --from slides.pdf` |
| 从 URL 生成（异步） | `vibeknow create --from https://example.com/doc --async` |
| 指定声音模板 | `vibeknow create --from doc.pdf --voice v_warm_female` |
| Agent 模式（NDJSON 流） | `vibeknow create --from doc_abc --output ndjson` |
| 查看任务状态 | `vibeknow video status <task_id>` |
| 等待任务完成 | `vibeknow video wait <task_id>` |
| 下载视频 | `vibeknow video download <task_id>` |
| 断点续传下载 | `vibeknow video download <task_id> --output ./my-video.mp4` |
| 列出声音模板 | `vibeknow voice list` |

**Exit Code 速查（任务相关）**:

| Exit | 含义 | Agent 应对 |
|---|---|---|
| 0 | 成功 | 提取 video_url |
| 4 | 任务失败可重试 | 重新提交 |
| 5 | 任务失败不可重试 | 报告错误，不重试 |
| 6 | 流中断，状态未知 | `vibeknow video wait <task_id>` 恢复 |

### 7.3 vibeknow-doc

**TRIGGER**: 上传文档到 vectoria、查看文档处理状态。

**SKIP**: 视频生成（→ vibeknow-create）、RAG 查询（命令未实现）。

**Core Concepts**:
- **doc upload**: 上传文件到 vectoria，自动创建知识库并轮询直到处理完成，返回 doc_id
- **doc get**: 查询文档状态（processing / completed / failed）
- **--cache-id**: 可选开启本地 doc_id 缓存（`~/.cache/vibeknow/doc-ids.json`），重复提交时秒级复用

**Common Tasks**:

| 任务 | 命令 |
|---|---|
| 上传文档 | `vibeknow doc upload ./report.pdf` |
| 上传并缓存 ID | `vibeknow doc upload ./report.pdf --cache-id` |
| 查看文档状态 | `vibeknow doc get <doc_id>` |

## 8. 写作风格约定

- SKILL.md 和 references 全部用**英文**（spec §8.9: "SKILL.md A 阶段只出英文版本"）。
- 命令示例用 `bash` 代码块。
- 表格优先于长段落。
- 指向 reference 时用相对路径 `[commands.md](references/commands.md)`。
- 不重复主 spec 内容，而是引用（如 "see main spec §11.1 for canonical schema"）。

## 9. 实现计划概要

11 个文件，建议按 Skill 维度串行、每个 Skill 内 SKILL.md 先于 references：

1. `vibeknow-core/SKILL.md` → `references/commands.md` → `references/errors.md`
2. `vibeknow-create/SKILL.md` → `references/commands.md` → `references/events.md` → `references/errors.md` → `references/recipes.md`
3. `vibeknow-doc/SKILL.md` → `references/commands.md` → `references/errors.md`

每个 Skill 完成后可独立 review。

## 10. 未来扩展

当以下命令实现后，对应 Skill 才会发布或扩展：

| 命令 | 对应 Skill | 说明 |
|---|---|---|
| `rag query` | 新建 `vibeknow-rag` | RAG 查询 |
| `video cancel` | 扩展 `vibeknow-create` | 取消正在运行的任务 |
| `project *` | 扩展 `vibeknow-create` | 项目模板管理 |
