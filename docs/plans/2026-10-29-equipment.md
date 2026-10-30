# 装备 实施计划

> **Status:** done  
> **Design:** [`2026-10-29-equipment-design.md`](2026-10-29-equipment-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 十二个部位各穿一件，加成进入现有生命、攻击、防御公式。

**Architecture:** `internal/gameapp/equip` 管穿着和路由。背包只提供同一回合内的取出和放回。战斗只接收三项属性的固定值和百分比。背包格子和 `player_equipment` 同一次落库。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis

---

## 任务清单

### Task 1: 配表、协议和错误码

**Files:**
- Create: `internal/protocolpb/proto/equip.proto`
- Modify: 道具表、背包格数、`cfg_equip_slot`、`cfg_equip_attr`、`internal/code/code.go`

- [x] 十二个部位、木剑、布衣、铜戒种子
- [x] `40066`、`40067`
- [x] 生成协议别名

### Task 2: 穿脱与属性

**Files:**
- Create: `internal/gameapp/equip/`
- Modify: `internal/gameapp/bag/`、`internal/gameapp/combat/`、`internal/gameapp/player/`、`internal/gameapp/app.go`

- [x] 穿、换、脱、背包满、部位不对
- [x] 换装后下一拍推属性变化
- [x] 进场和切图重新叠上穿着
- [x] 与背包同一次 flush

### Task 3: 文档

- [x] README 补上穿脱路由和木剑攻击 +5
- [x] `go test ./...` 通过

## 验证

- [x] `go test ./...`
- [x] `go build -o bin/game.exe ./cmd/game`
