# Scanner device authentication — issue #53

This ticket starts the attendance module with device identity and Admin management.
It does not ingest QR scans, toggle presence or implement offline event retries.

## API and credential contract

| Method/path | Authentication/body | Success |
|---|---|---|
| POST `/api/v1/scanner-devices` | Firebase Admin; JSON `{ "name": "Gate-01" }` | 201 `{device,credential}` |
| GET `/api/v1/scanner-devices` | Firebase Admin; optional limit/offset | 200 `{items,limit,offset,has_more}` |
| POST `/api/v1/scanner-devices/{deviceID}/rotate-credential` | Firebase Admin; empty body | 200 `{device,credential}` |
| POST `/api/v1/scanner-devices/{deviceID}/deactivate` | Firebase Admin; empty body | 200 device |
| GET `/api/v1/device/check` | Device credential; empty body | 200 `{device_id,name,status:"authenticated"}` |

Only active Admins manage or list devices. Management transactions recheck current
Admin status while holding the users-table SHARE lock, coordinating with role and
account-status writes. Warden, Student, Sub-Warden and Security Staff are denied.
Names are trimmed, nonblank, valid UTF-8, without control characters, at most 120
characters, and globally unique ignoring case. Inactive device names stay reserved.
Listing defaults to limit 20/offset 0, accepts limits 1–100 and offsets 0–1,000,000,
and rejects unknown or repeated query parameters. It never returns secret/hash data.

Each scanner receives `hhd1.<device UUID>.<43-character base64url secret>`.
The secret contains 256 cryptographically random bits. The UUID selects the device;
it does not authenticate it. SHA-256 of the complete credential is stored as a
32-byte verifier and compared in constant time. Fast hashing here applies to a
high-entropy machine secret, not to a human-selected password.

Send device credentials as `Authorization: Bearer ...`, never in URLs or bodies.
They are accepted only by device routes; Firebase user tokens cannot authenticate
a scanner and scanner credentials cannot authenticate user/Admin routes. Production
device requests require HTTPS and certificate validation. Local HTTP tests below
use loopback only. HTTPS termination and deployment limits remain infrastructure
work; there is no per-device rate limiter in this ticket.

Registration and rotation return a raw credential only in that operation's response.
No retrieval endpoint exists and the raw value is never persisted. Configure the
scanner securely with that value. Lost response/credential: list the device and
rotate it; retrying registration with the same name returns 409. Retrying rotation
issues another secret and invalidates the previous one. There is no idempotency key.

Rotation updates the verifier/version atomically, without changing device identity
or active state. Subsequent authentication checks after commit reject older secrets.
Concurrent rotations serialize; only the last committed secret works. An operator
must install the newly issued credential on the device. Rotating an inactive device
does not reactivate it. No activation, deletion or renaming endpoint is introduced.

Deactivation is idempotent: repeat calls return the inactive device without changing
its timestamp or duplicating its audit event. There is no credential cache. Invalid,
unknown or inactive devices all receive 401 `unauthorized`. Database failure returns
503 `scanner_devices_unavailable` and fails closed. A request authenticated before
a change is not retrospectively undone. Future scan writes must recheck current
device authority transactionally as part of the attendance event contract.

Migration 000012 creates `scanner_devices` and `scanner_device_audit`. The audit
records registration/rotation/deactivation, Admin user ID, version and timestamp
in the same transaction as each change; it stores no credential or hash. Device
responses include IDs, name, active state, version, creation/update times and actors.

Other failures: missing/invalid Firebase authentication 401; unauthorized Admin actor
403; invalid input/UUID 400; absent target 404; duplicate name 409
`scanner_name_conflict`; non-JSON registration 415; body larger than 2048 bytes 413.
Query parameters are forbidden except on listing. Action/check/list bodies must
be empty. Unknown registration fields and trailing JSON are rejected. All responses
are `Cache-Control: no-store`; credentials and SQL details are not logged or echoed
in error responses. Deployments must preserve that logging policy.

## 1. Automated checks

Docker Desktop must be running. From the backend directory:

