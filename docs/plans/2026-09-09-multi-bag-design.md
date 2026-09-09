# 多背包系统 策划文档

> **Status:** approved
> **实施计划：** [`2026-09-09-multi-bag.md`](2026-09-09-multi-bag.md)

**For agentic workers:** 填完整文各章节；`Status: approved` 前不得创建实施计划、不得改业务代码。无把握处写「待定」并列入风险。

---

## 背景与目标

当前背包是**单容器模型**：`inventory_items` 只有 `(player_id, slot)`，没有「背包类型」维度。实际需求是——`slot` 只是背包的格子，而「背包」只是众多容器类型之一；后续会有多种背包（装备/材料/任务/仓库…），每类背包装不同种类道具，道具按配置路由到指定背包。

目标：为背包引入**可配置的 `bag_type` 维度**，让「新增一种背包」变成「加一行配置」，并让道具按配置自动进入对应背包。原「扩容 / 仓库 / 装备栏」等方向统一收编为 `bag_type` 之上的应用层。

---

## 用户场景

1. **发放道具自动入包：** GM 或玩法 `game.bag.add(itemId, count)` → 服务端按 `item.bag_type` 路由到对应背包，客户端在对应背包看到新道具。
2. **查看/操作指定背包：** 客户端 `game.bag.list(bagType)` 渲染某背包；`remove/move/split` 均携带 `bagType`，在该背包内生效。
3. **GM 指定背包发放：** GM 显式指定 `bagType` 发放；若道具类别与指定背包不符，被 `40028` 拒绝。

---

## 功能范围

### 包含

- `bag_type` 配置表（gameconfig 驱动，`id`/`name`/`slot_count`）。
- `inventory_items` 加 `bag_type` 列，唯一索引 `(player_id, bag_type, slot)`。
- `item.csv` 加 `bag_type`(int32) 字段，声明目标背包。
- `game.bag.add` 按 `item.bag_type` 自动路由；GM 可显式指定背包（仍严格校验）。
- `list/remove/move/split` 携带 `bag_type`，在指定背包内操作。
- 新增错误码 `40028 BagTypeMismatch`（道具类别与目标背包不符）。
- 存量数据迁移：回填 `bag_type = 通用背包(1)`。
- 单测覆盖多背包隔离、迁移回填、类型校验拒绝。

### 不包含

- 跨背包 `move`（装备穿脱、仓库转移）—— 二期。
- 装备栏/仓库等应用层玩法 —— 二期，基于 `bag_type`。
- 物品绑定/冷却/耐久（item_id 级或实例级）—— 三期。
- 服务端自动整理、槽位扩容（`slot_count` 已在配置中）。

---

## 协议设计

| 类型 | Route / 事件 | 请求 | 响应 / Push | 说明 |
|------|----------------|------|-------------|------|
| RPC | `game.bag.list` | `BagListRequest`（`bag_type`） | `BagListResponse`（`BagItem` 含 `bag_type`） | 指定背包 |
| RPC | `game.bag.add` | `BagAddRequest`（`bag_type` 可选） | `BagListResponse` | 缺省按 `item.bag_type` 自动路由 |
| RPC | `game.bag.remove` | `BagRemoveRequest`（`bag_type`） | `BagListResponse` | 指定背包内扣减 |
| RPC | `game.bag.move` | `BagMoveRequest`（`bag_type`） | `BagListResponse` | 同背包内移动 |
| RPC | `game.bag.split` | `BagSplitRequest`（`bag_type`） | `BagListResponse` | 同背包内拆分 |
| Push | `onBagChange` | — | `BagListResponse` | 变更成功后下发 |

**消息变化：** `BagAddRequest`/`BagRemoveRequest`/`BagMoveRequest`/`BagSplitRequest` 加 `bag_type` 字段；`BagItem` 加 `bag_type`（`list` 返回时带出，客户端据此渲染多背包）。`list` 需新增 `BagListRequest`（或复用带 `bag_type` 的请求消息），不再用 `google.protobuf.Empty`。

**业务错误码：**

| 码 | 常量 | 含义 |
|----|------|------|
| 40028 | `BagTypeMismatch` | 道具类别与目标背包不符（`item.bag_type != 目标背包`） |

**前置条件：** 网关已登录；`game.player.enter` 完成（Session 含 `PlayerID`），否则 `40009 PlayerNotEntered`。

---

## 数据与持久化

### MySQL `inventory_items`（加列）

| 字段 | 说明 |
|------|------|
| `id` | 自增主键 |
| `player_id` | 角色 ID |
| `bag_type` | 背包类型键（int32），新增 |
| `slot` | 槽位，与 `player_id`、`bag_type` 组成唯一索引 `idx_player_bag_slot` |
| `item_id` | 物品类型 ID |
| `count` | 数量 |

