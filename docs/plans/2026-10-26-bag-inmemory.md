# 背包内存化 实施计划

> **Status:** in_progress
> **Design:** [`2026-10-26-bag-inmemory-design.md`](2026-10-26-bag-inmemory-design.md)（已 `approved`）
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** bag Actor 成为背包唯一内存权威——玩家 6 个背包操作全程免 DB，脏包定时/关键事件整包 diff 落库，GM/reward 外部写在线改内存、离线直写 DB。

**Architecture:** 内存态按 `bagType → slots` 存于 bag 子 Actor（按 uid 串行，免锁）；`dirty` + 变更集（added/changed/removed）驱动整包 diff 落库；Actor 常驻、玩家内存数据离线 5 分钟卸载。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis

---

## 任务清单

### Task 1: bag Actor 内存态 + 进场加载

**Files:**
- Create: `internal/gameapp/bag/mem.go`（内存结构 + 组装 `BagListResponse`）
- Modify: `internal/gameapp/bag/actor_bag.go`（加 `load` 路由、持有内存态）
- Modify: `internal/gameapp/player/actor_player.go`（`enter` 后发 `load` 预热）

- [x] `memBag`/`memSlot` 结构与 `toProto` 组装（含空槽位，与现有一致）
- [x] `actorBag` 持 `bags map[int32]*memBag`，`OnInit` 注册 `load`
- [x] `load` 从 MySQL 读全部 `bagType` 装内存；失败退化为懒加载
- [x] `enter` 成功后 fire-and-forget 调 `bag.load`
- [x] `go test ./...` 通过

### Task 2: 6 操作改内存 + 变更集

**Files:**
- Modify: `internal/gameapp/bag/actor_bag.go`（list/add/remove/move/split/use 改内存）
- Modify: `internal/gameapp/bag/mem.go`（add/remove/move/split 内存语义 + 变更集）

- [x] `list` 读内存返回
- [x] `add` 内存堆叠/占空槽，标 dirty + 记录变更集
- [x] `remove/move/split` 内存语义复用现有错误码（`ErrBagNotEnough` 等）
- [x] `use` 先挂 Buff 成功才扣内存 + 标脏
- [x] 变更集维护 added/changed/removed
- [x] `go test ./...` 通过

### Task 3: flush 整包 diff 落库

**Files:**
- Create: `internal/persistence/bag_flush.go`（变更集单事务写库）
- Modify: `internal/gameapp/bag/actor_bag.go`（定时 flush + 关键事件 flush）

- [x] `FlushBagDiff(ctx, playerID, bagType, diff)` 单 `WithinTx` 写 added/changed/removed
- [x] 提交后清变更集 + 置 dirty=false，并删除 Redis 背包缓存 key
- [x] 定时 flush：`Timer().Add(5s)` 循环，只 flush 带 dirty 的包
- [x] 关键事件 flush：断线（sessionClose）、切图、下线立即 flush
- [x] `go test ./...` 通过

### Task 4: 离线 5 分钟卸载

**Files:**
- Modify: `internal/gameapp/bag/actor_bag.go`

- [x] 断线时立即 flush + 起 5 分钟 `AddOnce` 倒计时
- [x] 重新进场（`load`）取消倒计时、复用内存
- [x] 倒计时到期 flush 余量 + 清空该玩家内存
- [ ] 5 分钟值走 profile 配置（默认 300s）
- [x] `go test ./...` 通过

### Task 5: GM grant/deduct 收编

**Files:**
- Modify: `internal/gameapp/gm/actor_gm_bag.go`
- Modify: `internal/gameapp/bag/actor_bag.go`（`grant`/`deduct` 远程入口）

- [x] `doGrant`/`doDeduct` 在线（`world.AgentPath` 命中）改 `CallWait` 到 bag Actor
- [x] 离线仍走现有 `persistence.AddOrStackItem`/`RemoveItem` 直写 DB
- [x] 同步拿回最新背包/错误码给 GM
- [x] `go test ./...` 通过

### Task 6: reward 击杀/领奖收编

**Files:**
- Modify: `internal/gameapp/reward/apply.go`
- Modify: `internal/gameapp/reward/actor_reward.go`
- Modify: `internal/gameapp/bag/actor_bag.go`（容量判断 + 写入入口）

- [x] `DeliverKillItem`/`TakeRewardClaim` 在线改经 bag Actor（含容量判断，不跨 Actor 分裂）
- [x] 离线仍直接落库（现有 `persistence.DeliverKillItem`/`TakeRewardClaim`）
- [x] 待领取（RewardClaim）决策迁入 bag Actor
- [x] `go test ./...` 通过

### Task 7: Redis 背包缓存弱化 + 回归

**Files:**
- Modify: `internal/persistence/bag.go`（或 bag_flush.go）

- [x] flush 后删除 Redis 背包缓存 key（不再写）
- [x] 读路径加载前从 MySQL 重读（删除缓存重建即可）
- [x] 复测 LT-11（`cmd/lt11`）：50 人并发 `add` 不再死锁
- [x] 全链路 2 人背包序列通过（fullChain）
- [x] `go test ./...` 通过；`go build ./...` 通过

---

## 验证

- [x] `go test ./...`
- [x] `go build ./...`
- [x] 相关节点 `go build`（game / gm）
- [x] Docker 内复测 LT-11：50 人并发 `add` 无 1213 死锁、无 40020
- [x] 全链路 2 人背包序列（堆叠/扣减/移动/拆分）通过

## 备注

- 离线卸载时长（`offlineUnloadAfter = 5 * time.Minute`）与定时 flush 间隔（`flushInterval = 5 * time.Second`）暂为常量，未接 profile 配置。若需可配，参照 `nodecfg.ApplyCallTimeout` 的 `cprofile.GetConfig(...)` 模式加一个 `bag` 段即可。
- 在线外部写（GM/reward）框架码失败降级直写 DB 后，会 fire-and-forget 调 `bag.invalidate` 失效内存，避免内存与 DB 分叉。
