# GM 运营后台 P2（管控核）策划文档

> **Status:** approved  
> **实施计划：** [`2026-09-21-gm-ops-p2.md`](2026-09-21-gm-ops-p2.md)  
> **依赖：** [`2026-09-21-gm-ops-p1-design.md`](2026-09-21-gm-ops-p1-design.md)  
> **后续：** [`backlog-gm-ops.md`](backlog-gm-ops.md)

**For agentic workers:** 填完整文各章节；`Status: approved` 前不得创建实施计划、不得改业务代码。无把握处写「待定」并列入风险。

---

## 背景与目标

P1 已有永久封禁与踢连接，但聊天无禁言、封号不能限时、Redis token 无 uid 反向索引（解封后旧 token 仍可鉴权）、没有维护开关。

**一期目标：** 在现有「HTTP（会话鉴权）→ NATS ClusterPacket → `ActorGM`」链路上交付禁言、限时封号、封号吊销该 uid token、强制维护（Redis 拒登 + 全网关踢连接），并在 Naive UI 提供对应操作。

---

## 用户场景

1. **禁言：** 运营对辱骂账号禁言（可限时）；玩家仍在场景，发聊天返回 `40034`。
2. **限时封号：** 封号可填时长；到期后下次登录惰性视为未封。封号同时删除该 uid 全部 access/refresh。
3. **维护：** 开维护先写 Redis 拒登，再踢光网关连接（含已登录未进场）。关维护后旧 token 仍可用（不全服吊销）。

---

## 功能范围

### 包含

- 禁言/解禁：账号级，`durationSeconds=0` 永久
- 限时封号：扩展 `POST /gm/account/ban` 的 `durationSeconds`（0=永久，兼容 P1）
- 封号成功后 `RevokeAllTokensForUID` + 现有踢人
- 强制维护：Redis 旗 + login 拒发/鉴权/刷新 + 全网关踢连接
- 查账号响应带禁言/到期字段；控制台：查账号页禁言+封号时长；新页「维护」
- 错误码 `40034 ChatMuted`、`40035 ServerMaintenance`

### 不包含

- 角色级禁言、聊天持久化、禁言踢人
- 维护期间吊销全服 token、定时扫表解封
- 改 `cherry-framework/`；`SCAN` 清理本部署前已签发、无索引的 token
- 邮件/礼包码、改名、清背包、细权限、SSO、多 game 节点（仍见 backlog）

---

## 协议设计

鉴权与 P1 相同。写操作由 **game** 成功后写 `gm_ops_logs`。维护旗由 GM 进程先写 Redis；查询不写审计。

### HTTP（gm 进程，JSON）

| 方法 | 路径 | 请求 | 响应 | 说明 |
|------|------|------|------|------|
| POST | `/gm/account/ban` | `{uid\|nickname, reason, durationSeconds}` | `{uid,banned,kicked,bannedUntil,tokensRevoked}` | durationSeconds 缺省 0；负数 → 40030 |
| POST | `/gm/account/unban` | `{uid\|nickname}` | `{uid,banned}` | 清 `banned_until` |
| POST | `/gm/account/mute` | `{uid\|nickname, reason, durationSeconds}` | `{uid,muted,mutedUntil}` | |
| POST | `/gm/account/unmute` | `{uid\|nickname}` | `{uid,muted}` | |
| GET | `/gm/maintenance` | — | `{enabled,reason}` | 读 Redis |
| POST | `/gm/maintenance` | `{enabled,reason}` | `{enabled,reason,kicked}` | 先写 Redis；enabled=true 再踢；game 失败时 kicked 反映实际 |

`GmAccountView` 增加 `bannedUntil`、`muted`、`muteReason`、`mutedUntil`（unix 秒，0=永久或未设置）。

### NATS / ClusterPacket

| Domain | FuncName | 请求 | 响应 |
|--------|----------|------|------|
| `account` | `ban` | `GmBanRequest`（含 duration_seconds） | `GmBanResponse` |
| `account` | `mute` / `unmute` | `GmBanRequest` | `GmMuteResponse` |
| `world` | `maintenance` | `GmMaintenanceRequest` | `GmMaintenanceResponse` |
| `{gate}.ops` | `kickAll` | `GmKickAllRequest` | `GmKickAllResponse` |

login **不**新增 Remote：每次 `issueToken` / `authToken` / `refreshToken` 读 Redis 维护旗；维护检查在 `LoginOrCreateAccount` **之前**。

### proto 草案

