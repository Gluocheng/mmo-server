# 战斗骨架 实施计划

> **Status:** done  
> **Design:** [`2026-09-22-combat-skeleton-design.md`](2026-09-22-combat-skeleton-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 配表驱动的战斗框架，支持同场景 50 人按心跳结算点名与范围技能，并按人合并广播。

**Architecture:** `gameconfig` 增加技能、Buff、战斗常量表。`internal/gameapp/combat` 持有内存状态并对外提供 Enter/Leave/Cast/ApplyBuff/Snapshot。Actor 只负责收意图、按配表心跳调用结算、Push `onCombatFrame`。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Luban

---

## 任务清单

### Task 1: 配表

**Files:** `gameconfig/defines`、`gameconfig/datas`、`gameconfig/gen/data`、`gameconfig/pkg/schema`、`gameconfig/pkg/importdata`、`gameconfig/pkg/runtime`

- [x] 三张表的 Luban 定义、CSV 与导入 JSON
- [x] schema / import / Load / Reload 接入

### Task 2: 协议与错误码

**Files:** `internal/protocolpb/proto/combat.proto`、`internal/code/code.go`、`internal/protocol/types.go`、`README.md`

- [x] `CombatCastRequest` / `CombatFrame` 与 `40040`–`40046`
- [x] genproto

### Task 3: 战斗框架

**Files:** `internal/gameapp/combat`、`internal/gameapp/world/scene.go`、`internal/gameapp/player/actor_player.go`、`internal/gameapp/app.go`

- [x] 心跳结算、Buff 快照、合并广播
- [x] 进场离场挂钩；`game.combat.cast`
- [x] 单测：范围、Buff、合包、50 人 frame 数

---

## 验证

- [x] `go test ./...`
- [x] `go build` game / gateway

## 备注

药水、控制 Buff、阵营和怪物不在本期。
