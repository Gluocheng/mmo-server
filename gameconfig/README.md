# 策划配表（gameconfig）

Excel/CSV 策划数据经 Luban 导出 JSON 与 Go 代码，import 工具写入 MySQL，game 节点从 DB 加载到内存。

## 目录

| 路径 | 说明 |
|------|------|
| `datas/` | 策划 CSV 源表（Luban 格式：首行 `##,列名`，后续行数据） |
| `defines/` | Luban schema（`__root__.xml` + 各表 bean 定义 `*.xml`） |
| `tools/gen.ps1` | 一键导表（Luban → `gen/cfg` Go 类型 + `gen/data` JSON） |
| `tools/luban/` | Luban.ClientServer（classic 版，已提交） |
| `gen/cfg/` | Luban 生成的 Go 类型（**生成物，勿手改**） |
| `gen/data/` | Luban 生成的 JSON（import 输入） |
| `cmd/import/` | JSON → MySQL |
| `pkg/schema/` | GORM 配置表模型（`CfgItem`、`CfgBagType`、`CfgVersion`） |
| `pkg/runtime/` | 运行时 Load / Reload / 查询 |

## 工作流

```powershell
# 1. 导表（需 .NET 6+，Luban 工具已随仓库提交于 tools/luban/）
.\gameconfig\tools\gen.ps1

# 2. 导入 MySQL（使用 configs/mmo-cluster.json 中的 DSN）
go run ./gameconfig/cmd/import -profile configs/mmo-cluster.json

# 3. 启动 game 节点（启动时自动 Load）
go run ./cmd/game -profile configs/mmo-cluster.json -node 10001
```

## 配置表

### 道具（item）

| 字段 | 说明 |
|------|------|
| `id` | 道具 id |
| `name` | 名称 |
| `type` | 分类（consumable / material / equipment / quest） |
| `max_stack` | 单槽最大堆叠 |
| `stackable` | 可堆叠 |
| `discardable` | 可丢弃 |
| `bind_type` | 绑定类型 |
| `bag_type` | 目标背包类型（对应 `bag_type` 表 id） |

### 背包类型（bag_type）

| 字段 | 说明 |
|------|------|
| `id` | 类型 id |
| `name` | 名称 |
| `slot_count` | 槽位数 |

| id | name | slot_count |
|----|------|-----------|
| 1 | 通用背包 | 32 |
| 2 | 消耗品 | 32 |
| 3 | 材料 | 32 |
| 4 | 装备 | 8 |
| 5 | 任务 | 32 |

## 演示道具

| id | name | bag_type | max_stack |
|----|------|----------|-----------|
| 1001 | 小型生命药水 | 2 | 99 |
| 1002 | 铜币袋 | 3 | 9999 |
| 2001 | 新手木剑 | 4 | 1 |
| 3001 | 任务信件 | 5 | 1 |

## 新增一张配置表

1. `datas/` 加 CSV（首行 `##,列名`）
2. `defines/` 加 `<module>` + `<bean>` + `<table>` 定义
3. 跑 `gen.ps1` 生成 `gen/cfg` / `gen/data`
4. `pkg/schema/models.go` 加 GORM 模型并登记 `Models()`
5. `cmd/import/main.go` 加导入逻辑
6. `pkg/runtime/` 加 Load / 查询

## 热更

profile 中设置 `"gameconfig": { "allow_reload": true }` 后，可调用 RPC `game.config.reload`（或 GM HTTP `/gm/config/reload`）。支持按表名重载：`item`、`bag_type`。
