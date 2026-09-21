# GM 运营后台 P2（Backlog）

> **Status:** planned  
> **P0：** [`2026-09-14-gm-ops-p0-design.md`](2026-09-14-gm-ops-p0-design.md)  
> **P1：** [`2026-09-21-gm-ops-p1-design.md`](2026-09-21-gm-ops-p1-design.md)

P1（扣道具、封号/解封、场景在线列表、全服/场景公告、调游戏时间）已立项。本文件只收纳 **P2**。立项时复制 `_template-design.md` 升级为独立策划。

## P2 产品化

- 禁言 / 解禁
- 清背包、改槽（高危，二次确认）
- 改名、软删恢复、重置密码
- 强制维护（踢全服 + 拒绝登录）
- 查设备会话
- 邮件 / 附件、礼包码、活动开关
- 独立货币账本
- 细权限树、SSO、TLS（账号密码登录已拆至 [2026-09-14-gm-prod-console-design.md](2026-09-14-gm-prod-console-design.md)）
- 多 game 节点目标发现（突破单 `-game`）
- 限时封号
- Redis 全量吊销被封账号 token

Web 控制台已立项：[2026-09-14-gm-web-ui-design.md](2026-09-14-gm-web-ui-design.md)。

## 明确不做（需其它系统先落地）

- 战斗 / 掉落发奖：见 [`backlog-combat.md`](backlog-combat.md)
- DB 版本化迁移：见 [`backlog-db-migrate.md`](backlog-db-migrate.md)
