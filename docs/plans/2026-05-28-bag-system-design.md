# 背包系统 策划文档

> **Status:** approved  
> **说明：** 已实现功能回溯归档（v1 堆叠读写 + v2 槽位整理）；非待开发需求。  
> **实施计划：** [v1](2026-05-28-bag-system.md) · [v2](2026-05-28-bag-v2.md)（均 **done**）

---

## 背景与目标

为演示服提供**玩家级背包**：持久化道具、支持查看与变更，供客户端与 `client-demo` 联调。

- **v1：** `list` / `add` / `remove`，MySQL `inventory_items` 按 `item_id` 堆叠，Redis 缓存加速读取。
- **v2：** 固定 **32 槽**（`slot` 0–31）、`move` / `split`、变更后 **Push `onBagChange`**；`add` 优先堆叠再占空槽。

架构：game 节点 `bag` 根 Actor，**按 uid 动态子 Actor**；写库走 `WithinTx`，提交后 `AfterCommit` 刷新背包 Redis 缓存。须 **进场**（Session 含 `PlayerID`）后方可操作（与聊天一致）。

---

## 用户场景

1. **查看背包：** 登录 → `game.player.enter` → `game.bag.list` → 收到 `BagListResponse`（含 `slot`、`itemId`、`count`）。
2. **发放道具（测试/GM）：** `game.bag.add`（`itemId` + `count`）→ RPC 返回最新背包 + Push `onBagChange`。
3. **玩家整理：** `game.bag.move`（拖放/合并/交换）或 `game.bag.split`（拆堆）→ RPC + Push；`game.bag.remove` 按槽或按物品 ID 扣减。

---

## 功能范围

### 包含

- RPC：`game.bag.list` / `add` / `remove` / `move` / `split`
- Push：`onBagChange`（`BagListResponse`，与变更类 RPC 响应体一致）
- 持久化：`inventory_items` 表 + Redis protojson 缓存
- 启动迁移：`migrateInventorySlots`（v1 无槽数据 → 按 id 递增分配 slot）
- 单元测试：`internal/persistence/bag_test.go`；`cmd/client-demo` 冒烟（含 move/split）
- 业务错误码：`40018`–`40022`

### 不包含

- 物品配置表 / `item_id` 合法性校验（见 backlog 物品配置表）
- 装备栏、快捷栏、仓库、交易、掉落、绑定/冷却
- 服务端主动整理（自动排序）、槽位扩容、物品实例 UUID

---

## 协议设计

| 类型 | Route / 事件 | 请求 | 响应 / Push | 说明 |
|------|----------------|------|-------------|------|
| RPC | `game.bag.list` | `google.protobuf.Empty` | `BagListResponse` | 只读，不 Push |
| RPC | `game.bag.add` | `BagAddRequest` | `BagListResponse` | `itemId`、`count`（默认 ≥1） |
| RPC | `game.bag.remove` | `BagRemoveRequest` | `BagListResponse` | 见下方 `bySlot` |
| RPC | `game.bag.move` | `BagMoveRequest` | `BagListResponse` | `fromSlot`、`toSlot` |
| RPC | `game.bag.split` | `BagSplitRequest` | `BagListResponse` | `fromSlot`、`count` |
| Push | `onBagChange` | — | `BagListResponse` | `add`/`remove`/`move`/`split` 成功后下发 |

**`BagRemoveRequest`：**

- `bySlot = true`：从 `slot` 扣减 `count`
- `bySlot = false`：按 `itemId` 从 slot **升序**各槽合计扣减 `count`

**消息定义：** `internal/protocolpb/proto/bag.proto` → `internal/protocolpb/gen/bag.pb.go`

**业务错误码：**

| 码 | 常量 | 含义 |
|----|------|------|
| 40018 | `BagItemInvalid` | itemId/count 非法或 move 合并失败 |
| 40019 | `BagItemNotEnough` | 持有数量不足 |
| 40020 | `BagLoadFail` | 加载背包失败 |
| 40021 | `BagSlotInvalid` | 槽位越界或源槽为空 |
| 40022 | `BagFull` | 无空槽（add 占槽 / split 新槽） |

**前置条件：** 网关已登录；`game.player.enter` 完成（Session `PlayerID`）；否则 `40009 PlayerNotEntered`。

