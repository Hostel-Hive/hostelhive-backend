# Creates its own disposable database; never uses the developer DATABASE_URL.
param([switch]$Race)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$container = 'hostelhive-coverage-' + [Guid]::NewGuid().ToString('N').Substring(0,12)
$previousURL = $env:DATABASE_URL
$previousTestURL = $env:TEST_DATABASE_URL
$previousPassword = $env:POSTGRES_PASSWORD
$created = $false
Push-Location -LiteralPath $repoRoot
try {
    if (-not (Test-Path bin/migrate.exe)) { throw 'Run scripts/install-migrate.ps1 first.' }
    if (-not (Get-Command python -ErrorAction SilentlyContinue)) { throw 'Python 3 is required for the evidence report.' }
    & python scripts/check-go-format.py
    if ($LASTEXITCODE -ne 0) { throw 'Go formatting check failed.' }
    & go mod verify
    if ($LASTEXITCODE -ne 0) { throw 'Module verification failed.' }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Static checks failed.' }
    & go build -o bin/hostelhive-server-check.exe ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    & python scripts/test_coverage_report.py
    if ($LASTEXITCODE -ne 0) { throw 'Coverage evidence tests failed.' }
    $env:POSTGRES_PASSWORD = [Guid]::NewGuid().ToString('N')
    & docker run --name $container -d -p 127.0.0.1::5432 -e POSTGRES_PASSWORD -e POSTGRES_USER=hostelhive -e POSTGRES_DB=hostelhive_ci postgres:16-alpine | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Disposable PostgreSQL startup failed.' }
    $created = $true
    $ready = $false
    for ($n=0; $n -lt 60; $n++) {
        & docker exec $container pg_isready -h 127.0.0.1 -U hostelhive -d hostelhive_ci 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $ready=$true; break }
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) { throw 'Disposable database did not become ready.' }
    $address = (& docker port $container 5432/tcp | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $address -notmatch '^127\.0\.0\.1:\d+$') { throw 'Cannot resolve disposable database port.' }
    $env:DATABASE_URL = "postgres://hostelhive:$($env:POSTGRES_PASSWORD)@$address/hostelhive_ci?sslmode=disable"
    & ./scripts/migrate.ps1 up
    $env:TEST_DATABASE_URL = $env:DATABASE_URL
    New-Item -ItemType Directory -Force artifacts | Out-Null
    $goArgs = @('test','-p','1','-count=1','-timeout=120s','-json','-covermode=atomic','-coverpkg=./cmd/...,./internal/...','-coverprofile=artifacts/coverage.out','./...')
    if ($Race) { $goArgs = @('test','-race') + $goArgs[1..($goArgs.Length-1)] }
    & go @goArgs | Out-File -Encoding utf8 artifacts/tests.jsonl
    if ($LASTEXITCODE -ne 0) { throw 'Backend tests failed; inspect artifacts/tests.jsonl.' }
    & python scripts/coverage-report.py --profile artifacts/coverage.out --events artifacts/tests.jsonl --output artifacts/coverage.md --require-integration
    if ($LASTEXITCODE -ne 0) { throw 'Coverage/integration evidence validation failed.' }
    & go tool cover '-html=artifacts/coverage.out' -o artifacts/coverage.html
    if ($LASTEXITCODE -ne 0) { throw 'Coverage HTML generation failed.' }
    Get-Content artifacts/coverage.md
} finally {
    $cleanupPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { if ($created) { & docker rm -f -v $container 2>$null | Out-Null } }
    finally { $ErrorActionPreference=$cleanupPreference }
    $env:DATABASE_URL=$previousURL
    $env:TEST_DATABASE_URL=$previousTestURL
    $env:POSTGRES_PASSWORD=$previousPassword
    Pop-Location
}