```powershell
git branch --show-current
.\scripts\test-backend-coverage.ps1
```

Expected branch `feature/53-scanner-device-authentication`. The script creates its
own disposable database, applies migrations and runs format, dependencies, vet,
build and all unit/integration tests. Attendance is a required ninth integration
package. Evidence lives under ignored `artifacts`. CI additionally runs race tests
and the full migration rollback/reapply cycle. Never run destructive migration
checks against the development database.

## 2. Terminal 1: migrate, rebuild and run

Stop an old server with Ctrl+C. These are the established local development settings.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
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

Expected migration version 12, clean; leave this terminal running.

## 3. Terminal 2: Admin login and registration

Enter the Firebase web API key and active Admin email/password.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'
$adminToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders
$deviceName = 'LOCAL-GATE-53-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
$registerBody = @{ name = $deviceName } | ConvertTo-Json
$issued = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices" -Headers $adminHeaders -ContentType 'application/json' -Body $registerBody
$deviceID = $issued.device.device_id
$originalCredential = $issued.credential
$deviceHeaders = @{ Authorization = "Bearer $originalCredential" }
$issued.device | ConvertTo-Json -Depth 5
```

Expected a new active device, version 1, and the Admin actor IDs. The raw credential
is held in your session variables; avoid posting it or including it in evidence.

## 4. Authentication check and listing

```powershell
Invoke-RestMethod -Uri "$baseURL/api/v1/device/check" -Headers $deviceHeaders
Invoke-RestMethod -Uri "$baseURL/api/v1/scanner-devices?limit=100" -Headers $adminHeaders | ConvertTo-Json -Depth 5
```

Expected matching device ID, name and `status: authenticated`; listing has no raw
credential/hash. The check does not scan a student or record attendance.

## 5. Define the rejection helper

```powershell
function Assert-HttpStatus {
    param([int]$Expected, [scriptblock]$Request)
    try {
        & $Request | Out-Null
        throw "FAIL: expected HTTP $Expected, but request succeeded."
    } catch {
        if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne $Expected) { throw }
        Write-Host "PASS: HTTP $Expected."
    }
}
Assert-HttpStatus 401 { Invoke-RestMethod -Uri "$baseURL/api/v1/device/check" }
Assert-HttpStatus 401 { Invoke-RestMethod -Uri "$baseURL/api/v1/device/check" -Headers $adminHeaders }
Assert-HttpStatus 401 { Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $deviceHeaders }
Assert-HttpStatus 409 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices" -Headers $adminHeaders -ContentType 'application/json' -Body $registerBody }
Assert-HttpStatus 400 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices" -Headers $adminHeaders -ContentType 'application/json' -Body '{"name":" "}' }
Assert-HttpStatus 404 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices/00000000-0000-0000-0000-000000000000/deactivate" -Headers $adminHeaders }
```

## 6. Rotate, then test old and new credentials

```powershell
$rotation = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices/$deviceID/rotate-credential" -Headers $adminHeaders
$newCredential = $rotation.credential
$newDeviceHeaders = @{ Authorization = "Bearer $newCredential" }
if ($newCredential -eq $originalCredential -or $rotation.device.credential_version -ne 2) { throw 'FAIL: credential rotation.' }
Assert-HttpStatus 401 { Invoke-RestMethod -Uri "$baseURL/api/v1/device/check" -Headers $deviceHeaders }
Invoke-RestMethod -Uri "$baseURL/api/v1/device/check" -Headers $newDeviceHeaders
Write-Host 'PASS: old credential rejected; new credential accepted.'
```

## 7. Deactivate and verify idempotency

```powershell
$disabled = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices/$deviceID/deactivate" -Headers $adminHeaders
if ($disabled.is_active -ne $false) { throw 'FAIL: scanner still active.' }
Assert-HttpStatus 401 { Invoke-RestMethod -Uri "$baseURL/api/v1/device/check" -Headers $newDeviceHeaders }
$disabledAgain = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices/$deviceID/deactivate" -Headers $adminHeaders
if ($disabledAgain.updated_at -ne $disabled.updated_at) { throw 'FAIL: repeated deactivation mutated scanner.' }
Write-Host 'PASS: deactivation and repeated deactivation.'
```

## 8. Warden permission check

Sign in using an active Warden's Firebase credentials, not Admin.

```powershell
$wardenToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $wardenHeaders
Assert-HttpStatus 403 { Invoke-RestMethod -Uri "$baseURL/api/v1/scanner-devices" -Headers $wardenHeaders }
Assert-HttpStatus 403 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices" -Headers $wardenHeaders -ContentType 'application/json' -Body $registerBody }
```

Expired Firebase tokens yield 401: renew the token rather than counting expiry as
a permission test. Automated tests cover all other roles, inactive Admins,
concurrent rotation, forged credentials and fresh authority during blocked writes.

Optional audit check in pgAdmin's Query Tool (replace device UUID):

```sql
SELECT action, credential_version, actor_id, occurred_at
FROM hostelhive.scanner_device_audit
WHERE device_id = 'PASTE_TEST_DEVICE_ID'
ORDER BY occurred_at, audit_id;
```

Expected registered/version 1, rotated/version 2, and deactivated/version 2:
three events, no duplicate event for repeated deactivation. Retain the disabled
test device and audit history. Don't directly delete rows or reactivate it with SQL.

## 9. Record evidence and commit

Record actual results on issue #53 without secret values: registration/listing,
authentication separation, rotation old/new results, deactivation/repetition,
permissions and audit. Record local automation and the successful CI URL separately.

```powershell
git status
git diff --check
git diff
git add internal/modules/attendance internal/app/app.go migrations/000012_scanner_devices.up.sql migrations/000012_scanner_devices.down.sql tests/integration/attendance scripts/coverage-report.py scripts/test_coverage_report.py go.mod README.md CHANGELOG.md docs/scanner-devices.md docs/module-layout.md docs/architecture.md docs/backend-ci.md
git diff --cached --check
git diff --cached --stat
git commit -m "feat(attendance): add scanner device authentication" -m "Refs #53"
git push -u origin feature/53-scanner-device-authentication
```

PR base develop; compare feature/53-scanner-device-authentication.
Title: `[DDP-#53] feat(attendance): add scanner device authentication`.

```markdown
Add Admin scanner registration, listing, credential rotation and deactivation.
Authenticate scanner requests separately from Firebase users.
Migration 000012 stores random-secret verifiers and atomic Admin audit events.

Refs #53

Validation:
- Automated: [actual test and attendance coverage results].
- Live: [registration, rotation, deactivation, permissions and audit checks].
- CI: [successful run URL, including migration rollback/reapply and race tests].

Attendance event ingestion remains a separate ticket.
Staging HTTPS remains tracked in Hostel-Hive/hostelhive-infra#1.
```

Link #53 through Development. Wait for successful CI and teammate approval before
merging. Then delete the remote feature branch and update locally:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/53-scanner-device-authentication
```

The design repository's scanner-device implementation record is a separate commit.
D06's event/direction/replay/timestamp rules must be resolved before scan ingestion.

## Recorded automated results — 11 October 2026

- Formatting, dependency verification, vet, build and full Go suite passed.
- All nine required PostgreSQL integration packages executed with no skips.
- Scanner lifecycle integration passed: hash-only persistence, authentication
  separation, Admin permissions, rotation, deactivation/repetition, audit events,
  concurrent credential updates and authority changes during blocked writes.
- Migration 000012 applied, rolled back preserving earlier schema and reapplied
  successfully in a separate disposable database.
- Attendance statement coverage: 252/290 (86.9%).
- Overall application statement coverage: 2221/2542 (87.4%).

Local runs use real PostgreSQL and fake Firebase verification, without race mode.
Live Firebase smoke checks, hosted CI/race evidence, peer review, actual ESP32
configuration and staging HTTPS remain separate evidence. Test containers were
removed; the local development database was not migrated or changed by these checks.
