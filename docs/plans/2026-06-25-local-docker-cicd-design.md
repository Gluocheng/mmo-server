# 本地 Docker CI/CD 一条龙 策划文档

> **Status:** approved
> **实施计划：** [`2026-06-25-local-docker-cicd.md`](2026-06-25-local-docker-cicd.md)
> **说明：** 本地预发布模拟环境，非生产部署方案。

---

## 背景与目标

当前仓库仅有 Windows PowerShell 启停脚本与 GitHub Actions 单元测试，MySQL/Redis 需本机手动准备，无统一容器编排与本地发布/回滚流程。

**一期目标：** 用 Docker Compose 在本机拉起 MySQL + Redis + NATS + 五服务，并提供 `scripts/cicd.ps1` 串联 test → package → deploy → smoke → rollback。

---

## 用户场景

1. **开发改代码后自测：** `cicd.ps1 -Stage test` 跑单测，`-Stage release` 一键打包镜像并启动预发布栈。
2. **模拟发布：** 镜像打 tag（如 `mmo-server:20260625-1430`），记录 `.release/current` 与 `previous`。
3. **冒烟验证：** 检查 gateway 10100、gm 9080、GM config reload HTTP。
4. **回滚：** `cicd.ps1 -Stage rollback` 切回上一 tag 并重启 compose。

---

## 功能范围

### 包含

- 单镜像多二进制（master/login/game/gateway/gm/import-config）
- `docker-compose.yml` 编排依赖与应用服务
- `configs/mmo-docker.json`（服务名互访）
- `scripts/cicd.ps1`、`scripts/docker-smoke.ps1`
- README 与 GitHub Actions 补充 gm build

### 不包含

- Jenkins/Gitea Runner
- 远端镜像仓库 push
- 生产级密钥/TLS/日志采集

---

## 验收标准

- [x] `docker build` 成功
- [x] `docker compose up` 五服务 + 依赖均 running
- [x] import-config 初始化 MySQL 配表
- [x] GM reload HTTP 返回 code=0
- [x] `cicd.ps1 -Stage rollback` 可切回上一 tag

---

## 实施索引

见 [`2026-06-25-local-docker-cicd.md`](2026-06-25-local-docker-cicd.md)。
