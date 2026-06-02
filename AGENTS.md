# 项目说明

本文件为 AI 助手提供项目上下文。

## 项目类型: Go

### 命令
- 构建: `go build ./...`
- 测试: `go test ./...`
- 运行: `go run ./cmd/<节点>`（各节点独立启动；启动顺序见 README.md）
- 格式化: `go fmt ./...`

### 文档
项目概述及启动说明见 README.md。

### 版本控制
本项目使用 Git。排除规则见 .gitignore。

## AI 助手指引

- **CodeWhale 读取文件:** AGENTS.md（兼容 CodeWhale 及其他 AI 助手）
- **只读区域:** `cherry-framework/` — 游戏框架为 vendor 依赖（见 `go.mod` 中的 `replace`）。可阅读以理解框架，除非修复框架级 bug 否则不要修改。`bin/` 和 `logs/` 为运行时产物 — 仅在调试输出时阅读。
- **禁止编辑:** `internal/protocolpb/gen/`（Protobuf 生成的 Go 代码），`gameconfig/gen/`（Luban 生成的配置加载代码），`bin/`（构建产物），`logs/`（运行时日志），`go.sum`（自动维护的依赖校验和）
- **验证命令:** `go build ./...`（修改后验证编译）；`go test ./...`（完整测试套件）；针对性测试使用 `go test ./internal/persistence/...` 等。

## 架构

基于 Cherry 框架（Actor 模型 + NATS 集群）的 MMO 游戏服务端骨架。五个独立进程组成集群，通过 NATS 消息通信。MySQL 存储持久数据；Redis 处理会话、频率限制和时间偏差。

### 入口节点

| 服务 | 源码 | 标志 | 默认节点 ID | 职责 |
|--------|--------|------|-----------------|------|
| master | `cmd/master/` | `-node` | `master-1` | NATS 集群发现注册 |
| login | `cmd/login/` | `-node` | `login-1` | 账号认证：签发/校验/刷新令牌 |
| game | `cmd/game/` | `-node` | `10001` | 游戏逻辑：角色、场景（AOI）、聊天、背包、GM 指令 |
| gateway | `cmd/gateway/` | `-node` | `gate-1` | WebSocket + Pomelo 协议，鉴权路由转发至 login/game |
| gm | `cmd/gm/` | `-http`, `-nats`, `-prefix`, `-game` | `gm-1` | 管理进程：HTTP API → NATS → game 节点 |

所有节点共用 `-path=configs/mmo-cluster.json` 作为 profile 配置。启动顺序必须为: **master → login → game → gateway → gm**。

### 核心模块

| 目录 | 职责 |
|-----------|------|
| `cherry-framework/` | Vendor 游戏框架：Actor 模型、NATS 集群、网络层（TCP/WS）、日志、序列化、服务发现、时间轮 |
| `internal/gameapp/` | 游戏服：选角/创角、进场景/移动（AOI）、聊天、背包/物品栏、GM 指令处理 |
| `internal/loginapp/` | 登录服：账号/密码认证、令牌签发、校验、刷新、登出 |
| `internal/gatewayapp/` | 网关：WebSocket 接收器、Pomelo 协议解析、会话管理、鉴权路由转发 |
| `internal/masterapp/` | 发现主节点：NATS 模式集群注册，无业务路由 |
| `internal/gmapp/` | GM 管理：HTTP 监听、NATS 请求中转到 game 节点 |
| `internal/persistence/` | 数据层：GORM 模型（账号、玩家、背包、物品栏）、仓储、事务、Redis 频率限制、迁移 |
| `internal/protocolpb/` + `internal/protocol/` | 通信协议：`.proto` 定义 → `gen/` 下生成的 Go 类型；`types.go` 别名导出 |
| `internal/gtime/` | 游戏时间：时钟抽象、Redis 时间偏差、日历工具 |
| `gameconfig/` | 策划配表管线：CSV 源数据（`datas/`）→ Luban 模式（`defines/`）→ 生成的 Go 配置加载器（`gen/`）→ 导入 CLI（`cmd/import/`） |
| `cmd/client-demo/` | 集成测试客户端：WebSocket 冒烟测试（令牌 → 登录 → 选角 → 进场景 → 背包 → 移动） |

### 数据流

```
客户端 (WebSocket)
  → 网关 (WebSocket 接收器，Pomelo 协议解码)
    → NATS 路由至 login (auth.*) 或 game (player.*, scene.*, chat.*, bag.*)
      → Login: MySQL 账号表，Redis 会话令牌
      → Game: Actor 处理器 → MySQL 持久化 (GORM) / Redis (会话、频率限制)
  ← 网关 (Pomelo 编码，WebSocket 响应) ← 客户端
```

GM 管理流: `HTTP POST → gm 进程 → NATS ClusterPacket → game ActorGM → 处理器 → NATS 响应 → gm → HTTP JSON`。

通信格式: Protobuf 二进制（各节点 `SetSerializer(NewProtobuf())`）。路由约定: `节点类型.处理器名.方法`（Pomelo 风格）。

## 缓存稳定性

- **频繁重建文件:** `bin/*.exe`（每次 `go build` 重建），`gameconfig/gen/`（CSV 变更时重新生成），`internal/protocolpb/gen/`（`.proto` 变更时重新生成）。这些文件经常变化 — 避免将其作为稳定上下文。
- **稳定骨架:** `configs/mmo-cluster.json`，`go.mod`，`README.md`，`AGENTS.md`。这些文件很少变化，是项目的锚点 — 保持跨轮次字节稳定。
- **追加而非重排:** 新上下文放在请求末尾。重新排序或改写之前的消息会使其后所有内容的缓存失效。

## 规范

- 遵循现有代码风格和模式
- 为新功能编写测试
- 保持改动聚焦且原子化
- 为公开 API 编写文档
- 项目约定变更时更新本文件