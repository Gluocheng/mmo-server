# 一键导表：Luban 生成 Go + JSON；无 Luban 时跳过并提示使用已提交的 gen/ 产物。
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root

$lubanDll = Join-Path $PSScriptRoot "luban/Luban.ClientServer.dll"
if (-not (Test-Path $lubanDll)) {
    Write-Host "Luban not found at $lubanDll"
    Write-Host "See gameconfig/tools/luban/README.md — using committed gen/cfg and gen/data."
    exit 0
}

$defineFile = "gameconfig/defines/__root__.xml"
$dataDir = "gameconfig/datas"
$codeDir = "gameconfig/gen/cfg"
$outData = "gameconfig/gen/data"

dotnet $lubanDll -j cfg -- `
  -d $defineFile `
  --input_data_dir $dataDir `
  --output_code_dir $codeDir `
  --output_data_dir $outData `
  --gen_types code_go_json,data_json `
  -s server `
  --go:bright_module_name github.com/example/mmo-server/gameconfig/gen/cfg

Write-Host "gen done: $codeDir , $outData"
