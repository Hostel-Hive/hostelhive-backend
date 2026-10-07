# Interactive local verification helper. Never writes passwords or tokens to disk.
[CmdletBinding()]
param([string]$ApiKey, [string]$Email)
$ErrorActionPreference = 'Stop'
if (-not $ApiKey) { $ApiKey = Read-Host 'Firebase web API key' }
if (-not $Email) { $Email = Read-Host 'Firebase user email' }
$password = Read-Host 'Firebase user password' -AsSecureString
try {
    $body = @{
        email = $Email
        password = ([System.Net.NetworkCredential]::new('', $password)).Password
        returnSecureToken = $true
    } | ConvertTo-Json
    $result = Invoke-RestMethod -Method Post -Uri "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=$ApiKey" -ContentType 'application/json' -Body $body -ErrorAction Stop
    if (-not $result.idToken) { throw 'Firebase returned no ID token.' }
    return $result.idToken
} catch {
    $details = $_.ErrorDetails.Message
    if (-not $details -and $_.Exception.Response) {
        $reader = [IO.StreamReader]::new($_.Exception.Response.GetResponseStream())
        try { $details = $reader.ReadToEnd() } finally { $reader.Dispose() }
    }
    $code = 'unknown error; check credentials and Firebase project configuration'
    if ($details) {
        try {
            $message = ($details | ConvertFrom-Json).error.message
            if ($message -match '^[A-Z_]+$') { $code = $message }
        } catch { }
    }
    throw "Firebase sign-in failed: $code"
} finally {
    Remove-Variable body, password, result -ErrorAction SilentlyContinue
}
