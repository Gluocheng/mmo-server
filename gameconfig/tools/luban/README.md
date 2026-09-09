# Luban 工具

本目录放置 **Luban.ClientServer**（classic 版，含 `Luban.ClientServer.dll`）。

## 来源

当前使用的版本来自 [luban_examples](https://github.com/focus-creative-games/luban_examples) 的 **`classic` 分支**（`Tools/Luban.ClientServer/` 目录，已编译好的工具，`net6.0` 运行时）。

> 注意：不要用 luban 主仓库最新 release（v5.0.0 起工具改名 `Luban.dll`，且需重新构建），与 `gen.ps1` 的 `Luban.ClientServer.dll` 调用约定不兼容。

## 安装

1. 安装 [.NET 6+ SDK](https://dotnet.microsoft.com/download)（`winget install Microsoft.DotNet.SDK.6`）
2. 获取 classic 分支的 `Tools/Luban.ClientServer/` 内容：

```bash
git clone --depth 1 --branch classic --filter=blob:none --sparse https://github.com/focus-creative-games/luban_examples.git
cd luban_examples && git sparse-checkout set Tools/Luban.ClientServer
cp -r Tools/Luban.ClientServer/. gameconfig/tools/luban/
```

3. 在仓库根目录运行 `.\gameconfig\tools\gen.ps1`

## 说明

- 本目录二进制已加入 `.gitignore`，不提交仓库（17M）。CI 或新环境按上述步骤重建。
- 仅 `README.md` 提交。
