# Attendance scans — issue #55

## Contract

Apply migration 000013. Existing scanners have `direction: null` and cannot
record new scans until Admin configures their direction. Registration remains
compatible with #53. Changing direction does not rotate credentials or activate
a disabled scanner. Repeating the same direction is idempotent and creates no
extra audit event. Admin mutations retain actor and time in scanner_device_audit.

| Method and path | Authentication | Result |
|---|---|---|
| PUT /api/v1/scanner-devices/{deviceID}/direction | active Firebase Admin | Device; body `{"direction":"IN"}` or OUT |
| POST /api/v1/device/scans | active scanner credential | Event; body `{"event_id":"UUID","qr":"HH1:64-lowercase-hex"}` |
| GET /api/v1/attendance/presence?student_id=UUID | active Firebase Admin/Warden | student_id, status IN/OUT/UNKNOWN, updated_at |
| GET /api/v1/attendance/events?student_id=UUID&limit=20&offset=0 | active Firebase Admin/Warden | items, limit, offset, has_more |

All successfully processed scans return 200. Inspect `outcome`: `recorded`,
`cooldown`, `already_in`, or `already_out`. Ignored scans are retained for audit
and retry protection but never change presence. Each new event ID belongs to one
physical scan attempt; network retries reuse that ID. Reusing it for another QR
returns 409 event_id_conflict. The original response is returned for matching
retries, even after direction/student eligibility changes; disabled or rotated
device credentials are still rejected before replay.

New events require an active Student account and unarchived profile with a known
generated HostelHive QR. No bed allocation prerequisite is introduced. Unknown
or ineligible QR returns 422 active_student_qr_required; unconfigured direction
409 scanner_direction_required; invalid JSON/UUID/QR/query 400; user token used
as scanner credential 401; unauthorized reader/manager role 403; missing resource
404; wrong media type 415; oversized JSON 413; persistence failure 503.

`ATTENDANCE_COOLDOWN` defaults to 10s, must be positive and at most 5m. It applies
to the same student/direction across scanners, measured since that direction's
last recorded movement. It does not block opposite direction. An IN→OUT→IN
within 10 seconds still suppresses the second IN. After cooldown, a repeated
direction while already in that state remains ignored and never toggles status.
Hardware testing may lead to a different configured interval.

Initial state UNKNOWN avoids assuming a student is outside. The first accepted
IN or OUT establishes their recorded status. Times come from PostgreSQL in UTC
after locks, not a device clock. Do not send client timestamps or queue delayed
offline batches. Scanner success feedback requires the server response; a lost
response can be retried with the same event ID. During outages use a manual staff
procedure. The backend cannot distinguish a delayed individual request from a
fresh scan; firmware must not enqueue offline movements for later submission.
Only server processing time is recorded in this version. Presence records remain
historical after profile archival/deactivation.
Scans do not prove physical passage. Realtime projections, aggregate counts,
student self attendance, manual corrections, offline synchronization and leave
interactions remain separate tickets.

## Start the server — Terminal 1

Stop the old server with Ctrl+C before rebuilding. Docker Desktop and the existing
hostelhive-postgres container must be running. Do not run migration down on your
development database; rollback removes the ticket's attendance records.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'
$env:FIREBASE_PROJECT_ID = 'hostel-hive-152ef'
$env:GOOGLE_APPLICATION_CREDENTIALS = 'C:\Users\TASHEEN\Desktop\Projects\private\hostelhive-service-account.json'
$env:HTTP_ADDR = '127.0.0.1:18080'
$env:R2_ENABLED = 'false'
$env:ATTENDANCE_COOLDOWN = '10s'
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

Expected: migration 13 clean, server listening. Leave this terminal running.

## Live verification — Terminal 2

Use a dedicated active, unarchived test student. Supply their **student_id** UUID,
not Firebase UID or local user_id. The QR endpoint below lazily creates their
credential if needed. Retrieve its token locally from pgAdmin; never paste it or
scanner credentials into public evidence.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'
$adminToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders
$studentID = Read-Host 'Dedicated test student_id'
New-Item -ItemType Directory -Force artifacts | Out-Null
Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/students/$studentID/qr" -Headers $adminHeaders -OutFile artifacts/student-qr-55.png
```

In **pgAdmin Query Tool**, run:

```sql
SELECT 'HH1:' || q.token AS qr_payload
FROM hostelhive.student_qr q
JOIN hostelhive.students s USING(student_id)
JOIN hostelhive.users u USING(user_id)
WHERE s.student_id = 'PASTE_TEST_STUDENT_ID'
  AND s.deleted_at IS NULL AND u.is_active AND u.role = 'student';
