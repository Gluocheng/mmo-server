# Docker 预发布栈冒烟检查
# 用法: powershell -ExecutionPolicy Bypass -File scripts/docker-smoke.ps1
param(
    [int]$WaitSec = 15,
    [int]$MaxRetries = 12,
    [int]$GMPort = 19080
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if ($env:GM_HTTP_PORT -and $PSBoundParameters.ContainsKey("GMPort") -eq $false) {
    $GMPort = [int]$env:GM_HTTP_PORT
}

function Test-PortOpen([int]$Port) {
    $r = Test-NetConnection -ComputerName 127.0.0.1 -Port $Port -WarningAction SilentlyContinue
    return [bool]$r.TcpTestSucceeded
}

function Wait-Port([int]$Port, [string]$Name) {
    for ($i = 1; $i -le $MaxRetries; $i++) {
        if (Test-PortOpen $Port) {
            Write-Host "[smoke] $Name port $Port open"
            return
        }
        Write-Host "[smoke] waiting $Name port $Port ($i/$MaxRetries)..."
        Start-Sleep -Seconds $WaitSec
    }
    throw "[smoke] $Name port $Port not open"
}

Write-Host "=== docker smoke ==="

& docker compose ps
if ($LASTEXITCODE -ne 0) {
    throw "docker compose ps failed"
}

$required = @("nats", "master", "login", "game", "gateway", "gm")
foreach ($name in $required) {
    $id = (& docker compose ps -q $name 2>$null)
    if (-not $id) {
        throw "[smoke] service $name not found"
    }
    $running = (& docker inspect -f "{{.State.Running}}" $id).Trim()
    if ($running -ne "true") {
        throw "[smoke] service $name not running"
    }
    Write-Host "[smoke] $name running"
}

Wait-Port -Port 10100 -Name "gateway"
Wait-Port -Port $GMPort -Name "gm"

$gmToken = $env:GM_HTTP_TOKEN
if (-not $gmToken) {
    $gmToken = "dev-gm-token"
}

$health = Invoke-RestMethod -Uri "http://127.0.0.1:$GMPort/gm/health" -Method Get
if ($null -eq $health -or $health.code -ne 0 -or $health.natsConnected -ne $true) {
    throw "[smoke] gm health failed: $($health | ConvertTo-Json -Compress)"
}
Write-Host "[smoke] gm health ok: target=$($health.targetPath)"

$body = '{"tableName":""}'
$headers = @{
    "Content-Type" = "application/json"
    "X-GM-Token"   = $gmToken
}
$resp = Invoke-RestMethod `
    -Uri "http://127.0.0.1:$GMPort/gm/config/reload" `
    -Method Post `
    -Headers $headers `
    -Body $body

if ($null -eq $resp -or $resp.code -ne 0) {
    throw "[smoke] gm reload failed: $($resp | ConvertTo-Json -Compress)"
}

Write-Host "[smoke] gm reload ok: version=$($resp.version) tables=$($resp.tables)"
Write-Host "=== smoke passed ==="
