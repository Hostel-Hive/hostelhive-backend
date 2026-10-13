# Complaint tracking and resolution - issue #64

Core FR025-FR027 workflow. Students submit maintenance/disciplinary complaints
and view their own history. Admin/Warden manage both; Sub-Warden manages maintenance
only. Photos and notifications are separate follow-up tickets, not implemented here.

## Contract

Migration 16 adds complaints and complaint_updates. Original description, category
and owner cannot be edited via API. No deletion endpoint exists. Creation, notes,
resolution and reopening are separate history entries with actor, server timestamp
and version. Account deactivation/archival preserves history.

New submissions require an active Student with an unarchived profile and a UUID
request_id, category maintenance/disciplinary, nonblank description up to 2000
characters. Exact retries return the same complaint; changed reuse returns 409.

Students and authorised staff add nonblank notes (max 2000 characters) to unresolved
complaints. All notes in this workflow are visible to the owning Student; confidential
staff notes need a separate design. Staff transition pending -> in_progress/resolved,
in_progress -> resolved, resolved -> in_progress. Resolution and reopening require
an explanation. Students cannot change status. Archived Students cannot add notes,
but active Student accounts can still read their own archived profile's history.
Staff can continue handling complaints belonging to inactive/archived Students.

Every note/status action requires request_id and expected_version. A transaction
rechecks authority, locks the complaint, compares version, changes the record and
appends history atomically. Concurrent changes using the same version have one
winner. Exact retries by the original actor/action/payload return their original
history entry, even after later changes. Changed reuse and stale versions conflict.
Clients must refetch after 409, then decide whether a new action is still appropriate.
Do not automatically retry a changed decision with a fresh version.

Sub-Warden scope is enforced on list/detail/history/notes/status in the repository,
not only the frontend. Disciplinary detail/history/actions are concealed as 404;
explicit disciplinary filtering is 403. Students querying another student_id get
403; another Student's detail/history/actions return 404. Security has no access.

## API

| Method/path | Body/query |
|---|---|
| POST /api/v1/complaints | request_id, category, description (Student only) |
| GET /api/v1/complaints | student_id, category, status, created_from, created_to, limit, offset |
| GET /api/v1/complaints/{id} | none |
| GET /api/v1/complaints/{id}/updates | limit, offset |
| POST /api/v1/complaints/{id}/notes | request_id, expected_version, text |
| POST /api/v1/complaints/{id}/status | request_id, expected_version, status, text (staff only) |

Complaint response contains complaint_id, request_id, student_id, submitted_by,
category, description, status, version, created_at and updated_at. Mutation response
is the immutable update entry with update_id, action, actor_id, old/new status,
old/new version, text and occurred_at. Refetch the complaint for current state.

List and history responses contain items,total,limit,offset,has_more. Limit defaults
to 20, max 100. Complaint lists newest first; history oldest version first. Counts
and pages use one statement snapshot and preserve total when the page is empty.
Creation filters are [created_from,created_to), RFC3339 instants with timezone.

Errors: 400 invalid_input, 401 unauthorized, 403 forbidden, 404 not_found,
409 complaint_conflict/active_student_required, 413 payload_too_large,
415 unsupported_media_type, 503 complaints_unavailable. JSON fields and queries
are strict. Image/notification fields are not silently accepted.

## Startup - Terminal 1

Stop the previous server with Ctrl+C; retain your existing database/Firebase
settings. Apply up only to your development database.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
git branch --show-current # feature/64-complaint-management
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version # 16, clean
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

## Live verification - Terminal 2

Use dedicated test accounts. Passwords are entered into the secure login prompt.
Never print ID tokens. Tokens can expire: if /me returns 401, sign in again and
rebuild the matching headers before continuing. Authorization runs before input
validation; 401 is not a successful expected-400 validation test.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'
$studentToken = .\scripts\get-firebase-token.ps1 # active Student with profile
$studentHeaders = @{ Authorization="Bearer $studentToken" }
$studentMe = Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $studentHeaders
if ($studentMe.role -ne 'student' -or -not $studentMe.is_active) { throw 'Use an active Student.' }
$adminToken = .\scripts\get-firebase-token.ps1 # Admin
$adminHeaders = @{ Authorization="Bearer $adminToken" }
$adminMe = Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders
if ($adminMe.role -ne 'admin') { throw 'Use Admin.' }
$wardenToken = .\scripts\get-firebase-token.ps1 # Warden
$wardenHeaders = @{ Authorization="Bearer $wardenToken" }
$wardenMe = Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $wardenHeaders
if ($wardenMe.role -ne 'warden') { throw 'Use Warden.' }
$subToken = .\scripts\get-firebase-token.ps1 # Sub-Warden
$subHeaders = @{ Authorization="Bearer $subToken" }
$subMe = Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $subHeaders
if ($subMe.role -ne 'sub_warden') { throw 'Use Sub-Warden.' }

