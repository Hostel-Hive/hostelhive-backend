param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('up', 'down', 'version')]
    [string]$Command
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$executable = Join-Path $repoRoot 'bin/migrate.exe'
if (-not (Test-Path -LiteralPath $executable)) {
    throw 'Run scripts/install-migrate.ps1 first.'
}
if ([string]::IsNullOrWhiteSpace($env:DATABASE_URL)) {
    throw 'Set DATABASE_URL in this terminal before running migrations.'
}
$expected = (Get-Content -LiteralPath (Join-Path $repoRoot '.migrate-version') -Raw).Trim()
# Windows PowerShell 5.1 wraps redirected native stderr in ErrorRecord objects.
# migrate writes its successful version output to stderr; use the exit code.
$ErrorActionPreference = 'Continue'
$versionOutput = & $executable -version 2>&1
$versionCode = $LASTEXITCODE
$ErrorActionPreference = 'Stop'
$actual = (($versionOutput | ForEach-Object { "$_" }) -join [Environment]::NewLine).Trim()
if ($versionCode -ne 0 -or $actual -ne $expected) {
    throw 'Migration tool version mismatch. Run scripts/install-migrate.ps1 again.'
}
# Pin metadata to public even when creating a schema matching the database user
# changes PostgreSQL's effective default search path.
try { $builder = [UriBuilder]$env:DATABASE_URL } catch { throw 'DATABASE_URL must be a valid PostgreSQL URL.' }
$pairs = @($builder.Query.TrimStart('?').Split('&', [StringSplitOptions]::RemoveEmptyEntries) | Where-Object {
    $key = [Uri]::UnescapeDataString($_.Split('=')[0])
    $key -notin @('x-migrations-table', 'x-migrations-table-quoted')
})
$pairs += 'x-migrations-table=%22public%22.%22schema_migrations%22'
$pairs += 'x-migrations-table-quoted=true'
$builder.Query = $pairs -join '&'
$migrationURL = $builder.Uri.AbsoluteUri
$arguments = @('-path', './migrations', '-database', $migrationURL, $Command)
if ($Command -eq 'down') { $arguments += '1' }
# Capture output to redact connection details before displaying errors.
$ErrorActionPreference = 'Continue'
Push-Location -LiteralPath $repoRoot
try {
    $output = & $executable @arguments 2>&1
    $code = $LASTEXITCODE
} finally {
    Pop-Location
}
$ErrorActionPreference = 'Stop'
$secret = ''
try {
    $parsed = [Uri]$env:DATABASE_URL
    if ($parsed.UserInfo.Contains(':')) {
        $secret = [Uri]::UnescapeDataString($parsed.UserInfo.Substring($parsed.UserInfo.IndexOf(':') + 1))
    }
} catch { }
foreach ($line in $output) {
    $safe = "$line".Replace($migrationURL, '[DATABASE_URL]').Replace($env:DATABASE_URL, '[DATABASE_URL]')
    if ($secret -ne '') { $safe = $safe.Replace($secret, '[REDACTED]') }
    Write-Output $safe
}
if ($code -ne 0) { throw "Migration command failed (exit $code)." }