唯一约束从 `(player_id, slot)` 改为 **`(player_id, bag_type, slot)`**，不同背包的 slot 互不冲突。

### gameconfig 配置

- 新增 `bag_type` 表：`id`(int32 主键) / `name`(string) / `slot_count`(int32)。
- `item.csv` 加 `bag_type`(int32) 列，声明目标背包键；现有 `type`(string) 降级为展示分类。

### Redis 缓存

- Key 改为 `{prefix}:bag:player:{playerId}:{bagType}`（每背包独立缓存，protojson，TTL 同现有）。

### 事务

- 写路径沿用 `WithinTx` + `SELECT ... FOR UPDATE` 行锁，提交后 `AfterCommit` 刷对应 `bagType` 的 Redis 缓存。

### 迁移

存量行统一回填 `bag_type = 通用背包(1)`（`slot_count` 保持 32 承接全部旧数据）。**不**在迁移里按 `item_id` 回填精确类型——因为迁移在 gameconfig 加载之前执行，runtime 尚无法解析 `item_id → bag_type`（见「风险」）。

---

## 业务规则

| 常量 | 值 | 说明 |
|------|-----|------|
| `通用背包 bag_type` | 1 | 承接存量数据，`slot_count = 32` |
| `MaxBagStack` | 9999 | 单槽最大堆叠（不变） |

- **槽位数按背包类型**：`slot_count` 从 `bag_type` 配置读取，替换原 `MaxBagSlots=32` 常量。
- **严格类型约束**：`add` 时 `item.bag_type != 目标背包` → `40028`。普通 `add` 由 `item.bag_type` 决定目标；GM 显式指定仅覆盖「路由目标」，**不**绕过类型校验。
- **add 流程**：`gcruntime.Get(itemID)` 取 `bag_type` → 该背包 `slot_count` → 堆叠 + 占空槽（沿用现有逻辑）。
- **remove/move/split**：在指定 `bag_type` 背包内，复用现有语义（按槽/按 itemId 扣、移动/合并/交换、拆分）。

---

## 验收标准

- [ ] `bag_type` 配置驱动：新增背包类型只需加配置，不改代码。
- [ ] `item.bag_type` 路由：`add` 自动进入正确背包；GM 指定错误背包被 `40028` 拒绝。
- [ ] 多背包隔离：不同 `bag_type` 的 slot 互不冲突，缓存 key 独立。
- [ ] 存量迁移：`bag_type` 回填 `通用背包(1)`，旧数据不丢。
- [ ] `go test ./internal/persistence/...` 通过（含多背包隔离、迁移回填、类型校验拒绝）。
- [ ] `go build ./...` 通过。
- [ ] `go run ./cmd/client-demo` 冒烟多背包序列。
- [ ] 根目录 `README.md` 协议表与错误码已更新。

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 迁移时机 | `migrateInventorySlots` 在 `persistence.Init()` 内、gameconfig 加载**之前**执行，无法用 runtime 解析 `item_id → bag_type`。处理：统一回填 `通用背包(1)`，不回填精确类型。 |
| 唯一索引变更 | `(player_id, slot)` → `(player_id, bag_type, slot)`，需处理存量索引（`AutoMigrate` 不会删旧唯一索引，需显式 drop，参考现有 `downgradePlayerUIDUniqueIndex` 模式）。 |
| 协议兼容 | `BagItem` 加 `bag_type`、`list` 不再用 `Empty`，旧客户端需升级。 |
| 依赖 | 依赖 gameconfig 的 `bag_type` 表 + `item.bag_type` 字段落地。 |
| 二期 | 跨背包 `move`（装备穿脱/仓库转移）不在本次；届时需定义锁序与是否要求同类型。 |

---

## 实施索引

策划 **approved** 后：

1. 复制 `_template.md` → `2026-09-09-multi-bag.md`
2. 实施 plan 文首 **Design:** 链接本文件
3. 任务清单从本章「协议 / 数据 / 验收」派生

**阶段划分（供实施参考）：**

- **阶段 1（地基，独立可交付）**：`bag_type` 配置表 + `item.bag_type` 字段 + `inventory_items.bag_type` 列 + 迁移回填 + `bag.go` 参数化 + `40028` + 单测。
- **阶段 2（协议 + 路由）**：`bag.proto` 加字段 + `actor_bag` 自动路由与严格校验 + `client-demo` 冒烟。
- **阶段 3（二期）**：跨背包 move / 装备栏 / 仓库应用层。
