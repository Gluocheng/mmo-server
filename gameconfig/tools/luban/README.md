# Luban 工具

本目录应放置 **Luban.ClientServer**（来自 [luban_examples](https://github.com/focus-creative-games/luban_examples) 的 `Tools/Luban.ClientServer/`）。

## 安装

1. 安装 [.NET 6+ SDK](https://dotnet.microsoft.com/download)
2. 从 luban_examples 拷贝 `Tools/Luban.ClientServer/` 到本目录
3. 在仓库根目录运行 `.\gameconfig\tools\gen.ps1`

## 无 Luban 时

仓库已提交 `gen/cfg/` 与 `gen/data/` 生成物，可直接 `go run ./gameconfig/cmd/import` 与编译服务端。

修改 Excel 后需重新导表并提交 `gen/` 变更，或由 CI 执行 `gen.ps1`。
