# Complaint notifications and inbox — issue #68

Migration 18 introduces durable notification delivery intent and a private inbox.
This release delivers in-app notifications, not email, SMS, FCM or browser push.
No Firebase RTDB notification grant or new third-party credentials are needed.
PostgreSQL is authoritative; frontends later call/poll these authenticated APIs.

## Policy and behavior

- NEW maintenance complaint: active Warden and Sub-Warden users at submission.
- NEW disciplinary complaint: active Warden users only.
- Actual status change: the owning Student. The intent is retained even if their
  account is inactive; access remains denied until reactivation. Active Students
  with archived profiles may read their historical notifications, as with complaints.
- Admin can use their own inbox but does not automatically receive these alerts.
  Security cannot access these complaint inbox endpoints. No cross-user admin read.
- Notes and photo additions/removals do not generate notifications in #68.
- New staff accounts do not receive earlier complaints. Migration does not backfill.
- Recipients are snapshotted when the event commits. Every list/count/mark-read
  request rechecks active account, recipient ownership and CURRENT complaint scope.
  A Warden downgraded to Sub-Warden cannot read their old disciplinary alerts.
- Notifications contain an event status/title and complaint ID, not complaint text,
  photo URLs, storage keys, credentials or other users' IDs. Status is the event's
  status, not necessarily the complaint's current status. Fetch the complaint to
  open its current details. List is newest event first with UUID tie-breaker.

## HTTP API

| Method/path | Result |
|---|---|
| GET /api/v1/notifications?limit=20&offset=0&unread=true | Own visible unread inbox page |
| GET /api/v1/notifications | Own visible inbox, default limit 20 |
| GET /api/v1/notifications/unread-count | {"unread": number} |
| POST /api/v1/notifications/{notificationID}/read | Own notification with read_at |

limit 1–100, offset >=0, unread exactly true/false (false means all items).
No recipient/user filters, duplicate or unknown query parameters. GET and POST
bodies must be empty; mark-read/count accept no query parameters. Errors: invalid
input 400; anonymous 401; inactive/wrong role 403; another user's/missing/hidden
notification 404; database failure 503. JSON is no-store. Mark-read is idempotent:
its first timestamp is preserved across retries/concurrent requests. No write API
allows callers to invent or alter notification content or recipients.

## Delivery design

An AFTER INSERT trigger on complaint_updates records event/recipient intent in
notification_outbox in the SAME transaction as the actual complaint workflow
change. Completed complaint retries do not insert updates, so no duplicate alert.
The queue and notifications have UNIQUE(event_id,recipient_id).

A worker immediately processes up to 100 rows and polls every two seconds, with
an eight-second attempt timeout. It locks queued rows with SKIP LOCKED, inserts
inbox entries and acknowledges them in one PostgreSQL transaction. Any failure
rolls back that delivery transaction; the complaint stays committed and intent
remains pending for the next poll/restart. Multiple backend workers are safe.
Delivery is eventual, not a timing guarantee; outages/backlogs delay inbox arrival.

An inability to persist the original intent prevents the complaint transaction
from committing. This is necessary to avoid losing its required notification.
Inbox-delivery failure AFTER that commit never undoes the complaint.

Foreign-key cascades support isolated test teardown. There is no public delete
API for complaints, notification intent or inbox history. Down migration 18 drops
notification history/queue; never use it as developer-data cleanup.

## Startup — Terminal 1

Stop the old executable with Ctrl+C. Restore the same DATABASE_URL, Firebase
project/service-account path and HTTP_ADDR you used for #66 in this terminal.
Existing attendance/R2 settings can remain. No new environment variables needed.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
git branch --show-current # feature/68-complaint-notifications
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version # 18, clean
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

## Live checks — Terminal 2

Use fresh active Student, Warden, Sub-Warden, Admin and DIFFERENT Student headers
from the #66 helpers. A Student needs an unarchived profile to submit. Include
Security headers for denial testing. If 401 occurs, refresh that account's token.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'

