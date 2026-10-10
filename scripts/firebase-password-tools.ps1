# Dot-source this file. Loading it performs no network requests or account changes.
# Live operations require the caller to invoke a function explicitly.
# Assign session results to variables; never print or persist their token fields.

function Invoke-FirebaseFlowRequest {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][string]$ApiKey,
        [Parameter(Mandatory=$true)][ValidateSet('signInWithPassword','lookup','update','sendOobCode','refresh')][string]$Action,
        [Parameter(Mandatory=$true)][hashtable]$Payload
    )
    $key = $ApiKey.Trim()
    if ($key -notmatch '^AIza[A-Za-z0-9_-]{20,}$') {
        throw 'Use the Firebase web API key from project configuration, not a Firebase UID.'
    }
    $escapedKey = [Uri]::EscapeDataString($key)
    $requestBody = $null
    try {
        if ($Action -eq 'refresh') {
            $uri = "https://securetoken.googleapis.com/v1/token?key=$escapedKey"
            $requestBody = $Payload
            $contentType = 'application/x-www-form-urlencoded'
        } else {
            $uri = "https://identitytoolkit.googleapis.com/v1/accounts:${Action}?key=$escapedKey"
            $requestBody = $Payload | ConvertTo-Json -Compress -Depth 5
            $contentType = 'application/json'
        }
        Invoke-RestMethod -Method Post -Uri $uri -ContentType $contentType -Body $requestBody -TimeoutSec 15 -MaximumRedirection 0 -ErrorAction Stop
    } catch {
        # Never propagate provider bodies, request URLs, credentials or tokens.
        $failureRecord = $_
        $details = $failureRecord.ErrorDetails.Message
        # Windows PowerShell 5.1 may leave ErrorDetails empty for HTTP errors.
        if (-not $details -and $failureRecord.Exception.Response) {
            try {
                $response = $failureRecord.Exception.Response
                if ($response.PSObject.Methods['GetResponseStream']) {
                    $reader = [IO.StreamReader]::new($response.GetResponseStream())
                    try { $details = $reader.ReadToEnd() } finally { $reader.Dispose() }
                } elseif ($response.Content) {
                    $details = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                }
            } catch { }
        }
        $safeCode = 'PROVIDER_REQUEST_FAILED'
        $allowedCodes = @('INVALID_LOGIN_CREDENTIALS','INVALID_PASSWORD','EMAIL_NOT_FOUND',
            'INVALID_EMAIL','USER_DISABLED','USER_NOT_FOUND','TOKEN_EXPIRED',
            'INVALID_REFRESH_TOKEN','INVALID_ID_TOKEN','CREDENTIAL_TOO_OLD_LOGIN_AGAIN',
            'REQUIRES_RECENT_LOGIN','WEAK_PASSWORD','PASSWORD_DOES_NOT_MEET_REQUIREMENTS',
            'OPERATION_NOT_ALLOWED','TOO_MANY_ATTEMPTS_TRY_LATER','RESET_PASSWORD_EXCEED_LIMIT')
        try {
            $message = ($details | ConvertFrom-Json -ErrorAction Stop).error.message
            $candidate = (($message -split '\s*:\s*',2)[0]).Trim()
            if ($allowedCodes -contains $candidate) { $safeCode = $candidate }
        } catch { }
        throw "Firebase request failed: $safeCode"
    } finally {
        $requestBody = $null
        $Payload = $null
    }
}

function Get-FirebaseTestSession {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][string]$ApiKey,
        [Parameter(Mandatory=$true)][string]$Email,
        [Security.SecureString]$Password
    )
    if (-not $Password) { $Password = Read-Host 'Test account current password' -AsSecureString }
    $payload = $null
    $result = $null
    try {
        $payload = @{
            email=$Email.Trim()
            password=([Net.NetworkCredential]::new('', $Password)).Password
            returnSecureToken=$true
        }
        $result = Invoke-FirebaseFlowRequest -ApiKey $ApiKey -Action signInWithPassword -Payload $payload
        if (-not $result.idToken -or -not $result.refreshToken -or -not $result.localId) {
            throw 'Firebase returned an incomplete sign-in result.'
        }
        # Deliberately exclude passwordHash and arbitrary provider properties.
        [pscustomobject]@{ IdToken=$result.idToken; RefreshToken=$result.refreshToken; FirebaseUID=$result.localId }
    } finally { $payload=$null; $Password=$null; $result=$null }
}

