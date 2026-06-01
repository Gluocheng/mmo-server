# GM 独立管理进程 策划文档

> **Status:** approved  
> **实施计划：** [`2026-06-01-gm-process.md`](2026-06-01-gm-process.md)  
> **说明：** 已完成功能回溯归档；非待开发需求。

---

## 背景与目标

策划/运维需要在不停服的前提下下发管理指令（首期：配表热更）。直接复用 Pomelo 客户端调用 `game.gm.config.reload` 需开发专用客户端、且鉴权与玩家通道耦合；外部脚本/HTTP 工具链则无法直接进入 Cherry 集群。

**一期目标：** 新增独立 `cmd/gm` 进程，对外暴露 HTTP API，对内通过 NATS 与 game 节点的 `ActorGM` 通信，承接配表热更，并为后续管理功能预留 domain 路由扩展点。

---

## 用户场景

1. **运维热更配表：** 策划导表入库后 `curl -X POST /gm/config/reload` → GM 进程经 NATS 通知 game 节点 → 内存原子替换。
2. **未来扩展：** 玩家封禁、邮件下发、活动开关等，按 `domain` 在 GM Actor 添加子 Actor 与 HTTP 路由，无需改 game 业务代码。
3. **故障隔离：** GM 进程独立于 game 集群，HTTP 故障不影响在线玩家；NATS 不可达时 HTTP 返回 504 而非阻塞。

---

## 功能范围

### 包含

- `cmd/gm` 独立进程入口，CLI 参数：`-http`、`-nats`、`-prefix`、`-game`
- `internal/gmapp`：HTTP 路由（`net/http`）+ NATS 客户端
- `internal/gameapp/gm`：`ActorGM` 根 Actor + `actorGMConfig` 子 Actor，支持 Pomelo Local 与 Cluster Remote 双路调用
- `POST /gm/config/reload`：JSON body `{tableName}`（空 = 全量），转 `RefreshTokenRequest` 经 ClusterPacket 下发
- 启停脚本 `scripts/start.ps1` / `stop.ps1` 接入 GM 进程
- 业务错误码：`40024 ConfigReloadDenied`、`40025 ConfigReloadFail`

### 不包含

- HTTP 鉴权（一期内网部署，依赖运维边界）
- 集群模式 GM（多实例 / 选主）
- WebUI 控制台
- 玩家相关管理指令（封号、改属性等）
- TLS / mTLS

---

## 协议设计

### HTTP（GM 对外）

| 方法 | 路径 | 请求 Body | 响应 |
|------|------|-----------|------|
| POST | `/gm/config/reload` | `{"tableName": "item"}`（空字符串=全量） | `{"code":0,"message":"ok","version":"<verstr>","tables":<n>}` |

**HTTP 状态码：**
- 200：业务成功 / 业务失败（看 body `code`）
- 400：JSON 解析失败
- 405：非 POST
- 500：编解码异常
- 504：NATS 请求超时（5s）

### Cluster（GM → game）

| 类型 | 路径 | 请求 | 响应 | 说明 |
|------|------|------|------|------|
| Cluster Call | `game.gm.config.reload` | `RefreshTokenRequest`（`refreshToken` 承载表名） | `RefreshTokenResponse`（`accessExpireAt`=version，`refreshExpireAt`=tableCount） | NATS subject `cherry-{prefix}.remote.game.{nodeID}` |

**业务错误码（沿用 gameconfig 策划）：** `40024 ConfigReloadDenied` · `40025 ConfigReloadFail`

**前置条件：** profile `gameconfig.allow_reload = true`；NATS 4222 可达。

---

## 数据与持久化

GM 进程**无持久化状态**：HTTP 处理纯粹路由 + 转发，所有写操作落在 game 节点的 `gameconfig.ReloadTable / Reload`，沿用 gameconfig 一期的事务/快照机制。

---

## 业务规则

- **路由约定：** Cherry ActorPath `{nodeID}.gm.{domain}`；GM 进程 `gm-1.gm.config` → game 节点 `{gameNodeID}.gm.config`
- **双路注册：** `actorGMConfig.OnInit` 同时 `Local().Register("reload", ...)` 和 `Remote().Register("reload", ...)`，支持 Pomelo 客户端与 GM 进程两种调用方
- **超时：** HTTP handler → NATS Request 5 秒超时；超时返回 504
- **幂等：** 多次 reload 安全，全量 reload 通过 RWMutex 指针 swap，部分失败保留旧快照
- **扩展：** 新 domain 在 `ActorGM.OnFindChild` switch 中加 case + 新增子 Actor 文件 + 新增 HTTP handler，三步落地

---

## 验收标准

- [x] `cmd/gm` 二进制可独立运行
- [x] `curl POST /gm/config/reload` 在 game 节点 `allow_reload=true` 下成功
- [x] 关闭 `allow_reload` 后返回 `code=40024`
- [x] game 节点未启动时 HTTP 返回 504，进程不崩溃
- [x] 启停脚本拉起/关闭 gm 进程
- [x] `go test ./...` 通过

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | gameconfig 一期（`runtime.Reload` / `ReloadTable`）、Cherry NATS 集群 |
| 鉴权 | 一期无；生产部署须放在内网或加反向代理鉴权层 |
| 协议复用 | 复用 `RefreshTokenRequest/Response` 承载表名/版本，避免新建 proto；后续若 GM 命令膨胀建议新增 `gm.proto` |
| 多 game 节点 | 当前 CLI 单 `-game` 参数；多节点须遍历或下发到 `nodeType=game` |

---

## 实施索引

见 [`2026-06-01-gm-process.md`](2026-06-01-gm-process.md)。