# 一账号多角 策划文档

> **Status:** approved
> **实施计划：** [`2026-06-25-multi-character.md`](2026-06-25-multi-character.md)

**For agentic workers:** `Status: approved` 前不得创建实施计划、不得改业务代码。

---

## 背景与目标

当前 `players.uid` 带 `uniqueIndex`，`createPlayerInTx` 在查到已有角色后直接返回旧角色，导致一个账号只能持有一个角色。`select` 也只按 `uid` 取单条，`enter` 虽已接收 `player_id` 但加载仍走 `GetPlayerByUID`，并未校验该角色是否属于当前账号。

一期目标：放开「一账号一角色」限制，支持单账号创建、查看、选择、软删多个角色；上限由配置可调。多角数据必须保证合服友好（沿用全局 `player_id`，不绑定服务器语义）。

---

## 用户场景

1. 玩家登录后调用 `game.player.select`，返回当前账号下全部未删除角色列表（含名字、`player_id`）。
2. 玩家可继续 `game.player.create` 创建新角色，直到达到配置上限；达到上限时返回业务错误。
3. 玩家选定某角色后用 `game.player.enter`（必带 `player_id`）进场；若 `player_id` 不属于该账号或已删除，拒绝进场。
4. 玩家可 `game.player.delete` 软删除角色；软删后该角色不再出现在 `select` 列表，且不能进场。

---

## 功能范围

### 包含

- 去掉 `players.uid` 的 `uniqueIndex`，改为普通索引，允许同账号多角色。
- `players` 新增软删除字段（`deleted_at` 或 `is_deleted`），`select` 与进场校验需排除已删除角色。
- `PlayerInfo` 协议补充 `level`/`created_at` 之外的最小展示字段（暂不扩，见协议设计）。
- `select` 改为返回该账号下未删除角色列表。
- `create` 改为真正允许多次创建，受配置上限约束；重名处理见业务规则。
- `enter` 必须校验 `player_id` 属于当前账号且未删除，缺失 `player_id` 直接拒绝。
- 新增 `game.player.delete` 软删除路由。
- 新增配置项控制角色上限。
- 数据迁移：现有单角数据保留；唯一索引降级为普通索引需在迁移脚本中处理。

### 不包含

- 角色改名、转服、转职等扩展操作。
- 软删角色的恢复 / 回收站功能。
- 角色等级、外观等额外属性建模。
- 跨账号角色转移。
- 多角色并发登入同一账号的互斥策略（沿用现有 `auth.session_policy`）。

---

## 协议设计

| 类型 | Route / 事件 | 请求 | 响应 / Push | 说明 |
|------|----------------|------|-------------|------|
| RPC | `game.player.select` | `google.protobuf.Empty` | `PlayerSelectResponse`（`list` 改为多角色） | 返回当前账号未删除角色列表 |
| RPC | `game.player.create` | `PlayerCreateRequest{name}` | `PlayerCreateResponse{player}` | 受上限约束，达上限返回 `40025` |
| RPC | `game.player.enter` | `EnterGameRequest{player_id, scene_id}` | `EnterGameResponse` | `player_id` 必填且必须属于该账号、未删除 |
| RPC | `game.player.delete` | `PlayerDeleteRequest{player_id}` | `google.protobuf.Empty` | 软删除；校验归属 |

**新增协议草案（player.proto）：**

```proto
message PlayerDeleteRequest {
  int64 player_id = 1 [json_name = "playerId"];
}
```

**业务错误码草案：**

- `40026 PlayerLimitExceeded` — 账号角色数已达上限（`create` 时触发）
- `40027 PlayerDeleted` — 角色已删除（`enter`/`delete` 命中已删除角色）
- 复用 `40008 PlayerNotFound` 表示 `player_id` 不属于该账号或不存在

**前置条件：** `select/create/delete` 需已登录（Session 含 `uid`）；`enter` 需 `player_id > 0`。

---

## 数据与持久化

