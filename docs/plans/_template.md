# [功能名称] 实施计划

> **Status:** planned | in_progress | done | paused  
> **Design:** [`YYYY-MM-DD-<feature>-design.md`](YYYY-MM-DD-<feature>-design.md)（须 **Status: approved**）  
> **For agentic workers:** 按任务勾选推进；完成后更新 `docs/plans/README.md` 总览表。

**硬性顺序：** 策划文档 `approved` 后方可创建本文件并写业务代码。任务从策划的协议/数据/验收派生，勿重复长篇策划正文。

**Goal:** （一句话：要交付什么，与策划一致）

**Architecture:** （2–3 句：方案要点）

**Tech Stack:** Go / Cherry Actor / Protobuf / GORM / Redis

---

## 任务清单

### Task 1: （组件名）

**Files:**
- Create:
- Modify:
- Test:

- [ ] 步骤 1
- [ ] 步骤 2
- [ ] `go test ./...` 通过
- [ ] 更新 README（若涉及协议/启动）

---

## 验证

- [ ] `go test ./...`
- [ ] 相关节点 `go build`（gateway / login / game / master）
- [ ] （可选）`go run ./cmd/client-demo` 冒烟

## 备注

（实施期发现的问题、与策划差异需回写 design 或记此处）
