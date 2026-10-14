# Leave notifications — issue #70

Extends #62 leave requests/decisions and #68 private inbox. Migration 19 introduces
shared notification events so both workflows use the same durable outbox, worker,
inbox, unread count and mark-read APIs. No new provider credentials or HTTP routes.
In-app only: email, SMS, FCM/browser/phone push and frontend integration remain
separate work. Approval/rejection never changes scan-based attendance or RTDB.

## Contract

- New leave submission: active Admin/Warden users at that event.
- Actual approval/rejection: the owning Student. No Sub-Warden/Security leave alerts.
- Staff recipients are snapshotted; no historical/new-staff backfill. Decision
  alerts persist even when the owner is inactive or archived. An inactive account
  cannot read its inbox; an active archived Student retains historical reads.
- Current role, recipient ownership and source-specific complaint/leave scope
  govern list, unread count and mark-read. A staff downgrade to Sub-Warden hides
  old leave alerts. Promotion to staff does not expose old Student decision alerts.
- Exact submission/decision retries do not generate new events/intent/inbox items.
  Failed or competing decisions cannot create another final decision alert.
- Generic titles only. Leave reasons and decision notes are available through
  the existing authorised leave API, not notification summaries. No recipient IDs,
  credential material or private image URLs in inbox responses.

### Mixed inbox JSON

The existing endpoints are unchanged:

GET /api/v1/notifications?limit=20&offset=0&unread=true
GET /api/v1/notifications/unread-count
POST /api/v1/notifications/{notificationID}/read (empty body)

Each item now has source_type='complaint' or 'leave'. Complaint items retain their
existing complaint_id/category/kind/title/status fields. Leave items have leave_id
and omit complaint_id/category. Leave kinds are leave_submitted (pending) and
leave_decided (approved/rejected). Generic titles are 'New leave request' and
'Your leave request is approved/rejected'. Other fields retain #68 semantics.
Status is an event snapshot, not a promise that the source record has not changed.

Totals and unread count cover ALL currently visible sources. unread=false means
all visible items, not read-only. There is no source_type/user/recipient query
filter in this ticket; frontend can distinguish sources by the returned field.
Ownership denials are 404; inactive/wrong role 403 (Firebase revocation can yield
401); malformed input 400; unavailable database 503. Responses are no-store.

### Persistence and migration

Migration 19 copies existing complaint events into notification_events without
changing event IDs, notification IDs, created/read timestamps or delivery state.
The existing outbox FK then references that shared event table. Complaint triggers
preserve #68 policy. Leave INSERT and a pending->approved/rejected transition
record source events and recipient intent in the SAME workflow transaction.
There is one submitted and at most one decided event per leave. Event snapshots
retain their original status/time. Existing leave records are NOT backfilled,
but a new decision on an existing Pending leave produces its decision alert.

The #68 worker is unchanged: bounded batches, two-second polling, SKIP LOCKED,
atomic inbox insertion/acknowledgment and safe retry after rollback/restart.
A delivery failure never undoes a committed leave. Failure to persist original
intent prevents its originating transaction from committing to avoid silent loss.

Down19 deletes leave alerts/intent and restores the complaint-only schema, keeping
complaint state. It is for disposable verification/controlled rollback, not
normal cleanup of developer data. Re-upgrade does not resurrect removed leave
history. CI checks an existing schema18 inbox with read/delivered/pending state.

## Terminal 1 — update the running backend

Stop the old server with Ctrl+C. Keep/restore the existing DATABASE_URL, Firebase
project/service-account path, HTTP_ADDR and optional R2/RTDB settings from #68.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
git branch --show-current # feature/70-leave-notifications
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version # 19, clean
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

## Terminal 2 — live checks

Load the Get-TestHeaders and Assert-HttpError helpers from the #68 guide/test
commands. Use fresh active Student/Admin/Warden/Sub-Warden, DIFFERENT Student and
Security credentials. Submitting Student needs an unarchived profile. Use a
future interval that does not overlap their existing Pending/Approved leave.
The samples use one-day requests 30 and 32 days ahead; adjust if occupied.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'

