# 策划配表（gameconfig）

Excel/CSV 策划数据经 Luban 导出 JSON 与 Go 代码，import 工具写入 MySQL，game 节点从 DB 加载到内存。

## 目录

| 路径 | 说明 |
|------|------|
| `datas/` | 策划 Excel/CSV 源表 |
| `defines/` | Luban schema（`__root__.xml`） |
| `tools/gen.ps1` | 一键导表 |
| `tools/luban/` | Luban.ClientServer（见 README 安装） |
| `gen/cfg/` | 生成的 Go 类型（可手维护或与 Luban 同步） |
| `gen/data/` | 生成的 JSON（import 输入） |
| `cmd/import/` | JSON → MySQL |
| `pkg/schema/` | GORM 配置表模型 |
| `pkg/runtime/` | 运行时 Load / Reload / 查询 |

## 工作流

```powershell
# 1. 导表（需 .NET 6+ 与 Luban，见 tools/luban/README.md）
.\gameconfig\tools\gen.ps1

# 2. 导入 MySQL（使用 configs/mmo-cluster.json 中的 DSN）
go run ./gameconfig/cmd/import -profile configs/mmo-cluster.json

# 3. 启动 game 节点（启动时自动 Load）
go run ./cmd/game -profile configs/mmo-cluster.json -node 10001
```

## 演示道具

| id | name | max_stack |
|----|------|-----------|
| 1001 | 小型生命药水 | 99 |
| 1002 | 铜币袋 | 9999 |
| 2001 | 新手木剑 | 1 |
| 3001 | 任务信件 | 1 |

## 热更

profile 中设置 `"gameconfig": { "allow_reload": true }` 后，可调用 RPC `game.config.reload`。
