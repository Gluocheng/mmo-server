# 策划配表系统 Implementation Plan

> **Design:** [2026-06-01-gameconfig-design.md](2026-06-01-gameconfig-design.md)  
> **Status:** done

**Goal:** Excel/Luban 导表 → MySQL → game 运行时内存加载，首期道具表 + 背包校验 + 手动 reload。

---

## 任务

- [x] 策划文档与 Roadmap
- [x] gameconfig/ 脚手架 + Luban gen
- [x] schema + import CLI + migrate
- [x] runtime Load/Reload
- [x] bag 集成 + config Actor + 测试
