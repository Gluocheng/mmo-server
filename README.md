# MMO 后端骨架（Cherry）

基于 [cherry-game/cherry](https://github.com/cherry-game/cherry) 的 Actor + 集群（NATS）的最小可运行示例：**网关 → 登录服（账号密码签发 access+refresh）→ 游戏服（选角/创角 + 进场景 + 移动广播）**。

本仓库 `mmo-server` 通过 `replace` 引用同级的 `cherry-framework` 源码（由计划中的克隆步骤得到）。

## 依赖

- Go 1.24+
- [NATS Server](https://github.com/nats-io/nats-server)（默认 `nats://127.0.0.1:4222`，见 `configs/mmo-cluster.json`）
- MySQL 8+（默认 DSN 在 `configs/mmo-cluster.json` 的 `mysql.dsn`）
- Redis 7+（默认 `127.0.0.1:6379`）

## 启动顺序

在 `mmo-server` 目录下（保证 `configs/mmo-cluster.json` 相对路径可用）：

1. 启动 MySQL，并创建库：`mmo`
2. 启动 Redis
3. 启动 NATS：`nats-server`（或等价命令）
4. 启动登录：`go run ./cmd/login -path=configs/mmo-cluster.json -node=login-1`
5. 启动游戏：`go run ./cmd/game -path=configs/mmo-cluster.json -node=10001`
6. 启动网关：`go run ./cmd/gateway -path=configs/mmo-cluster.json -node=gate-1`

网关 WebSocket 地址：`ws://127.0.0.1:10100`（与 profile 中 `gate-1` 的 `address` 一致）。

## 客户端协议（Pomelo + Protobuf）

业务包体使用 **Protobuf 二进制**（`google.golang.org/protobuf`），由三节点 `app.SetSerializer(NewProtobuf())` 统一序列化；与旧 JSON 客户端 **不兼容**，需同步升级客户端编解码。

路由格式仍为：`nodeType.handlerName.method`。

1. **签发 token 对（首包可用）**  
   - Route: `gate.user.issueToken`  
   - Body: `IssueTokenRequest` 的 protobuf 编码（字段语义同原 JSON：`nickname`、`password`、`deviceId`；`clientIp` 由网关注入）  
   - 成功返回：`IssueTokenResponse`（`uid`、`accessToken`、`accessExpireAt`、`refreshToken`、`refreshExpireAt`）  
   - 说明：服务端会按“账号+IP”做失败限流（默认 5 次失败触发 10 分钟封禁）

2. **access token 登录（首包必须之一）**  
   - Route: `gate.user.login`  
   - Body: `TokenLoginRequest`（`accessToken` 或兼容字段 `token`、`serverId`、`deviceId`）  
   - 成功返回：`TokenLoginResponse`（`uid`）

3. **刷新 access token**
   - Route: `gate.user.refreshToken`
   - Body: `RefreshTokenRequest`（`refreshToken`）
   - 成功返回：`RefreshTokenResponse`
   - 说明：refresh token 为单次使用，刷新成功后会返回新的 refresh token（旧的立即失效）

4. **登出（使 access/refresh 失效）**
   - Route: `gate.user.logout`
   - Body: `LogoutRequest`（`accessToken` / `refreshToken` / 兼容 `token`；不传时会尝试读取当前 session 缓存）
   - 成功返回：`LogoutResponse`（`ok`）

会话并发策略（`auth.session_policy`）：
- `kick_old`：同账号新登录会挤掉旧会话（默认）
- `coexist`：同账号允许共存
- `device_limit`：按设备数量限制，超过 `auth.max_devices_per_uid` 则拒绝登录

5. **查角**  
   - Route: `game.player.select`  
   - Body: `google.protobuf.Empty`（空消息）  
   - 返回：`PlayerSelectResponse`

6. **创角**  
   - Route: `game.player.create`  
   - Body: `PlayerCreateRequest`（`name`）  
   - 返回：`PlayerCreateResponse`

7. **进入场景**  
   - Route: `game.player.enter`  
   - Body: `EnterGameRequest`（可选 `playerId`、`sceneId`）  
   - 成功返回：`EnterGameResponse`（`sceneId`、`players`）

8. **移动（同场景广播）**  
   - Route: `game.player.move`  
   - Body: `MoveRequest`（`x`/`y`/`z`）  
   - 成功返回：`google.protobuf.Empty`  
   - 其他在线客户端会收到 Push：`route = onMove`，body 为 `MoveBroadcast`（`uid` 与坐标）

说明：
- 网关会限制未完成“进入场景”的玩家请求，除 `select/create/enter` 外会被拒绝。
- `accounts` / `players` 表会在服务首次启动时自动迁移创建。
- `account:nickname` 与 `player:uid` 数据会写入 Redis 缓存（带 TTL）；`PlayerInfo` 缓存值为 **protojson** 文本，与 Pomelo 包体编码相互独立。
- access token 会写入 Redis（默认 TTL 15 分钟，可通过 `redis.access_ttl_seconds` 调整）。
- refresh token 会写入 Redis（默认 TTL 24 小时，可通过 `redis.refresh_ttl_seconds` 调整）。
- refresh token 刷新采用原子消费（`GETDEL`），并记录已消费标记，避免重放。
- 登录失败限流参数：
  - `redis.login_fail_limit`
  - `redis.login_fail_window_seconds`
  - `redis.login_block_seconds`
- 设备会话 TTL 参数：`redis.device_session_ttl_seconds`
- 账号密码采用 `bcrypt` 哈希存储，不保存明文密码。

`.proto` 定义见 `internal/protocolpb/proto/`；Go 侧对外类型别名见 `internal/protocol/types.go`。

### 重新生成 `internal/protocolpb/gen`

需安装 [Docker](https://docs.docker.com/get-docker/)，在仓库根目录执行：

```powershell
powershell -File scripts/genproto.ps1
```

或等价命令：

```powershell
docker run --rm -v "${PWD}:/work" -w /work namely/protoc-all:1.51_1 `
  -d internal/protocolpb/proto -l go -o internal/protocolpb/gen --go-source-relative
```

## 构建

```powershell
go build -o bin/gateway.exe ./cmd/gateway
go build -o bin/login.exe ./cmd/login
go build -o bin/game.exe ./cmd/game
```

## 测试

```powershell
go test ./...
```

## 目录说明

| 路径 | 说明 |
|------|------|
| `cmd/gateway` | 网关节点（WebSocket + Pomelo） |
| `cmd/login` | 登录节点（账号密码签发 token + token 验证） |
| `cmd/game` | 游戏节点（场景占位 + 移动广播） |
| `configs/mmo-cluster.json` | 集群与节点配置（`discovery.mode=default` + NATS） |
| `internal/protocolpb/proto` | 业务 Pomelo 包体 `.proto` 定义 |
| `internal/protocolpb/gen` | `protoc-gen-go` 生成代码（勿手改） |
| `internal/protocol` | 对外类型别名（`types.go`），业务 import 入口 |
| `../cherry-framework` | Cherry 框架源码（与 `go.mod` 中 `replace` 对应） |

## 后续扩展建议

- 持久化帐号/角色（MySQL + GORM 等）
- 将 `discovery.mode` 换为 `nats` 并增加 master 节点做注册发现
- AOI、战斗、聊天等按子系统增量迭代
