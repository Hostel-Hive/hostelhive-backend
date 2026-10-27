# Leave requests - issue #62

> Retired by #118: submission, decision and cancellation routes return410 after
> authentication/role checks. Leave list/detail and reports are historical reads.
> No return date or approval is required for a trip home. See
> [current policy](hostel-movement-policy.md); the original implementation is below.

Students request extended leave; Admin/Warden approve or reject it. PostgreSQL
stores permanent request/decision history. Approval does not update presence or
Firebase counts: entry/exit scans remain the source of recorded presence.

## Contract

Migration 15 adds hostelhive.leave_requests. No delete/edit endpoint is exposed.
Active Student accounts with unarchived profiles may submit. Students cannot
choose another student_id; the backend derives ownership from the verified UID.
Active Admin/Warden can read all records and decide pending requests. Security
and Sub-Warden roles are denied. Archived profiles' history remains available
while the owning Student account is active; new submissions are denied.

Use RFC3339 starts_at/ends_at including timezone offset. The start must be in the
future and the end later than the start. Reason and decision notes allow up to
1000 characters. Rejection needs a note. Approval needs an eligible student and
future start. Pending/approved intervals cannot overlap for a student; adjacent
intervals are allowed. Rejected requests permit a replacement.

A client-generated UUID request_id makes submission retries safe. Reuse it with
exactly the same normalized payload to receive the original record; changing the
payload returns 409. The same staff actor retrying an identical decision receives
its original result. Another final decision returns 409. Concurrent submissions
and decisions are serialized by locking the owning student row. Current account
permissions are rechecked in the database transaction.

No leave scheduling, notifications, report export, late access, cancellation,
or self-service Going Home action is claimed by this ticket. The user approved
changing FR024 to scan-based presence on 2026-10-12; see the documentation repo's
leave-management amendment. Source SRS/SDS revision still needs controlled review.

## Endpoints

| Method/path | Allowed accounts | Body/query |
|---|---|---|
| POST /api/v1/leaves | Student | request_id, starts_at, ends_at, reason |
| GET /api/v1/leaves | Student own; Admin/Warden all | optional student_id, status, limit, offset |
| GET /api/v1/leaves/{leaveID} | Student own; Admin/Warden all | none |
| POST /api/v1/leaves/{leaveID}/decision | Admin/Warden | status approved/rejected; note |

List: items,total,limit,offset,has_more. Default limit 20, max 100. Latest first;
empty pages retain total. Another student's detail is 404; attempting to expand
Student list ownership returns 403. Other errors: 400 invalid input, 401 absent
login, 403 role/account denial, 409 leave_conflict/active_student_required,
415 wrong content type, 413 oversized body, 503 leave_unavailable. Unknown JSON
fields, trailing JSON, duplicate/unknown query parameters are rejected.

## Local verification with two terminals

Terminal 1: stop the old server with Ctrl+C, keep PostgreSQL running, and retain
your existing environment settings, including DATABASE_URL and Firebase settings.
Only apply up to your development database; never run down for these live checks.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
git branch --show-current # feature/62-leave-management
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version # 15, clean
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

Terminal 2: use dedicated test accounts. Do not print their ID tokens.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'
$studentToken = .\scripts\get-firebase-token.ps1 # active test Student
$studentHeaders = @{ Authorization = "Bearer $studentToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $studentHeaders
$adminToken = .\scripts\get-firebase-token.ps1 # Admin
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders

$start = [DateTimeOffset]::UtcNow.AddDays(2)
$end = $start.AddDays(1)
$body = @{
 request_id = [Guid]::NewGuid().ToString()
 starts_at = $start.ToString('o')
 ends_at = $end.ToString('o')
 reason = 'LOCAL-TEST-62 family visit'
} | ConvertTo-Json
$before = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $adminHeaders
$request = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves" -Headers $studentHeaders -ContentType 'application/json' -Body $body
$request | ConvertTo-Json
$leaveID = $request.leave_id
$retry = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves" -Headers $studentHeaders -ContentType 'application/json' -Body $body
if ($retry.leave_id -ne $leaveID) { throw 'Submission retry created a duplicate.' }
Invoke-RestMethod -Uri "$baseURL/api/v1/leaves?status=pending" -Headers $studentHeaders | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri "$baseURL/api/v1/leaves/$leaveID" -Headers $adminHeaders
$decisionBody = @{ status='approved'; note='LOCAL-TEST-62 approved' } | ConvertTo-Json
$approved = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves/$leaveID/decision" -Headers $adminHeaders -ContentType 'application/json' -Body $decisionBody
$approved | ConvertTo-Json
$again = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves/$leaveID/decision" -Headers $adminHeaders -ContentType 'application/json' -Body $decisionBody
if ($again.decided_at -ne $approved.decided_at) { throw 'Decision retry changed its timestamp.' }
$after = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $adminHeaders
# Compare only in a quiet test environment with no simultaneous scans/account changes.
if ($before.total -ne $after.total -or $before.in -ne $after.in -or $before.out -ne $after.out -or $before.unknown -ne $after.unknown) { throw 'Investigate changed attendance counts.' }
```

Record actual results for these additional checks:

1. Submit another request with a new request_id and overlapping times: 409.
2. Submit a request after the first ends. Sign in as test Warden and reject it
   with a reason: 200, status rejected, decided_by set. Exact retry preserves time.
3. Approve an already rejected request: 409. Reject without a note: 400.
4. Sign in as a second test Student: first student's detail 404 and list filter
   for that student 403. Their own unfiltered list must not show those records.
5. Student decision request 403; Admin submission 403; Security/Sub-Warden read
   403; no Authorization header 401.
6. Past start, reversed dates, blank reason, malformed UUID, forged student_id:
   400. Wrong content type 415. Empty page (offset=999) retains total.
7. Deactivate/archive only dedicated fixtures: no new submission or approval;
   staff still read history. Rejection clears ineligible pending requests.
   Restore active status afterward if the fixture is needed again.

Records are deliberately retained. Do not delete history as test cleanup. No
production/provider settings need to change for these checks.

## Automated verification and CI

```powershell
.\scripts\test-backend-coverage.ps1
```

This creates and removes its own PostgreSQL container; it does not migrate or
clean the developer database. It checks formatting, dependencies, vet, build and
all tests and generates artifacts/coverage.md. CI also applies migrations,
rolls back fully and reapplies, runs race detection and requires the leave
integration package to pass without skips. RTDB tests remain unchanged.

Local verification on 2026-10-12 passed formatting, module verification, vet,
build and the complete Go test suite. All ten required PostgreSQL integration
packages passed without skips. Statement coverage: 85.5% overall (2999/3509),
85.3% leave (272/319). Migration 1-15 apply/full rollback/reapply and existing SQL
schema checks passed in a separate disposable database. Tests verify overlapping
submission and competing decision races, ownership, safe retries, eligibility,
HTTP validation, audit preservation and unchanged existing IN presence/Firebase
publication revision. Local tests did not use the race detector; CI and live
Firebase-authenticated API verification remain pending.

Local and live results must be recorded separately. Commit/PR title:
[DDP-#62] feat(leave): add student leave requests and approval workflow
Commit subject without the ticket prefix; body Refs #62. PR base develop.

## Pending request cancellation (#101)

Migration28 adds owner-only cancellation and the terminal `cancelled` status.
See [the cancellation contract](pass-cancellation.md) for routes, audit, retries,
permissions and race handling. Earlier ticket-scope exclusions above are historical.
