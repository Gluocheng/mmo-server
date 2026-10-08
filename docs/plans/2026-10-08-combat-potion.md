# 战斗药水 实施计划

> **Status:** done  
> **Design:** [`2026-10-08-combat-potion-design.md`](2026-10-08-combat-potion-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 道具使用只调用 `ApplyBuff`，治疗量在 Buff 表，成功才扣 1 个。

**Architecture:** 道具表增加 `use_buff_id`。`game.bag.use` 读槽位上的道具，先挂 Buff，成功后再按槽扣 1 个并推 `onBagChange`。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Luban

---

## 任务清单

### Task 1: 配表

**Files:** `gameconfig/defines/item.xml`、`gameconfig/datas/item.csv`、`gameconfig/datas/buff.csv`、`schema`、`importdata`

- [x] `use_buff_id` 与治疗 Buff 种子
- [x] Luban 生成、schema / import / Load 接上新列

### Task 2: 使用

**Files:** `bag.proto`、`internal/code/code.go`、`internal/gameapp/bag`、`README.md`

- [x] `BagUseRequest` 与 `40054`
- [x] `game.bag.use`：失败不扣，成功扣 1 个并 Push

### Task 3: 单测

- [x] 下一跳加血并扣 1 个
- [x] 死亡不扣
- [x] `use_buff_id=0` 拒绝
- [x] 禁战地图仍能喝

---

## 验证

- [x] `go test ./...`
- [x] `go build` game

## 备注

不做客户端，不改 `Cast` / `ApplyBuff` 签名。
