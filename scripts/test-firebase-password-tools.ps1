# Offline tests: every HTTP call is replaced in this isolated PowerShell process.
$ErrorActionPreference='Stop'
$helper=Join-Path $PSScriptRoot 'firebase-password-tools.ps1'
$tokens=$null;$parseErrors=$null
$null=[Management.Automation.Language.Parser]::ParseFile($helper,[ref]$tokens,[ref]$parseErrors)
if ($parseErrors.Count) { throw 'Password helper has syntax errors.' }
$script:calls=@();$script:mode='signin';$script:errorCode=$null;$script:responseOnly=$false;$script:rejectReauth=$false;$script:reauthUID='test-uid'
function Assert-Flow($condition,[string]$message) { if (-not $condition) {throw $message} }
function Invoke-RestMethod {
    param($Method,$Uri,$ContentType,$Body,$TimeoutSec,$MaximumRedirection,$ErrorAction)
    $script:calls+=@{Method=$Method;Uri=$Uri;ContentType=$ContentType;Body=$Body;Timeout=$TimeoutSec;Redirects=$MaximumRedirection}
    if ($script:rejectReauth -and $Uri -like '*accounts:signInWithPassword?*') {
        $failure=[Management.Automation.ErrorRecord]::new([Exception]::new('private reauthentication details'),'OfflineReauthFailure',[Management.Automation.ErrorCategory]::AuthenticationError,$null)
        $failure.ErrorDetails=[Management.Automation.ErrorDetails]::new('{"error":{"message":"INVALID_LOGIN_CREDENTIALS"}}')
        throw $failure
    }
    if ($script:errorCode) {
        $exception=[Exception]::new('private provider response with test token')
        $record=[Management.Automation.ErrorRecord]::new($exception,'OfflineProviderError',[Management.Automation.ErrorCategory]::InvalidOperation,$null)
        if ($script:responseOnly) {
            $script:responseJSON=@{error=@{message=$script:errorCode}}|ConvertTo-Json
            $response=[pscustomobject]@{}
            $response | Add-Member -MemberType ScriptMethod -Name GetResponseStream -Value {
                return [IO.MemoryStream]::new([Text.Encoding]::UTF8.GetBytes($script:responseJSON))
            }
            $exception | Add-Member -MemberType NoteProperty -Name Response -Value $response
        } else {
            $record.ErrorDetails=[Management.Automation.ErrorDetails]::new((@{error=@{message=$script:errorCode}}|ConvertTo-Json))
        }
        throw $record
    }
    if ($Uri -like '*accounts:lookup?*') { return [pscustomobject]@{users=@([pscustomobject]@{localId='test-uid';email='test@example.invalid'})} }
    if ($Uri -like '*accounts:signInWithPassword?*' -and $script:mode -eq 'update') {
        return [pscustomobject]@{idToken='reauth-id';refreshToken='reauth-refresh';localId=$script:reauthUID}
    }
    switch ($script:mode) {
        signin { return [pscustomobject]@{idToken='test-id';refreshToken='test-refresh';localId='test-uid';passwordHash='must-not-return';extra='must-not-return'} }
        refresh { return [pscustomobject]@{id_token='refreshed-id';refresh_token='refreshed-refresh';user_id='test-uid'} }
        incomplete { return [pscustomobject]@{email='example@example.invalid'} }
        default { return [pscustomobject]@{passwordHash='must-not-return';idToken='must-not-print'} }
    }
}
function Expect-FlowFailure([scriptblock]$operation,[string]$expected) {
    $failure=$null
    try { $null=& $operation } catch { $failure=$_.Exception.Message }
    Assert-Flow ($failure -and $failure.Contains($expected)) 'Expected safe failure was not returned.'
    Assert-Flow (-not $failure.Contains('test-id') -and -not $failure.Contains('private') -and -not $failure.Contains('must-not-return')) 'Sensitive provider details leaked.'
}
. $helper
Assert-Flow ($script:calls.Count -eq 0) 'Loading the helper performed a network request.'
$key='AIza' + ('x'*35)
$password=ConvertTo-SecureString 'Offline-test-password-20' -AsPlainText -Force
$currentPassword=ConvertTo-SecureString 'Previous-test-password-20' -AsPlainText -Force
$session=Get-FirebaseTestSession -ApiKey $key -Email 'test@example.invalid' -Password $password
Assert-Flow ($session.IdToken -eq 'test-id' -and $session.RefreshToken -eq 'test-refresh' -and $session.FirebaseUID -eq 'test-uid') 'Sign-in session mapping failed.'
Assert-Flow ($session.PSObject.Properties.Count -eq 3 -or @($session.PSObject.Properties).Count -eq 3) 'Unapproved response properties returned.'
$request=$script:calls[-1];$payload=$request.Body|ConvertFrom-Json
Assert-Flow ($request.Uri -like 'https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=*' -and $payload.returnSecureToken -eq $true) 'Wrong sign-in contract.'
Assert-Flow ($request.Timeout -eq 15 -and $request.Redirects -eq 0) 'HTTP calls are not bounded.'
$script:mode='refresh'
$refreshed=Update-FirebaseTestSession -ApiKey $key -RefreshToken $session.RefreshToken
Assert-Flow ($refreshed.IdToken -eq 'refreshed-id' -and $refreshed.FirebaseUID -eq $session.FirebaseUID) 'Refresh response mapping failed.'
$request=$script:calls[-1]
Assert-Flow ($request.Uri -like 'https://securetoken.googleapis.com/v1/token?key=*' -and $request.Body.grant_type -eq 'refresh_token' -and $request.ContentType -eq 'application/x-www-form-urlencoded') 'Wrong refresh contract.'
$script:mode='update'
$output=Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $password -CurrentPassword $currentPassword
Assert-Flow ($null -eq $output) 'Password mutation returned provider/token data.'
$payload=$script:calls[-1].Body|ConvertFrom-Json
Assert-Flow ($payload.idToken -eq 'reauth-id' -and $payload.password -eq 'Offline-test-password-20' -and $payload.returnSecureToken -eq $false) 'Wrong password-change contract.'
Assert-Flow ($script:calls[-3].Uri -like '*accounts:lookup?*' -and $script:calls[-2].Uri -like '*accounts:signInWithPassword?*') 'Password change did not reauthenticate the token owner.'
$sameBefore=$script:calls.Count
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $password -CurrentPassword $password} 'different from the current password'
Assert-Flow ($script:calls.Count -eq $sameBefore) 'Same password reached the provider.'
$script:rejectReauth=$true
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $password -CurrentPassword $currentPassword} 'INVALID_LOGIN_CREDENTIALS'
Assert-Flow ($script:calls[-1].Uri -like '*accounts:signInWithPassword?*') 'Rejected current password still updated Firebase.'
$script:rejectReauth=$false;$script:reauthUID='other-account'
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $password -CurrentPassword $currentPassword} 'account mismatch'
Assert-Flow ($script:calls[-1].Uri -like '*accounts:signInWithPassword?*') 'Mismatched account still updated Firebase.'
$script:reauthUID='test-uid'
$script:passwordPrompts=@($currentPassword,$password,$currentPassword)
function Read-Host { param($Prompt,[switch]$AsSecureString)
    $next=$script:passwordPrompts[0]
    $script:passwordPrompts=@($script:passwordPrompts|Select-Object -Skip 1)
    return $next
}
$confirmationBefore=$script:calls.Count
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken} 'confirmation does not match'
Assert-Flow ($script:calls.Count -eq $confirmationBefore) 'Mismatched confirmation reached Firebase.'
$before=$script:calls.Count
$short=ConvertTo-SecureString ('a'*11) -AsPlainText -Force
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $short -CurrentPassword $currentPassword} '12-128'
$tooLong=ConvertTo-SecureString ('a'*129) -AsPlainText -Force
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $tooLong -CurrentPassword $currentPassword} '12-128'
$emoji=[char]::ConvertFromUtf32(0x1F600)
$shortUnicode=ConvertTo-SecureString ($emoji*6) -AsPlainText -Force
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $shortUnicode -CurrentPassword $currentPassword} '12-128'
Assert-Flow ($script:calls.Count -eq $before) 'Invalid password reached the provider.'
Expect-FlowFailure {Get-FirebaseTestSession -ApiKey 'firebase-uid-not-an-api-key' -Email 'test@example.invalid' -Password $password} 'web API key'
$output=Send-FirebaseTestPasswordReset -ApiKey $key -Email 'test@example.invalid'
Assert-Flow ($null -eq $output) 'Reset request returned sensitive output.'
$payload=$script:calls[-1].Body|ConvertFrom-Json
Assert-Flow ($payload.requestType -eq 'PASSWORD_RESET' -and $payload.email -eq 'test@example.invalid' -and @($payload.PSObject.Properties).Count -eq 2) 'Wrong reset request.'
foreach($code in @('INVALID_LOGIN_CREDENTIALS','TOKEN_EXPIRED','USER_DISABLED','CREDENTIAL_TOO_OLD_LOGIN_AGAIN')) {
    $script:errorCode=$code
    Expect-FlowFailure {Get-FirebaseTestSession -ApiKey $key -Email 'test@example.invalid' -Password $password} $code
}
$script:errorCode='WEAK_PASSWORD : private provider password detail'
Expect-FlowFailure {Set-FirebaseTestPassword -ApiKey $key -IdToken $session.IdToken -NewPassword $password -CurrentPassword $currentPassword} 'WEAK_PASSWORD'
$script:errorCode='test-id private token details'
Expect-FlowFailure {Update-FirebaseTestSession -ApiKey $key -RefreshToken 'test-refresh'} 'PROVIDER_REQUEST_FAILED'
# Exercise the Windows PowerShell response-stream path without real HTTP.
$script:responseOnly=$true
$script:errorCode='INVALID_LOGIN_CREDENTIALS'
Expect-FlowFailure {Get-FirebaseTestSession -ApiKey $key -Email 'test@example.invalid' -Password $password} 'INVALID_LOGIN_CREDENTIALS'
$script:errorCode='private unknown provider detail test-id'
Expect-FlowFailure {Get-FirebaseTestSession -ApiKey $key -Email 'test@example.invalid' -Password $password} 'PROVIDER_REQUEST_FAILED'
$script:responseOnly=$false
$script:errorCode=$null;$script:mode='incomplete'
Expect-FlowFailure {Get-FirebaseTestSession -ApiKey $key -Email 'test@example.invalid' -Password $password} 'incomplete sign-in'
Expect-FlowFailure {Update-FirebaseTestSession -ApiKey $key -RefreshToken 'test-refresh'} 'incomplete refresh'
Write-Host 'PASS: offline Firebase password/session helper contracts, bounds and redaction. No network or email sent.'