| 表 / 缓存 | 字段 / Key | 索引 / TTL | 说明 |
|-----------|------------|------------|------|
| `players` | `player_id` | 主键 | 全局短数字 ID（沿用 `id_sequences`） |
| `players` | `uid` | 唯一索引 → 普通索引 | 允许同账号多角色 |
| `players` | `deleted_at` | 普通索引 | 软删除时间，`NULL` 表示未删除 |
| `players` | `name` | 普通索引（可选） | 名称唯一范围见业务规则 |
| `id_sequences` | `player_id` | — | 沿用全局短 ID 分配 |
| Redis | `mmo:player:uid:<uid>` | TTL 沿用 | 原「单角缓存」语义失效，见下方说明 |

**缓存变更：** 现有 `playerUIDKey(uid)` 只缓存单个角色，多角后语义不再成立。一期方案：
- 移除该单角色缓存路径，`select` 直接查 DB（带 `uid` 普通索引 + `deleted_at IS NULL`）。
- 不引入角色列表缓存，避免软删后一致性问题；如后续需要再加 `mmo:player:list:uid:<uid>`。

**事务：** `create` 在同一事务内推进 `id_sequences` 并插入 `players`；`delete` 在事务内将 `deleted_at` 置为当前时间。`AfterCommit` 不再刷单角色缓存。

**迁移：**
- `autoMigrateModels` 需要把 `players.uid` 从唯一索引降级为普通索引。GORM `AutoMigrate` 不会自动删除既有索引，需在迁移代码中显式 `DROP INDEX uniq_players_uid`（兼容 MySQL 语法）后再创建普通索引。
- 现有单角数据保留，`deleted_at` 默认 `NULL`。

**配置：**

| 配置路径 | 默认 | 说明 |
|----------|------|------|
| `player.max_characters` | `3` | 单账号最大角色数（含未删除） |

---

## 业务规则

- 角色上限按「未删除角色数」计算；软删除不释放配额不释放（如要释放需另立恢复/回收站机制，一期不做）。
- 名称唯一性范围：暂定「全服未删除角色名唯一」，靠 `players.name` + `deleted_at IS NULL` 的唯一约束/查询保证。若实现成本高，退化为仅校验同账号内不重名。
- `enter` 必须带 `player_id`：`player_id == 0` 或不传，返回 `40008`；不属于该账号或已删除返回对应错误。
- `delete` 必须校验 `player_id` 属于当前账号；已删除返回 `40027`。
- 软删后该 `player_id` 不可复用（不回滚 sequence），避免背包/存档等历史数据错挂。
- 合服约束沿用全局 ID 策略：`player_id` 仍由 `id_sequences` 全局分配，不含服务器语义。

---

## 验收标准

- [ ] 同一账号可创建 ≥2 个角色，`select` 返回全部未删除角色。
- [ ] 达到配置上限后 `create` 返回 `40026 PlayerLimitExceeded`。
- [ ] `enter` 带 `player_id=0` 或他人 `player_id` 时被拒绝（`40008`）。
- [ ] `delete` 软删角色后，该角色不再出现在 `select`，且 `enter` 返回 `40027`。
- [ ] `players.uid` 不再是唯一索引，现有单角数据迁移后可正常 `select/enter`。
- [ ] `go test ./...` 通过。
- [ ] `client-demo` 冒烟可完成「选角为空 → 创角 → 选角列表 → 进场」。

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | 依赖已完成的全局 ID 策划（`2026-06-25-global-id`） |
| 迁移 | GORM `AutoMigrate` 不删旧唯一索引，需显式迁移；MySQL 与 SQLite（测试用）语法需兼容 |
| 缓存 | 移除单角色缓存可能增加 `select` 的 DB 压力，一期可接受 |
| 兼容 | `enter` 语义变严（必须带 `player_id`），旧客户端不带 ID 会失败，需同步更新 `client-demo` |
| 合服 | 软删 `player_id` 不复用，保证历史背包/存档不串号 |

---

## 实施索引

策划 **approved** 后：

1. 复制 [`_template.md`](_template.md) → `2026-06-25-multi-character.md`
2. 实施 plan 文首 **Design:** 链接本文件
3. 任务清单从本章「协议 / 数据 / 验收」派生，不重复粘贴策划正文
