# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概览

基于 [cherry-game/cherry](https://github.com/cherry-game/cherry)（Actor 模型 + NATS 集群）的 MMO 游戏服务端骨架。五个独立进程组成集群，通过 NATS 消息通信：**master → login → game → gateway → gm**。MySQL 存持久数据，Redis 存会话/token/限流/时间偏置/缓存。

Cherry 框架以 `go.mod` 的 `replace` 指向同级目录 `cherry-framework/` 源码（vendor 依赖）。

> 更完整的节点表、协议表、错误码表、配表管线、配置项见根目录 `README.md` 与 `AGENTS.md`。本文件只保留跨文件才能得出的「大图景」与常用命令。

## 命令

```powershell
# 构建 / 测试
go build ./...
go test ./...
go test ./internal/persistence/...        # 单包测试
go test ./internal/persistence -run TestX # 单测单个用例

# 格式化：只对本次改动的 .go 文件，禁止 go fmt ./...（见 .cursor/rules/go-format.mdc）
gofmt -w internal/xxx/foo.go

# 编译各节点二进制到 bin/
go build -o bin/game.exe ./cmd/game   # master/login/gateway/gm 同理
go build -o bin/import-config.exe ./gameconfig/cmd/import

# 一键启停（Windows，推荐）
powershell -ExecutionPolicy Bypass -File scripts/start.ps1 -Build
powershell -ExecutionPolicy Bypass -File scripts/stop.ps1

# 手动逐节点启动（必须按顺序）
go run ./cmd/master  -path=configs/mmo-cluster.json -node=master-1
go run ./cmd/login   -path=configs/mmo-cluster.json -node=login-1
go run ./cmd/game    -path=configs/mmo-cluster.json -node=10001
go run ./cmd/gateway -path=configs/mmo-cluster.json -node=gate-1
go run ./cmd/gm      -http=:9080 -nats=nats://127.0.0.1:4222 -prefix=mmo -game=10001

# 联调冒烟客户端
go run ./cmd/client-demo -ws 127.0.0.1:10100

# 重新生成 Protobuf Go 代码（需 Docker）
powershell -ExecutionPolicy Bypass -File scripts/genproto.ps1

# 策划配表：导表 → 导入 MySQL → game 节点启动自动 Load
.\gameconfig\tools\gen.ps1
go run ./gameconfig/cmd/import -profile configs/mmo-cluster.json
```

对外端口：网关 WebSocket `ws://127.0.0.1:10100`；GM HTTP `http://127.0.0.1:9080`（`/gm/health`、`/gm/config/reload`）。

## 架构（跨文件大图景）

### 五节点与职责

| 节点 | 源码 | 默认 node_id | 职责 |
|------|------|-------------|------|
| master | `cmd/master/` | `master-1` | NATS 集群发现注册（无业务 Actor） |
| login | `cmd/login/` | `login-1` | 账号认证、token 签发/校验/刷新/登出 |
| game | `cmd/game/` | `10001`（须数字） | 角色/场景(AOI)/聊天/背包/GM 指令 |
| gateway | `cmd/gateway/` | `gate-1` | WebSocket + Pomelo 协议，鉴权路由转发 |
| gm | `cmd/gm/` | `gm-1` | 独立管理进程：HTTP → NATS → game |

所有节点共用 `configs/mmo-cluster.json`（Docker 栈用 `configs/mmo-docker.json`）。启动顺序严格为 master → login → game → gateway → gm。

### 节点内装配链

每个 `cmd/<node>/` 只是薄入口，调用 `internal/<node>app.Run(profilePath, nodeID)`：

- 后端节点（master/login/game）用 `cherry.Configure(..., isFrontend=false, cherry.Cluster)`，然后 `SetSerializer(NewProtobuf())`、`AddActors(...)`、`Startup()`。
- gateway 用 `isFrontend=true`，通过 `pomelo.NewActor("user")` + `AddConnector(NewWS(...))` + `SetNetParser(...)` 装配前端，`SetOnDataRoute` 指向 `internal/gatewayapp/actor.OnPomeloDataRoute`。
- gm 是**非 Cherry 进程**，直接用 `nats.Connect` + 标准库 `net/http`（见 `internal/gmapp/app.go`、`http.go`），加载同一 profile 连 MySQL/Redis，并 `go:embed` 托管 `web/gm-console` 构建产物。

### 消息流

```
客户端(WebSocket) → gateway(Pomelo 解码) → NATS → login(auth.*) 或 game(player.*/chat.*/bag.*)
  login → MySQL(accounts) + Redis(token)
  game  → Actor 处理器 → persistence(GORM 事务) + Redis
```

- **路由约定**：`nodeType.handlerName.method`（Pomelo 风格）。前端节点本节点处理（gate.user.*）；game 节点路由经 `ClusterLocalDataRoute` 转发到 `{serverID}.{handler}.{uid}` 子 Actor。
- **协议**：包体 Protobuf 二进制；`.proto` 在 `internal/protocolpb/proto/`，生成的 Go 代码在 `internal/protocolpb/gen/`，业务统一从 `internal/protocol/types.go` 的别名引用。
- **Session 字段流转**：gateway 登录后写入 `internal/sessionkey/` 定义的键（Uid/ServerID/PlayerID/AccessToken...）。进场景时 game 的 `player.enter` 通过 `p.Call(agentPath, "setSession", ...)` 回写 `player_id` 到网关 session，从而解锁后续 gameplay 路由。未进场只允许 `select`/`create`/`enter`（见 `internal/gatewayapp/actor/agent.go` 的 `beforeEnterRoutes`）。

### 持久化核心模式

写路径统一走 `internal/persistence/tx.go` 的 `WithinTx` + `AfterCommit`：

- `WithinTx(ctx, fn)`：MySQL 事务；若 ctx 已在事务内则复用。`DBFromContext(ctx)` 取事务 DB 或全局 DB。
- `AfterCommit(ctx, fn)`：注册事务提交后的副作用（刷 Redis 缓存），避免脏缓存；非事务 ctx 下立即执行。
- GORM 模型在 `internal/persistence/model_*.go`，`AutoMigrate` 在 `persistence.Init()` 时建表（含 gameconfig 配置表）。

### 时间与 ID

- 业务逻辑用 `internal/gtime`（`gtime.Now()`/`UnixNow()`），**不要** `time.Now()`；真实时间用 `gtime.RealNow()`。
- UID：snowflake，由 login 节点以 nodeID 哈希初始化（`persistence.ConfigureIDNodeFromString`）。PlayerID：MySQL `id_sequences` 表行锁自增（`nextPlayerIDInTx`）。

## 只读 / 禁止编辑区域

- `cherry-framework/` — vendor 框架。可读以理解框架，除非修框架级 bug 否则不改。
- `internal/protocolpb/gen/`、`gameconfig/gen/` — 生成代码，改后会被重新生成覆盖。
- `bin/`、`logs/`、`go.sum` — 构建/运行产物与自动维护文件。

## 开发工作流（docs/plans 双文件）

**新功能 / 行为变更必须先策划后编码**（见 `.cursor/rules/feature-plans.mdc`）：

1. 复制 `docs/plans/_template-design.md` → `docs/plans/YYYY-MM-DD-<feature>-design.md`，填齐，`Status: draft`。
2. 更新 `docs/plans/README.md` 的 Roadmap 总览表（`planned`）。
3. **未获用户明确确认前不得写代码**。
4. 评审通过后复制 `_template.md` → `YYYY-MM-DD-<feature>.md`（`planned → in_progress → done`），文首链接策划。

纯重构、单文件小修、依赖升级不强制走此流程。提交信息用 `/git-staged-commit-draft` 生成 Conventional Commits（中文主题行）。

## 代码约定（来自 .cursor/rules）

- **注释**：导出函数/类型、业务常量、非显而易见分支、副作用（事务/双下发）用**中文**注释，风格对齐 `internal/persistence/tx.go`。不写废话注释，不整仓补注释。
- **格式化**：交付前只 `gofmt -w` 本次改动的 `.go` 文件，禁止 `go fmt ./...`。
- **行为准则**（karpathy.md）：先想后写、保持最小实现、外科手术式改动、以可验证目标驱动。修改前显式陈述假设，不确定就问。
