# Early exit and late entry - issue #74

> Policy #118 supersedes the old OUT-after20:00 exemption below: both IN and OUT
> before06:00 or at/after20:00 need Admin/Warden approval. See
> [current movement policy](hostel-movement-policy.md). Historical records stay intact.

Students request permission for a specific movement; Admin/Warden decide it.
The authenticated Student owns the request. Neither approval nor rejection changes
attendance, presence or Firebase RTDB counts. Only actual scans establish presence.

## Agreed policy and traceability

Tasheen approved both directions on 2026-10-14:
- OUT before 06:00 requires permission; exactly 06:00 and later do not.
- IN at 20:00 or later requires permission (Students must return before 20:00).
- Overnight IN before 06:00 is treated as a return after the previous evening's
  deadline. This covers a return crossing midnight, rather than treating 01:00
  as ordinary daytime entry.
- OUT after 20:00 is not restricted by this rule. Permission is direction-specific.

All comparisons use hostel local time, regardless of the submitted timestamp's
UTC offset. Defaults are Asia/Colombo, 06:00 and 20:00. Configure at startup:

| Variable | Default | Validation |
|---|---|---|
| HOSTEL_TIMEZONE | Asia/Colombo | IANA zone |
| HOSTEL_EXIT_OPENING | 06:00 | HH:MM |
| HOSTEL_ENTRY_DEADLINE | 20:00 | HH:MM; later than opening |

Windows/minimal containers use Go's embedded IANA timezone data. Explicit empty
or malformed settings fail startup. Configuration changes affect new requests;
existing request/decision history is retained, not automatically re-evaluated.

Sources: SRS UC010/UC011, printed pages 38/39 (PDF46/47), activity Figure3
printed50 (PDF58); SDS sequence4.10/4.11 and LATE_ACCESS entity. Do not relabel
FR023/FR024 to justify late access. Admin decision permission is the user's
approved extension of the Warden use case. See the documentation repository's
late-access amendment for the controlled source-document reconciliation.

This ticket implements submission, review, history and in-app alerts. UC008
unapproved-scan detection, scan-to-permission time tolerance, scheduled reminders,
external push, firmware and frontend integration remain separate. No door is
unlocked and no scanner response is changed by approving a request. The source
diagram's pass-used/expired COMPLETED state is reserved for that integration;
this ticket retains the original approved/rejected decision as history.

## API

| Method/path | Permission |
|---|---|
| POST /api/v1/late-access-requests | Student with active account, unarchived profile |
| GET /api/v1/late-access-requests | Student own; Admin/Warden all |
| GET /api/v1/late-access-requests/{lateAccessID} | Student own; Admin/Warden all |
| POST /api/v1/late-access-requests/{lateAccessID}/decision | Admin/Warden |

Submit JSON: request_id (UUID), direction (IN/OUT), expected_at (RFC3339 with
explicit offset), reason (trimmed 1-1000 Unicode characters). The expected time
must be future and require permission under the current policy. No client
student_id, submitted_by, decision fields or unknown fields are accepted.

Decision JSON: status approved/rejected; note up to 1000 characters. Rejection
requires a nonblank note. Approval requires a future movement and an active
Student account with unarchived profile. Expired/ineligible requests can still
be rejected and historical requests can still be read by authorised accounts.
Sub-Warden and Security have no access. Deactivation denies requests immediately;
archival blocks new submissions/approval but preserves owning Student history.

Submission request_id is scoped to the Student. An exact normalized retry returns
the original record (HTTP201, same as the leave API); changing its payload returns
409. Equivalent offsets normalize to UTC microseconds. A different request UUID
for the same Student/direction/expected_at is blocked while pending/approved;
rejection allows a replacement. Different movement times are distinct requests,
not overlapping leave intervals. No edit/delete/cancel endpoint is exposed.
An exact final-decision retry by its original staff actor preserves decided_at;
other final decisions return409. Student-row locks serialize concurrent requests
and decisions. Account eligibility is checked inside the database transaction.

List filters: student_id, status, limit(1-100;default20), offset(nonnegative).
Newest created_at/ID first; empty pages retain total. Students expanding ownership
get403; another Student's detail is404. Unknown/duplicate query parameters and
bodies on GET are400. Invalid IDs/input/time are400; missing login401; role denial
403; movement/decision conflict409; content type415; body over16KiB413; database
failure503. JSON responses use no-store.

