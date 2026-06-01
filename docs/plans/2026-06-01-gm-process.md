# GM 独立管理进程 实施计划

> **Status:** done  
> **Design:** [`2026-06-01-gm-process-design.md`](2026-06-01-gm-process-design.md)  
> **完成日期：** 2026-06-01

**Goal:** 新增 `cmd/gm` 独立进程，HTTP 收管理请求，经 NATS 转发到 game 节点 `ActorGM`，承接配表热更并预留扩展点。

**Architecture:** GM 直接连 NATS（不接入 Cherry 集群），手工拼 `ClusterPacket` 走 `cherry-{prefix}.remote.game.{nodeID}` subject；game 侧在 `actorGMConfig` 双路注册 Pomelo Local 与 Cluster Remote handler。

**Tech Stack:** Go / `net/http` / `nats.go` / Cherry Actor / Protobuf

---

## 任务清单

### Task 1: GM 进程脚手架

**Files:**
- Create: `cmd/gm/main.go`
- Create: `internal/gmapp/app.go`
- Create: `internal/gmapp/http.go`

- [x] CLI 参数：`-http`、`-nats`、`-prefix`、`-game`
- [x] `App.Run` 连接 NATS、构造 `remoteSubject` / `sourcePath` / `targetPath`
- [x] 注册 `POST /gm/config/reload` 路由
- [x] HTTP handler 编码 `ClusterPacket` → NATS Request → 解码 `Response{code,data}` → 解码 `RefreshTokenResponse`

### Task 2: game 节点 GM Actor

**Files:**
- Create: `internal/gameapp/gm/actor_gm.go`
- Create: `internal/gameapp/gm/actor_gm_config.go`
- Modify: `internal/gameapp/app.go`（注册 `ActorGM`）

- [x] `ActorGM.AliasID = "gm"`，`OnFindChild` 按 domain 创建子 Actor
- [x] `actorGMConfig.OnInit` 同时 `Local().Register("reload")` 和 `Remote().Register("reload")`
- [x] `reloadCluster` 返回 `(*RefreshTokenResponse, int32)` 符合 Cherry retValue 约定
- [x] 业务码 `40024` / `40025` 对应未启用与失败

### Task 3: 配表单表热更

**Files:**
- Create: `gameconfig/pkg/runtime/loadtable.go`
- Modify: `internal/gameapp/gm/actor_gm_config.go`

- [x] `ReloadTable(ctx, db, tableName)`：仅替换指定表的快照
- [x] `tableName == ""` 走全量 `Reload`

### Task 4: 启停脚本接入

**Files:**
- Modify: `scripts/start.ps1`
- Modify: `scripts/stop.ps1`

- [x] `Ensure-Binaries` 加入 `cmd/gm`
- [x] 新增 `Start-GMNode` 函数（GM 用专属 CLI 参数）
- [x] 启动顺序：master → login → game → gateway → gm
- [x] `stop.ps1` 先停 gm
- [x] 9080 端口检查

### Task 5: 文档与依赖

**Files:**
- Modify: `README.md`
- Modify: `docs/plans/README.md`
- Modify: `go.mod`（`nats.go` indirect → direct）

- [x] README 增加 GM 节点章节、HTTP 示例、错误码扩充至 40025
- [x] Roadmap 总览新增本计划行
- [x] `go mod tidy`

---

## 验证

- [x] `go build ./cmd/gm`
- [x] `go build ./cmd/game`
- [x] `go vet ./internal/gmapp/... ./cmd/gm/... ./internal/gameapp/gm/...`
- [x] `go test ./internal/persistence/...`
- [x] 手动 curl：`curl.exe -X POST http://127.0.0.1:9080/gm/config/reload -H "Content-Type: application/json" -d '{"tableName":""}'`

## 备注

- HTTP body 复用 `RefreshTokenRequest.RefreshToken` 承载 `tableName`，是为了避开 protoc 工具链当前不可用的限制；后续 GM 命令增多时建议新建 `gm.proto`。
- GM 进程未接入 Cherry 集群（不注册到 master），原因：管理通道与玩家通道生命周期不同，独立 NATS 客户端足够且更轻量。
- `gmNodeType` 常量目前未参与路由（仅 `gmNodeID` 用于 `sourcePath`），保留以备未来 `nodeType=gm` 集群化场景。