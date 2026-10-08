# 登录服集群 实施计划

> **Status:** done  
> **Design:** [`2026-10-08-login-cluster-design.md`](2026-10-08-login-cluster-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 多个登录节点并行签发，每个节点内再用一组工人做密码哈希。

**Architecture:** 网关轮询 `{node}.session.{工人}`。父 Actor 只留调时间。GM 把时间偏置推到 profile 里的每一个登录节点。

**Tech Stack:** Go / Cherry Actor / NATS

---

## 任务清单

### Task 1: 节点

- [x] `login-2` 与 `session_workers`
- [x] 启动脚本按节点拉起

### Task 2: 工人与分发

- [x] 签发、登录、刷新、登出在子 Actor
- [x] 网关轮询；GM 通知全部登录节点

### Task 3: 验收

- [x] 轮询单测覆盖全部工人
- [x] 50 并发签发不再出现 `40005`，限流仍是 `40014`

---

## 验证

- [x] `go test ./...`
- [x] 重启后 50 并发签发

## 备注

不改客户端协议，不改密码哈希成本。