```

Copy the local qr_payload into the following prompt. Using a general QR reader
to decode the PNG gives the same value.

```powershell
$qrPayload = Read-Host 'Test QR payload (HH1:...)'
$fixture = 'LOCAL-GATE-55-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
$entry = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices" -Headers $adminHeaders -ContentType 'application/json' -Body (@{ name = "$fixture-Entry" } | ConvertTo-Json)
$exit = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices" -Headers $adminHeaders -ContentType 'application/json' -Body (@{ name = "$fixture-Exit" } | ConvertTo-Json)
$entryID = $entry.device.device_id
$exitID = $exit.device.device_id
$entryHeaders = @{ Authorization = "Bearer $($entry.credential)" }
$exitHeaders = @{ Authorization = "Bearer $($exit.credential)" }

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

$entryBody = @{ event_id = [Guid]::NewGuid().ToString(); qr = $qrPayload } | ConvertTo-Json
Assert-HttpStatus 409 {
    Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $entryBody
}
Invoke-RestMethod -Method Put -Uri "$baseURL/api/v1/scanner-devices/$entryID/direction" -Headers $adminHeaders -ContentType 'application/json' -Body '{"direction":"IN"}' | Out-Null
Invoke-RestMethod -Method Put -Uri "$baseURL/api/v1/scanner-devices/$exitID/direction" -Headers $adminHeaders -ContentType 'application/json' -Body '{"direction":"OUT"}' | Out-Null

# Establish OUT first, so this guide can run even if the student was already IN.
# The wait lets prior test movements leave the default cooldown window.
Start-Sleep -Seconds 11
$baselineBody = @{ event_id = [Guid]::NewGuid().ToString(); qr = $qrPayload } | ConvertTo-Json
$baseline = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $exitHeaders -ContentType 'application/json' -Body $baselineBody
if ($baseline.outcome -notin @('recorded','already_out')) { throw 'Baseline OUT failed.' }
$first = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $entryBody
if ($first.outcome -ne 'recorded' -or $first.direction -ne 'IN') { throw 'Entry not recorded.' }
$first | ConvertTo-Json -Depth 5
$retry = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $entryBody
if (($retry | ConvertTo-Json -Compress) -ne ($first | ConvertTo-Json -Compress)) { throw 'Retry changed result.' }
$repeatBody = @{ event_id = [Guid]::NewGuid().ToString(); qr = $qrPayload } | ConvertTo-Json
$repeat = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $repeatBody
if ($repeat.outcome -ne 'cooldown') { throw 'Cooldown failed; repeat immediately after entry.' }
$presence = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/presence?student_id=$studentID" -Headers $adminHeaders
if ($presence.status -ne 'IN') { throw 'Presence changed on duplicate entry.' }

# Baseline OUT starts its own cooldown; wait before testing another exit.
Start-Sleep -Seconds 11
$exitBody = @{ event_id = [Guid]::NewGuid().ToString(); qr = $qrPayload } | ConvertTo-Json
$left = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $exitHeaders -ContentType 'application/json' -Body $exitBody
if ($left.outcome -ne 'recorded' -or $left.direction -ne 'OUT') { throw 'Exit failed.' }
$presence = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/presence?student_id=$studentID" -Headers $adminHeaders
if ($presence.status -ne 'OUT') { throw 'Exit did not update presence.' }

Start-Sleep -Seconds 11
$repeatExitBody = @{ event_id = [Guid]::NewGuid().ToString(); qr = $qrPayload } | ConvertTo-Json
$already = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $exitHeaders -ContentType 'application/json' -Body $repeatExitBody
if ($already.outcome -ne 'already_out') { throw 'Repeated exit toggled or failed.' }

# Replaying entry after exit returns entry history without changing current OUT.
$retry = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $entryBody
$presence = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/presence?student_id=$studentID" -Headers $adminHeaders
if ($retry.event_id -ne $first.event_id -or $presence.status -ne 'OUT') { throw 'Historical retry changed presence.' }

