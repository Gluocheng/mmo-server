# 本地 Docker CI/CD 一条龙 实施计划

> **Status:** done
> **Design:** [`2026-06-25-local-docker-cicd-design.md`](2026-06-25-local-docker-cicd-design.md)
> **完成日期：** 2026-06-25

**Goal:** 本机 Docker Compose 模拟预发布：test → build image → deploy → smoke → rollback。

---

## 任务清单

### Task 1: Docker 基础设施

- [x] `Dockerfile` 多阶段构建六二进制
- [x] `.dockerignore`
- [x] `docker-compose.yml`（mysql/redis/nats + 五服务 + import job）
- [x] `configs/mmo-docker.json`

### Task 2: 流水线脚本

- [x] `scripts/cicd.ps1`（test/package/deploy/smoke/release/rollback/down）
- [x] `scripts/docker-smoke.ps1`

### Task 3: 文档与 CI

- [x] `docs/plans/README.md` Roadmap
- [x] `README.md` Docker CI/CD 章节
- [x] `.github/workflows/go.yml` 补 gm build

### Task 4: 验证

- [x] `go test ./...`
- [x] `docker build`
- [x] `docker compose up` + smoke

---

## 备注

- 镜像 tag 默认 `local`，release 时写入 `.release/current` 与 `previous`。
- gm 仍用 CLI 参数，compose 中 `-nats=nats://nats:4222`。
