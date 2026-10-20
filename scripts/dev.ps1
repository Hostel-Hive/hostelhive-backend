[CmdletBinding()]
param(
    [string]$EnvFile = '.env.local',
    [string]$DatabaseContainer = 'hostelhive-postgres',
    [switch]$SkipDatabase,
    [switch]$Migrate,
    [switch]$Check
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
. (Join-Path $PSScriptRoot 'local-environment.ps1')
if (-not [IO.Path]::IsPathRooted($EnvFile)) { $EnvFile = Join-Path $repoRoot $EnvFile }
$values = Read-LocalEnvironment -Path $EnvFile
# Only application configuration is accepted; never replace PATH or shell settings.
$allowed = @{}
foreach ($line in Get-Content -LiteralPath (Join-Path $repoRoot '.env.example')) {
    if ($line -match '^\s*#?\s*([A-Z][A-Z0-9_]*)=') { $allowed[$Matches[1]] = $true }
}
foreach ($name in $values.Keys) {
    if (-not $allowed.ContainsKey($name)) { throw "Unsupported application setting: $name." }
}
$previous = @{}
foreach ($name in $values.Keys) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
Push-Location -LiteralPath $repoRoot
try {
    foreach ($name in $values.Keys) { [Environment]::SetEnvironmentVariable($name, $values[$name], 'Process') }
    if ($env:APP_ENV -and $env:APP_ENV -ne 'development') { throw 'dev.ps1 is only for APP_ENV=development.' }
    foreach ($name in @('DATABASE_URL', 'FIREBASE_PROJECT_ID', 'GOOGLE_APPLICATION_CREDENTIALS')) {
        if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name, 'Process'))) {
            throw "Set $name in the local configuration file."
        }
    }
    $database = $null
    if (-not [Uri]::TryCreate($env:DATABASE_URL, [UriKind]::Absolute, [ref]$database) -or
        $database.Scheme -notin @('postgres', 'postgresql') -or -not $database.Host -or
        -not $database.UserInfo -or -not $database.UserInfo.Split(':')[0] -or $database.AbsolutePath.Trim('/') -eq '') {
        throw 'DATABASE_URL must be a PostgreSQL URL with host, user and database.'
    }
    if ($env:DATABASE_URL.Contains('REPLACE_ME')) { throw 'Replace the database password placeholder in your local configuration.' }
    if ($env:FIREBASE_PROJECT_ID -match '\s|/' -or $env:FIREBASE_PROJECT_ID -eq 'your-firebase-project-id') {
        throw 'Set a valid FIREBASE_PROJECT_ID in your local configuration.'
    }
    if (-not (Test-Path -LiteralPath $env:GOOGLE_APPLICATION_CREDENTIALS -PathType Leaf)) {
        throw 'Firebase service-account file not found. Check GOOGLE_APPLICATION_CREDENTIALS.'
    }
    if (-not $SkipDatabase -and ($database.Host -notin @('127.0.0.1', 'localhost') -or $database.Port -ne 15433)) {
        throw 'Automatic Docker startup expects localhost:15433. Use -SkipDatabase for a different or hosted database.'
    }
    if ($Check) {
        Write-Host 'Local configuration checks passed. No services started; connectivity and migrations were not checked.'
        return
    }
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go is not installed or is missing from PATH.' }
    if (-not $SkipDatabase) {
        if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Docker CLI not found. Install and start Docker Desktop.' }
        docker start $DatabaseContainer | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Could not start the existing PostgreSQL container. Check Docker Desktop and the container name.' }
        $ready = $false
        $dbUser = [Uri]::UnescapeDataString($database.UserInfo.Split(':')[0])
        $dbName = [Uri]::UnescapeDataString($database.AbsolutePath.TrimStart('/'))
        for ($attempt = 0; $attempt -lt 20; $attempt++) {
            docker exec $DatabaseContainer pg_isready -U $dbUser -d $dbName | Out-Null
            if ($LASTEXITCODE -eq 0) { $ready = $true; break }
            Start-Sleep -Milliseconds 500
        }
        if (-not $ready) { throw 'PostgreSQL did not become ready. Check the container logs.' }
    }
    if ($Migrate) { & (Join-Path $PSScriptRoot 'migrate.ps1') up }
    Write-Host 'Starting HostelHive. Press Ctrl+C to stop. Swagger is available at /docs/ on the configured backend address.'
    go run ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Backend exited with an error. Check the server output above.' }
}
finally {
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    Pop-Location
}
