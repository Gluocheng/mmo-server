# 怪物生成 实施计划

> **Status:** done  
> **Design:** [`2026-10-08-combat-monster-design.md`](2026-10-08-combat-monster-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 玩法只配怪物 id，就能在指定地图刷出会追击、出手、死后回出生点复活的怪物。

**Architecture:** `monster` 是模板，`spawn` 是刷怪点。实例用雪花 uid，和玩家进同一个心跳。坐标先写入房间，再进入战斗；移动先算出，放开战斗锁后再写房间。

**Tech Stack:** Go / Luban / Protobuf

---

## 任务清单

### Task 1: 配表

**Files:** `gameconfig/defines/monster.xml`、`spawn.xml`、`datas/*.csv`、`schema`、`importdata`、`runtime`

- [x] 怪物模板与刷怪点种子
- [x] Luban JSON、schema / import / Load

### Task 2: 刷出与协议

**Files:** `scene.proto`、`world`、`player`、`combat`

- [x] 附近列表带类型和模板 id，不占人数
- [x] 启动和热更同步实例，出生信息不变则保留生命和 uid

### Task 3: 行动

**Files:** `internal/gameapp/combat/engine.go`、`monster.go`

- [x] 防御减伤，0 伤害仍是 0
- [x] 追击、攻击间隔、回家、禁手停手
- [x] 怪物回出生点复活，玩家仍原地复活

### Task 4: 单测

- [x] 附近列表、扣血、追击、回家、禁手、复活、热更

---

## 验证

- [x] `go test ./...`
- [x] `go build` game

## 备注

不做寻路、掉落、阵营和客户端。
