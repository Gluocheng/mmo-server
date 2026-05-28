# 持久化 GORM 模型拆分

> **Status:** done  
> **完成日期：** 2026-05-28

**Goal:** 避免 `models.go` 无限膨胀；统一 `AutoMigrate` 注册点。

**Architecture:** 每表一个 `model_<table>.go`；`migrate.go` 提供 `autoMigrateModels`；测试用 `resetDBForTest`。

---

## 任务清单

- [x] `model_account.go` / `model_player.go` / `model_inventory_item.go`
- [x] `migrate.go` + `store.go` 调用
- [x] 删除 `models.go`
- [x] `tx_test` / `bag_test` 共用 `resetDBForTest`
- [x] `go test ./internal/persistence/...` 通过
