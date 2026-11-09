# 登录签发吞吐提升 实施计划

> **Status:** done
> **Design:** [`2026-11-09-login-issue-throughput-design.md`](2026-11-09-login-issue-throughput-design.md)（**Status: approved**）
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**Goal:** 让服务端在 20 秒 `request_timeout` 窗口内自己吃下 2000 个全新建号，不需要客户端重试。

**Architecture:** bcrypt 哈希移出 `WithinTx`，先算哈希再开事务写库。密码工人数改为按可用连接数优先（上限 16），连接池由 20 提到 40。`40036` 语义不变，仍是超容量时的快速拒绝。

**Tech Stack:** Go / Cherry Actor / GORM / bcrypt

---

## 任务清单

### Task 1: 哈希移出事务

**Files:**
- Modify: `internal/persistence/account.go`、`internal/persistence/service.go`

- [x] 新建路径先在事务外计算 `bcrypt` 哈希，再进 `WithinTx`
- [x] 事务内仍完成「查账号 → 命中校验 → 未命中插入」，保证并发不建重号
- [x] 「账号已存在」分支丢弃事务外算的哈希，沿用校验逻辑
- [x] `AfterCommit` 刷缓存行为不变

### Task 2: 工人数与连接池

**Files:**
- Modify: `internal/authcfg/authcfg.go`、`internal/authcfg/workers_test.go`
- Modify: `configs/mmo-cluster.json`、`configs/mmo-docker.json`

- [x] `PasswordWorkersFrom` 改为 `min(max(2, openConns − sessionWorkers − 4), 16)`，不再按核数平分
- [x] `max_open_conns` 20 → 40、`max_idle_conns` 10 → 40（两个 profile）
- [x] 单测覆盖新公式：40 连接 8 session 时每节点 16 个工人

### Task 3: 文档

- [x] `docs/plans/load-test.md` 记录新口径与重跑结果
- [x] README 错误码说明补 `40036` 的退避约定

## 验证

- [x] `go test ./...`
- [x] `go build ./...`
- [x] 压测：2000 个全新建号全成功，无 `40036`、无 `40005`（单波 19.25s / 20.47s 两次）

## 备注

放宽核数约束后单节点会用更多核做哈希，压测时需采样 game / gateway 的 CPU 与延迟，确认其它业务未受明显影响。
