# GM 运营后台 P1 策划文档

> **Status:** approved  
> **实施计划：** [`2026-09-21-gm-ops-p1.md`](2026-09-21-gm-ops-p1.md)  
> **依赖：** [`2026-09-14-gm-ops-p0-design.md`](2026-09-14-gm-ops-p0-design.md)、[`2026-09-14-gm-prod-console-design.md`](2026-09-14-gm-prod-console-design.md)  
> **后续：** [`backlog-gm-ops.md`](backlog-gm-ops.md)（P2）

**For agentic workers:** 填完整文各章节；`Status: approved` 前不得创建实施计划、不得改业务代码。无把握处写「待定」并列入风险。

---

## 背景与目标

P0 已能查人、发奖、踢下线；控制台已有账号密码登录。运营仍缺扣道具、封号、看谁在线、发公告、调游戏时间。

**一期目标：** 在现有「HTTP（会话鉴权）→ NATS ClusterPacket → `ActorGM` 子域」链路上交付五项管控，并在 Naive UI 控制台提供对应页面。查询不写审计；写操作由 **game 节点** 成功后写入 `gm_ops_logs`。

---

## 用户场景

1. **纠错发奖：** 运营对角色 `playerId` 扣道具（按 `itemId` 跨槽或按 `slot`），在线 Push `onBagChange`。
2. **封号：** 按 uid 或 nickname 永久封禁并踢下线；玩家无法再签发/刷新/校验 token（与登录失败限流分离）。解封后可正常登录。
3. **巡场：** 查看当前进场列表（uid + sceneId），可按场景过滤；向全服或单场景发公告（客户端 `onNotice`）。
4. **调时间：** 设置游戏时间偏置（秒，≥0）；Redis 持久化并热更新 gm / game / login。

---

## 功能范围

### 包含

- 扣道具：`POST /gm/bag/deduct`；`itemId` 与 `slot` 二选一
- 封号/解封：`POST /gm/account/ban`、`POST /gm/account/unban`；查账号响应带封禁字段
- 场景在线列表：`GET /gm/world/online?sceneId=`
- 全服/场景公告：`POST /gm/notice`；Push `onNotice`，不跳过任何在线 uid
- 调游戏时间：`GET/POST /gm/time`；Redis 偏置 + NATS 热更新 game 与 login
- Naive UI：查账号结果区封/解封；新页扣道具、在线、公告、游戏时间
- 错误码 `40033 AccountBanned`

### 不包含

- 禁言、清背包、改槽、改名、软删恢复、重置密码、限时封号
- 强制维护、查设备会话、邮件/礼包码、细权限/SSO/TLS
- 多 game 节点目标发现（仍单 `-game`）
- Redis 全量吊销该 uid 的 token（踢连接 + 下次鉴权失败即可）

---

## 协议设计

### HTTP（gm 进程，JSON）

鉴权与 P0/控制台相同：会话 Cookie 或共享 token。写操作 `operator` 来自会话。

| 方法 | 路径 | 请求 | 响应 | 说明 |
|------|------|------|------|------|
| POST | `/gm/bag/deduct` | `{playerId,itemId,slot,count,bagType}` | 同 grant：`bagType,bag,pushed` | itemId 与 slot 恰好一个；count 缺省 1 |
| POST | `/gm/account/ban` | `{uid}` 或 `{nickname}` + `{reason}` | `{uid,banned,kicked}` | 永久封禁后踢下线 |
| POST | `/gm/account/unban` | `{uid}` 或 `{nickname}` | `{uid,banned}` | |
| GET | `/gm/world/online` | query `sceneId` 可选，0/缺省=全部 | `{list:[{uid,sceneId}]}` | 仅已进场 |
| POST | `/gm/notice` | `{sceneId,text}` | `{pushed}` | sceneId=0 全服；空 text → 40030 |
| GET | `/gm/time` | — | `{biasSeconds,unixNow,realUnixNow}` | 读当前进程偏置 |
| POST | `/gm/time` | `{biasSeconds}` | `{biasSeconds,unixNow,realUnixNow,gameUpdated,loginUpdated}` | ≥0 绝对值 |

现有查账号响应增加 `banned`、`banReason`。

统一失败：业务码在 JSON `code`；参数非法 HTTP 400 / `40030`；鉴权失败 401 / `40029`。

### NATS / ClusterPacket

| Domain | FuncName | 请求 | 响应 |
|--------|----------|------|------|
| `bag` | `deduct` | `GmDeductRequest` | `GmGrantResponse`（复用 bag/pushed） |
| `account` | `ban` / `unban` | `GmBanRequest` | `GmBanResponse` |
| `world` | `online` | `GmOnlineRequest` | `GmOnlineResponse` |
| `world` | `notice` | `GmNoticeRequest` | `GmNoticeResponse` |
| `time` | `set` | `GmTimeSetRequest` | `GmTimeGetResponse` |

时间：GM 进程先 `SaveBiasToRedis`，再 NATS 通知 game `{game}.gm.time.set` 与 login `{login}.session.setGameTime`。`cmd/gm` 增加 `-login`（默认 `login-1`）。login 失败时 Redis 已落盘，响应 `loginUpdated=false`。