function Wait-ComplaintAlert {
    param([hashtable]$Headers,[string]$ComplaintID,[string]$Kind)
    for ($n=0; $n -lt 15; $n++) {
        $page = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $Headers
        $found = @($page.items | Where-Object {
            $_.complaint_id -eq $ComplaintID -and $_.kind -eq $Kind
        })
        if ($found.Count -gt 0) { return $found[0] }
        Start-Sleep -Seconds 1
    }
    throw 'No alert found in newest 100 notifications within the test window.'
}

$payload = @{
    request_id = [Guid]::NewGuid().ToString()
    category = 'maintenance'
    description = 'LOCAL-TEST-68 notification evidence'
}
$c = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints" -Headers $studentHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
$complaintID = $c.complaint_id
$wardenAlert = Wait-ComplaintAlert -Headers $wardenHeaders -ComplaintID $complaintID -Kind 'created'
$subAlert = Wait-ComplaintAlert -Headers $subWardenHeaders -ComplaintID $complaintID -Kind 'created'
$wardenAlert | ConvertTo-Json

# Exact complaint retry: still one alert per recipient.
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints" -Headers $studentHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json) | Out-Null
$page = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $wardenHeaders
if (@($page.items | Where-Object { $_.complaint_id -eq $complaintID -and $_.kind -eq 'created' }).Count -ne 1) { throw 'Duplicate/missing alert.' }

# Mark Warden's own alert read; repeat preserves the timestamp.
$readURL = "$baseURL/api/v1/notifications/$($wardenAlert.notification_id)/read"
$first = Invoke-RestMethod -Method Post -Uri $readURL -Headers $wardenHeaders
$retry = Invoke-RestMethod -Method Post -Uri $readURL -Headers $wardenHeaders
if (-not $first.read_at -or $retry.read_at -ne $first.read_at) { throw 'Read timestamp changed.' }
Invoke-RestMethod -Uri "$baseURL/api/v1/notifications/unread-count" -Headers $wardenHeaders
$unread = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?unread=true&limit=100" -Headers $wardenHeaders
if (@($unread.items | Where-Object { $_.notification_id -eq $wardenAlert.notification_id }).Count) { throw 'Read alert is still unread.' }

# Ownership: even Admin cannot mark Warden's alert read.
foreach ($headers in @($studentHeaders,$otherStudentHeaders,$adminHeaders,$subWardenHeaders)) {
    Assert-HttpError -ExpectedStatus 404 -Action {
        Invoke-RestMethod -Method Post -Uri $readURL -Headers $headers
    }
}

