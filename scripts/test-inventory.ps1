# Isolated integration tests; never migrate or modify the developer database.
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$container = 'hostelhive-inventory-check-' + [Guid]::NewGuid().ToString('N').Substring(0,12)
$oldDatabaseURL = $env:DATABASE_URL
$oldTestURL = $env:TEST_DATABASE_URL
$oldPassword = $env:POSTGRES_PASSWORD
try {
    $env:POSTGRES_PASSWORD = [Guid]::NewGuid().ToString('N')
    & docker run --name $container -d -p 127.0.0.1::5432 -e POSTGRES_PASSWORD -e POSTGRES_USER=hostelhive -e POSTGRES_DB=hostelhive postgres:16-alpine | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Temporary PostgreSQL startup failed.' }
    $ready = $false
    for ($n=0; $n -lt 30; $n++) {
        & docker exec $container pg_isready -h 127.0.0.1 -U hostelhive -d hostelhive 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $ready=$true; break }
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) { throw 'Temporary PostgreSQL did not become ready.' }
    $address = (& docker port $container 5432/tcp | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $address -notmatch '^127\.0\.0\.1:\d+$') { throw 'Cannot resolve temporary database port.' }
    $env:DATABASE_URL = "postgres://hostelhive:$($env:POSTGRES_PASSWORD)@$address/hostelhive?sslmode=disable"
    & (Join-Path $PSScriptRoot 'migrate.ps1') up
    $env:TEST_DATABASE_URL = $env:DATABASE_URL
    Push-Location -LiteralPath $repoRoot
    try {
        & go test -count=1 -timeout=45s -v ./tests/integration/inventory
        if ($LASTEXITCODE -ne 0) { throw 'Inventory integration tests failed.' }
    } finally { Pop-Location }
} finally {
    # Cleanup must not replace the original startup/test error (PowerShell 5.1).
    $cleanupPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { & docker rm -f -v $container 2>$null | Out-Null }
    finally { $ErrorActionPreference = $cleanupPreference }
    $env:DATABASE_URL = $oldDatabaseURL
    $env:TEST_DATABASE_URL = $oldTestURL
    $env:POSTGRES_PASSWORD = $oldPassword
}