## Delivery and persistence

Migration21 adds late_access_requests and extends notification_events without
replacing existing event IDs, inbox IDs or read/delivery state. Triggers persist
request/decision and delivery intent in the same transaction. The existing worker
retries failed delivery and prevents duplicate alerts. Submission targets active
Admin/Warden accounts; decisions target the owning Student. No historic backfill.

Inbox uses existing endpoints and ownership checks:
- source_type late_access; late_access_id points to the request.
- kind late_access_submitted, status pending, title New late-access request.
- kind late_access_decided, status approved/rejected, decision title.
- complaint_id, leave_id, announcement_id are omitted.
- read_at is per recipient in hostelhive.notifications, not global request state.

## Local API checks

Use dedicated test accounts and retain existing Firebase settings. Restart the
server after rebuilding. Run migrations up only on the development database.
Do not print or commit access tokens. No R2/RTDB settings change is needed.

Terminal1:

```powershell
. {
$ErrorActionPreference = "Stop"
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = "Stop"
$env:HOSTEL_TIMEZONE = "Asia/Colombo"
$env:HOSTEL_EXIT_OPENING = "06:00"
$env:HOSTEL_ENTRY_DEADLINE = "20:00"
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version # 21, clean

go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw "Build failed." }
.\bin\hostelhive-server.exe
}
```

Terminal2: Student identity is inferred by the backend; do not guess student_id.
Paste each entire code block. The dot-sourced block retains variables but stops
at an error, preventing later commands or PASS messages from running.

```powershell
. {
$ErrorActionPreference = "Stop"
$ErrorActionPreference = "Stop"
$baseURL = "http://127.0.0.1:18080"
$studentToken = .\scripts\get-firebase-token.ps1
$studentHeaders = @{ Authorization = "Bearer $studentToken" }
$me = Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $studentHeaders
if ($me.role -ne "student" -or -not $me.is_active) { throw "Use an active Student." }
$adminToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
$adminMe = Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders
if ($adminMe.role -ne "admin" -or -not $adminMe.is_active) { throw "Use an active Admin." }

# Select a future day at 9 PM in Sri Lanka, including its explicit offset.
$date = [DateTimeOffset]::UtcNow.ToOffset([TimeSpan]::FromHours(5.5)).AddDays(30)
$expected = [DateTimeOffset]::new($date.Year, $date.Month, $date.Day, 21, (Get-Random -Minimum 0 -Maximum 60), (Get-Random -Minimum 0 -Maximum 60), [TimeSpan]::FromHours(5.5))
$payload = @{
 request_id = [Guid]::NewGuid().ToString()
 direction = "IN"
 expected_at = $expected.ToString("o")
 reason = "LOCAL-TEST-74 return after travel"
}
$url = "$baseURL/api/v1/late-access-requests"
$before = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $adminHeaders
$request = $null
$request = Invoke-RestMethod -Method Post -Uri $url -Headers $studentHeaders -ContentType "application/json" -Body ($payload | ConvertTo-Json)
if (-not $request.late_access_id -or $request.status -ne "pending" -or $request.submitted_by -ne $me.user_id) { throw "Stop: a valid Pending request from this Student is required." }
$lateAccessID = $request.late_access_id
$retry = Invoke-RestMethod -Method Post -Uri $url -Headers $studentHeaders -ContentType "application/json" -Body ($payload | ConvertTo-Json)
if ($retry.late_access_id -ne $lateAccessID) { throw "Submission retry duplicated the request." }
Invoke-RestMethod -Uri "$url/$lateAccessID" -Headers $adminHeaders
$decision = @{ status = "approved"; note = "LOCAL-TEST-74 approved" } | ConvertTo-Json
$approved = Invoke-RestMethod -Method Post -Uri "$url/$lateAccessID/decision" -Headers $adminHeaders -ContentType "application/json" -Body $decision
$again = Invoke-RestMethod -Method Post -Uri "$url/$lateAccessID/decision" -Headers $adminHeaders -ContentType "application/json" -Body $decision
if ($again.decided_at -ne $approved.decided_at) { throw "Decision retry changed its timestamp." }
# In PowerShell use braces to keep the variable separate from query punctuation.
Invoke-RestMethod -Uri "${url}?status=approved&limit=20" -Headers $studentHeaders
$alert = @()
for ($attempt = 0; $attempt -lt 15; $attempt++) {
 $inbox = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $studentHeaders
 $alert = @($inbox.items | Where-Object { $_.late_access_id -eq $lateAccessID -and $_.kind -eq "late_access_decided" })
 if ($alert.Count -gt 0) { break }
 Start-Sleep -Seconds 1
}
if ($alert.Count -ne 1) { throw "Expected one decision alert; wait and retry if delivery is pending." }
$readURL = "$baseURL/api/v1/notifications/$($alert[0].notification_id)/read"
Invoke-RestMethod -Method Post -Uri $readURL -Headers $studentHeaders
$after = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $adminHeaders
$before | ConvertTo-Json
$after | ConvertTo-Json
# Counts remain equal when no actual scans or account/profile changes occurred.
Write-Host "PASS: submission, retries, approval and Student alert."
}
```

