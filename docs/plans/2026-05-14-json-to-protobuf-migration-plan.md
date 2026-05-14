# MMO JSON 到 Protobuf 迁移计划

## 目标

在不影响当前可运行链路的前提下，将高频消息从 JSON 迁移到 Protobuf，降低带宽与序列化开销；中低频管理协议保留在后续阶段迁移。

## 范围

- 第一阶段：`scene` / `move` 高消息频率链路
- 第二阶段：`player` 业务链路（查角/创角/进场）
- 第三阶段：`auth` 链路（issue/login/refresh/logout）

## 迁移步骤

- [ ] 冻结现有协议字段，避免迁移过程中频繁改名
- [ ] 新建 proto 目录：`internal/protocolpb/proto/`
- [ ] 按领域定义 proto 文件：
  - [ ] `common.proto`
  - [ ] `scene.proto`
  - [ ] `player.proto`
  - [ ] `auth.proto`
- [ ] 生成 Go 代码到：`internal/protocolpb/gen/`
- [ ] 替换业务代码中的协议类型（先从 scene 开始）：
  - [ ] `internal/gameapp/player/actor_player.go`
  - [ ] `internal/gatewayapp/actor/agent.go`
  - [ ] `internal/loginapp/actor/actor_session.go`
- [ ] 统一切换序列化器为 Protobuf（节点级一致）：
  - [ ] `internal/gatewayapp/app.go`
  - [ ] `internal/loginapp/app.go`
  - [ ] `internal/gameapp/app.go`
- [ ] 更新 README 协议示例与生成命令

## 兼容与风险控制

- [ ] 关键路由可加版本后缀（例如 `game.player.move.v2`）进行灰度
- [ ] 删除字段前使用 `reserved`，禁止复用历史字段号
- [ ] 避免跨节点出现 JSON/Protobuf 混序列化状态

## 验证清单

- [ ] `go test ./...`
- [ ] `go build -o bin/gateway.exe ./cmd/gateway`
- [ ] `go build -o bin/login.exe ./cmd/login`
- [ ] `go build -o bin/game.exe ./cmd/game`
- [ ] 手工联调闭环：
  - [ ] `issueToken`
  - [ ] `login`
  - [ ] `select/create/enter`
  - [ ] `move` 广播

## 里程碑

1. M1：`scene` 协议迁移完成并通过压测
2. M2：`player` 协议迁移完成并稳定运行
3. M3：`auth` 协议迁移完成并文档齐全