function Update-FirebaseTestSession {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][string]$ApiKey,
        [Parameter(Mandatory=$true)][ValidateNotNullOrEmpty()][string]$RefreshToken
    )
    $result=$null
    try {
        $result=Invoke-FirebaseFlowRequest -ApiKey $ApiKey -Action refresh -Payload @{grant_type='refresh_token';refresh_token=$RefreshToken}
        if (-not $result.id_token -or -not $result.refresh_token -or -not $result.user_id) {
            throw 'Firebase returned an incomplete refresh result.'
        }
        [pscustomobject]@{ IdToken=$result.id_token; RefreshToken=$result.refresh_token; FirebaseUID=$result.user_id }
    } finally { $result=$null; Remove-Variable RefreshToken -ErrorAction SilentlyContinue }
}

function Set-FirebaseTestPassword {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][string]$ApiKey,
        [Parameter(Mandatory=$true)][ValidateNotNullOrEmpty()][string]$IdToken,
        [Security.SecureString]$NewPassword,
        [Security.SecureString]$CurrentPassword
    )
    $payload=$null; $plain=$null; $confirmation=$null; $confirmed=$null
    $currentPlain=$null; $lookup=$null; $reauth=$null
    try {
        if (-not $CurrentPassword) { $CurrentPassword=Read-Host 'Current TEST password before this change' -AsSecureString }
        if (-not $NewPassword) {
            $NewPassword=Read-Host 'New TEST password (12-128 characters)' -AsSecureString
            $confirmation=Read-Host 'Confirm new TEST password' -AsSecureString
            $confirmed=([Net.NetworkCredential]::new('', $confirmation)).Password
        }
        $plain=([Net.NetworkCredential]::new('', $NewPassword)).Password
        # Match provisioning's Unicode code-point length bounds, not UTF-16 length.
        $count=0
        for ($i=0; $i -lt $plain.Length; $i++) {
            $count++
            if ([char]::IsHighSurrogate($plain[$i]) -and ($i+1 -lt $plain.Length) -and [char]::IsLowSurrogate($plain[$i+1])) { $i++ }
        }
        if ($count -lt 12 -or $count -gt 128) { throw 'New test password must contain 12-128 Unicode characters.' }
        if ($confirmation -and $plain -cne $confirmed) { throw 'Password confirmation does not match.' }
        $currentPlain=([Net.NetworkCredential]::new('', $CurrentPassword)).Password
        if ([string]::Equals($plain,$currentPlain,[StringComparison]::Ordinal)) {
            throw 'New password must be different from the current password.'
        }
        # Derive the target account from the supplied token; do not trust a caller's email.
        $lookup=Invoke-FirebaseFlowRequest -ApiKey $ApiKey -Action lookup -Payload @{idToken=$IdToken}
        $users=@($lookup.users)
        if ($users.Count -ne 1 -or -not $users[0].localId -or -not $users[0].email) {
            throw 'Firebase returned an incomplete account lookup.'
        }
        $reauth=Get-FirebaseTestSession -ApiKey $ApiKey -Email $users[0].email -Password $CurrentPassword
        if ($reauth.FirebaseUID -cne $users[0].localId) { throw 'Reauthentication account mismatch.' }
        $payload=@{idToken=$reauth.IdToken;password=$plain;returnSecureToken=$false}
        $null=Invoke-FirebaseFlowRequest -ApiKey $ApiKey -Action update -Payload $payload
        Write-Host 'Firebase accepted the password change. Sign in again with the new password.'
    } finally { $payload=$null;$plain=$null;$NewPassword=$null;$confirmation=$null;$confirmed=$null;$currentPlain=$null;$CurrentPassword=$null;$lookup=$null;$reauth=$null;Remove-Variable IdToken -ErrorAction SilentlyContinue }
}

function Send-FirebaseTestPasswordReset {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][string]$ApiKey,
        [Parameter(Mandatory=$true)][string]$Email
    )
    $null=Invoke-FirebaseFlowRequest -ApiKey $ApiKey -Action sendOobCode -Payload @{requestType='PASSWORD_RESET';email=$Email.Trim()}
    Write-Host 'Reset request accepted. Check the test mailbox; acceptance alone does not prove delivery or account existence.'
}
