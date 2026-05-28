# 背包系统 v1 实施计划

> **Status:** done  
> **Design:** [2026-05-28-bag-system-design.md](2026-05-28-bag-system-design.md)  
> **完成日期：** 2026-05-28

**Goal:** 在 game 节点提供 `game.bag.list` / `add` / `remove`，MySQL `inventory_items` 堆叠持久化 + Redis 缓存。

**Architecture:** 独立 `bag` Actor（按 uid 子 Actor）；`WithinTx` 写库 + `AfterCommit` 刷缓存；进场后可用（与 chat 相同）。

**Tech Stack:** Protobuf / GORM / Redis / Pomelo

---

## 任务清单

- [x] `bag.proto` + `genproto` + `protocol/types.go`
- [x] `InventoryItem` 模型 + `persistence/bag.go`
- [x] 错误码 `40018`–`40020`
- [x] `internal/gameapp/bag` Actor + `app.go` 注册
- [x] `persistence/bag_test.go`
- [x] README 协议表 + `client-demo` 冒烟

## 二期

见 [2026-05-28-bag-v2.md](2026-05-28-bag-v2.md)（已完成）：槽位、`move`/`split`、`onBagChange` Push。
