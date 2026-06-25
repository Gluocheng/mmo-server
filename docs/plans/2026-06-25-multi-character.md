# 一账号多角 实施计划

> **Status:** done
> **Design:** [`2026-06-25-multi-character-design.md`](2026-06-25-multi-character-design.md)（须 **Status: approved**�?
> **For agentic workers:** 按任务勾选推进；完成后更�?`docs/plans/README.md` 总览表�?

**Goal:** 放开一账号一角色限制，支持多角色 select/create/enter/delete，上限配表可调，软删除不复用 player_id�?

**Architecture:** 去掉 `players.uid` 唯一索引改普通索引并�?`deleted_at` 软删字段；`select` 返回账号下未删除角色列表；`create` 受配置上限约束；`enter` 必带 player_id 并校验归属与未删除；新增 `game.player.delete` 软删路由。缓存移除单角色缓存路径�?

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis / SQLite 单测

---

## 任务清单

### Task 1: 协议与错误码

**Files:**
- Modify: `internal/protocolpb/proto/player.proto`
- Modify: `internal/protocolpb/gen/player.pb.go`
- Modify: `internal/protocol/types.go`
- Modify: `internal/code/code.go`

- [x] `player.proto` 新增 `PlayerDeleteRequest{player_id}`
- [x] 重新生成 `player.pb.go`（若�?protoc 环境则手工补生成代码�?
- [x] `types.go` 增加 `PlayerDeleteRequest` 别名
- [x] `code.go` 新增 `40026 PlayerLimitExceeded`、`40027 PlayerDeleted`

### Task 2: 持久化层多角模型与迁�?

**Files:**
- Modify: `internal/persistence/model_player.go`
- Modify: `internal/persistence/migrate.go`
- Test: `internal/persistence/tx_test.go`

- [x] `Player` 取消 `uid` 唯一索引改普通索引，新增 `deleted_at` 软删字段
- [x] `autoMigrateModels` 显式删除旧唯一索引并建普通索�?
- [x] 测试：迁移后�?uid 可插入多�?player

### Task 3: 持久化层多角查询/创建/删除

**Files:**
- Modify: `internal/persistence/player.go`
- Modify: `internal/persistence/service.go`
- Modify: `internal/persistence/store.go`
- Test: `internal/persistence/tx_test.go`

- [x] 新增 `ListPlayersByUIDContext` 返回未删除角色列�?
- [x] `createPlayerInTx` 去掉「已有就返回旧」逻辑，改为受上限约束的新�?
- [x] 新增 `DeletePlayerContext` 软删（校验归属、已删返回标记）
- [x] 新增 `GetPlayerByPlayerIDContext` 校验归属与未删除
- [x] 移除单角色缓存路�?`playerUIDKey` 相关逻辑
- [x] 测试：多角色创建、上限拦截、软删后 select 不可见、enter 校验归属

### Task 4: actor 层路由调�?

**Files:**
- Modify: `internal/gameapp/player/actor_player.go`
- Modify: `internal/gameapp/app.go`（若需注册�?

- [x] `select` 改用 `ListPlayersByUID`
- [x] `create` 改用新版多角创建，上限错误返�?`40026`
- [x] `enter` 必带 `player_id`，用 `GetPlayerByPlayerID` 校验归属与未删除
- [x] 新增 `game.player.delete` 路由，软删后返回 `40027`/`40008`

### Task 5: client-demo 适配

**Files:**
- Modify: `cmd/client-demo/main.go`

- [x] select 返回多角色时取列表第一个；create 后用返回�?player_id enter

### Task 6: 验证与收�?

**Files:**
- Modify: `docs/plans/2026-06-25-multi-character.md`
- Modify: `docs/plans/README.md`

- [x] 对本次改动的 `.go` 文件执行 `gofmt -w`
- [x] `go test ./internal/persistence`
- [x] `go test ./...`
- [x] 更新本实施计划状态为 `done`，并更新 `docs/plans/README.md` 总览状�?

---

## 验证

- [x] `go test ./internal/persistence`
- [x] `go test ./...`
- [x] （可选）`go run ./cmd/client-demo -nickname luocheng2 -password 123456 -player hero2`

## 备注

- 名称唯一性：全服未删除角色名唯一�?
- 软删不释放配额，player_id 不复用�?
- `player.max_characters` 默认 3，从配置读取�?
