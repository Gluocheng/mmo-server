# 本地 Docker CI/CD 流水线
# 用法（仓库根目录）:
#   powershell -ExecutionPolicy Bypass -File scripts/cicd.ps1 -Stage test
#   powershell -ExecutionPolicy Bypass -File scripts/cicd.ps1 -Stage release
#   powershell -ExecutionPolicy Bypass -File scripts/cicd.ps1 -Stage rollback
#   powershell -ExecutionPolicy Bypass -File scripts/cicd.ps1 -Stage down
param(
    [ValidateSet("test", "build", "package", "deploy", "smoke", "release", "rollback", "down")]
    [string]$Stage = "release",
    [string]$Tag = "",
    [int]$GMPort = 19080,
    [switch]$SkipTest
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$releaseDir = Join-Path $root ".release"
$currentFile = Join-Path $releaseDir "current"
$previousFile = Join-Path $releaseDir "previous"
$imageName = "mmo-server"

function Ensure-Dir([string]$Path) {
    if (-not (Test-Path $Path)) {
        New-Item -ItemType Directory -Path $Path | Out-Null
    }
}

function Get-ReleaseTag {
    if ($Tag -ne "") {
        return $Tag
    }
    return (Get-Date -Format "yyyyMMdd-HHmmss")
}

function Invoke-GoTest {
    Write-Host "[cicd] go test ./..."
    & go test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed"
    }
}

function Invoke-GmConsoleBuild {
    $dir = Join-Path $root "web/gm-console"
    if (-not (Test-Path (Join-Path $dir "package.json"))) {
        return
    }
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
        Write-Warning "[cicd] npm not found; skip gm-console. Docker package stage still builds UI."
        return
    }
    Write-Host "[cicd] npm run build (web/gm-console)..."
    Push-Location $dir
    try {
        if (-not (Test-Path "node_modules")) {
            & npm install
            if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
        }
        & npm run build
        if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }
    } finally {
        Pop-Location
    }
}

function Invoke-GoBuild {
    Write-Host "[cicd] go build local binaries..."
    Invoke-GmConsoleBuild
    Ensure-Dir (Join-Path $root "bin")
    $targets = @(
        @{ Name = "master"; Path = "cmd/master" },
        @{ Name = "login"; Path = "cmd/login" },
        @{ Name = "game"; Path = "cmd/game" },
        @{ Name = "gateway"; Path = "cmd/gateway" },
        @{ Name = "gm"; Path = "cmd/gm" },
        @{ Name = "import-config"; Path = "gameconfig/cmd/import" }
    )
    foreach ($t in $targets) {
        $out = Join-Path $root ("bin/{0}.exe" -f $t.Name)
        & go build -o $out ("./{0}" -f $t.Path)
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed: $($t.Path)"
        }
    }
}

function Invoke-Package([string]$releaseTag) {
    $fullTag = "{0}:{1}" -f $imageName, $releaseTag
    Write-Host "[cicd] docker build -t $fullTag . (includes Node UI stage)"
    & docker build -t $fullTag .
    if ($LASTEXITCODE -ne 0) {
        throw "docker build failed"
    }
    # 同时打 local 别名，便于 compose 默认引用
    if ($releaseTag -ne "local") {
        & docker tag $fullTag "${imageName}:local"
    }
    return $releaseTag
}

function Save-ReleaseTag([string]$releaseTag) {
    Ensure-Dir $releaseDir
    if (Test-Path $currentFile) {
        $prev = Get-Content $currentFile -Raw
        if ($prev) {
            Set-Content -Path $previousFile -Value $prev.Trim() -NoNewline
        }
    }
    Set-Content -Path $currentFile -Value $releaseTag -NoNewline
    Write-Host "[cicd] release tag=$releaseTag (previous in .release/previous)"
}

function Invoke-Deploy([string]$releaseTag) {
    $env:MMO_IMAGE_TAG = $releaseTag
    $env:GM_HTTP_PORT = "$GMPort"
    Write-Host "[cicd] docker compose up -d (MMO_IMAGE_TAG=$releaseTag, GM_HTTP_PORT=$GMPort)"
    & docker compose up -d
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose up failed"
    }
}

function Invoke-Smoke {
    & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "docker-smoke.ps1") -GMPort $GMPort
    if ($LASTEXITCODE -ne 0) {
        throw "smoke test failed"
    }
}

function Invoke-Rollback {
    if (-not (Test-Path $previousFile)) {
        throw "no previous release in .release/previous"
    }
    $prevTag = (Get-Content $previousFile -Raw).Trim()
    if ($prevTag -eq "") {
        throw "previous release tag is empty"
    }
    Write-Host "[cicd] rollback to tag=$prevTag"
    $current = ""
    if (Test-Path $currentFile) {
        $current = (Get-Content $currentFile -Raw).Trim()
    }
    Set-Content -Path $currentFile -Value $prevTag -NoNewline
    if ($current -ne "") {
        Set-Content -Path $previousFile -Value $current -NoNewline
    }
    Invoke-Deploy $prevTag
    Invoke-Smoke
}

function Invoke-Down {
    Write-Host "[cicd] docker compose down"
    & docker compose down
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose down failed"
    }
}

Write-Host "=== MMO local Docker CI/CD ($Stage) ==="

switch ($Stage) {
    "test" {
        Invoke-GoTest
    }
    "build" {
        Invoke-GoBuild
    }
    "package" {
        $t = Get-ReleaseTag
        Invoke-Package $t | Out-Null
    }
    "deploy" {
        $t = if ($Tag -ne "") { $Tag } elseif (Test-Path $currentFile) { (Get-Content $currentFile -Raw).Trim() } else { "local" }
        if ($t -eq "") { $t = "local" }
        Save-ReleaseTag $t
        Invoke-Deploy $t
    }
    "smoke" {
        Invoke-Smoke
    }
    "release" {
        if (-not $SkipTest) {
            Invoke-GoTest
        }
        $t = Get-ReleaseTag
        Invoke-Package $t | Out-Null
        Save-ReleaseTag $t
        Invoke-Deploy $t
        Invoke-Smoke
    }
    "rollback" {
        Invoke-Rollback
    }
    "down" {
        Invoke-Down
    }
}

Write-Host "=== cicd $Stage done ==="
