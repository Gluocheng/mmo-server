# 网关按配置集群 实施计划

> **Status:** done  
> **Design:** [`2026-10-08-gate-cluster-design.md`](2026-10-08-gate-cluster-design.md)（**Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** 配置里启用几个网关、几个登录节点，就启动几个进程。

**Architecture:** 启动脚本读 profile 的 `node.gate` 和 `node.login`。每个网关绑定自己的 `address`。客户端自行选择入口。

**Tech Stack:** PowerShell / Cherry 集群配置

---

## 任务清单

### Task 1: 配置

- [x] `gate-2` 监听 `:10101`，日志独立

### Task 2: 启动

- [x] 按启用节点逐个拉起网关和登录
- [x] 打印每个网关地址

### Task 3: 验收

- [x] 两个端口都能签发
- [x] `enable=false` 不起 `gate-2`

---

## 验证

- [x] 两个网关都注册到 master
- [x] 验收后 `gate-2` 恢复启用

## 备注

不加负载均衡器，不改签发逻辑。
