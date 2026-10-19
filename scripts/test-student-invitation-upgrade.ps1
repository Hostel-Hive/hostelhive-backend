# Isolated migration25->26->25->26 checks. Never uses the developer database.
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$container = 'hostelhive-invitation-upgrade-' + [Guid]::NewGuid().ToString('N').Substring(0,12)
$previousPassword = $env:POSTGRES_PASSWORD
$created = $false
function Run-InvitationSQL([string]$Name) {
    Get-Content -LiteralPath (Join-Path $repoRoot "tests/migration/$Name") -Raw |
        & docker exec -i $container psql -U hostelhive -d hostelhive_ci -v ON_ERROR_STOP=1
    if ($LASTEXITCODE -ne 0) { throw "Invitation migration fixture failed: $Name" }
}
function Run-InvitationMigration([string[]]$Commands) {
    # Windows PowerShell5.1 wraps native stderr; migrate logs success there.
    $ErrorActionPreference = 'Continue'
    $output = & $migrate -path './migrations' -database $migrationURL @Commands 2>&1
    $code = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    foreach ($line in $output) {
        $safe = "$line".Replace($migrationURL, '[DISPOSABLE_DATABASE]')
        if ($env:POSTGRES_PASSWORD) { $safe = $safe.Replace($env:POSTGRES_PASSWORD, '[REDACTED]') }
        Write-Host $safe
    }
    if ($code -ne 0) { throw 'Invitation migration command failed.' }
}
try {
    $env:POSTGRES_PASSWORD = [Guid]::NewGuid().ToString('N')
    & docker run --name $container -d -p 127.0.0.1::5432 -e POSTGRES_PASSWORD -e POSTGRES_USER=hostelhive -e POSTGRES_DB=hostelhive_ci postgres:16-alpine | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Disposable database startup failed.' }
    $created = $true
    $ready = $false
    for ($n=0; $n -lt 60; $n++) {
        & docker exec $container pg_isready -h 127.0.0.1 -U hostelhive -d hostelhive_ci 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $ready=$true; break }
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) { throw 'Disposable database did not become ready.' }
    $address = (& docker port $container 5432/tcp | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $address -notmatch '^127\.0\.0\.1:\d+$') { throw 'Invalid disposable port.' }
    $migrationURL = "postgres://hostelhive:$($env:POSTGRES_PASSWORD)@$address/hostelhive_ci?sslmode=disable&x-migrations-table=%22public%22.%22schema_migrations%22&x-migrations-table-quoted=true"
    $migrate = Join-Path $repoRoot 'bin/migrate.exe'
    Push-Location -LiteralPath $repoRoot
    try {
    Run-InvitationMigration @('goto','25')
    Run-InvitationSQL 'student_invitation_upgrade_before.sql'
    Run-InvitationMigration @('up')
    Run-InvitationSQL 'student_invitation_upgrade_after.sql'
    Run-InvitationSQL 'student_invitation_rollback_before.sql'
    Run-InvitationMigration @('down','1')
    Run-InvitationSQL 'student_invitation_rollback_after.sql'
    Run-InvitationMigration @('up')
    Run-InvitationSQL 'student_invitation_rollback_after.sql'
    Run-InvitationSQL 'student_invitation_upgrade_cleanup.sql'
    Write-Host 'PASS: legacy data preserved; pending accounts denied after rollback/reapply; fixtures cleaned.'
    } finally { Pop-Location }
} finally {
    if ($created) { & docker rm -f -v $container | Out-Null }
    $env:POSTGRES_PASSWORD = $previousPassword
}
