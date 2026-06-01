# 策划配表系统（Excel → MySQL → 运行时加载）策划文档

> **Status:** approved  
> **实施计划：** [`2026-06-01-gameconfig.md`](2026-06-01-gameconfig.md)  
> **替代：** [`2026-05-28-item-system-design.md`](2026-05-28-item-system-design.md)（`superseded`）

---

## 背景与目标

当前背包仅校验 `item_id >= 1`，无道具元数据。策划需通过 Excel/CSV 配表，经工具链导入 MySQL，game 节点启动时与手动 reload 时从 DB 加载到内存，供背包等业务校验。

**一期目标：** 建立 `gameconfig/` 子系统（Luban 导表 + import CLI + runtime loader），落地道具表 `TbItem`，接入背包校验与 `game.config.reload`。

---

## 用户场景

1. **策划改表：** 编辑 `gameconfig/datas/*.xlsx` → 运行 `gen.ps1` → 运行 `go run ./gameconfig/cmd/import` → MySQL 更新。
2. **game 启动：** 节点启动时 `runtime.Load` 读 DB，Load 失败则拒绝启动。
3. **热更（手动）：** 运维调用 `game.config.reload`（profile 开关）→ 内存原子替换，失败保留旧配置。

---

## 功能范围

### 包含

- `gameconfig/` 独立目录（datas / defines / tools / gen / cmd / pkg）
- Luban 导表脚本（`code_go_json` + `data_json`）
- `cmd/import`：JSON → MySQL `cfg_item` + `cfg_version`
- `pkg/runtime`：Load / Reload / Exists / MaxStack
- game 启动加载；`game.config.reload` RPC
- 背包接入：`40023 ItemNotFound`、`effectiveMaxStack`

### 不包含

- 自动轮询 DB 版本
- 运营后台 UI
- 客户端配表导出
- Redis 配置缓存

---

## 协议设计

| 类型 | Route | 请求 | 响应 | 说明 |
|------|-------|------|------|------|
| RPC | `game.config.reload` | `google.protobuf.Empty` | `RefreshTokenResponse`（`accessExpireAt`=version，`refreshExpireAt`=tableCount） | profile `gameconfig.allow_reload=true` |

**ConfigReloadResponse：** `version`（int64）、`table_count`（int32）

**业务错误码：** `40023 ItemNotFound` — `item_id` 不在配置表；`40024 ConfigReloadDenied` — 未开启 reload；`40025 ConfigReloadFail` — reload 失败

---

## 数据与持久化

| 表 | 字段 | 说明 |
|----|------|------|
| `cfg_version` | `id=1`, `version`, `updated_at` | 全局配置版本 |
| `cfg_item` | `id`, `name`, `type`, `max_stack`, `stackable`, `discardable`, `bind_type` | 道具静态配置 |

**事务：** import 工具单事务内替换 `cfg_item` 并递增 `cfg_version`；runtime Load 只读。

---

## 业务规则

- 有效堆叠上限：`min(MaxBagStack, item.max_stack)`；配置不存在 → `ItemNotFound`
- reload：先构建新 Registry，再 RWMutex 指针 swap
- 权威来源：Excel → gen → import；DB 手改仅应急

---

## 验收标准

- [ ] `gen.ps1` 可生成 Go + JSON（或仓库已提交生成物可编译）
- [ ] `go run ./gameconfig/cmd/import` 写入 MySQL
- [ ] game 启动 Load 成功；非法 `item_id` add 返回 40023
- [ ] `game.config.reload` 在 dev profile 下可用
- [ ] `go test ./...` 通过

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 依赖 | 背包系统（done）、MySQL/GORM |
| Luban | 需 .NET 6+；工具见 `gameconfig/tools/luban/README.md` |
| 迁移 | AutoMigrate 追加 cfg 表 |

---

## 实施索引

见 [`2026-06-01-gameconfig.md`](2026-06-01-gameconfig.md)。
