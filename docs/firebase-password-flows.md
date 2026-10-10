# Firebase password reset/change verification — issue #40

## Scope and security contract

FR004 and ADR-002 delegate credentials, password changes, reset emails/codes and
session refresh to Firebase. This ticket adds interactive verification helpers
and evidence instructions; it does not add a Go password API or local reset store.
Loading scripts/firebase-password-tools.ps1 performs no network calls. Only
explicit function calls change a password or request a reset email. Assign session
results to variables: their ID/refresh tokens are intentionally kept in memory.
Do not print the session object, use a transcript, or commit tokens/reset links.
The helper suppresses arbitrary provider response properties and unsafe errors.

Policy under ADR-002:

- Password-change helper rejects a new password exactly equal to the current
  password and reauthenticates the token owner before updating Firebase. This
  is a helper/application-flow rule, not a provider-wide password-history policy.
- Before a password change, freshly sign in with the current password. If Firebase
  requires recent login, reauthenticate; never disable that requirement.
- After changing/resetting a password, sign in again. Verify the previous password
  fails and fresh credentials can call /api/v1/me for the same account and role.
- Previously valid ID/refresh tokens must no longer provide application access.
  Backend verification uses VerifyIDTokenAndCheckRevoked, not signature checks alone.
- Firebase credential changes do not activate a local user or alter their role.
  A valid Firebase identity with an inactive local account receives 403. Normal
  application deactivation also disables/revokes Firebase and can result in 401.
- Do not label reset request acceptance as proof of email delivery or account
  existence. User-facing forgot-password messages should remain generic; email
  enumeration protection and abuse controls must be reviewed in the project.

Firebase records revocation times with second precision. The guide waits at least
two seconds between the baseline sign-in and a password mutation, to avoid testing
tokens minted in the same second. Allow short provider propagation time; record
what happened rather than repeatedly resetting passwords. Persistent acceptance
of old credentials/tokens blocks completion and needs investigation.

