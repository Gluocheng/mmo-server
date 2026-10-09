# 世界 BOSS 实施计划

> **Status:** done  
> **Design:** [`2026-10-09-world-boss-design.md`](2026-10-09-world-boss-design.md)（**Status: approved**）

**Goal:** 地图 4 每一条线刷多只 BOSS 和小怪。击杀按单独、组队、伤害前三和最后一击发奖。背包放不下进待领取。

**Architecture:** `line=0` 的刷怪点展开到该图每一条线。伤害榜在战斗进程里，按实际扣血记账，复活清空。死亡后读 Redis 队伍名单，只给还在本图本线的人发奖。进背包和待领取在同一事务里决定去向。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis

## 任务清单

- [x] 配表：地图 4、怪物 `kind`、`line=0`、`cfg_kill_reward`
- [x] 击杀归类与实际扣血记账
- [x] `player_reward_claims`，`game.reward.list` / `game.reward.take`，推送 `onKillSettle`
- [x] `go test ./...` 与 `go build -o bin/game.exe ./cmd/game`
