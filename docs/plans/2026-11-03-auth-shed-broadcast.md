# 登录过载与广播回包 实施计划

> **Status:** done  
> **Design:** [`2026-11-03-auth-shed-broadcast-design.md`](2026-11-03-auth-shed-broadcast-design.md)（须 **Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 签发和刷新拆开。密码队列满了立刻返回 `40036`。移动、聊天先回包，再异步广播。

**Architecture:** 登录进程增加 `pwd` 工人，网关签发只打这组工人。每个工人 32 个名额，满了换人，都满则马上回复。游戏进程一条广播队列，移动按人合并，聊天占不到名额就返回 `40037`。

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis

---

## 任务清单

### Task 1: 密码工人与繁忙码

**Files:**
- Modify: `internal/code/code.go`、`internal/authcfg/authcfg.go`、`internal/loginroute/path.go`、`internal/loginapp/actor/actor_session.go`、`internal/loginapp/app.go`、`internal/gatewayapp/actor/agent.go`
- Create: `internal/loginapp/admit/admit.go`、`internal/loginapp/actor/actor_pwd.go`

- [x] `40036`，密码工人数量公式，轮询路径 `node.pwd.N`
- [x] session 工人不再处理签发
- [x] 名额满了回复 `40036`，不算 bcrypt
- [x] 单测覆盖名额溢出和工人数公式

### Task 2: 移动和聊天先回包

**Files:**
- Create: `internal/gameapp/broadcast/broadcast.go`
- Modify: `internal/gameapp/player/actor_player.go`、`internal/gameapp/player/actor_players.go`、`internal/gameapp/chat/actor_chat.go`、`internal/code/code.go`

- [x] 队列长度 4096，移动合并，聊天满了 `40037`
- [x] 进场、切图记下视野；断线丢掉未推的移动
- [x] 坐标写入后立刻回包

### Task 3: 文档

- [x] README 错误码和登录说明
- [x] `docs/plans/load-test.md` 增加 LT-21，压测列写未压测
- [x] `go test` 覆盖改动包，`go build` login、gateway、game

## 验证

- [x] `go test` 改动包
- [x] `go build` login / gateway / game

## 备注

不改 cherry，不降低 bcrypt 成本。2000 人同时签发仍可能得到 `40036`。