**节点：** game 服 `ActorBags`（`AliasID: bag`），子 Actor 路由 `game.bag.*`。

---

## 数据与持久化

### MySQL `inventory_items`

| 字段 | 说明 |
|------|------|
| `id` | 自增主键 |
| `player_id` | 角色 ID |
| `slot` | 槽位 0..31，与 `player_id` 组成唯一索引 `idx_player_slot` |
| `item_id` | 物品类型 ID（当前不校验配置表） |
| `count` | 数量 1..9999 |

模型：`internal/persistence/model_inventory_item.go`  
逻辑：`internal/persistence/bag.go`

### Redis 缓存

- Key：`{KeyPrefix}:bag:player:{playerId}`（`KeyPrefix` 见 persistence 配置）
- 值：`BagListResponse` protojson（`EmitUnpopulated: true`）
- 读：`GetBagByPlayerID` 先 Redis，未命中查库并回填
- 写：变更事务提交后 `AfterCommit` 重载 DB 并 `SET`（TTL 同其它玩家缓存）

### 事务

- 变更类 API：`WithinTx` + 行锁 `SELECT ... FOR UPDATE`
- 成功提交后：`scheduleBagCacheRefresh` 注册 `AfterCommit` 刷 Redis

### 迁移

`migrateInventorySlots`：将 v1 时期同玩家多行 / `slot=0` 数据按 `id` 升序重写为 `slot` 0,1,2...

---

## 业务规则

| 常量 | 值 | 说明 |
|------|-----|------|
| `MaxBagStack` | 9999 | 单槽最大堆叠 |
| `MaxBagSlots` | 32 | 槽位 0..31 |

**add（`AddOrStackItem`）：**

1. 锁定该玩家所有同 `item_id` 行，按堆叠上限填满
2. 剩余数量占**最小编号空槽**；单槽最多放 `MaxBagStack`，可跨多槽
3. 无空槽 → `ErrBagFull`（40022）

**remove：**

- **按槽：** 源槽数量须 ≥ `count`；扣完删行，否则减 `count`
- **按 itemId：** 从 slot 小到大扣，合计须 ≥ `count`

**move（`MoveItem`）：**

- 目标槽空：更新源行 `slot`
- 目标同 `item_id`：合并至目标（受 `MaxBagStack` 限制），源槽可部分剩余或删除
- 目标异 `item_id`：交换两槽的 `item_id` 与 `count`

**split（`SplitItem`）：**

- 源槽 `count` 须 **严格大于** 拆分数量
- 新堆叠写入最小编号空槽；源槽减去 `count`

**list：** 按 `slot` 排序返回；过滤 `count<1` 或 `itemId<1` 的脏行。

---

## 验收标准

- [x] `game.bag.list` / `add` / `remove` 可用，持久化与缓存一致
- [x] v2：`move`（空槽移动、同物品合并、异物品交换）、`split`、`onBagChange` Push
- [x] `remove` 支持 `bySlot` 与按 `itemId` 跨槽扣减
- [x] `go test ./internal/persistence/...` 通过（含 move/swap/split/满包等）
- [x] 根目录 `README.md` 协议表与错误码已更新
- [x] `go run ./cmd/client-demo` 冒烟（add、list、move、split）

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | 道具配置见 [2026-05-28-item-system-design.md](2026-05-28-item-system-design.md)（策划 draft）；当前代码仍不校验 `item_id` |
| 迁移 | 仅 AutoMigrate + `migrateInventorySlots`；无版本化 migration 工具 |
| 兼容 | 客户端须处理 `slot` 字段与 `onBagChange`；旧客户端仅 list/add/remove 仍可用 |
| 二期范围 | 装备、掉落、配置表校验等明确不在本策划 |

---

## 实施索引

| 阶段 | 文档 | 状态 |
|------|------|------|
| v1 堆叠 list/add/remove | [2026-05-28-bag-system.md](2026-05-28-bag-system.md) | done |
| v2 槽位 move/split/Push | [2026-05-28-bag-v2.md](2026-05-28-bag-v2.md) | done |

代码入口：

- `internal/gameapp/bag/`（`ActorBags`、`actorBag`）
- `internal/persistence/bag.go`
- `internal/protocolpb/proto/bag.proto`