function Wait-LeaveAlert {
    param([hashtable]$Headers,[string]$LeaveID,[string]$Kind,[string]$Status='')
    for ($n=0; $n -lt 15; $n++) {
        $page = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $Headers
        $found = @($page.items | Where-Object {
            $_.source_type -eq 'leave' -and $_.leave_id -eq $LeaveID -and
            $_.kind -eq $Kind -and ($Status -eq '' -or $_.status -eq $Status)
        })
        if ($found.Count) { return $found[0] }
        Start-Sleep -Seconds 1
    }
    throw 'No matching leave alert in newest 100 items within the test window.'
}
function Get-LeaveAlerts {
    param([hashtable]$Headers,[string]$LeaveID)
    $page = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $Headers
    $page.items | Where-Object { $_.source_type -eq 'leave' -and $_.leave_id -eq $LeaveID }
}

$start = [DateTimeOffset]::UtcNow.AddDays(30)
$payload = @{
    request_id = [Guid]::NewGuid().ToString()
    starts_at = $start.ToString('o')
    ends_at = $start.AddDays(1).ToString('o')
    reason = 'LOCAL-TEST-70 family visit'
}
$leave = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves" -Headers $studentHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
$leaveID = $leave.leave_id
$adminAlert = Wait-LeaveAlert -Headers $adminHeaders -LeaveID $leaveID -Kind 'leave_submitted'
$wardenAlert = Wait-LeaveAlert -Headers $wardenHeaders -LeaveID $leaveID -Kind 'leave_submitted'
$adminAlert | ConvertTo-Json

# Exact submission retry: one staff alert per recipient.
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves" -Headers $studentHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json) | Out-Null
foreach ($headers in @($adminHeaders,$wardenHeaders)) {
    if (@(Get-LeaveAlerts -Headers $headers -LeaveID $leaveID).Count -ne 1) { throw 'Duplicate staff alert.' }
}
foreach ($headers in @($subWardenHeaders,$studentHeaders,$otherStudentHeaders)) {
    if (@(Get-LeaveAlerts -Headers $headers -LeaveID $leaveID).Count) { throw 'Wrong leave submission recipient.' }
}

# Each staff member owns a different inbox entry.
$readURL = "$baseURL/api/v1/notifications/$($adminAlert.notification_id)/read"
$read = Invoke-RestMethod -Method Post -Uri $readURL -Headers $adminHeaders
$retry = Invoke-RestMethod -Method Post -Uri $readURL -Headers $adminHeaders
if (-not $read.read_at -or $retry.read_at -ne $read.read_at) { throw 'Read retry changed timestamp.' }
Assert-HttpError -ExpectedStatus 404 -Action {
    Invoke-RestMethod -Method Post -Uri $readURL -Headers $wardenHeaders
}

# Warden approves; Student gets one approval notification.
$approve = @{status='approved';note='LOCAL-TEST-70 approved'} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves/$leaveID/decision" -Headers $wardenHeaders -ContentType 'application/json' -Body $approve | Out-Null
$ownerAlert = Wait-LeaveAlert -Headers $studentHeaders -LeaveID $leaveID -Kind 'leave_decided' -Status 'approved'
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves/$leaveID/decision" -Headers $wardenHeaders -ContentType 'application/json' -Body $approve | Out-Null
if (@(Get-LeaveAlerts -Headers $studentHeaders -LeaveID $leaveID).Count -ne 1) { throw 'Duplicate approval alert.' }
Assert-HttpError -ExpectedStatus 404 -Action {
    Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/notifications/$($ownerAlert.notification_id)/read" -Headers $otherStudentHeaders
}

# A separate nonoverlapping leave is rejected by Admin.
$payload.request_id = [Guid]::NewGuid().ToString()
$payload.starts_at = $start.AddDays(2).ToString('o')
$payload.ends_at = $start.AddDays(3).ToString('o')
$second = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves" -Headers $studentHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
$reject = @{status='rejected';note='LOCAL-TEST-70 insufficient details'} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves/$($second.leave_id)/decision" -Headers $adminHeaders -ContentType 'application/json' -Body $reject | Out-Null
$rejectedAlert = Wait-LeaveAlert -Headers $studentHeaders -LeaveID $second.leave_id -Kind 'leave_decided' -Status 'rejected'
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/leaves/$($second.leave_id)/decision" -Headers $adminHeaders -ContentType 'application/json' -Body $reject | Out-Null
if (@(Get-LeaveAlerts -Headers $studentHeaders -LeaveID $second.leave_id).Count -ne 1) { throw 'Duplicate rejection alert.' }

