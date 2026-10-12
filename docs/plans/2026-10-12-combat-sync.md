# 战斗同步 实施计划

> **Status:** done  
> **Design:** [`2026-10-12-combat-sync-design.md`](2026-10-12-combat-sync-design.md)（**Status: approved**）

**Goal:** 进视野发一次战斗快照，心跳只推命中、Buff 和三项属性的变化，离开视野发消失。战斗仍在游戏节点内结算。

**Architecture:** 快照和消失由场景移动与进场触发。`onCombatFrame` 追加拍序号、Buff 变化和属性变化。没有变化不发包。截断时把自己的生命、属性和 Buff 放在 `self` 里。

**Tech Stack:** Go / Protobuf / 现有战斗心跳与 AOI

## 任务清单

- [x] `CombatSnapshot`、`CombatGone`，以及 `CombatFrame` 的 `tick`、`buffs`、`stats`、`self`
- [x] 进场、切图后推自己和附近单位的快照；走进 15 格推快照，走出推消失
- [x] 同一拍顺序：`onMove`、新快照、`onCombatFrame`。无变化不发帧，序号仍递增
- [x] 截断时保留自己的当前生命、三项属性和自己的 Buff
- [x] 验收单测与 `go test ./...`
- [x] 根目录 `README.md` 补上快照、消失和心跳里的 Buff、属性变化
