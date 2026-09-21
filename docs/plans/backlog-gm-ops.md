# GM 运营后台其余 P2（Backlog）

> **Status:** planned  
> **P0：** [`2026-09-14-gm-ops-p0-design.md`](2026-09-14-gm-ops-p0-design.md)  
> **P1：** [`2026-09-21-gm-ops-p1-design.md`](2026-09-21-gm-ops-p1-design.md)  
> **P2 管控核：** [`2026-09-21-gm-ops-p2-design.md`](2026-09-21-gm-ops-p2-design.md)

管控核（禁言/解禁、限时封号、全量吊销被封账号 token、强制维护）已立项。本文件只收纳 **其余 P2**。立项时复制 `_template-design.md` 升级为独立策划。

## 其余产品化

- 清背包、改槽（高危，二次确认）
- 改名、软删恢复、重置密码
- 查设备会话
- 邮件 / 附件、礼包码、活动开关
- 独立货币账本
- 细权限树、SSO、TLS（账号密码登录已拆至 [2026-09-14-gm-prod-console-design.md](2026-09-14-gm-prod-console-design.md)）
- 多 game 节点目标发现（突破单 `-game`）

Web 控制台已立项：[2026-09-14-gm-web-ui-design.md](2026-09-14-gm-web-ui-design.md)。

## 明确不做（需其它系统先落地）

- 战斗 / 掉落发奖：见 [`backlog-combat.md`](backlog-combat.md)
- DB 版本化迁移：见 [`backlog-db-migrate.md`](backlog-db-migrate.md)
