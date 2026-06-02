# MMO Server 功能计划与进度

本目录存放**策划文档**与**实施计划**。新功能须**先生成策划、评审通过后再写代码**（双文件流程）。

## 如何使用

| 阶段 | 动作 |
|------|------|
| 策划 | 复制 [`_template-design.md`](_template-design.md) → `YYYY-MM-DD-<feature>-design.md`，填齐章节；`Status: draft` → `review` |
| 评审 | 用户确认范围/协议/数据；策划改为 `approved` |
| 实施 | 复制 [`_template.md`](_template.md) → `YYYY-MM-DD-<feature>.md`，文首链策划；`planned` → `in_progress` |
| 开发 | 按实施 plan 勾选 `- [ ]` → `- [x]`；会话 `@` 实施 plan |
| 完成 | 实施 plan `done`、策划可保持 `approved`；更新下表 |
| 搁置 | 实施或策划标 `paused` / `superseded`，注明原因 |

**策划状态：** `draft` · `review` · `approved` · `superseded`  
**实施状态：** `planned` · `in_progress` · `done` · `paused`

## 总览（Roadmap）

| 状态 | 功能 | 计划文档 | 说明 |
|------|------|----------|------|
| done | Protobuf 三节点 + 集群 NATS | — | 基线已合入主干 |
| done | 帐号/角色持久化 + 全局事务 | — | `WithinTx` / `AfterCommit` |
| done | 场景分房 + 简单 AOI 移动广播 | — | `internal/gameapp/world` |
| done | 最小聊天 | — | `game.chat.send` |
| done | 背包系统（v1+v2） | [2026-05-28-bag-system-design.md](2026-05-28-bag-system-design.md) | 策划归档；实施 [v1](2026-05-28-bag-system.md) · [v2](2026-05-28-bag-v2.md) |
| done | 统一游戏时间 gtime | [2026-05-28-gtime.md](2026-05-28-gtime.md) | `internal/gtime` + Redis 偏置 |
| done | 持久化 GORM 模型拆分 | [2026-05-28-persistence-models-split.md](2026-05-28-persistence-models-split.md) | `model_*.go` + `migrate.go` |
| done | persistence 日志统一 clog | — | 业务码 `40001–40022` |
| done | 策划配表（Excel→MySQL→runtime） | [2026-06-01-gameconfig-design.md](2026-06-01-gameconfig-design.md) | 实施 [2026-06-01-gameconfig.md](2026-06-01-gameconfig.md) |
| superseded | 道具系统（JSON 文件方案） | [2026-05-28-item-system-design.md](2026-05-28-item-system-design.md) | 已由 gameconfig 策划替代 |
| planned | 战斗骨架 | [backlog-combat.md](backlog-combat.md) | — |
| planned | 一账号多角 | [backlog-multi-character.md](backlog-multi-character.md) | 表结构 + 协议评审 |
| planned | DB 版本化迁移 | [backlog-db-migrate.md](backlog-db-migrate.md) | 替代仅 AutoMigrate |

> 无 `*-design.md` 的 done 项为历史基线；**新迭代**须策划 + 实施双文件，并更新本表。

## 目录约定

```
docs/plans/
├── README.md                    # 本文件：总览与进度
├── _template-design.md          # 策划文档模板（先写）
├── _template.md                 # 实施计划模板（策划 approved 后）
├── YYYY-MM-DD-<feature>-design.md  # 策划：范围、协议、验收
├── YYYY-MM-DD-<feature>.md         # 实施：任务勾选、验证
└── backlog-<topic>.md           # 远期 backlog（立项时升级为 *-design.md）
```

## Agent / 协作者

- **新功能：** 先生成 `*-design.md`，待用户确认后再建实施 plan 与写代码（[`.cursor/rules/feature-plans.mdc`](../../.cursor/rules/feature-plans.mdc)）。
- **评审：** `@YYYY-MM-DD-<feature>-design.md`
- **开发：** `@YYYY-MM-DD-<feature>.md`
- 提交说明：可用 `/git-staged-commit-draft` 根据暂存区生成 commit message。