### proto 草案

```protobuf
message GmDeductRequest {
  int64 player_id = 1;
  int32 item_id = 2;
  int32 slot = 3;
  int32 count = 4;
  int32 bag_type = 5;
  string operator = 6;
}

message GmBanRequest {
  int64 uid = 1;
  string nickname = 2;
  string reason = 3;
  string operator = 4;
}
message GmBanResponse {
  int64 uid = 1;
  bool banned = 2;
  bool kicked = 3;
}

message GmOnlineRequest {
  int32 scene_id = 1;
}
message GmOnlinePlayer {
  int64 uid = 1;
  int32 scene_id = 2;
}
message GmOnlineResponse {
  repeated GmOnlinePlayer list = 1;
}

message GmNoticeRequest {
  int32 scene_id = 1;
  string text = 2;
  string operator = 3;
}
message GmNoticeResponse {
  int32 pushed = 1;
}
message GmNoticePush {
  string text = 1;
  int32 scene_id = 2;
}

message GmTimeSetRequest {
  int64 bias_seconds = 1;
  string operator = 2;
}
message GmTimeGetResponse {
  int64 bias_seconds = 1;
  int64 unix_now = 2;
  int64 real_unix_now = 3;
}
```

`GmAccountView` 增加 `bool banned`、`string ban_reason`。

Push：`onNotice` / `GmNoticePush`。不复用 `BroadcastChat`（会跳过发送者）。

### 业务错误码

| 码 | 常量 | 含义 |
|----|------|------|
| 40033 | `AccountBanned` | 账号已封禁（登录/刷新/鉴权） |

参数互斥/空公告继续 `40030`；目标不存在 `40031`；扣不够 `40019`。封号与 `40014 LoginRateLimited` 分离。

**前置条件：** 不要求玩家 Session。调时间需要 Redis；热更新需要对应节点在线。封号写库不依赖 game 在线… 实际仍走 game ActorGM，game 必须在线。

---

## 数据与持久化

| 表 / 缓存 | 字段 / Key | 说明 |
|-----------|------------|------|
| `accounts` | `banned` bool 默认 false；`ban_reason` string | AutoMigrate；解封清空 reason |
| `gm_ops_logs` | 现有表 | action：`bag.deduct` / `account.ban` / `account.unban` / `world.notice` / `time.set` |
| Redis `{prefix}:meta:time_bias` | 偏置秒，无 TTL | `gtime.SaveBiasToRedis` |

**事务：** 扣道具走现有 `RemoveItem*`（`WithinTx` + `AfterCommit` 刷缓存）。封号单行 Update。审计独立插入。

---

## 业务规则

- **扣道具：** 与玩家 `bag.remove` 同一套规则。`bagType` 缺省时按 `item.bag_type` 自动路由（按 itemId）；按 slot 时必须显式 `bagType`。已删除角色拒绝。
- **封号：** 永久直至解封。成功后对 uid 走现有踢人。`issueToken` / `authToken` / `refreshToken` 查 `banned` → `40033`。
- **在线列表：** 仅 `world.inRoom`；未进场的登录连接不出现。
- **公告：** `sceneId==0` 推全部在房玩家，否则按场景；**不跳过任何 uid**。
- **时间：** `biasSeconds>=0` 绝对值设置（Cherry `AddOffsetTime` 实为赋值）。负值拒绝 `40030`。login 用 `gtime.Now()` 算 token 过期，快进可能导致未过期 token 立即失效，可接受。
- **审计：** 查询（含 online、GET time）不写；写操作 game 成功后写。时间 set 的审计在 game `time.set` 成功后写；GM 本地 Redis 写入失败则整单失败。

---

## 验收标准

- [x] 按 itemId / slot 能扣道具；数量不足 `40019`；在线 Push `onBagChange`
- [x] 封号后 issueToken/authToken/refreshToken 返回 `40033`；解封后可登录；与限流码 `40014` 不同
- [x] `GET /gm/world/online` 反映进场玩家；可按 sceneId 过滤
- [x] 全服/场景公告到达在线客户端 `onNotice`（含本会跳过聊天发送者的那些人）
- [x] `POST /gm/time` 写入 Redis；game 与 login 进程偏置更新（login 宕机时 `loginUpdated=false`，重启后 LoadBias）
- [x] 控制台可完成上述五项（封/解封在查账号页）
- [x] `go test ./...` 通过

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | P0 ActorGM、背包 RemoveItem*、gtime Redis、网关 kick |
| 迁移 | AutoMigrate 给 accounts 加列；存量 `banned=false` |
| 兼容 | 客户端需处理新 Push `onNotice`；旧客户端忽略未知 Push 即可 |
| 时间 | login 热更新依赖 `-login` 与 NATS subject `cherry-{prefix}.remote.login.{id}` |
| proto | 改 `.proto` 须 `scripts/genproto.ps1`，禁止手改 `gen/` |

---

## 实施索引

策划 **approved** 后：

1. 复制 [`_template.md`](_template.md) → `2026-09-21-gm-ops-p1.md`
2. 实施 plan 文首 **Design:** 链接本文件
3. 任务清单从本章「协议 / 数据 / 验收」派生
