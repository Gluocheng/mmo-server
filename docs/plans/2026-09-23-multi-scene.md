# 多地图与分线 实施计划

> **Status:** done  
> **Design:** [`2026-09-23-multi-scene-design.md`](2026-09-23-multi-scene-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 登录出生在配表主城，按配表分线和切图，并由每张图决定能否战斗。

**Architecture:** `cfg_scene` 进入现有 Luban 导入和 `runtime.Load`。世界层用地图 id + 线号占坑。玩家 Actor 负责进场、切图和地图列表。战斗只在 `allow_combat` 为真且地图仍在配表时结算。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Luban

---

## 任务清单

### Task 1: 配表

**Files:** `gameconfig/defines/scene.xml`、`gameconfig/datas/scene.csv`、`gameconfig/pkg/schema`、`gameconfig/pkg/importdata`、`gameconfig/pkg/runtime`

- [x] 地图表定义、种子、导入与 Load / Reload

### Task 2: 协议与错误码

**Files:** `internal/protocolpb/proto/scene.proto`、`internal/protocolpb/proto/gm.proto`、`internal/code/code.go`、`README.md`

- [x] 切图、地图列表、`onScenePresence`、在线列表线号
- [x] `40050`–`40053`

### Task 3: 进场、切图与战斗

**Files:** `internal/gameapp/world`、`internal/gameapp/player`、`internal/gameapp/combat`、`internal/gameapp/chat`、`internal/gameapp/gm`

- [x] 同一把锁选线占坑；切回当前图留线；断线让出名额
- [x] `Cast` 读 `allow_combat`；成功换图才重置战斗
- [x] 移动、聊天、公告按地图和分线隔离

---

## 验证

- [x] `go test ./...`
- [x] `go build` game / gateway

## 备注

手动选线、传送门和怪物不在本期。
