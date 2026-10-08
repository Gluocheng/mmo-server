# 战斗禁手 实施计划

> **Status:** done  
> **Design:** [`2026-10-08-combat-control-design.md`](2026-10-08-combat-control-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 配表效果 `stun` 在框架内禁止出手和喝药，已入队技能在结算时作废。

**Architecture:** `Cast` 与 `ApplyBuff` 看到未到期禁手就返回 `40055`。技能命中仍走内部挂 Buff。心跳对 `stun` 不跳数值。

**Tech Stack:** Go / Luban

---

## 任务清单

### Task 1: 结算

**Files:** `internal/gameapp/combat/engine.go`、`internal/code/code.go`

- [x] `stun` 合法且心跳不跳数值
- [x] 出手、喝药、已入队结算都认禁手

### Task 2: 配表

**Files:** `gameconfig/datas/buff.csv`、`gameconfig/datas/skill.csv`、`README.md`

- [x] 禁手 Buff 与点名技能种子
- [x] Luban JSON 与错误码说明

### Task 3: 单测

- [x] 禁手期间与到期
- [x] 命中施加
- [x] 入队后作废且不掉血

---

## 验证

- [x] `go test ./...`
- [x] `go build` game

## 备注

不做移动限制、阵营、怪物和客户端。
