# 背包内存化 策划文档

> **Status:** approved
> **实施计划：** [`2026-10-26-bag-inmemory.md`](2026-10-26-bag-inmemory.md)

**For agentic workers:** 填完整文各章节；`Status: approved` 前不得创建实施计划、不得改业务代码。无把握处写「待定」并列入风险。

---

## 背景与目标

当前背包是**数据库权威 + Redis 旁路缓存**：`actor_bag` 的每一次 `list/add/remove/move/split/use` 都同步打 MySQL（`SELECT ... FOR UPDATE` + `INSERT/UPDATE/DELETE`），提交后 `AfterCommit` 刷 Redis。LT-11 压测暴露了这条路径的死锁——`addOrStackItemInTx` 对空背包做空集 `SELECT ... FOR UPDATE`，在 InnoDB REPEATABLE READ 下产生间隙锁，50 人并发 `add` 时不同玩家互相等锁，MySQL 报 `1213`，44/50 失败。

目标：**背包状态内存化**——进场时整包加载进 bag Actor 内存，操作只改内存（免锁、免 DB），脏包由定时器 + 关键事件（断线/切图/下线）统一落库。这是国内 MMO 背包的主流终态，顺带把「Redis 双写一致性」问题也消解掉。

一期交付边界一句话：**bag Actor 成为背包的唯一内存权威，玩家主动操作全程免 DB；reward / GM 两条外部写路径统一收编到 bag Actor 或失效重载，脏包定时 + 关键事件落库。**

---

## 用户场景

1. **进场加载：** 玩家 `enter` 成功后，服务端异步把该 uid 全部背包读进 bag Actor 内存。之后 `game.bag.list` 直接读内存返回，不碰 MySQL。
2. **高频操作：** 玩家连续 `add/remove/move/split/use`，全部在内存完成并立即 `Response` + 推 `onBagChange`，不落库；每个包打上 `dirty` 标记。
3. **定时落库：** 每隔 N 秒（默认 5s），bag Actor 把带 `dirty` 的包 flush 到 MySQL（整包 diff）。
4. **关键事件落库：** 玩家断线（sessionClose）、切图、下线时，立即 flush 该玩家的脏包，保证退出后数据不丢。
5. **离线卸载：** 玩家离线后内存态保留 5 分钟（承接快速重连），超时仍未进场则 flush 余量并清空该玩家内存数据，落回 MySQL。
6. **外部发放：** GM 发奖 / 击杀奖励发放到**在线**玩家（`world.AgentPath(uid)` 命中）时，改 bag Actor 内存；**离线**玩家直接写 DB。

---

## 功能范围

### 包含

- bag Actor 内存数据结构：按 `bagType` 持有槽位 → `(itemId, count)`，附 `dirty` 标记。
- 进场时整包加载进内存；断线 / 下线时立即 flush；离线 5 分钟后 flush 余量并清空内存。
- 玩家 6 个操作（list/add/remove/move/split/use）改走内存，不再逐次打 MySQL。
- 定时 flush（`Timer().Add`，间隔可配）+ 关键事件 flush（断线、切图、下线）+ 离线超时卸载。
- 外部写路径收编：GM grant/deduct、reward 击杀奖励 / 领奖统一——**在线（`world.AgentPath(uid)` 命中）改 bag Actor 内存，离线直接写 DB**。
- 落库复用现有 `WithinTx` 写路径（保留 `SELECT ... FOR UPDATE` + 唯一索引兜底），并移除或弱化 Redis 背包缓存。

### 不包含

- 跨 game 节点的背包迁移 / 分布式一致性（背包仍按 uid 路由到单一 game 节点，见「风险」）。
- 背包操作的事务回滚语义向客户端暴露（内存操作即时生效，落库失败只告警 + 重试，不阻塞玩家）。
- 仓库 / 装备栏等跨背包 `move`（二期，不在本次）。

---

## 协议设计

**协议不变**：`game.bag.list/add/remove/move/split/use` 的请求/响应/Push 全部保持现状，客户端无感。本次是纯服务端存储层重构。

| 类型 | Route / 事件 | 请求 | 响应 / Push | 说明 |
|------|----------------|------|-------------|------|
| RPC | `game.bag.list` | `BagListRequest` | `BagListResponse` | 改为读内存 |
| RPC | `game.bag.add/remove/move/split/use` | 同现状 | `BagListResponse` | 改为改内存 + 标脏 |
| Push | `onBagChange` | — | `BagListResponse` | 内存态即时下发，不变 |

**业务错误码：** 不变（`40018/40019/40020/40021/40022/40023/40028`）。

**前置条件：** 不变——须 `enter` 完成（Session 含 `PlayerID`）。

---

## 数据与持久化

### 内存结构（bag Actor 内）

```
actorBag {
    bags map[int32]*memBag   // bagType -> 内存背包
}
memBag {
    slots map[int32]memSlot  // slot -> 堆叠
    dirty bool               // 是否有未落库变更
}
memSlot { itemID int32; count int32 }
```

- 内存态即权威；`list` 由 `memBag` 组装 `BagListResponse`（含空槽位，与现有一致）。
- `dirty=true` 的包进入定时 flush 队列；flush 成功后置 `dirty=false`。

### MySQL `inventory_items`（不变）

表结构与唯一索引 `(player_id, bag_type, slot)` **完全不变**，作为内存态的下沉快照。落库仍走 `WithinTx` + 行锁，保留并发兜底（离线 GM 直写时仍安全）。

### Redis 缓存（弱化 / 移除）

