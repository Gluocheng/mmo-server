# 停止 MMO 五节点（gm -> gateway -> game -> login -> master）
# 用法:
#   powershell -ExecutionPolicy Bypass -File scripts/stop.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/stop.ps1 -StopNats
param(
    [switch]$StopNats
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Stop-MMOProcess {
    param(
        [string]$Name
    )
    $procs = Get-Process -Name $Name -ErrorAction SilentlyContinue
    if (-not $procs) {
        Write-Host "[$Name] not running."
        return
    }
    foreach ($p in $procs) {
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        Write-Host "[$Name] stopped pid=$($p.Id)"
    }
}

Write-Host "=== MMO server stop ==="

# 先停 GM，再停网关，再后端，最后 master
Stop-MMOProcess "gm"
Stop-MMOProcess "gateway"
Stop-MMOProcess "game"
Stop-MMOProcess "login"
Stop-MMOProcess "master"

if ($StopNats) {
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        docker stop local-nats 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "[nats] Docker container local-nats stopped."
        } else {
            Write-Host "[nats] local-nats container not running or not found."
        }
    } else {
        Write-Warning "[nats] docker not found, skip."
    }
}

Write-Host "=== done ==="