```protobuf
message GmAccountView {
  // ...既有字段...
  int64 banned_until = 6;
  bool muted = 7;
  string mute_reason = 8;
  int64 muted_until = 9;
}
message GmBanRequest {
  int64 uid = 1;
  string nickname = 2;
  string reason = 3;
  string operator = 4;
  int64 duration_seconds = 5;
}
message GmBanResponse {
  int64 uid = 1;
  bool banned = 2;
  bool kicked = 3;
  int64 banned_until = 4;
  bool tokens_revoked = 5;
}
message GmMuteResponse {
  int64 uid = 1;
  bool muted = 2;
  int64 muted_until = 3;
}
message GmMaintenanceRequest {
  bool enabled = 1;
  string reason = 2;
  string operator = 3;
}
message GmMaintenanceResponse {
  bool enabled = 1;
  string reason = 2;
  int32 kicked = 3;
}
message GmKickAllRequest {}
message GmKickAllResponse {
  int32 kicked = 1;
}
```

### 业务错误码

| 码 | 常量 | 含义 |
|----|------|------|
| 40034 | `ChatMuted` | 账号禁言中（发聊天） |
| 40035 | `ServerMaintenance` | 服务器维护中（登录/刷新/鉴权） |

封号仍 `40033`（含未到期的限时封）。

**前置条件：** 不要求玩家 Session。维护踢人需要 game + gateway 在线；Redis 写入成功即可拒登。

---

## 数据与持久化

| 表 / 缓存 | 字段 / Key | 说明 |
|-----------|------------|------|
| `accounts` | `banned_until` int64 默认 0 | `banned=true` 且 until=0 为永久 |
| `accounts` | `muted` bool；`mute_reason`；`muted_until` | 禁言；until=0 为永久 |
| Redis `{prefix}:meta:maintenance` | JSON `{enabled,reason}` | 无 TTL |
| Redis `{prefix}:tokens:uid:{uid}` | SET，成员 `a:{access}` / `r:{refresh}` | 签发时 SADD，EXPIRE=RefreshTTL |

**惰性过期：** `IsAccountBanned` / `IsAccountMuted` 若 `until>0 && gtime.UnixNow()>=until` 视为否，并尽力清列。无 sweeper。

**事务：** 封号/禁言单行 Update；审计独立插入。token 吊销在写库成功后、踢人前。

---

## 业务规则

- **禁言：** 不踢线、不吊销 token；只拦 `game.chat.send`。
- **封号：** 写库 → `RevokeAllTokensForUID` → `kickUIDOnGates`。解封后须重新 `issueToken`。
- **维护开：** GM 写 Redis → game 对全部 gate `{id}.ops.kickAll`（`pomelo.ForeachAgent`，含未 bind）。维护关：只清 Redis。
- **token 索引：** `IssueTokenPair` / `Rotate` 维护；`RevokeTokens` SREM。本部署前无索引的 token 不 SCAN；封禁期内鉴权仍 `40033`。
- **审计：** `account.mute` / `account.unmute` / `world.maintenance`；`account.ban` 的 detail 带时长。

---

## 验收标准

- [x] 禁言后 `game.chat.send` → `40034`；解禁或到期后可发
- [x] `durationSeconds>0` 封号，到期后 `issueToken` 成功；未到期 `40033`
- [x] 封号后用封前 refresh/access 无法 `authToken`/`refreshToken`（有索引的新签发 token）；解封后须重新密码登录
- [x] `POST /gm/maintenance enabled=true` 后 issue/auth/refresh → `40035`；在线 WS 断开；`enabled=false` 后可登录
- [x] 查账号 JSON 含封禁/禁言到期字段；控制台可完成禁言、限时封号、开关维护
- [x] `go test ./...` 通过

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | P1 ActorGM、accounts.banned、网关 kick、gtime |
| 迁移 | AutoMigrate 加列；存量 until=0、muted=false |
| 兼容 | 旧客户端忽略未知错误码即可 |
| token | 部署前已签发、未入索引的 token，解封后可能在 TTL 内复活 |
| 维护 | game 宕机时 Redis 仍拒登，但踢人依赖 game+gateway |
| proto | 改 `.proto` 须 `scripts/genproto.ps1`，禁止手改 `gen/` |

---

## 实施索引

策划 **approved** 后：

1. 复制 [`_template.md`](_template.md) → `2026-09-21-gm-ops-p2.md`
2. 实施 plan 文首 **Design:** 链接本文件
3. 任务清单从本章「协议 / 数据 / 验收」派生
