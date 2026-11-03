# 装备属性扩充 实施计划

> **Status:** done  
> **Design:** [`2026-11-03-equip-stats-design.md`](2026-11-03-equip-stats-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的出手顺序、种子和验收派生，勿重复长篇策划正文。

**Goal:** 暴击率、暴击伤害、穿透、伤害加成、受伤减免、吸血进入结算。没这些加成时掉血与现在相同。

**Architecture:** 六项仍用现有固定值加百分比公式，基础为 0，配表里的百分点加进固定项。出手在减防前后按暴击、穿透、伤害加成、受伤减免计算。吸血写成同一拍的治疗。快照和属性变化带上最终值。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis

---

## 任务清单

### Task 1: 配表和协议

**Files:**
- Modify: `cfg_stat`、`cfg_equip_attr`、`cfg_item`、`internal/protocolpb/proto/combat.proto`

- [x] 六项 `settle=true`
- [x] 木剑、布衣、铜戒、锋锐戒种子
- [x] 快照、属性变化、命中追加字段

### Task 2: 出手与同步

**Files:**
- Modify: `internal/gameapp/combat/`、`internal/gameapp/equip/`

- [x] 暴击、穿透、加减伤、吸血，持续伤害不参与
- [x] 测试可注入暴击命中或落空
- [x] 快照和 `stats` 带六项最终值

### Task 3: 文档

- [x] README 补上出手顺序
- [x] `go test ./...` 通过

## 验证

- [x] `go test ./...`
- [x] `go build -o bin/game.exe ./cmd/game`
