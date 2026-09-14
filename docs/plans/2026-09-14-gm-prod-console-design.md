# GM 上线鉴权与 Naive UI 改版 策划文档

> **Status:** approved  
> **实施计划：** [`2026-09-14-gm-prod-console.md`](2026-09-14-gm-prod-console.md)  
> **依赖：** [`2026-09-14-gm-ops-p0-design.md`](2026-09-14-gm-ops-p0-design.md)、[`2026-09-14-gm-web-ui-design.md`](2026-09-14-gm-web-ui-design.md)

**For agentic workers:** 填完整文各章节；`Status: approved` 前不得创建实施计划、不得改业务代码。无把握处写「待定」并列入风险。

---

## 背景与目标

P0 用进程级共享 token，操作者靠请求头 `X-GM-Operator` 可伪造；Web 控制台是手写草稿风，没有按人查审计。上线需要真实账号、不可抵赖的操作记录、能看的后台。

**一期目标：** GM 账号密码登录（与玩家 `accounts` 分离）、管理员开号/禁用/改密、写操作审计绑定登录名、日志查询页、Naive UI 重做控制台。curl/CI 仍可用共享 token，记为操作者 `system`。

---

## 用户场景

1. **登录：** 打开控制台 → 用户名密码 → HttpOnly Cookie 会话 → 进入后台。错误密码进不去。
2. **运营：** 查人/发奖/踢人/热更，权限相同；审计 `operator` 为登录名，不能手填。
3. **管理员：** 开运营号、禁用、重置密码；看「操作记录」按人/动作/时间筛选。

---

## 功能范围

### 包含

- 表 `gm_users`；空表时用 `GM_BOOTSTRAP_USER` / `GM_BOOTSTRAP_PASSWORD` 种子管理员
- `POST /gm/auth/login|logout`、`GET /gm/auth/me`；Redis 会话 12h
- 管理员用户管理接口；不能禁用最后一个 admin
- `GET /gm/ops/logs` 分页筛选；控制台「操作记录」「账号管理」
- Naive UI 布局/表格/表单/确认框；登录页不再出现 token
- GM 进程 `-path` 加载 profile 后 `persistence.Init`（同 MySQL/Redis）
- 共享 `-token` 仅机器调用，operator=`system`，不能管号

### 不包含

- TLS / SSO / 验证码 / 2FA / 细权限树
- 封号、扣道具等玩法 P1（见 [`backlog-gm-ops.md`](backlog-gm-ops.md)）

---

## 协议设计

| 方法 | 路径 | 请求 | 响应 | 说明 |
|------|------|------|------|------|
| POST | `/gm/auth/login` | `{username,password}` | `{code,username,role,displayName}` + Set-Cookie | 公开 |
| POST | `/gm/auth/logout` | — | `{code}` | 清会话 |
| GET | `/gm/auth/me` | Cookie 或 token | `{username,role,displayName}` | |
| GET | `/gm/users` | — | `{list:[{id,username,displayName,role,disabled,createdAtUnix}]}` | 仅 admin |
| POST | `/gm/users` | `{username,password,displayName,role}` | 用户视图 | 仅 admin |
| POST | `/gm/users/disable` | `{id,disabled}` | `{code}` | 仅 admin |
| POST | `/gm/users/password` | `{id,password}` | `{code}` | 仅 admin |
| GET | `/gm/ops/logs` | query：`operator,action,from,to,page,pageSize` | `{list,total,page,pageSize}` | `from`/`to` 为 unix 秒 |

现有 `/gm/account|player|bag|grant|kick|reload|health` 不变。鉴权：会话 Cookie **或** `X-GM-Token` / Bearer。

**业务错误码：** `40029` 未登录/密码错；`40030` 参数非法；`40032 GmForbidden` 非 admin（HTTP 403）。登录失败过多可 `40014`。

**前置条件：** GM 已连 MySQL/Redis；种子账号或已有 `gm_users`。

---

## 数据与持久化

| 表 / 缓存 | 字段 / Key | 索引 / TTL | 说明 |
|-----------|------------|------------|------|
| `gm_users` | id, username, password_hash, display_name, role, disabled, created_at | username 唯一 | 非玩家表 |
| `gm_ops_logs` | 现有 + operator 索引 | idx_gm_ops_operator | game 写、GM 读 |
| Redis `{prefix}:gm:sess:{token}` | `{userId,username,role,displayName}` | 12h | HttpOnly Cookie `gm_session` |
| Redis `{prefix}:gm:login:fail:{user}` | 计数 | 与登录服失败窗口同量级 | 防撞库 |

**事务：** 建号/改密单行写；审计仍由 game 在业务后 `WriteGMOpLog`，失败不回滚。

---

## 业务规则

- 运营与管理员玩法权限相同；仅管号接口查 `role==admin`
- 写操作 `operator` 只来自会话用户名或 token 的 `system`；忽略 `X-GM-Operator`
- 禁用账号不能登录；不能禁用最后一个未禁用 admin
- 种子只在 `gm_users` 为空且环境变量齐全时执行一次
- Cookie：`Path=/`、`HttpOnly`、`SameSite=Lax`、明文 HTTP 不设 `Secure`

---

## 验收标准

- [x] 用户名密码登录成功；错误密码不能进业务页
- [x] 发奖/踢人/热更审计 `operator` 为登录名；token 调用记 `system`
- [x] 操作记录可按人筛选；admin 可开号/禁用/改密，operator 进管号页被拒
- [x] 控制台为 Naive UI，无共享 token 登录框
- [x] `go test ./...` 通过；`npm run build`；浏览器可走登录与日志

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | P0 API + 现 Web 嵌入；GM 新增 profile/MySQL/Redis |
| 迁移 | AutoMigrate `gm_users`；`gm_ops_logs.operator` 加索引 |
| 兼容 | curl 仍可用 `-token`；Web 不再用手填 operator |
| 安全 | 仍是内网 HTTP；须改掉开发 bootstrap 密码 |

---

## 实施索引

策划 **approved** 后：

1. 复制 [`_template.md`](_template.md) → `2026-09-14-gm-prod-console.md`
2. 实施 plan 文首 **Design:** 链接本文件
3. 任务清单从本章「协议 / 数据 / 验收」派生
