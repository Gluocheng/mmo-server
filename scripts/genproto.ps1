# 在仓库根目录生成 internal/protocolpb/gen（需 Docker）
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
docker run --rm -v "${root}:/work" -w /work namely/protoc-all:1.51_1 `
  -d internal/protocolpb/proto -l go -o internal/protocolpb/gen --go-source-relative