$conflictBody = @{ event_id = $first.event_id; qr = 'HH1:' + ('0' * 64) } | ConvertTo-Json
Assert-HttpStatus 409 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $conflictBody }
Assert-HttpStatus 401 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $adminHeaders -ContentType 'application/json' -Body $entryBody }
$unknownBody = @{ event_id = [Guid]::NewGuid().ToString(); qr = 'HH1:' + ('0' * 64) } | ConvertTo-Json
Assert-HttpStatus 422 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $unknownBody }
$offlineBody = @{ event_id = [Guid]::NewGuid().ToString(); qr = $qrPayload; occurred_at = '2020-01-01T00:00:00Z' } | ConvertTo-Json
Assert-HttpStatus 400 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $offlineBody }
Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/events?student_id=$studentID&limit=100" -Headers $adminHeaders | ConvertTo-Json -Depth 6
```

Permissions: log in as Warden below. Reads succeed; direction changes fail.
Student login reads return 403. These operations do not change the test student.

```powershell
$wardenToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $wardenHeaders
Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/presence?student_id=$studentID" -Headers $wardenHeaders
Assert-HttpStatus 403 { Invoke-RestMethod -Method Put -Uri "$baseURL/api/v1/scanner-devices/$entryID/direction" -Headers $wardenHeaders -ContentType 'application/json' -Body '{"direction":"OUT"}' }
$studentToken = .\scripts\get-firebase-token.ps1
Assert-HttpStatus 403 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/events?student_id=$studentID" -Headers @{ Authorization = "Bearer $studentToken" } }

$rotation = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices/$entryID/rotate-credential" -Headers $adminHeaders
Assert-HttpStatus 401 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $entryHeaders -ContentType 'application/json' -Body $entryBody }
$newEntryHeaders = @{ Authorization = "Bearer $($rotation.credential)" }
$retry = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $newEntryHeaders -ContentType 'application/json' -Body $entryBody
if ($retry.event_id -ne $first.event_id) { throw 'Rotated credential retry failed.' }
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices/$entryID/deactivate" -Headers $adminHeaders | Out-Null
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/scanner-devices/$exitID/deactivate" -Headers $adminHeaders | Out-Null
Assert-HttpStatus 401 { Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/device/scans" -Headers $newEntryHeaders -ContentType 'application/json' -Body $entryBody }
Write-Host 'PASS: attendance live checks completed.'
```

In pgAdmin inspect history and current presence. Keep audit history; no manual
deletes are needed. Only `recorded` rows represent accepted movements.

```sql
SELECT event_id,device_id,direction,outcome,occurred_at
FROM hostelhive.attendance_events
WHERE student_id='PASTE_TEST_STUDENT_ID'
ORDER BY occurred_at,device_id,event_id;
SELECT student_id,status,updated_at FROM hostelhive.student_presence
WHERE student_id='PASTE_TEST_STUDENT_ID';
SELECT device_id,action,direction,actor_id,occurred_at
FROM hostelhive.scanner_device_audit
WHERE device_id IN ('PASTE_ENTRY_DEVICE_ID','PASTE_EXIT_DEVICE_ID')
ORDER BY occurred_at,audit_id;
```

## Automated checks and commit/PR

```powershell
.\scripts\test-backend-coverage.ps1
git status
git diff --check
git diff
git add .env.example internal/app/app.go internal/config/config.go internal/config/config_test.go internal/modules/attendance migrations/000013_attendance_scans.up.sql migrations/000013_attendance_scans.down.sql tests/integration/attendance/scans_test.go README.md CHANGELOG.md docs/attendance-scans.md docs/scanner-devices.md docs/module-layout.md docs/architecture.md docs/backend-ci.md
git diff --cached --check
git diff --cached --stat
git commit -m "feat(attendance): record student entry and exit scans" -m "Refs #55"
git push -u origin feature/55-attendance-scans
```

PR base develop, compare feature/55-attendance-scans. Title:
`[DDP-#55] feat(attendance): record student entry and exit scans`.
Describe explicit direction, online server timestamps, cooldown/idempotency,
transactional checks, migration 13 and read permissions. Use `Refs #55`, link
the ticket in Development, and include actual automated/live/CI results. Wait
for review and CI migration rollback/reapply plus race tests before merging.
Documentation-repository amendments need a separate scoped commit.

After merge and remote branch deletion:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/55-attendance-scans
```

Live Firebase, scanner hardware and staging HTTPS evidence are separate from
the disposable-database automated suite. Staging remains infra issue #1.

## Recorded automated results — 11 October 2026

Formatting, dependency verification, static checks, build, coverage-report tests,
unit/architecture tests and all nine required PostgreSQL integration packages
passed with no skips. Attendance statement coverage was 481/570 (84.4%); overall
application coverage was 2453/2827 (86.8%). Scan integration tests cover simultaneous
readers and retries, event-ID conflict, first exit from UNKNOWN, direction audit,
cooldown, non-toggling duplicates, server time, student eligibility, permissions,
pagination, and authentication invalidated between authentication and the write.

An independent disposable container passed migration 13 rollback/reapply and
full SQL migration rollback/reapply. This local run did not use the race detector
or live Firebase credentials, and did not alter the development database. PR CI
must still supply its own successful race and migration-CLI verification run.