# Reuse the same generic inbox APIs for both sources.
Invoke-RestMethod -Uri "$baseURL/api/v1/notifications/unread-count" -Headers $studentHeaders
$unread = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?unread=true&limit=100" -Headers $studentHeaders
$ownItems = @($unread.items | Where-Object { $_.leave_id -in @($leaveID,$second.leave_id) })
if ($ownItems.Count -ne 2) { throw 'Expected two unread leave decisions.' }
$public = $ownItems | ConvertTo-Json -Depth 5
if ($public -match 'LOCAL-TEST-70|decision_note|"reason"|"recipient_id"') { throw 'Sensitive details exposed in alerts.' }
Assert-HttpError -ExpectedStatus 403 -Action {
    Invoke-RestMethod -Uri "$baseURL/api/v1/notifications" -Headers $securityHeaders
}
Assert-HttpError -ExpectedStatus 400 -Action {
    Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?source_type=leave" -Headers $studentHeaders
}
Write-Host 'PASS: leave alerts, recipients, privacy and retries.'
Write-Host "Approved leave ID: $leaveID"
Write-Host "Rejected leave ID: $($second.leave_id)"
```

Also verify before/after selected Student presence with the #55 Admin/Warden
presence API (or SQL below): compare status/updated_at/last event, not an entire
history response. No scanner should scan that test Student during this check.
Existing complaint alert IDs/read timestamps must remain after migration19;
repeat a #68 complaint creation/status scenario to confirm mixed-source routing.
Mark a leave alert read and verify it disappears from unread=true, while remaining
in the full inbox. Retest another recipient's marking (404), inactive Student
access (401/403) and dedicated test staff downgrade to Sub-Warden (leave list/count/
mark-read hidden). Restore accounts in finally and get fresh tokens after activation.
These authority/failure/concurrency scenarios are also covered by database tests.

In pgAdmin (use actual fixture UUIDs; do not expose reasons/notes or credentials):

```sql
SELECT e.source_type,e.kind,e.status,e.occurred_at,u.role AS recipient_role,
       o.delivered_at,n.notification_id,n.read_at
FROM hostelhive.notification_events e
JOIN hostelhive.notification_outbox o ON o.event_id=e.event_id
JOIN hostelhive.users u ON u.user_id=o.recipient_id
LEFT JOIN hostelhive.notifications n ON n.event_id=o.event_id AND n.recipient_id=o.recipient_id
WHERE e.leave_id='PASTE_LEAVE_ID'
ORDER BY e.occurred_at,u.role;

SELECT student_id,status,updated_at,device_id,event_id
FROM hostelhive.student_presence
WHERE student_id='PASTE_STUDENT_ID';
```

Multiple active Admin/Warden users each receive a separate submission alert. Do
not assume the whole inbox starts empty or that unread count equals just these
fixtures. Retain notification/leave history. Do not delete rows as normal cleanup.

## Automated checks and completion

```powershell
.\scripts\test-backend-coverage.ps1
git diff --check
git status --short
```

CI runs the existing twelve integration packages, now including leave-notification
coverage, and an explicit schema18->19 preservation fixture. No extra standalone
module or duplicate leave endpoints. Local provider/auth tests use fakes and real
PostgreSQL; live Firebase-authenticated API checks remain separate evidence.
Do not commit/push until requested. Ticket/PR title:
[DDP-#70] feat(notification): add leave request and decision alerts

### Local verification — 2026-10-14

Formatting, dependency verification, static analysis, build, all Go tests and
coverage reporting passed. All twelve required PostgreSQL integration packages
passed without skips. Notification coverage: 137/159 (86.2%); overall application:
3720/4395 (84.6%). Focused notification, leave, complaint and architecture tests
passed. Migration 1–19 full rollback/reapply and populated 18->19 downgrade/
re-upgrade preservation passed in disposable PostgreSQL. Tests verified inbox
ownership, recipient roles, privacy, archived/inactive history, competing decisions,
concurrent delivery, failure recovery, mixed complaint/leave state and unchanged
attendance. Local runs did not enable race detection. Live Firebase-authenticated
API checks and the GitHub CI run remain pending. No commit or push performed.

Live verification: the user reported that all manual #70 commands passed, including recipient alerts, retries, read state, permissions and attendance checks. GitHub CI remains a post-push check.

Issue #72 adds announcement entries to this same mixed inbox while preserving complaint/leave IDs, read state and permissions. See [announcement contract](hostel-announcements.md).