function Assert-HttpError {
 param([scriptblock]$Action,[int]$ExpectedStatus)
 $actual=$null
 try { & $Action | Out-Null } catch {
  if ($null -eq $_.Exception.Response) { throw }
  $actual=[int]$_.Exception.Response.StatusCode
 }
 if ($actual -ne $ExpectedStatus) { throw "Expected $ExpectedStatus; got '$actual'." }
 Write-Host "PASS: expected HTTP $ExpectedStatus."
}
function Send-Complaint {
 param([hashtable]$Headers,[hashtable]$Payload)
 Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints" -Headers $Headers -ContentType 'application/json' -Body ($Payload | ConvertTo-Json)
}
function Send-ComplaintChange {
 param([hashtable]$Headers,[string]$ID,[string]$Action,[hashtable]$Payload)
 Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints/$ID/$Action" -Headers $Headers -ContentType 'application/json' -Body ($Payload | ConvertTo-Json)
}

$payload=@{request_id=[Guid]::NewGuid().ToString();category='maintenance';description='LOCAL-TEST-64 leaking tap'}
$c=Send-Complaint -Headers $studentHeaders -Payload $payload
$c | ConvertTo-Json
$id=$c.complaint_id
if ($c.status -ne 'pending' -or $c.version -ne 1 -or $c.submitted_by -ne $studentMe.user_id) { throw 'Incorrect creation.' }
$retry=Send-Complaint -Headers $studentHeaders -Payload $payload
if ($retry.complaint_id -ne $id) { throw 'Duplicate submission retry.' }
$changed=$payload.Clone();$changed.description='Changed description'
Assert-HttpError -ExpectedStatus 409 -Action { Send-Complaint -Headers $studentHeaders -Payload $changed }

$note=@{request_id=[Guid]::NewGuid().ToString();expected_version=1;text='Water leaks continuously'}
$n=Send-ComplaintChange -Headers $studentHeaders -ID $id -Action notes -Payload $note
if ($n.version -ne 2) { throw 'Incorrect note revision.' }
$n2=Send-ComplaintChange -Headers $studentHeaders -ID $id -Action notes -Payload $note
if ($n2.update_id -ne $n.update_id -or $n2.occurred_at -ne $n.occurred_at) { throw 'Retry changed history.' }

$progress=@{request_id=[Guid]::NewGuid().ToString();expected_version=2;status='in_progress';text='Plumber scheduled'}
Send-ComplaintChange -Headers $subHeaders -ID $id -Action status -Payload $progress | ConvertTo-Json
$stale=@{request_id=[Guid]::NewGuid().ToString();expected_version=2;text='Stale note'}
Assert-HttpError -ExpectedStatus 409 -Action { Send-ComplaintChange -Headers $adminHeaders -ID $id -Action notes -Payload $stale }

$resolve=@{request_id=[Guid]::NewGuid().ToString();expected_version=3;status='resolved';text='Tap repaired'}
Send-ComplaintChange -Headers $wardenHeaders -ID $id -Action status -Payload $resolve | ConvertTo-Json
$lateNote=@{request_id=[Guid]::NewGuid().ToString();expected_version=4;text='New information'}
Assert-HttpError -ExpectedStatus 409 -Action { Send-ComplaintChange -Headers $studentHeaders -ID $id -Action notes -Payload $lateNote }
$reopen=@{request_id=[Guid]::NewGuid().ToString();expected_version=4;status='in_progress';text='Leak returned'}
Send-ComplaintChange -Headers $adminHeaders -ID $id -Action status -Payload $reopen | ConvertTo-Json
$finish=@{request_id=[Guid]::NewGuid().ToString();expected_version=5;status='resolved';text='LOCAL-TEST-64 verification completed'}
Send-ComplaintChange -Headers $adminHeaders -ID $id -Action status -Payload $finish | Out-Null
$history=Invoke-RestMethod -Uri "$baseURL/api/v1/complaints/$id/updates?limit=100" -Headers $studentHeaders
$history | ConvertTo-Json -Depth 5
if ($history.total -ne 6 -or $history.items[0].action -ne 'created' -or $history.items[3].text -ne 'Tap repaired') { throw 'History missing.' }

