# vibeknow auth login 交互式登录设计

- **日期**：2026-04-17
- **状态**：Design, pending implementation plan
- **作者**：nullkey（与 Claude 协作 brainstorm）
- **对标**：[GitHub CLI](https://github.com/cli/cli)（Device Code Flow）、[飞书 CLI](https://github.com/larksuite/cli)（双阶段 + 跨进程刷新锁）
- **版本**：v1.1（v1.0 经 review 后修正 9 处问题）

## 1. 背景与目标

### 1.1 现状

当前 vibeknow CLI（P1）的认证流程需要用户手动完成 4 步：

1. 去 Web 控制台获取 JWT token
2. `export VIBEKNOW_TOKEN="eyJ..."`
3. `vibeknow profile add prod --credential-ref ... --endpoint-account ... --endpoint-vectoria ... --endpoint-figlens ... --endpoint-vibeknow ...`
4. `vibeknow auth whoami` 验证

这对新用户门槛过高，也不符合主流 CLI 的交互标准。

### 1.2 目标

- **一条命令完成登录**：`vibeknow auth login` → 浏览器授权 → 自动存储凭证
- **Agent/CI 原生支持**：`--no-wait` 两阶段模式 + PAT 环境变量
- **Token 静默刷新**：access_token 过期时自动用 refresh_token 续期，用户无感
- **安全**：CLI 永不经手用户密码，符合 OAuth 2.0 最佳实践

### 1.3 非目标

- 不实现 OAuth Authorization Code Flow（localhost callback），Device Code Flow 覆盖所有场景
- 不实现 CLI 端的 PAT 创建/管理命令（P2，当前通过 Web 控制台管理）
- 不实现认证日志持久化（P2）

## 2. 技术方案

采用 **OAuth 2.0 Device Authorization Grant（RFC 8628）** + **PAT 双轨制**。

### 2.1 认证双轨

| 轨道 | 适用场景 | 机制 | Token 类型 |
|------|---------|------|-----------|
| 交互式登录 | 人类在终端 | Device Code Flow → access_token + refresh_token | OAuth token |
| PAT 模式 | Agent / CI / 自动化 | 环境变量或 `--with-token` 粘贴 | Personal Access Token |

### 2.2 选型理由

- **vs Localhost Callback（Authorization Code Flow）**：Device Code 不需要启动本地 HTTP server，适配 SSH 远程、无头服务器、Agent 环境
- **vs 纯 Token 粘贴**：Device Code 用户体验更好（浏览器登录 → 自动换 token），无需手动复制粘贴长 JWT
- **对标**：GitHub CLI、飞书 CLI 均采用 Device Code Flow 作为主要登录方式

## 3. Token 生命周期

### 3.1 Token 类型

| Token | 有效期 | 签发方 | 用途 |
|-------|--------|--------|------|
| access_token | 2 小时 | go-account | API 请求鉴权（沿用现有 `X-Authorization-Token` header，不迁移到 `Authorization: Bearer`） |
| refresh_token | 30 天 | go-account | 静默续期 access_token |
| PAT | 永不过期（用户手动吊销） | go-account Web 控制台 | Agent / CI 场景 |

### 3.2 三级状态模型

对标飞书 CLI，本地预判 token 状态，提前 5 分钟主动刷新，避免等 401 的额外延迟：

| 状态 | 条件 | 行为 |
|------|------|------|
| `valid` | now < expires_at - 5min | 直接使用 |
| `needs_refresh` | now >= expires_at - 5min 且 now < refresh_expires_at | 静默刷新（获取新 access_token + refresh_token） |
| `expired` | now >= refresh_expires_at | 删除本地 token，报错要求重新 `vibeknow auth login` |

### 3.3 凭证优先级

保持现有设计，扩展支持 Keychain JSON 格式：

```
1. VIBEKNOW_TOKEN 环境变量        ← Agent/CI（PAT 或手动 token）
2. Keychain 中的 access_token     ← 人类交互式登录
3. 加密文件中的 token              ← Headless Linux fallback
→ 全部缺失 → 报错："请执行 vibeknow auth login 或设置 VIBEKNOW_TOKEN 环境变量"
```

### 3.4 Keychain 存储格式

从单纯 token string 升级为 JSON 结构：

```json
{
  "version": "1",
  "access_token": "eyJ...",
  "refresh_token": "rt_...",
  "token_type": "oauth",
  "expires_at": "2026-04-17T16:00:00Z",
  "refresh_expires_at": "2026-05-17T14:00:00Z"
}
```

- `version`：存储格式版本号，便于未来扩展（如增加 scope 字段）
- `token_type`：`"oauth"`（交互式登录）或 `"pat"`（`--with-token` 存入）
- `expires_at` / `refresh_expires_at`：CLI 收到后端响应时计算 `time.Now().Add(expires_in)`，减去 30 秒安全余量以应对网络延迟和时钟偏差
- PAT 模式下 `refresh_token`、`expires_at`、`refresh_expires_at` 均为空
- **向下兼容**：读取时如果不是 JSON，当作纯 access_token 处理（`token_type` 视为 `"pat"`，不触发刷新）

## 4. 命令设计

### 4.1 命令签名

```bash
vibeknow auth login [flags]

Flags:
  --with-token              从 stdin 读取 PAT，跳过���互式流程（bool）
  --no-wait                 获取 device code 后立即返回 JSON，不轮询（bool，Agent 用）
  --device-code <string>    恢复之前 --no-wait 的轮询，传入 device_code 值（Agent 用）

Mutual exclusions:
  --with-token + --no-wait       互斥
  --with-token + --device-code   互斥
  --no-wait + --device-code      互斥

Exit codes:
  0    登录成功
  1    参数错误 / flag 互斥
  3    授权被拒绝 / 验证码过期 / token 验证失败（对齐主设计文档 §5.4 exit code 3 = auth error）
```

### 4.2 流程 1：交互式登录（默认）

```
$ vibeknow auth login

正在请求设备验证码...

✓ 验证码: ABCD-1234
  请在浏览器中打开: https://vibeknow.com/device
  并输入上方验证码完成登录

  按 Enter 打开浏览器，或按 Ctrl+C 取消...

✓ 已打开浏览器

⠋ 等待授权... (14:55 剩余)
✓ 登录成功！欢迎，张三 (zhangsan@example.com)
  凭证已保存到系统密钥链
```

**详细步骤**：

1. **TTY 检测** — 无 TTY 时报错："请使用 --with-token 或 --no-wait 进行非交互式登录"
2. **已登录检测** — 如果 Keychain 中已有有效 token，提示 "已登录为 张三，是否重新登录？(y/N)"
3. `POST /v1/auth/device/code` → 获取 `device_code`, `user_code`, `verification_uri`, `expires_in`, `interval`
4. 显示 `user_code` 和 `verification_uri`
5. 等待用户按 Enter → 调用系统 `open`（macOS）/ `xdg-open`（Linux）打开浏览器
6. 按 `interval` 间隔轮询 `POST /v1/auth/device/token`（带 `device_code`）：
   - `authorization_pending`（40010）→ 继续轮询，更新剩余时间
   - `slow_down`（40011）→ 增加间隔 5s，继续轮询
   - `expired_token`（40012）→ 报错 "验证码已过期，请重新执行 vibeknow auth login"
   - `access_denied`（40013）→ 报错 "授权被拒绝"
   - 成功 → 返回 `access_token` + `refresh_token` + `expires_in`
7. 调用 `GET /v1/user/profile` 验证 token 有效，获取用户信息
8. 将 token JSON 写入 Keychain（key = profile 的 `credential_ref`）
9. 打印欢迎信息

### 4.3 流程 2：PAT 模式（`--with-token`）

```bash
# 方式 1：管道输入
echo $MY_PAT | vibeknow auth login --with-token

# 方式 2：交互输入（有 TTY 时隐藏回显）
$ vibeknow auth login --with-token
? 粘贴你的 Personal Access Token: ████████████████

✓ 登录成功！欢迎，张三
  凭证已保存到系统密钥链
```

**步骤**：

1. 从 stdin 读取 token（有 TTY 时隐藏输入回显）
2. 调用 `GET /v1/user/profile` 验证 token
3. 写入 Keychain（`token_type: "pat"`）
4. 打印欢迎信息

### 4.4 流程 3：两阶段模式（Agent 场景核心）

**阶段 1 — Agent 发起，立即返回**：

```bash
$ vibeknow auth login --no-wait
{
  "verification_uri": "https://vibeknow.com/device",
  "user_code": "ABCD-1234",
  "device_code": "dc_xxxxxxxxxxxxxxxx",
  "expires_in": 900,
  "hint": "请在浏览器中打开 verification_uri 并输入 user_code，然后执行: vibeknow auth login --device-code dc_xxxxxxxxxxxxxxxx"
}
```

- 调用 `POST /v1/auth/device/code`
- 以 JSON 格式输出到 stdout，不轮询
- Agent 通过其他渠道（消息、通知）将 verification_uri + user_code 发送给人类

**阶段 2 — 人类完成授权后，Agent 恢复轮询**：

```bash
$ vibeknow auth login --device-code dc_xxxxxxxxxxxxxxxx
⠋ 等待授权...
✓ 登录成功！欢迎，张三
  凭证已保存到系统密钥链
```

- 直接进入轮询 `POST /v1/auth/device/token`
- 成功后写入 Keychain

### 4.5 边界情况

| 场景 | 行为 |
|------|------|
| 已登录再次 login | 提示 "已登录为 张三，是否重新登录？(y/N)" |
| 浏览器打开失败 | 打印 URL 提示手动复制打开 |
| Keychain 不可用 | 降级到加密文件存储，告知用户 |
| 无 profile | 自动创建 default profile（生产环境默认端点） |
| `VIBEKNOW_TOKEN` 已设置 | 提示 "检测到环境变量 VIBEKNOW_TOKEN，交互式登录的凭证优先级低于环境变量" |
| `--no-wait` + `--with-token` | 互斥，报错 |
| `--device-code` + `--with-token` | 互斥，报错 |
| `--no-wait` + `--device-code` | 互斥，报错 |
| `--device-code` 在非 TTY 环境 | 正常工作（轮询 + JSON 输出），因为 Agent 可能自行恢复轮询 |
| 倒计时显示 | 每个 `interval` 轮询周期更新一次剩余时间（非每秒） |

## 5. Token 自动刷新

### 5.1 架构

在现有 `httpclient` middleware chain 中，扩展 `TokenProvider` 接口，集成刷新逻辑：

**新增接口**（`internal/httpclient/token_provider.go`）：

```go
// TokenProvider 保持不变（向下兼容）
type TokenProvider interface {
    Token(ctx context.Context) (string, error)
}

// RefreshableTokenProvider 扩展接口，支持 401 兜底刷新
type RefreshableTokenProvider interface {
    TokenProvider
    TokenType() string                              // "oauth" 或 "pat"
    ForceRefresh(ctx context.Context) (string, error) // 强制刷新，返回新 access_token
}
```

**请求流程**：

```
RefreshableTokenProvider.Token(ctx)
  → 读取 Keychain token JSON
  → 判断三级状态
    → valid: 返回 access_token
    → needs_refresh: 获取锁 → 刷新 → 更新 Keychain → 返回新 access_token
    → expired: 返回错误

请求 → AuthMiddleware(调用 TokenProvider) → RefreshRetryMiddleware → [TraceID/Verbose/...] → 网络
                                                  ↓
                                            收到 401？
                                              → TokenProvider 实现了 RefreshableTokenProvider？
                                                → 是且 TokenType()=oauth → ForceRefresh() → 用新 token 重试一次
                                                → 是且 TokenType()=pat → 直接返回 401
                                                → 否（普通 TokenProvider）→ 直接返回 401（向下兼容）
```

- **主路径**：`Token()` 内部本地预判状态，提前刷新（无额外 RTT）
- **兜底路径**：`RefreshRetryMiddleware` 拦截 401，通过 type assertion 检查是否支持 `ForceRefresh()`
- **向下兼容**：现有使用 `TokenProvider` 的代码不受影响，只有传入 `RefreshableTokenProvider` 时才启用 401 重试
- **PAT 不刷新**：`TokenType()` 返回 `"pat"` 时跳过刷新

### 5.2 并发刷新保护

对标飞书 CLI，采用双层锁 + double-check：

**进程内**：`singleflight.Group`
- 相同 credential_ref 的并发刷新请求合并为一次

**跨进程**：文件锁 `flock`
- 锁文件路径：`~/.config/vibeknow/locks/refresh_{credential_ref}.lock`
- 获取锁超时：30s（500ms 检查间隔）
- **Double-check**：获取锁后重新读取 Keychain，如果另一个进程已经刷新完成（token 的 expires_at 已更新），直接使用新 token，跳过刷新请求

**刷新流程**：

```
Token 需要刷新
  → singleflight 去重（进程内）
    → 获取 flock（跨进程）
      → double-check: 重新读取 Keychain
        → 已被其他进程刷新？→ 使用新 token，释放锁
        → 未刷新 → POST /v1/auth/token/refresh
          → 成功 → 更新 Keychain，释放锁
            → Keychain 写入失败？→ 内存缓存新 token 供当前进程使用，日志警告 "token 未能持久化，下次启动需重新登录"
          → 失败 → 删除 token，释放锁，报错要求重新登录
```

### 5.3 关键约束

- 单次请求最多触发一次刷新重试，防止死循环
- 刷新失败时清除本地 token，下次请求直接提示重新登录
- `--verbose` 下打印刷新日志（token 脱敏）

## 6. 后端接口契约（go-account）

所有接口遵循 go-account 现有 envelope 风格（`{"code": 0, "data": {...}}`）。

### 6.1 获取设备验证码

```
POST /v1/auth/device/code
Content-Type: application/json

Request:
{
  "client_id": "vibeknow-cli",
  "scope": "full"
}

Response 200:
{
  "code": 0,
  "data": {
    "device_code": "dc_xxxxxxxxxxxxxxxx",
    "user_code": "ABCD-1234",
    "verification_uri": "https://vibeknow.com/device",
    "expires_in": 900,
    "interval": 5
  }
}
```

字段说明：

| 字段 | 说明 |
|------|------|
| `device_code` | CLI 轮询用，高熵随机值，不展示给用户 |
| `user_code` | 8 字符（大写字母 + 数字），中间加横杠便于阅读，如 `ABCD-1234` |
| `verification_uri` | 用户在浏览器中打开的验证页面 |
| `expires_in` | 验证码有效期（秒），建议 900（15 分钟） |
| `interval` | 最小轮询间隔（秒），CLI 必须遵守 |

### 6.2 轮询换取 Token

```
POST /v1/auth/device/token
Content-Type: application/json

Request:
{
  "client_id": "vibeknow-cli",
  "device_code": "dc_xxxxxxxxxxxxxxxx",
  "grant_type": "device_code"
}

Response 200 (授权成功):
{
  "code": 0,
  "data": {
    "access_token": "eyJ...",
    "refresh_token": "rt_...",
    "token_type": "Bearer",
    "expires_in": 7200,
    "refresh_expires_in": 2592000
  }
}

Response 200 (等待/错误):
code: 40010 → authorization_pending（继续轮询）
code: 40011 → slow_down（增加间隔 5s）
code: 40012 → expired_token（验证码过期）
code: 40013 → access_denied（用户拒绝）
```

### 6.3 刷新 Token

```
POST /v1/auth/token/refresh
Content-Type: application/json

Request:
{
  "refresh_token": "rt_..."
}

Response 200:
{
  "code": 0,
  "data": {
    "access_token": "eyJ...",
    "refresh_token": "rt_new...",
    "expires_in": 7200,
    "refresh_expires_in": 2592000
  }
}
```

- **Refresh Token Rotation**：每次刷新返回新的 refresh_token，旧 refresh_token 立即失效
- `refresh_expires_in`：新 refresh_token 的有效期（秒），30 天

### 6.4 Web 验证页面（前端）

```
URL: https://vibeknow.com/device

页面流程:
1. 用户输入 user_code
2. 后端验证 user_code 有效 → 展示授权确认页（"vibeknow-cli 请求访问你的账号"）
3. 用户点击"授权" → 后端标记 device_code 为已授权
4. 展示"授权成功，请返回终端"页面

前置条件:
- 用户必须已登录 Web（未登录则先跳转登录页，登录后回到 device 页面）
- 支持现有的所有登录方式（邮箱、手机、微信、Google、GitHub）
```

### 6.5 PAT 管理（P2）

```
POST   /v1/auth/pat         — 创建 PAT（返回完整 token，仅展示一次）
GET    /v1/auth/pat         — 列出 PAT（脱敏显示）
DELETE /v1/auth/pat/:id     — 吊销 PAT
```

P2 阶段实现。当前用户通过 Web 控制台创建 PAT，CLI 通过 `--with-token` 或环境变量使用。

## 7. CLI 模块变更

### 7.1 新增

| 文件 | 说明 |
|------|------|
| `cmd/auth/login.go` | login 命令主逻辑（交互式 / --with-token / --no-wait / --device-code） |
| `client/account/device.go` | `DeviceCode()`, `DeviceToken()` 方法。使用无认证 client（`New(baseURL, nil)`），因为这两个接口在用户获取 token 之前调用。`DeviceToken()` 直接解析 40010-40013 业务码（不走通用 `mapEnvelopeCode`），返回结构化的轮询状态 |
| `client/account/refresh.go` | `RefreshToken()` 方法 |
| `internal/credential/token.go` | `StoredToken` JSON 结构体、三级状态判断、序列化/反序列化、`ParseStored(raw string) StoredToken` 统一解析（JSON 或纯 string 兼容） |
| `internal/credential/refresh_lock.go` | 跨进程刷新锁（flock + double-check） |
| `internal/httpclient/mw_refresh_retry.go` | `RefreshRetryMiddleware`：拦截 401，type-assert `RefreshableTokenProvider`，触发 `ForceRefresh()` 并重试。插入在 `AuthMiddleware` 之后、现有 `RetryMiddleware`（5xx）之前 |

### 7.2 修改

| 文件 | 变更 |
|------|------|
| `cmd/auth/status.go` | 输出增强：token 过期时间、刷新状态、认证方式 |
| `cmd/auth/logout.go` | 清除 Keychain 中完整 JSON（access_token + refresh_token） |
| `internal/credential/resolver.go` | `KeychainSource.Get()` 内部调用 `ParseStored()` 解析 JSON，`Resolve()` 继续返回 `(string, string, error)`（纯 access_token string），保持所有现有调用方（如 `whoami.go`）不变 |
| `internal/cliauth/resolver.go` | 新增 `TokenProviderFor(profile) RefreshableTokenProvider`，内部持有 `StoredToken` + account client，实现 `Token()`（三级预判）、`TokenType()`、`ForceRefresh()`。原有 `ResolverFor()` 保留不变（���下兼容） |
| `internal/httpclient/transport.go` | `StandardChain()` 在 Auth 和 Retry 之间插入 `RefreshRetryMiddleware` |

### 7.3 不变

| 文件 | 说明 |
|------|------|
| `cmd/auth/whoami.go` | 保持不变（继续使用 `Resolver.Resolve()` 获取纯 token string） |
| `internal/keychain/` | Keychain 接口不变，存储内容从 string 变为 JSON string |
| `internal/credential/file_store.go` | 加密文件存储接口不变，内容格式随 Keychain 对齐 |

## 8. 默认 Profile 自动创建

### 8.1 动机

当前用户必须手动 `profile add` 配置 4 个 endpoint，是新用户上手最大的摩擦点。

### 8.2 方案

首次执行 `auth login` 时，如果 `~/.config/vibeknow/profiles.yaml` 不存在或无任何 profile，自动创建：

```yaml
schema_version: "2"
current: "default"
profiles:
  - name: "default"
    credential_ref: "vibeknow.default"
    endpoints:
      account: "https://account.vibeknow.com"
      vectoria: "https://vectoria.vibeknow.com"
      figlens: "https://figlens.vibeknow.com"
      vibeknow: "https://api.vibeknow.com"
    trust: "user"
    is_production: true
```

- 默认端点硬编码在 CLI 中（常量包 `internal/endpoints/defaults.go`），与 `profile add` 的默认值对齐
- `auth login` 在无 profile 时，先用硬编码的 account endpoint 请求 device code，登录成功后再创建 default profile
- 已有 profile 的用户不受影响
- `--profile` flag 正常工作（使用指定 profile 的 endpoint）

## 9. `auth status` 输出增强

```
$ vibeknow auth status

vibeknow.com
  ✓ 已登录为 张三 (zhangsan@example.com)
  - 认证方式: Device Code Flow
  - Token 来源: 系统密钥链 (vibeknow.default)
  - Token 状态: 有效 (1小时42分后过期)
  - Active profile: default
```

```
$ vibeknow auth status --output json
{
  "logged_in": true,
  "user": { "uid": 12345, "nickname": "张三", "email": "zhangsan@example.com" },
  "token_type": "oauth",
  "token_source": "keychain",
  "credential_ref": "vibeknow.default",
  "token_status": "valid",
  "expires_at": "2026-04-17T16:00:00Z",
  "refresh_expires_at": "2026-05-17T14:00:00Z",
  "profile": "default"
}
```

## 10. 用户体验对比

### Before（P1 当前）

```bash
# 步骤 1: 去 Web 控制台获取 JWT token
# 步骤 2: 配置环境变量
export VIBEKNOW_TOKEN="eyJ..."
# 步骤 3: 手动创建 profile（4 个 endpoint flag）
vibeknow profile add prod \
  --credential-ref vibeknow.prod \
  --endpoint-account https://account.vibeknow.com \
  --endpoint-vectoria https://vectoria.vibeknow.com \
  --endpoint-figlens https://figlens.vibeknow.com \
  --endpoint-vibeknow https://api.vibeknow.com
# 步骤 4: 验证
vibeknow auth whoami
```

### After（本设计）

**人类**：
```bash
vibeknow auth login    # 一条命令，浏览器授权，自动存储
vibeknow create --from my-doc.pdf
```

**Agent**：
```bash
# 方式 1: PAT 环境变量（推荐）
export VIBEKNOW_TOKEN="pat_..."
vibeknow create --from my-doc.pdf

# 方式 2: 两阶段（Agent 委托人类授权）
vibeknow auth login --no-wait          # Agent 获取 device code
# ... Agent 将验证信息发给人类 ...
vibeknow auth login --device-code dc_xxx  # 人类授权后 Agent 恢复
```

## 11. 对标验证

| 能力 | GitHub CLI | 飞书 CLI | 本设计 |
|------|-----------|---------|--------|
| Device Code Flow | ✅ | ✅ | ✅ |
| --no-wait 两阶段 | ❌ | ✅ | ✅ |
| --with-token PAT | ✅ | ❌ | ✅ |
| 双 Token + Refresh | ✅ | ✅ | ✅ |
| Refresh Token Rotation | ❌ | 可选 | ✅ 强制 |
| 三级状态预判 | ❌ | ✅ | ✅ |
| 提前 5min 主动刷新 | ❌ | ✅ | ✅ |
| 跨进程刷新锁 | ❌ | ✅ | ✅ |
| Keychain 存储 | ✅ | ✅ | ✅ |
| 默认 Profile 自动创建 | ✅ | ❌ | ✅ |
| TTY 检测 | ✅ | ✅ | ✅ |
| 向下兼容旧 token | N/A | N/A | ✅ |