# Actual status change notifies the owning Student once.
$current = Invoke-RestMethod -Uri "$baseURL/api/v1/complaints/$complaintID" -Headers $wardenHeaders
$change = @{
    request_id = [Guid]::NewGuid().ToString()
    expected_version = $current.version
    status = 'in_progress'
    text = 'LOCAL-TEST-68 handling started'
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints/$complaintID/status" -Headers $wardenHeaders -ContentType 'application/json' -Body $change | Out-Null
$studentAlert = Wait-ComplaintAlert -Headers $studentHeaders -ComplaintID $complaintID -Kind 'status'
if ($studentAlert.status -ne 'in_progress') { throw 'Wrong status alert.' }
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints/$complaintID/status" -Headers $wardenHeaders -ContentType 'application/json' -Body $change | Out-Null
$own = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $studentHeaders
if (@($own.items | Where-Object { $_.complaint_id -eq $complaintID -and $_.kind -eq 'status' }).Count -ne 1) { throw 'Duplicate status alert.' }
Assert-HttpError -ExpectedStatus 404 -Action {
    Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/notifications/$($studentAlert.notification_id)/read" -Headers $otherStudentHeaders
}

# Disciplinary: Warden gets an alert, Sub-Warden does not.
$payload.request_id = [Guid]::NewGuid().ToString()
$payload.category = 'disciplinary'
$d = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints" -Headers $studentHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
$disciplinaryAlert = Wait-ComplaintAlert -Headers $wardenHeaders -ComplaintID $d.complaint_id -Kind 'created'
$subPage = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $subWardenHeaders
if (@($subPage.items | Where-Object { $_.complaint_id -eq $d.complaint_id }).Count) { throw 'Disciplinary alert leaked.' }

# Strict API input and role checks.
Assert-HttpError -ExpectedStatus 401 -Action {
    Invoke-RestMethod -Uri "$baseURL/api/v1/notifications"
}
Assert-HttpError -ExpectedStatus 403 -Action {
    Invoke-RestMethod -Uri "$baseURL/api/v1/notifications" -Headers $securityHeaders
}
foreach ($query in @('limit=0','limit=101','offset=-1','unread=1','limit=1&limit=2','recipient_id=invalid','unknown=1')) {
    Assert-HttpError -ExpectedStatus 400 -Action {
        Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?$query" -Headers $studentHeaders
    }
}
Assert-HttpError -ExpectedStatus 400 -Action {
    Invoke-RestMethod -Method Post -Uri $readURL -Headers $wardenHeaders -ContentType 'application/json' -Body '{}'
}
Write-Host 'PASS: recipient policy, ownership, retries and inbox verified.'
Write-Host "Maintenance complaint ID: $complaintID"
```

Before/after unread counts can include unrelated test alerts: compare the selected
notification ID rather than assuming the whole inbox was empty. Extra events due
to concurrent users can arrive between calls. Student photo/note checks from #66
must keep working, without producing new notification intent.

In pgAdmin (replace the fixture UUID; no credentials or complaint text selected):

```sql
SELECT e.action, u.role, o.delivered_at, n.notification_id, n.read_at
FROM hostelhive.notification_outbox o
JOIN hostelhive.complaint_updates e ON e.update_id=o.event_id
JOIN hostelhive.users u ON u.user_id=o.recipient_id
LEFT JOIN hostelhive.notifications n
  ON n.event_id=o.event_id AND n.recipient_id=o.recipient_id
WHERE e.complaint_id='PASTE_MAINTENANCE_COMPLAINT_ID'
ORDER BY e.version,u.role;
```

Expected created rows for Warden/Sub-Warden users and status rows for the owner.
Delivered rows have delivered_at and a notification ID. Retry must not add rows.
Retain these records as test evidence; do not delete inbox/history through SQL.

## Automated verification and CI

```powershell
.\scripts\test-backend-coverage.ps1
```

Uses a disposable database, not the developer DATABASE_URL. CI's coverage evidence
now requires twelve PostgreSQL integration package passes including notification;
missing/skipped notification tests fail the report. The existing workflow already
runs all Go tests and migrations, so no duplicate CI job is needed.

Tests cover committed intent, inbox insertion failure and recovery, cancellation,
concurrent workers, retries, unread filtering, idempotent mark-read, HTTP strict
validation, ownership, active/archived accounts, recipient policy and role downgrade.
Database integration packages run serially (-p 1), as in CI, because fixture staff
receive legitimate notifications from all complaints created while active.

Before commit: run live Firebase login/API checks, review git diff and record the
actual results. Do not claim email/push delivery or frontend integration complete.

### Local results — 2026-10-14

Formatting, dependency verification, static analysis, build, all Go tests and
coverage reporting passed. All twelve required PostgreSQL integration packages
passed without skips. Notification statement coverage: 136/159 (85.5%); overall
application coverage: 3719/4395 (84.6%). Migration 1–18 apply/full rollback/reapply
passed in a disposable database. Focused notification/architecture checks passed.
The integration suite uses local test identities with real PostgreSQL, not live
Firebase credentials. Local runs did not enable race detection; GitHub CI race
checks and live authenticated API verification are still pending. No commit/push.
