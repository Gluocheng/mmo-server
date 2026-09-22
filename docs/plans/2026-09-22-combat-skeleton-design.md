# 战斗骨架 策划文档

> **Status:** approved  
> **实施计划：** [`2026-09-22-combat-skeleton.md`](2026-09-22-combat-skeleton.md)

**For agentic workers:** 一期只做配表驱动的战斗框架与 50 人同场结算。阵营、怪物、药水不在本期。

---

## 背景与目标

场景目前只有进场和 AOI 移动，角色没有生命，不能互相攻击。需要一条服务端权威的战斗管线：技能和 Buff 全部来自配表，其它模块只通过接口进场、离场、放技能和挂 Buff。

**一期目标：** 同一场景按 50 人挤在一起互打来设计。心跳结算点名与自身圆心技能，命中可挂配表 Buff；每个观众每拍最多收到一条合并后的 `onCombatFrame`。

---

## 用户场景

1. 两人进同一场景，靠近后放普攻，下一心跳扣血，双方和 AOI 内的人收到同一拍的战斗包。
2. 多人挤在一起放范围斩，半径内除自己外的存活玩家都掉血，并挂上配表里的持续伤害。
3. 血量到 0 后不能出手也不能被打，配表里的复活毫秒过后原地满血。

---

## 功能范围

### 包含

- Luban 表 `skill` / `buff` / `combat_const`，导入 MySQL 后由现有 `runtime.Load` 读入
- 进程内接口 `Enter` / `Leave` / `Cast` / `ApplyBuff` / `Snapshot`
- `game.combat.cast` 与 Push `onCombatFrame`
- 心跳内结算冷却、范围、Buff 跳伤与到期、死亡复活
- 广播按观众合并，单包条数上限读配表，优先保留与自己有关的命中
- 错误码 `40040`–`40046`；未进场仍是 `40009`

### 不包含

- 阵营、吟唱、装备、仇恨、怪物、掉落、生命落库、客户端预演
- 药水（下一期只调用 `ApplyBuff`）
- 控制类 Buff（例如禁手）

---

## 协议设计

| 类型 | Route / 事件 | 请求 | 响应 / Push | 说明 |
|------|----------------|------|-------------|------|
| RPC | `game.combat.cast` | `CombatCastRequest` | `Empty` | 只接受意图；成败用业务码 |
| Push | `onCombatFrame` | — | `CombatFrame` | 该观众这一拍的命中列表 |

**业务错误码：**

| 码 | 常量 | 说明 |
|----|------|------|
| 40040 | `CombatSkillInvalid` | 技能不存在或目标方式非法 |
| 40041 | `CombatBuffInvalid` | Buff 不存在 |
| 40042 | `CombatTargetInvalid` | 目标不在同场景，或点名打到自己 |
| 40043 | `CombatOutOfRange` | 超出技能距离 |
| 40044 | `CombatCooldown` | 技能冷却中 |
| 40045 | `CombatSelfDead` | 自己已死亡 |
| 40046 | `CombatTargetDead` | 目标已死亡 |

**前置条件：** 已 `enter`。网关白名单不改。

---

## 数据与持久化

| 表 / 缓存 | 字段 / Key | 索引 / TTL | 说明 |
|-----------|------------|------------|------|
| `cfg_skill` | id、name、target、cast_range、radius、cooldown_ms、damage、buff_id | 主键 id | `target`=`single` 或 `aoe_self`；`buff_id=0` 不挂 |
| `cfg_buff` | id、name、duration_ms、interval_ms、effect、value、max_stack | 主键 id | `effect`=`dot` 或 `hot` |
| `cfg_combat_const` | id=1、max_hp、tick_ms、respawn_ms、frame_event_cap | 主键 id | 单行 |
| 进程内存 | 单位生命、冷却、Buff 实例、技能意图队列 | 离场即删 | 不写 `players` |

**事务：** 配表导入沿用现有 import 单事务。战斗结算不落库。

**热更：** 沿用 `game.config.reload`。新释放的技能读新表。Buff 在施加时拷贝当时的数值，热更不改已经挂上的实例。

**种子：** 普攻 id=1（距离 3、冷却 1000ms、伤害 10）；范围斩 id=2（半径 5、冷却 3000ms、伤害 8、命中挂 Buff 1）；Buff 1 为 3000ms 持续伤害，每 1000ms 跳 4 点。最大生命 100，心跳 100ms，复活 5000ms，单包上限 64。

---

## 业务规则

- `Cast` 当时就拒绝非法技能、冷却、距离、死亡；通过后把技能参数拷进队列，下一拍才扣血。
- 范围技能以施法者坐标为圆心，不打自己，不打死亡目标，不打其它场景。
- 观众与命中目标或来源处于 AOI（半径 15）内才收到该条；每人每拍一条 `onCombatFrame`。超过 `frame_event_cap` 时截断，优先留「自己打出」和「打到自己」。
- 死亡清除身上 Buff。复活只恢复生命，不补 Buff。
- 进场满血读 `max_hp`。离场由 `player` 的进场与断线各调用一次 `Enter` / `Leave`。

---

## 验收标准

- [ ] 配表可导入，启动 `Load` 能读到技能、Buff 和常量
- [ ] 范围技能打中半径内的活人，打不中圈外、自己和死人
- [ ] Buff 按间隔跳伤，到期后不再跳
- [ ] 同一拍多次命中合成每个观众一条 frame
- [ ] 50 个重叠单位一次范围技能只产生 50 条 frame
- [ ] `go test ./...` 通过

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | 现有场景坐标与 AOI；配表 import / Load |
| 迁移 | 新表由 AutoMigrate 创建，空表直到重新 import |
| 兼容 | 新路由，旧客户端不发 `cast` 则无行为变化 |

---

## 实施索引

策划已 approved。实施计划见 [`2026-09-22-combat-skeleton.md`](2026-09-22-combat-skeleton.md)。
