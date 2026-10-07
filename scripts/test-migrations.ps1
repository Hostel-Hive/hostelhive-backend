# Runs only against a new disposable PostgreSQL container, never DATABASE_URL.
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$container = 'hostelhive-migrations-check-' + [Guid]::NewGuid().ToString('N').Substring(0, 12)
$previousURL = $env:DATABASE_URL
$previousPassword = $env:POSTGRES_PASSWORD
$created = $false
function Assert-SQL([string]$SQL, [string]$Expected) {
    $result = & docker exec $container psql -U hostelhive -d hostelhive -At -v ON_ERROR_STOP=1 -c $SQL
    if ($LASTEXITCODE -ne 0 -or ($result | Out-String).Trim() -ne $Expected) {
        throw "Migration verification failed: expected $Expected."
    }
}
try {
    if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'bin/migrate.exe'))) {
        throw 'Run scripts/install-migrate.ps1 first.'
    }
    $env:POSTGRES_PASSWORD = [Guid]::NewGuid().ToString('N')
    & docker run --name $container -d -p 127.0.0.1::5432 -e POSTGRES_PASSWORD -e POSTGRES_USER=hostelhive -e POSTGRES_DB=hostelhive postgres:16-alpine | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not start temporary PostgreSQL container.' }
    $created = $true
    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        & docker exec $container pg_isready -U hostelhive -d hostelhive 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $ready = $true; break }
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) { throw 'Temporary PostgreSQL did not become ready.' }
    $address = (& docker port $container 5432/tcp | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $address -notmatch '^127\.0\.0\.1:\d+$') {
        throw 'Could not resolve temporary database port.'
    }
    $env:DATABASE_URL = "postgres://hostelhive:$($env:POSTGRES_PASSWORD)@$address/hostelhive?sslmode=disable"
    $schemaSQL = "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'hostelhive')::text;"
    Assert-SQL $schemaSQL 'false'
    Push-Location -LiteralPath ([IO.Path]::GetTempPath())
    try { & (Join-Path $PSScriptRoot 'migrate.ps1') up } finally { Pop-Location }
    Assert-SQL $schemaSQL 'true'
    Assert-SQL "SELECT version::text || ': ' || dirty::text FROM public.schema_migrations;" '1: false'
    & (Join-Path $PSScriptRoot 'migrate.ps1') version
    # Applying the same migration again must leave schema and version unchanged.
    & (Join-Path $PSScriptRoot 'migrate.ps1') up
    Assert-SQL "SELECT version::text || ': ' || dirty::text FROM public.schema_migrations;" '1: false'
    & (Join-Path $PSScriptRoot 'migrate.ps1') down
    Assert-SQL $schemaSQL 'false'
    Assert-SQL 'SELECT count(*)::text FROM public.schema_migrations;' '0'
    Push-Location -LiteralPath ([IO.Path]::GetTempPath())
    try { & (Join-Path $PSScriptRoot 'migrate.ps1') up } finally { Pop-Location }
    Assert-SQL $schemaSQL 'true'
    Assert-SQL "SELECT version::text || ': ' || dirty::text FROM public.schema_migrations;" '1: false'
    Write-Output 'PASS: apply, version, repeated apply, rollback and reapply.'
} finally {
    if ($created) {
        & docker rm -f -v $container | Out-Null
        if ($LASTEXITCODE -ne 0) { Write-Warning "Temporary container cleanup failed: $container" }
    } else {
        # A failed docker run may have created an unstarted container.
        & docker rm -f -v $container 2>$null | Out-Null
    }
    $env:DATABASE_URL = $previousURL
    $env:POSTGRES_PASSWORD = $previousPassword
}
