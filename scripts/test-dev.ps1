# Offline contract tests: no real Docker, Go server, database or Firebase calls.
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$launcher = Join-Path $PSScriptRoot 'dev.ps1'
. (Join-Path $PSScriptRoot 'local-environment.ps1')
function Assert-Dev($condition, [string]$message) {
    if (-not $condition) { throw $message }
}
function Expect-DevFailure([scriptblock]$action, [string]$expected) {
    $failure = $null
    try { & $action | Out-Null } catch { $failure = $_.Exception.Message }
    Assert-Dev ($failure -and $failure.Contains($expected)) "Expected failure: $expected"
    Assert-Dev (-not $failure.Contains('private-fixture-password')) 'A configuration secret was printed.'
}
foreach ($file in @('dev.ps1', 'local-environment.ps1', 'test-dev.ps1')) {
    $tokens = $null; $parseErrors = $null
    $null = [Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $file), [ref]$tokens, [ref]$parseErrors)
    Assert-Dev ($parseErrors.Count -eq 0) "PowerShell syntax failure in $file"
}
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('hostelhive-dev-' + [Guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $temporary
$credential = Join-Path $temporary 'fake service-account.json'
[IO.File]::WriteAllText($credential, '{}')
$fixture = Join-Path $temporary 'test.env'
$originalDirectory = (Get-Location).Path
$keys = @('APP_ENV', 'DATABASE_URL', 'FIREBASE_PROJECT_ID', 'GOOGLE_APPLICATION_CREDENTIALS', 'HTTP_ADDR')
$original = @{}
foreach ($key in $keys) { $original[$key] = [Environment]::GetEnvironmentVariable($key, 'Process') }
$global:devCalls = [Collections.Generic.List[string]]::new()
$global:devDockerExit = 0
$global:devGoExit = 0
$global:devSeen = $null
function global:docker {
    $global:devCalls.Add('docker ' + ($args -join ' '))
    $global:LASTEXITCODE = $global:devDockerExit
}
function global:go {
    $global:devCalls.Add('go ' + ($args -join ' '))
    $global:devSeen = @{
        Database = $env:DATABASE_URL
        Credentials = $env:GOOGLE_APPLICATION_CREDENTIALS
        Directory = (Get-Location).Path
    }
    $global:LASTEXITCODE = $global:devGoExit
}
function Write-DevFixture([string]$Database = 'postgres://test:private-fixture-password@127.0.0.1:15433/test') {
    [IO.File]::WriteAllLines($fixture, @(
        'APP_ENV=development',
        "DATABASE_URL=$Database",
        'FIREBASE_PROJECT_ID=hostelhive-offline',
        ('GOOGLE_APPLICATION_CREDENTIALS="' + $credential + '"'),
        'HTTP_ADDR=127.0.0.1:18080'
    ))
}
try {
    # Command-looking content is data, not executable PowerShell.
    [IO.File]::WriteAllLines($fixture, @('# comment', '', 'FIRST="literal # value=1"', "SECOND='literal `$([int]::Parse(`"bad`"))'"))
    $parsed = Read-LocalEnvironment -Path $fixture
    Assert-Dev ($parsed.FIRST -eq 'literal # value=1' -and $parsed.SECOND.StartsWith('literal $(')) 'Values were expanded or truncated.'
    foreach ($bad in @('BAD LINE private-fixture-password', 'KEY="private-fixture-password', "KEY=1`nKEY=private-fixture-password")) {
        [IO.File]::WriteAllText($fixture, $bad)
        Expect-DevFailure { Read-LocalEnvironment -Path $fixture } 'configuration'
    }
    Expect-DevFailure { Read-LocalEnvironment -Path (Join-Path $temporary 'missing.env') } 'not found'
    Write-DevFixture
    [Environment]::SetEnvironmentVariable('DATABASE_URL', 'previous-process-value', 'Process')
    Set-Location -LiteralPath $temporary
    & $launcher -EnvFile $fixture -Check
    Assert-Dev ($global:devCalls.Count -eq 0) 'Check mode started a service.'
    Assert-Dev ($env:DATABASE_URL -eq 'previous-process-value') 'Check mode failed to restore the environment.'
    & $launcher -EnvFile $fixture
    Assert-Dev (($global:devCalls -join '|') -eq 'docker start hostelhive-postgres|docker exec hostelhive-postgres pg_isready -U test -d test|go run ./cmd/server') 'Local startup orchestration is incorrect.'
    Assert-Dev ($global:devSeen.Database.Contains('private-fixture-password')) 'File settings did not override the process environment.'
    Assert-Dev ($global:devSeen.Credentials -eq $credential -and $global:devSeen.Directory -eq $repoRoot) 'Credential path or server working directory was incorrect.'
    Assert-Dev ($env:DATABASE_URL -eq 'previous-process-value' -and (Get-Location).Path -eq $temporary) 'Successful startup failed to restore environment/directory.'
    $global:devCalls.Clear()
    Write-DevFixture -Database 'postgres://test:private-fixture-password@remote.example.invalid:5432/test?sslmode=require'
    Expect-DevFailure { & $launcher -EnvFile $fixture } '-SkipDatabase'
    Assert-Dev ($global:devCalls.Count -eq 0) 'Hosted database triggered Docker startup.'
    & $launcher -EnvFile $fixture -SkipDatabase
    Assert-Dev (($global:devCalls -join '|') -eq 'go run ./cmd/server') 'SkipDatabase still invoked Docker.'
    Write-DevFixture
    $global:devCalls.Clear()
    $global:devDockerExit = 1
    Expect-DevFailure { & $launcher -EnvFile $fixture } 'Could not start'
    Assert-Dev ($global:devCalls.Count -eq 1) 'Server started after Docker failed.'
    $global:devDockerExit = 0
    $global:devGoExit = 1
    Expect-DevFailure { & $launcher -EnvFile $fixture -SkipDatabase } 'Backend exited'
    Assert-Dev ($env:DATABASE_URL -eq 'previous-process-value' -and (Get-Location).Path -eq $temporary) 'Failure did not restore environment/directory.'
    $global:devGoExit = 0
    Write-DevFixture
    [IO.File]::AppendAllText($fixture, "`nPATH=private-fixture-password")
    Expect-DevFailure { & $launcher -EnvFile $fixture -Check } 'Unsupported application setting'
    Write-DevFixture -Database 'private-fixture-password'
    Expect-DevFailure { & $launcher -EnvFile $fixture -Check } 'PostgreSQL URL'
    Write-DevFixture
    Remove-Item -LiteralPath $credential
    Expect-DevFailure { & $launcher -EnvFile $fixture -Check } 'service-account file not found'
    Write-Host 'PASS: literal dotenv parsing, scoped configuration, local/hosted startup and safe failure handling.'
}
finally {
    Set-Location -LiteralPath $originalDirectory
    foreach ($key in $original.Keys) { [Environment]::SetEnvironmentVariable($key, $original[$key], 'Process') }
    Remove-Item -LiteralPath $fixture -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $credential -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $temporary -ErrorAction SilentlyContinue
    Remove-Item Function:\docker, Function:\go -ErrorAction SilentlyContinue
}
# A simulated native failure must not become the caller's test exit status.
$global:LASTEXITCODE = 0
