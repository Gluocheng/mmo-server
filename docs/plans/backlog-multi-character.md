# 一账号多角（Backlog）

> **Status:** planned

**Goal:** 单 Uid 多角色；`select` 列表、`enter` 指定 `playerId`。

## 范围草案

- [ ] 去掉 `players.uid` 唯一约束
- [ ] 调整 `select/create/enter` 语义
- [ ] 数据迁移方案（现有单角数据）

> 牵动表结构与协议，实施前需单独评审。