内存化后，在线玩家的读不再走 Redis。唯一残留：**离线玩家**的背包可能被 GM 直写后需要缓存失效。建议：flush 落库后不再写 Redis 背包缓存，改为**删除**对应 key，由下次加载自然重建。这样「Redis 双写一致性」问题整体消失。

### 落库策略（整包 diff）

flush 只写变更行，不整包重写。内存态为每个包维护变更集：

- `added`：本次新增的槽（INSERT）。
- `changed`：数量/道具发生变化的槽（UPDATE）。
- `removed`：被清空的槽（DELETE）。

flush 把这批变更放进**同一个 `WithinTx`**，复用现有行锁与唯一索引兜底；提交成功后清空变更集并置 `dirty=false`。相比整包覆盖，binlog 量与索引压力最小；相比复用 *InTx 的增量语义，diff 是「内存态相对落库态」的自然表达，不引入新的空集锁。

---

## 业务规则

- **免锁来源**：bag 子 Actor 按 `uid` 路由，cherry 保证同一 Actor 的消息串行处理，内存 map 无需手动加锁。前提是**内存只被 bag Actor 触碰**（外部写入者不得直接改内存）。
- **加载**：`enter` 成功后，player Actor 向 bag Actor 发 `load` 预热（异步 fire-and-forget，不阻塞 enter 响应）；bag Actor 从 MySQL 读全部 `bagType` 装入内存。加载失败退化为懒加载（首次操作时再读）。
- **flush 触发**：① 定时（默认 5s，`Timer().Add` 循环）；② 关键事件（断线 sessionClose、切图、下线）。
- **离线卸载**：玩家离线时立即 flush 脏包（不卸载），再起一个 5 分钟倒计时；期间若重新进场则取消倒计时、重载内存；倒计时到期仍未进场则 flush 余量并清空该玩家内存（bag Actor 本身常驻不退）。
- **外部写路径全部收编进 bag Actor**（本次核心，已定）：
  - **GM grant/deduct**：GM 进程经 NATS 到 game 节点 `gm` Actor，`doGrant`/`doDeduct` 不再直接打 DB，改为 `CallWait` 跨 Actor 投递到 `bag` 子 Actor 改内存并同步拿回最新背包/错误码；`bag` 子 Actor 据此标脏，flush 时落库。离线玩家（`world.AgentPath` 未命中）仍走现有直接落库。
  - **reward 击杀奖励 / 领奖**：`DeliverKillItem` / `TakeRewardClaim` 当前在 combat / reward Actor 内直接打 DB。内存化后在线玩家改经 bag Actor 内存（整份入包或整份进待领取的决策也迁入 bag Actor，避免「容量判断」与「写入」跨 Actor 分裂）；离线玩家仍直接落库。
  - **一致性兜底**：任何仍走直写 DB 的路径（仅离线玩家），写完后必须失效该 uid 的内存或保证上线时从 MySQL 重读最新（删除 Redis 缓存即可）。

---

## 验收标准

- [ ] 进场后 `list/add/remove/move/split/use` 全程不产生 MySQL 写（可观测 SQL 日志）。
- [ ] 定时 flush 与关键事件 flush 能把脏包落库，重启后数据一致。
- [ ] 断线 / 下线后脏包不丢。
- [ ] 离线 5 分钟后内存数据被卸载、落回 MySQL；期间重连不丢数据、直接复用内存。
- [ ] GM 发奖、击杀奖励、领奖三条外部路径与内存态一致（在线改内存、离线落库）。
- [ ] 复测 LT-11：50 人并发 `add` 不再死锁（内存化后无 DB 写竞争）。
- [ ] 全链路 2 人背包序列通过（复用 `cmd/lt11` fullChain）。
- [ ] `go test ./...` 通过；`go build ./...` 通过。

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 外部写路径收编 | 已定「全部收编进 bag Actor」。难点在于 GM 需 `CallWait` 同步返回结果、reward 的容量判断与写入要一起迁入 bag Actor 不跨 Actor 分裂。 |
| 崩溃丢失 | 内存化固有 tradeoff：定时 flush 间隔内的变更在进程崩溃时丢失。间隔越短丢失越少、DB 压力越大。默认 5s。 |
| 单节点绑定 | 背包内存绑定单一 game 节点；多 game 节点下需确认 uid 路由保证同 uid 恒在同一节点（cherry `ClusterLocalDataRoute` 按 uid hash）。 |
| 落库策略 | 已定「整包 diff」，变更集 added/changed/removed 单事务写入。 |
| 离线直写一致性 | 离线 GM 直写 DB 后，玩家上线加载需读到最新（删除 Redis 缓存 + 从 MySQL 重读可保证）。 |
| Actor 生命周期 | 已定「Actor 常驻 + 内存数据 5 分钟卸载」：bag Actor 本身一旦创建常驻不退（不 `Exit()` 自毁，避免自毁竞态）；它持有的玩家内存数据在离线 5 分钟后 flush 余量并清空。GM 在线改内存的前提（bag Actor 存活）因此天然成立。 |

---

## 实施索引

策划 **approved** 后：

1. 复制 [`_template.md`](_template.md) → `2026-10-26-bag-inmemory.md`
2. 实施 plan 文首 **Design:** 链接本文件
3. 任务清单从本章「协议 / 数据 / 验收」派生

**阶段划分（供实施参考）：**

- **阶段 1（地基，独立可交付）**：bag Actor 内存结构 + 进场加载 + 6 操作改内存 + 定时/关键事件 flush（整包 diff）。
- **阶段 2（外部写收编）**：GM grant/deduct、reward 击杀/领奖全部改经 bag Actor（在线），离线仍直写 DB。
- **阶段 3（缓存清理 + 回归）**：移除/弱化 Redis 背包缓存；LT-11 复测 + 全链路回归。
