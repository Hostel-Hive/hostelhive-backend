$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$version = (Get-Content -LiteralPath (Join-Path $repoRoot '.migrate-version') -Raw).Trim()
$previousBin = $env:GOBIN
try {
    $env:GOBIN = Join-Path $repoRoot 'bin'
    New-Item -ItemType Directory -Force $env:GOBIN | Out-Null
    $ErrorActionPreference = 'Continue'
    $installOutput = & go install -tags postgres -ldflags "-X main.Version=$version" "github.com/golang-migrate/migrate/v4/cmd/migrate@$version" 2>&1
    $installCode = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    $installOutput | ForEach-Object { Write-Output "$_" }
    if ($installCode -ne 0) { throw 'Migration tool installation failed.' }
    $ErrorActionPreference = 'Continue'
    $versionOutput = & (Join-Path $env:GOBIN 'migrate.exe') -version 2>&1
    $versionCode = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    $actual = (($versionOutput | ForEach-Object { "$_" }) -join [Environment]::NewLine).Trim()
    if ($versionCode -ne 0 -or $actual -ne $version) { throw 'Migration tool version check failed.' }
    Write-Output $actual
} finally {
    $env:GOBIN = $previousBin
}