$d=Send-Complaint -Headers $studentHeaders -Payload @{request_id=[Guid]::NewGuid().ToString();category='disciplinary';description='LOCAL-TEST-64 noise complaint'}
$disciplineID=$d.complaint_id
Assert-HttpError -ExpectedStatus 404 -Action { Invoke-RestMethod -Uri "$baseURL/api/v1/complaints/$disciplineID" -Headers $subHeaders }
Assert-HttpError -ExpectedStatus 404 -Action { Invoke-RestMethod -Uri "$baseURL/api/v1/complaints/$disciplineID/updates" -Headers $subHeaders }
Assert-HttpError -ExpectedStatus 403 -Action { Invoke-RestMethod -Uri "$baseURL/api/v1/complaints?category=disciplinary" -Headers $subHeaders }
$subPage=Invoke-RestMethod -Uri "$baseURL/api/v1/complaints?limit=100" -Headers $subHeaders
if (@($subPage.items | Where-Object category -ne 'maintenance').Count -gt 0) { throw 'Sub-Warden category leak.' }
$forbidden=@{request_id=[Guid]::NewGuid().ToString();expected_version=1;status='resolved';text='Attempt'}
Assert-HttpError -ExpectedStatus 404 -Action { Send-ComplaintChange -Headers $subHeaders -ID $disciplineID -Action status -Payload $forbidden }
Assert-HttpError -ExpectedStatus 403 -Action { Send-ComplaintChange -Headers $studentHeaders -ID $disciplineID -Action status -Payload $forbidden }
Assert-HttpError -ExpectedStatus 403 -Action { Send-Complaint -Headers $adminHeaders -Payload $payload }
Assert-HttpError -ExpectedStatus 401 -Action { Invoke-RestMethod -Uri "$baseURL/api/v1/complaints" }
# Warden can resolve the disciplinary test complaint.
Send-ComplaintChange -Headers $wardenHeaders -ID $disciplineID -Action status -Payload @{request_id=[Guid]::NewGuid().ToString();expected_version=1;status='resolved';text='LOCAL-TEST-64 verification completed'} | Out-Null
```

Additional acceptance checks (use new IDs and current versions):

- A different Student: detail/history/notes for these IDs 404; student_id filtering
  for their owner 403. Their own list must never include another Student's records.
- Blank/overlong descriptions, invalid categories or UUIDs 400. Resolution without
  explanation 400; reopening without explanation 400. Wrong content type 415;
  oversized body 413. Unknown fields such as image_url are 400.
- Filter by status/category/student/creation interval; limit=1 returns at most one
  row. Offset beyond total returns empty items with preserved total and has_more=false.
- Dedicated Student deactivation denies their access but staff can read/handle
  history. Dedicated profile archival blocks submission/notes, not history. Restore
  account status afterward; authenticate again after token revocation.
- Security role reads 403. Sub-Warden disciplinary notes/status/history are all
  denied. Check database tables for preserved initial description and audit actors.

Do not delete the fixtures: history is retained. These are recorded complaint
requests, not actual maintenance tasks; use LOCAL-TEST descriptions.

## Automated verification and completion

```powershell
.\scripts\test-backend-coverage.ps1
```

Creates/removes its own PostgreSQL container; never uses your developer database
for test writes. Covers scope, retries, concurrent creation and competing updates,
eligibility, history, filters and HTTP validation. CI requires the complaint
integration package to pass without skips, runs race detection, and applies/full
rolls back/reapplies migrations including 16. Realtime jobs remain unchanged.

Local verification on 2026-10-13 passed formatting, dependency verification,
vet, build and the complete Go test suite. All eleven required PostgreSQL
integration packages passed without skips. Coverage: 85.3% overall (3322/3893),
84.6% complaint (324/383). Full migration apply/rollback/reapply through 16 and
SQL schema checks passed in a separate disposable database. The focused complaint
lifecycle test also passed after correcting explicit BIGINT version parameter
typing. Local tests did not use race detection; GitHub CI remains pending. The user
reported successful live API verification on 2026-10-13. Tool-run automated tests
used disposable databases; live tests created the user's dedicated development fixtures.

Record actual live results and CI URL on #64. Review and commit only when requested.
Commit subject: feat(complaint): add complaint workflow and clarify module filenames
Commit body: Refs #64
PR title: [DDP-#64] feat(complaint): add complaint workflow and clarify module filenames
PR base: develop. The separate documentation amendment needs its own reviewed
commit; no broad staging of unrelated documentation files.
