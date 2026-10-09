# 战斗属性 实施计划

> **Status:** done  
> **Design:** [`2026-10-09-combat-attr-design.md`](2026-10-09-combat-attr-design.md)（**Status: approved**）

**Goal:** 生命、攻击、防御用同一套基础加固定再乘百分比。技能伤害改为攻击力百分比再减防御。其余常见属性只登记。

**Architecture:** 进场或刷出时拷贝基础三围。出手前用未到期的 `attr` Buff 重算最终值。`cfg_stat.settle=false` 的属性挂上时返回 `40041`。

**Tech Stack:** Go / 配表 / 战斗心跳

## 任务清单

- [x] 战斗常量、技能 `factor`、Buff `stat`/`mode`、属性名单
- [x] 最终属性、出手值和生命上限变化
- [x] 验收单测与 `go test ./...`
