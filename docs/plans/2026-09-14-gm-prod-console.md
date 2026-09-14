# GM 上线鉴权与 Naive UI 改版 实施计划

> **Status:** done  
> **Design:** [`2026-09-14-gm-prod-console-design.md`](2026-09-14-gm-prod-console-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** GM 账号登录、按人审计查询、管理员管号、Naive UI 控制台。

**Architecture:** GM 加载 profile 连同一 MySQL/Redis；会话 Cookie；业务仍 NATS → game 写 `gm_ops_logs`。

**Tech Stack:** Go / GORM / Redis / Vue 3 / Naive UI / Vite

---

## 任务清单

### Task 1: 账号、会话、鉴权 HTTP

**Files:** `internal/persistence/model/model_gm_user.go`、`gm_user.go`、`gm_ops.go`、`migrate.go`、`internal/gmapp/*`、`cmd/gm/main.go`、`internal/code/code.go`、`configs/*.json`

- [x] `gm_users` + 种子管理员 + Redis/内存会话
- [x] 登录/登出/me；Cookie 或 token；admin 管号；operator 来自会话
- [x] `GET /gm/ops/logs`
- [x] 单测

### Task 2: Naive UI

**Files:** `web/gm-console/**`

- [x] naive-ui 登录/布局/表格/确认；日志页、账号页
- [x] `npm run build` 写入 `internal/gmapp/ui`

### Task 3: 启动与文档

- [x] start.ps1 / docker-compose / README 种子账号
- [x] `go test ./...`、`go build ./...`

---

## 验证

- [x] `go test ./internal/persistence/...` `go test ./internal/gmapp/...`
- [x] `go test ./...`
- [x] `go build ./...`
- [x] 浏览器账号登录与操作记录
