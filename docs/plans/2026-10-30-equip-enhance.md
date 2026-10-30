# 装备强化 实施计划

> **Status:** done  
> **Design:** [`2026-10-30-equip-enhance-design.md`](2026-10-30-equip-enhance-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议、落库和验收派生，勿重复长篇策划正文。

**Goal:** 只能强化穿在身上的装备。等级跟着这一件走，最高 10 级。高阶失败可掉 1 级，勾选保护符时这次不掉级。

**Architecture:** `game.equip.enhance` 交给该玩家的背包 Actor，在同一回合里扣材料和改穿着等级。等级写在背包格和 `player_equipment` 上。追加后的属性进入现有公式，变化留到下一拍推送。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis

---

## 任务清单

### Task 1: 配表、协议和错误码

**Files:**
- Modify: `internal/protocolpb/proto/equip.proto`、`internal/protocolpb/proto/bag.proto`、`internal/code/code.go`
- Create: `cfg_enhance` 种子、道具 1003 和 1004

- [x] `40068`、`40069`
- [x] 穿着和背包格带等级；强化请求带部位和是否用保护符
- [x] 1 到 10 级的强化石、成功率、掉级率

### Task 2: 强化与等级跟着装备走

**Files:**
- Modify: `internal/gameapp/equip/`、`internal/gameapp/bag/`、`internal/persistence/`

- [x] 先检查再掷点；成功不加保护符；失败按掉级率，保护符挡住掉级
- [x] 脱下、移动、再穿上时等级还在这件装备上
- [x] 等级变化后下一拍推三项属性
- [x] 与背包同一次 flush

### Task 3: 文档

- [x] README 补上强化路由、强化石和保护符
- [x] `go test ./...` 通过

## 验证

- [x] `go test ./...`
- [x] `go build -o bin/game.exe ./cmd/game`