Also test OUT at05:00, rejected requests with a note, a second Student's ownership,
Warden decisions, Security/Sub-Warden denial, a normal-hours request rejected400,
past times, changed-payload retries409 and deactivate/reactivate restoration.
At06:00 an OUT request is400 (permission unnecessary); at20:00 IN is allowed to
be requested. Refresh expired login tokens before testing validation. Automated
integration tests cover these boundaries, database delivery failure/recovery,
concurrent submissions, persistence and unchanged scanned IN/RTDB publication.

## Verification

Run scripts/test-backend-coverage.ps1; CI requires fourteen real PostgreSQL package
passes with no skips. Migration21 upgrade checks preserve the existing complaint,
leave and announcement inbox. Full rollback tests use only disposable databases.
Live Firebase API checks and GitHub CI must be recorded separately. No commit or
push is part of implementation.
### Local automated verification - 2026-10-14

Formatting, dependency verification, static analysis, server build, all Go tests,
architecture checks and coverage evidence validation passed. All fourteen real
PostgreSQL integration packages passed with no skips. Late-access statement
coverage is282/324 (87.0%); application-wide coverage is4224/4976 (84.9%).
Tests include ownership, role/account/profile eligibility, time boundaries,
canonical retry identities, competing final decisions, delivery failure/recovery,
strict HTTP inputs and preservation of scanned IN/RTDB publication state.
Populated20->21 downgrade/re-upgrade preserved complaint/leave/announcement inbox
IDs, delivery and read state. Full1-21 rollback/reapplication passed in disposable
PostgreSQL. Containers were removed; developer database was not modified.
Local tests use fake identities (not live Firebase) and did not enable race
instrumentation. Live Firebase verification remains separately recorded; GitHub CI is pending.
### Manual verification notes

The user confirmed the live ownership/role denial checks after refreshing rejected
login tokens. Full command-by-command verification instructions were supplied;
no consolidated final result for all seventeen live steps was provided before
the commit request. Do not infer a complete live pass from an unconditional PASS
message following earlier errors.

Each account has its own token. Refresh rejected logins as the same account and
verify /api/v1/me before continuing. An unexpected401 does not prove a403/404
permission check passed. Deactivation checks intentionally expect401/403 and
must not try signing in as the inactive Student during that check. Restore the
account in finally, then obtain a fresh Student login. Clear request variables
and verify a new Pending request ID before deactivating or deciding. The corrected
interactive commands refresh normal rejected logins once, preserve identity and
stop each numbered block on failure; they are not a single-run live test script.
## Extension in #76
[Attendance permission checks](attendance-permission-checks.md) now consume an approved request once for a recorded matching direction within30 minutes either side of expected_at. Existing detail/list exposes consumed_at,consumed_device_id,consumed_event_id; the original approved decision remains history. Unapproved incidents never prevent actual attendance recording.

## Pending request cancellation (#101)

Migration28 adds owner-only cancellation and the terminal `cancelled` status.
See [the cancellation contract](pass-cancellation.md) for routes, audit, retries,
permissions and race handling. Earlier ticket-scope exclusions above are historical.
