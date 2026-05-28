# 启动 MMO 四节点（master -> login -> game -> gateway）
# 用法（在仓库根目录）:
#   powershell -ExecutionPolicy Bypass -File scripts/start.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/start.ps1 -Build
#   powershell -ExecutionPolicy Bypass -File scripts/start.ps1 -SkipDockerNats
param(
    [string]$Profile = "configs/mmo-cluster.json",
    [switch]$Build,
    [switch]$SkipDockerNats
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Test-PortOpen([int]$Port) {
    $r = Test-NetConnection -ComputerName 127.0.0.1 -Port $Port -WarningAction SilentlyContinue
    return [bool]$r.TcpTestSucceeded
}

function Ensure-Dir([string]$Path) {
    if (-not (Test-Path $Path)) {
        New-Item -ItemType Directory -Path $Path | Out-Null
    }
}

function Ensure-Binaries {
    $targets = @(
        @{ Name = "master";  Path = "cmd/master" },
        @{ Name = "login";   Path = "cmd/login" },
        @{ Name = "game";    Path = "cmd/game" },
        @{ Name = "gateway"; Path = "cmd/gateway" }
    )
    $needBuild = $Build
    foreach ($t in $targets) {
        $exe = Join-Path $root ("bin/{0}.exe" -f $t.Name)
        if (-not (Test-Path $exe)) {
            $needBuild = $true
            break
        }
    }
    if (-not $needBuild) {
        return
    }
    Write-Host "[build] compiling four nodes..."
    Ensure-Dir (Join-Path $root "bin")
    foreach ($t in $targets) {
        $out = Join-Path $root ("bin/{0}.exe" -f $t.Name)
        & go build -o $out ("./{0}" -f $t.Path)
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed: $($t.Path)"
        }
    }
    Write-Host "[build] done."
}

function Ensure-Nats {
    if (Test-PortOpen 4222) {
        Write-Host "[nats] 4222 already open, skip."
        return
    }
    if ($SkipDockerNats) {
        throw "NATS (4222) is not reachable. Start nats-server or run without -SkipDockerNats."
    }
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "NATS (4222) is not reachable and docker is not available."
    }
    Write-Host "[nats] starting Docker container local-nats..."
    docker start local-nats 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) {
        docker run -d --name local-nats -p 4222:4222 nats:2.10-alpine | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "failed to start NATS container local-nats"
        }
    }
    Start-Sleep -Seconds 2
    if (-not (Test-PortOpen 4222)) {
        throw "NATS still not listening on 4222"
    }
    Write-Host "[nats] ready."
}

function Start-MMONode {
    param(
        [string]$ProcessName,
        [string]$NodeID,
        [int]$WaitSec = 2
    )
    if (Get-Process -Name $ProcessName -ErrorAction SilentlyContinue) {
        Write-Host "[$ProcessName] already running, skip."
        return
    }
    $exe = Join-Path $root "bin/$ProcessName.exe"
    if (-not (Test-Path $exe)) {
        throw "binary not found: $exe (run with -Build)"
    }
    $args = @("-path=$Profile", "-node=$NodeID")
    $out = Join-Path $root "logs/$ProcessName.out"
    $err = Join-Path $root "logs/$ProcessName.err"
    Start-Process `
        -FilePath $exe `
        -ArgumentList $args `
        -WorkingDirectory $root `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err `
        -WindowStyle Hidden | Out-Null
    Start-Sleep -Seconds $WaitSec
    if (-not (Get-Process -Name $ProcessName -ErrorAction SilentlyContinue)) {
        throw "[$ProcessName] failed to start, see logs/$ProcessName.err"
    }
    Write-Host "[$ProcessName] started (node=$NodeID)."
}

Write-Host "=== MMO server start ==="
Write-Host "root=$root"

Ensure-Dir (Join-Path $root "logs")
Ensure-Binaries

if (-not (Test-PortOpen 3306)) {
    Write-Warning "[mysql] 3306 not open — login may fail if MySQL is down."
}
if (-not (Test-PortOpen 6379)) {
    Write-Warning "[redis] 6379 not open — token/session may fail if Redis is down."
}

Ensure-Nats

Start-MMONode -ProcessName "master"  -NodeID "master-1" -WaitSec 2
Start-MMONode -ProcessName "login"   -NodeID "login-1"  -WaitSec 2
Start-MMONode -ProcessName "game"    -NodeID "10001"    -WaitSec 2
Start-MMONode -ProcessName "gateway" -NodeID "gate-1"   -WaitSec 3

if (-not (Test-PortOpen 10100)) {
    Write-Warning "[gateway] 10100 not open yet — check logs/gateway.err"
} else {
    Write-Host "[gateway] ws://127.0.0.1:10100"
}

Write-Host "=== all nodes started ==="
Write-Host "logs: $root/logs"
Write-Host "stop: powershell -ExecutionPolicy Bypass -File scripts/stop.ps1"