References checked 9 October 2026:
- [Firebase REST contracts](https://firebase.google.com/docs/reference/rest/auth)
- [Session revocation](https://firebase.google.com/docs/auth/admin/manage-sessions)
- [Password change and reauthentication](https://firebase.google.com/docs/auth/web/manage-users)
- [Project password policy](https://firebase.google.com/docs/auth/web/password-auth)

## 1. Prepare a dedicated account and project settings

Use an active, provisioned TEST student whose mailbox you control. Do not test on
the sole Admin or another person's account. The account needs no student profile.
Know its current password, and retain each new test password privately for the
next sign-in. Use different passwords for change and reset verification.

In the Firebase console, select hostel-hive-152ef and review Authentication:
- Email/Password provider enabled.
- Settings / Password policy enforcement. The application provisioning baseline
  is 12–128 Unicode characters; configure/verify the provider's Require policy
  with compatible bounds and any agreed complexity requirements. The helper's
  bounds do not enforce browser-reset passwords; Firebase must enforce its policy.
- Password reset email template/action handler uses the intended project/domain.
- Email enumeration protection, quotas/abuse settings and API key restrictions.

Record actual settings, review date and any unresolved differences. Do not weaken
project restrictions just to make a CLI test pass. Use an allowed test key/client
when restrictions prevent the REST helper. Formal security-equivalence/supervisor
approval and final React/Flutter UI tests remain separate evidence requirements.

## 2. Start the backend — Terminal 1 (PowerShell)

Stop the old server with Ctrl+C and open Docker Desktop first.

```powershell
Set-Location 'C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend'
git branch --show-current
docker start hostelhive-postgres
$env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'
$env:FIREBASE_PROJECT_ID = 'hostel-hive-152ef'
$env:GOOGLE_APPLICATION_CREDENTIALS = 'C:\Users\TASHEEN\Desktop\Projects\private\hostelhive-service-account.json'
$env:HTTP_ADDR = '127.0.0.1:18080'
$env:R2_ENABLED = 'false'
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o .\bin\hostelhive-server.exe .\cmd\server
.\bin\hostelhive-server.exe
```

Expected branch feature/40-firebase-password-flows; migration remains 9. Leave
Terminal 1 running. No new database migration or credential file is needed.

## 3. Sign in and capture a valid baseline — Terminal 2

```powershell
Set-Location 'C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend'
. .\scripts\firebase-password-tools.ps1
$firebaseApiKey = Read-Host 'Firebase WEB API key (not a user UID)'
$testEmail = Read-Host 'Your dedicated TEST student email'
$beforeChange = Get-FirebaseTestSession -ApiKey $firebaseApiKey -Email $testEmail
# Verify refresh before retaining its returned current session for later checks.
$beforeChange = Update-FirebaseTestSession -ApiKey $firebaseApiKey -RefreshToken $beforeChange.RefreshToken
$beforeMe = Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/me' -Headers @{ Authorization="Bearer $($beforeChange.IdToken)" }
$beforeMe | Format-List user_id,role,is_active
$testUserID = $beforeMe.user_id
```

Expected role student, active True and a nonempty local user_id. Assign helper
responses; do not print beforeChange. The API key comes from Firebase web-app
configuration, not firebase_uid or the private service-account key.

Define this status-only helper in Terminal 2. It prints no token/response body:

```powershell
function Get-TestMeStatus([string]$Token) {
    try {
        $null = Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/me' -Headers @{ Authorization="Bearer $Token" } -ErrorAction Stop
        return 200
    } catch {
        if ($_.Exception.Response) { return [int]$_.Exception.Response.StatusCode }
        throw 'Backend request failed without an HTTP response; check server availability.'
    }
}
```

## 4. Change the password while signed in

```powershell
Start-Sleep -Seconds 2
Set-FirebaseTestPassword -ApiKey $firebaseApiKey -IdToken $beforeChange.IdToken
$afterChange = Get-FirebaseTestSession -ApiKey $firebaseApiKey -Email $testEmail
$afterMe = Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/me' -Headers @{ Authorization="Bearer $($afterChange.IdToken)" }
$afterMe | Format-List user_id,role,is_active
if ($afterMe.user_id -ne $testUserID -or $afterMe.role -ne $beforeMe.role) { throw 'Account identity/role unexpectedly changed.' }
Get-TestMeStatus -Token $beforeChange.IdToken
```

For Set, enter the current password, then enter/confirm a different compliant
new password; enter that new password for the
fresh Get. Same-password input is rejected without a password update; a wrong
current password cannot authorize an update. Expected fresh API 200, same user_id/role; old ID token 401. A provider
error is not success. If old token initially remains valid, wait briefly and
retry that status check; record delay. Do not mark persistent old-token access pass.

Verify the OLD password by running the following and entering the pre-change
password at the prompt. Only credential rejection counts as success; a network,
API-key, quota or configuration failure does not:

```powershell
$oldPasswordAccepted = $false
try {
    $unexpectedSession = Get-FirebaseTestSession -ApiKey $firebaseApiKey -Email $testEmail
    $oldPasswordAccepted = $true
} catch {
    if ($_.Exception.Message -notmatch 'Firebase request failed: (INVALID_LOGIN_CREDENTIALS|INVALID_PASSWORD)$') { throw }
    Write-Host 'PASS: old password rejected.'
}
if ($oldPasswordAccepted) { throw 'STOP: old password still signs in.' }
```

Verify the previously working refresh token:

```powershell
$oldRefreshAccepted = $false
try {
    $unexpectedSession = Update-FirebaseTestSession -ApiKey $firebaseApiKey -RefreshToken $beforeChange.RefreshToken
    $oldRefreshAccepted = $true
} catch {
    if ($_.Exception.Message -notmatch 'Firebase request failed: (TOKEN_EXPIRED|INVALID_REFRESH_TOKEN)$') { throw }
    Write-Host 'PASS: pre-change refresh token rejected.'
}
if ($oldRefreshAccepted) { throw 'STOP: pre-change refresh token still works.' }
```

## 5. Request and complete a password reset

First obtain a fresh baseline with the changed password:

```powershell
$beforeReset = Get-FirebaseTestSession -ApiKey $firebaseApiKey -Email $testEmail
$beforeReset = Update-FirebaseTestSession -ApiKey $firebaseApiKey -RefreshToken $beforeReset.RefreshToken
Get-TestMeStatus -Token $beforeReset.IdToken
Start-Sleep -Seconds 2
Send-FirebaseTestPasswordReset -ApiKey $firebaseApiKey -Email $testEmail
```

This last command sends a real reset request to the specified TEST mailbox.
Open the delivered email yourself, verify it belongs to the intended Firebase
project, and complete the hosted reset page with a different compliant password.
Do not paste the email/reset link or action code into chat, Git or the ticket.
Record delivery and completion, not just request acceptance. Check policy rejection
on the reset form before choosing a compliant password. If delivery fails, inspect
the template/project/abuse settings without repeatedly sending messages.

After completing the reset:

```powershell
$afterReset = Get-FirebaseTestSession -ApiKey $firebaseApiKey -Email $testEmail
$resetMe = Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/me' -Headers @{ Authorization="Bearer $($afterReset.IdToken)" }
$resetMe | Format-List user_id,role,is_active
if ($resetMe.user_id -ne $testUserID -or $resetMe.role -ne $beforeMe.role) { throw 'Reset changed account identity/role.' }
Get-TestMeStatus -Token $beforeReset.IdToken
```

Use the reset password in Get. Expected fresh API 200 and old ID token 401.
Repeat the old-password block from step 4, this time entering the password used
immediately before reset. Repeat the refresh block using beforeReset.RefreshToken
instead of beforeChange.RefreshToken. Both must reject the old credentials.

## 6. Verify inactive accounts stay blocked

Use the same dedicated test account, with a separate Admin token:

```powershell
$adminIDToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization="Bearer $adminIDToken" }
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $adminHeaders
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/users/$testUserID/deactivate" -Headers $adminHeaders
Get-TestMeStatus -Token $afterReset.IdToken
```

Confirm the separate Admin /me first. Expected backend denial: 401 or 403, not
200. If revocation is pending, let the existing worker complete it. Do not treat
partial deactivation as full success. A subsequent Firebase sign-in with the
correct reset password should report USER_DISABLED after the disable completes:

```powershell
$disabledSignInAccepted = $false
try {
    $unexpectedSession = Get-FirebaseTestSession -ApiKey $firebaseApiKey -Email $testEmail
    $disabledSignInAccepted = $true
} catch {
    if ($_.Exception.Message -notmatch 'Firebase request failed: USER_DISABLED$') { throw }
    Write-Host 'PASS: disabled test account cannot sign in.'
}
if ($disabledSignInAccepted) { throw 'STOP: disabled account still signs in; inspect deactivation state.' }
```

The normal workflow disables Firebase, so a password change while disabled may
itself be rejected. Do not bypass disabling or directly enable Firebase for this
test. Existing middleware regression tests independently cover a valid identity
whose local account is inactive (403). Neither password operation reactivates
PostgreSQL users. Restore the dedicated test account only after revocation completes:

```powershell
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/users/$testUserID/activate" -Headers $adminHeaders
$restoredSession = Get-FirebaseTestSession -ApiKey $firebaseApiKey -Email $testEmail
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers @{ Authorization="Bearer $($restoredSession.IdToken)" }
```

Expected same user_id and student role; password remains the reset password.
Refresh the Admin token if expired. No SQL/bootstrap is needed.

## 7. Automated checks and evidence

```powershell
.\scripts\test-firebase-password-tools.ps1
go test ./...
go vet ./...
git diff --check
Remove-Variable beforeChange,afterChange,beforeReset,afterReset,restoredSession,unexpectedSession,adminIDToken,adminHeaders -ErrorAction SilentlyContinue
```

The PowerShell test runs completely offline with mocked HTTP, validating request
contracts, safe responses/errors and Unicode password bounds. Go tests already
cover revoked/expired/disabled identity and fresh inactive-local-account denial.
These do not prove real Firebase revocation or email delivery. Record live results
and project policy review separately. Frontend React/Flutter reset/change UI,
secure client token handling and end-to-end UX remain frontend tasks. The
frontend change form must enforce the same current/new-password rule. Hosted
Firebase reset and direct Firebase clients are not changed by this helper; a
provider-wide no-reuse policy is not claimed. Do not store password hashes or
password history in PostgreSQL to enforce this rule.

Add a progress comment to issue #40 with actual results only:

```markdown
Firebase password-flow verification:
- Project password/abuse policy review: [settings and result]
- Password change: [fresh login/API success; old password rejected]
- Password reset email delivery/completion: [result]
- Reset: [fresh login/API success; pre-reset password rejected]
- Previous ID and refresh tokens: [results and propagation delay, if any]
- Same account/role retained; inactive access denied: [results]
- Automated checks: [offline helper tests, Go tests/vet/build]
- Frontend UI and formal security review: [pending/separate evidence]
```

## 8. Commit, PR and merge

After successful verification:

```powershell
git branch --show-current
git status --short
git diff --check
git add README.md docs/identity-requirements.md docs/firebase-password-flows.md scripts/firebase-password-tools.ps1 scripts/test-firebase-password-tools.ps1
git diff --cached --stat
git diff --cached --check
git commit -m "test(identity): document Firebase password lifecycle verification" -m "Refs #40"
git push -u origin feature/40-firebase-password-flows
```

Create PR: base develop, compare feature/40-firebase-password-flows.
Title: `test(identity): verify Firebase password lifecycle`.

```markdown
Add interactive reset/change/session verification tools and document FR004
checks under ADR-002. Firebase remains the credential authority.

Validation: [actual automated and live results].
Refs #40
```

Request teammate review, address findings, merge after approval, and record the
individual PR URL/reviewer and verified results in an issue comment. Close #40
only after its live criteria pass. Delete the remote feature branch from the
merged PR; stop the server and update locally:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/40-firebase-password-flows
```

If safe deletion refuses after a squash merge, verify the merged contents first.
Do not merge to main solely to finish this ticket; use the agreed release process.
