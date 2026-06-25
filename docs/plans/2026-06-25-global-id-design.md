# 全局 ID 生成策略策划文档

> **Status:** approved
> **实施计划：** （策划通过后创建）[`2026-06-25-global-id.md`](2026-06-25-global-id.md)

**For agentic workers:** `Status: approved` 前不得创建实施计划、不得改业务代码。

---

## 背景与目标

当前 `accounts.uid` 与 `players.player_id` 都依赖 MySQL 自增。这个方案在单库单服下简单，但不适合后续多服、合服或跨进程扩展：不同数据库独立自增后，合服时可能出现账号 ID 或角色 ID 冲突。

一期目标是调整账号与角色 ID 生成策略：账号 `uid` 使用全局唯一 Snowflake `int64`；角色 `player_id` 使用不携带服务器含义的全局短数字 ID。该策略需要保留现有登录、创建角色、进场流程的业务语义，同时为后续合服降低数据冲突风险。

---

## 用户场景

1. 玩家使用昵称与密码请求登录；账号不存在时创建账号，返回全局唯一 `uid`。
2. 玩家账号登录成功后创建角色；角色创建时分配一个短数字 `player_id`，该 ID 不包含服务器 ID。
3. 后续多服数据合并时，`uid` 与 `player_id` 在源头避免冲突，不再依赖合服脚本批量重写主键。

---

## 功能范围

### 包含

- 将账号 `uid` 从 MySQL 自增改为服务端主动生成的 Snowflake `int64`。
- 将角色 `player_id` 从 MySQL 自增改为中心化短数字 sequence 生成。
- 新增全局 ID 分配持久化模型，优先使用 MySQL 事务保证 sequence 更新一致性。
- 更新账号创建、角色创建、迁移与测试，确保 Redis key、Token、Session、背包等既有引用继续使用 `int64`。

### 不包含

- 不新增 `users` 表；当前账号表仍为 `accounts`。
- 不实现完整合服工具，只保证新生成数据具备合服友好的唯一性。
- 不把 `player_id` 编码为 `serverID + localID`，避免合服后 ID 带旧服归属语义。
- 不改客户端协议字段类型，`uid` 与 `player_id` 继续保持 `int64`。

---

## 协议设计

| 类型 | Route / 事件 | 请求 | 响应 / Push | 说明 |
|------|----------------|------|-------------|------|
| RPC | `connector.auth.issueToken` | `IssueTokenRequest` | `IssueTokenResponse` | 响应中的 `uid` 改为 Snowflake 生成值 |
| RPC | `game.player.create` | `CreatePlayerRequest` | `CreatePlayerResponse` | 响应中的 `player_id` 改为全局短数字 ID |
| RPC | `game.player.select` | `SelectPlayerRequest` | `SelectPlayerResponse` | 继续返回已有 `player_id` |
| RPC | `game.player.enter` | `EnterGameRequest` | `EnterGameResponse` | 继续按 `player_id` 进场 |

**业务错误码草案：** 不新增错误码。ID 分配失败按现有系统错误路径返回。

**前置条件：** 创建角色前必须已有有效登录 Session；进场前必须提交属于当前账号的 `player_id`。

---

## 数据与持久化

| 表 / 缓存 | 字段 / Key | 索引 / TTL | 说明 |
|-----------|------------|------------|------|
| `accounts` | `uid` | 主键，取消自增语义 | 由 Snowflake 生成，账号全局唯一 |
| `players` | `player_id` | 主键，取消自增语义 | 由全局短数字 sequence 生成，不包含服务器 ID |
| `id_sequences` | `name`, `next_value`, `updated_at` | `name` 唯一 | 保存全局短 ID 当前值 |
| Redis | `mmo:player:uid:<uid>` | TTL 沿用现有配置 | key 仍按 `uid` 缓存角色信息 |

**事务：** 账号创建仍在 `WithinTx` 内完成；角色创建时在同一事务内读取并推进 `id_sequences`，再插入 `players`。事务提交后继续通过 `AfterCommit` 刷新 Redis。

**初始值：** `player_id` sequence 初始建议为 `100000` 或 `100001`，避免与历史小自增 ID 混淆。若数据库已有历史角色，实施时应以 `max(players.player_id)+1` 与配置初始值取较大者初始化。

---

## 业务规则

- `uid` 必须全局唯一，不能依赖 MySQL 自增；同一昵称重复登录必须返回原 `uid`。
- `player_id` 必须短且全局唯一，不含服务器 ID、区服 ID 或旧服归属语义。
- `id_sequences` 更新必须和角色插入在同一事务中完成；角色插入失败时 sequence 更新也应回滚。
- 并发创建同一账号角色时，仍以 `players.uid` 唯一约束保证一个账号只创建一个角色。
- 合服场景下，不能引入各服本地递增生成器；所有服若共用一套 ID 空间，必须连接同一个 ID 分配源。

---

## 验收标准

- [ ] 新账号创建后 `accounts.uid` 不是 MySQL 自增生成，而是 Snowflake `int64`。
- [ ] 新角色创建后 `players.player_id` 是短数字 sequence，不包含服务器 ID。
- [ ] 并发创建同一角色不会产生两个角色，也不会破坏 sequence 一致性。
- [ ] `client-demo` 能完成账号登录、自动创建角色、选择角色、进场流程。
- [ ] `go test ./...` 通过。
- [ ] Docker smoke 流程仍能通过登录与创建角色验证。

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | 需要 Cherry Snowflake 默认节点在账号创建所在进程初始化，或在 persistence 内显式初始化 ID 生成器 |
| 迁移 | 已有自增数据可保留；新 sequence 初始值必须大于现有 `players.player_id` 最大值 |
| 兼容 | 协议字段仍为 `int64`，客户端无需改字段类型；但测试断言不能继续假设 ID 从 1 递增 |
| 多服 | 若多套数据库独立运行又要保证合服免冲突，需要共享同一个 ID 分配源或后续设计跨服 ID 服务 |

---

## 实施索引

策划 **approved** 后：

1. 复制 [`_template.md`](_template.md) → `2026-06-25-global-id.md`
2. 实施 plan 文首 **Design:** 链接本文件
3. 任务清单从本章「数据与持久化 / 业务规则 / 验收标准」派生，不重复粘贴策划正文
