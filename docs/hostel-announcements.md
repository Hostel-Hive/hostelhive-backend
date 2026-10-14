# Hostel announcements — issue #72

Admin/Warden publish global, immutable plain-text announcements. Active Students
with unarchived profiles list/view published notices and receive inbox alerts.
The existing mixed inbox, unread count, mark-read and delivery worker are reused.
Staff have aggregate delivery/read counts. No new provider credentials required.

## Requirements and selected scope

SRS FR028 describes global broadcasts; FR029 describes Student mobile delivery.
SRS PDF page 34 requires title, content, date and target audience; PDF page 63
(printed page 55, Figure 8) shows Admin/Warden composition, normal alerts and
read receipts. SDS describes announcement title/priority. #72 implements backend
publication, global feed/detail, normal-priority inbox alerts and aggregate read
receipts. Mobile frontend integration and external push delivery remain separate.
Do not claim the complete mobile/push use case is finished by backend inbox tests.

- Audience is all_students only, omitted audience defaults to that value. No
  block/room-specific or individual targeting in this release.
- Title 1–160 and content 1–5000 Unicode characters after trimming. Plain text;
  multiline content allowed, control characters rejected. Title is single-line.
- Priority is normal; urgent/escalated delivery is not introduced in this ticket.
- Publication is immediate. No drafts, schedule, edit, delete or unpublish API.
  Correct a published message by publishing a new notice with a new request ID.
- Admin/Warden publish and view all notices/summary. Current eligible Students
  read the global feed/detail. Sub-Warden/Security cannot access announcement APIs.
  They are not announcement notification recipients; Sub-Warden's existing
  complaint inbox access remains unchanged.
- Student recipients are snapshotted at publication: active student-role accounts
  with unarchived profiles. Missing/archived/inactive/non-student profiles excluded.
- Later enrolled Students may view historical global notices, but receive no
  retroactive alert. Exact publication retries never resnapshot the audience.
- Current role/account/profile eligibility applies to announcement inbox list,
  count and marking. Archival hides announcement alerts while complaint/leave
  historical access remains as previously documented. Read timestamps survive
  later role/archive changes. Re-eligibility makes the existing alert visible again.
- Published content remains available even if inbox delivery fails. Retries use
  the existing atomic inbox insertion/acknowledgment, two-second polling worker.

## APIs

| Endpoint | Role/purpose |
|---|---|
| POST /api/v1/announcements | Admin/Warden publish |
| GET /api/v1/announcements?limit=20&offset=0 | Global eligible reader feed |
| GET /api/v1/announcements/{announcementID} | Global eligible reader detail |
| GET /api/v1/announcements/{announcementID}/delivery-summary | Admin/Warden aggregate receipts |

POST JSON: request_id UUID, title, content, optional audience='all_students'.
Publisher identity/date are derived from server authentication/clock. First POST
returns 201 with {announcement:{...},replayed:false}; exact canonical retry by
same publisher returns 200/replayed:true with the original ID/date. Changed reuse
409. Request IDs are publisher-scoped; another publisher has a separate namespace.
Unknown fields (including published_by/priority), trailing JSON, invalid text/IDs
400; wrong Content-Type 415; body >32768 bytes 413. No POST query parameters.

Feed supports limit 1–100, offset >=0, default limit20, newest publication first,
UUID tie-breaker; items/total/limit/offset/has_more. No duplicate/unknown filters.
GET bodies empty; detail/summary accept no query. Anonymous401, forbidden/inactive/
archived403 (revoked Firebase token may401), missing404, unavailable DB503.
Responses no-store. Render title/content as text in clients, not executable HTML.

Notifications add announcement_id and source_type='announcement',
kind='announcement_published', status='published', title=the published headline.
They omit complaint_id/leave_id/category. Content is fetched through announcement
GET. Existing inbox marking controls read_at; GET detail does NOT automatically
mark read (preloading a detail is not a deliberate read). Frontend calls mark-read
when the user opens the notice. Read counts reflect declared inbox reads, not
proof that the person understood the message.

Summary: announcement_id, recipients_total, delivered, pending_delivery, read,
unread. Original targeted audience is retained in counts, including later inactive/
archived recipients. delivered+pending_delivery=recipients_total;
read+unread=delivered. No individual recipient IDs/addresses/read roster is exposed.
An empty audience is a valid publication with zero counts.

## Persistence and concurrency

Migration20 adds announcements and a source-specific announcement reference to
notification_events. Existing complaint/leave event and inbox IDs/read/delivery
state are unchanged. A publication trigger writes one event and recipient intent
in the same transaction. Request UUID serialization and unique constraints prevent
concurrent duplicate publication. User authority and membership are locked while
snapshotting recipients; Student membership is locked during inbox/read checks.
Delivery failure cannot undo publication. Failure to persist original intent
prevents commit, avoiding silent alert loss.

Down20 removes announcement data and related alerts only; preserves complaint/
leave state. Use only for disposable checks or a controlled rollback, not normal
cleanup. Reapply does not restore dropped notices. Source migrations are unchanged.

## Terminal 1 — startup

Stop the previous server with Ctrl+C. Preserve/restore DATABASE_URL, Firebase
project/service-account path and HTTP_ADDR from #70. R2/RTDB settings can remain.
No new keys/configuration required.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
git branch --show-current # feature/72-hostel-announcements
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version # 20, clean
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

## Terminal 2 — live verification

Use Get-TestHeaders/Assert-HttpError from #70. Fresh active Admin, Warden and two
Student headers plus Sub-Warden/Security for denial checks. Students need
unarchived profiles. If a token expires, refresh that account before interpreting
401 as a business failure. Each eligible Student gets their own notification ID.

```powershell
$baseURL = 'http://127.0.0.1:18080'
$payload = @{
    request_id = [Guid]::NewGuid().ToString()
    title = 'LOCAL-TEST-72 water supply notice'
    content = 'Water will be unavailable tomorrow from 9 AM to 11 AM.'
    audience = 'all_students'
}
$published = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/announcements" -Headers $wardenHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
$announcementID = $published.announcement.announcement_id
if ($published.replayed) { throw 'Expected a new publication.' }
$retry = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/announcements" -Headers $wardenHeaders -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
if (-not $retry.replayed -or $retry.announcement.announcement_id -ne $announcementID -or $retry.announcement.published_at -ne $published.announcement.published_at) { throw 'Unsafe publication retry.' }
$bad = $payload.Clone(); $bad.content = 'Changed body with the same request ID'
Assert-HttpError -ExpectedStatus 409 -Action {
    Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/announcements" -Headers $wardenHeaders -ContentType 'application/json' -Body ($bad | ConvertTo-Json)
}

foreach ($headers in @($adminHeaders,$wardenHeaders,$studentHeaders,$otherStudentHeaders)) {
    $detail = Invoke-RestMethod -Uri "$baseURL/api/v1/announcements/$announcementID" -Headers $headers
    if ($detail.content -ne $payload.content) { throw 'Different announcement detail.' }
}
$page = Invoke-RestMethod -Uri "$baseURL/api/v1/announcements?limit=1&offset=0" -Headers $studentHeaders
if ($page.items.Count -ne 1 -or $page.total -lt 1) { throw 'Incorrect feed page.' }

function Wait-AnnouncementAlert {
    param([hashtable]$Headers,[string]$AnnouncementID)
    for ($n=0; $n -lt 15; $n++) {
        $page = Invoke-RestMethod -Uri "$baseURL/api/v1/notifications?limit=100" -Headers $Headers
        $alerts = @($page.items | Where-Object { $_.source_type -eq 'announcement' -and $_.announcement_id -eq $AnnouncementID })
        if ($alerts.Count) {
            if ($alerts.Count -ne 1) { throw 'Duplicate announcement alert.' }
            return $alerts[0]
        }
        Start-Sleep -Seconds 1
    }
    throw 'No announcement alert in newest 100 inbox entries within test window.'
}
$studentAlert = Wait-AnnouncementAlert -Headers $studentHeaders -AnnouncementID $announcementID
$otherAlert = Wait-AnnouncementAlert -Headers $otherStudentHeaders -AnnouncementID $announcementID
if ($studentAlert.notification_id -eq $otherAlert.notification_id) { throw 'Recipients share an inbox entry.' }

# Deliberate read uses the existing inbox endpoint; fetching detail alone does not.
$readURL = "$baseURL/api/v1/notifications/$($studentAlert.notification_id)/read"
$read = Invoke-RestMethod -Method Post -Uri $readURL -Headers $studentHeaders
$readRetry = Invoke-RestMethod -Method Post -Uri $readURL -Headers $studentHeaders
if (-not $read.read_at -or $readRetry.read_at -ne $read.read_at) { throw 'Read retry changed timestamp.' }
Assert-HttpError -ExpectedStatus 404 -Action {
    Invoke-RestMethod -Method Post -Uri $readURL -Headers $otherStudentHeaders
}
$summary = Invoke-RestMethod -Uri "$baseURL/api/v1/announcements/$announcementID/delivery-summary" -Headers $adminHeaders
$summary | ConvertTo-Json
if ($summary.recipients_total -ne ($summary.delivered+$summary.pending_delivery) -or $summary.delivered -ne ($summary.read+$summary.unread) -or $summary.read -lt 1) { throw 'Summary counts incorrect.' }

# Permissions: read summary is staff-only, publication is Admin/Warden-only.
Assert-HttpError -ExpectedStatus 403 -Action {
    Invoke-RestMethod -Uri "$baseURL/api/v1/announcements/$announcementID/delivery-summary" -Headers $studentHeaders
}
foreach ($headers in @($studentHeaders,$subWardenHeaders,$securityHeaders)) {
    Assert-HttpError -ExpectedStatus 403 -Action {
        Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/announcements" -Headers $headers -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
    }
}
foreach ($headers in @($subWardenHeaders,$securityHeaders)) {
    Assert-HttpError -ExpectedStatus 403 -Action {
        Invoke-RestMethod -Uri "$baseURL/api/v1/announcements/$announcementID" -Headers $headers
    }
}
Assert-HttpError -ExpectedStatus 401 -Action {
    Invoke-RestMethod -Uri "$baseURL/api/v1/announcements"
}
foreach ($query in @('limit=0','limit=101','offset=-1','limit=1&limit=2','audience=staff','unknown=1')) {
    Assert-HttpError -ExpectedStatus 400 -Action {
        Invoke-RestMethod -Uri "$baseURL/api/v1/announcements?$query" -Headers $studentHeaders
    }
}
foreach ($mutation in @('title','content','audience')) {
    $bad = $payload.Clone(); $bad.request_id = [Guid]::NewGuid().ToString()
    if ($mutation -eq 'audience') { $bad.audience='staff' } else { $bad[$mutation]=' ' }
    Assert-HttpError -ExpectedStatus 400 -Action {
        Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/announcements" -Headers $adminHeaders -ContentType 'application/json' -Body ($bad | ConvertTo-Json)
    }
}
Write-Host 'PASS: announcements, student alerts, receipts and permissions.'
Write-Host "Announcement ID: $announcementID"
```

Also check a second publication from Admin, archive/deactivate a dedicated Student
and confirm feed/inbox access changes without losing stored read state, restart
backend and confirm persistence, and repeat #68/#70 complaint/leave alert scenarios.
Always restore test accounts/profiles afterward. Automated tests cover missing/
archived/inactive profiles, role changes, no retroactive alerts to new Students,
concurrent retries, delivery failure/recovery and concurrent archival.

In pgAdmin (replace fixture UUID; no tokens or addresses selected):

```sql
SELECT announcement_id,title,audience,priority,published_by,published_at
FROM hostelhive.announcements WHERE announcement_id='PASTE_ANNOUNCEMENT_ID';
SELECT e.kind,e.status,o.delivered_at,n.notification_id,n.read_at
FROM hostelhive.notification_events e
JOIN hostelhive.notification_outbox o ON o.event_id=e.event_id
LEFT JOIN hostelhive.notifications n ON n.event_id=o.event_id AND n.recipient_id=o.recipient_id
WHERE e.announcement_id='PASTE_ANNOUNCEMENT_ID';
```

Do not assume there are exactly two recipients: all eligible Student profiles at
publication are targeted. No public deletion API; retain test publications/audit.

## Automated verification

```powershell
.\scripts\test-backend-coverage.ps1
git diff --check
git status --short
```

CI requires thirteen PostgreSQL integration package passes including announcement,
plus schema19->20 mixed-inbox preservation and the prior upgrade checks. Race
checks remain in GitHub CI. Record actual live API results before committing.
No commit/push until requested. Suggested PR title:
[DDP-#72] feat(announcement): add hostel announcements and inbox alerts

### Local verification — 2026-10-14

Formatting, dependency verification, static analysis, build, all Go tests and
coverage reporting passed. All thirteen required PostgreSQL integration packages
passed without skips. Announcement statement coverage: 200/231 (86.6%); overall
application coverage: 3925/4633 (84.7%). Notification coverage: 142/165 (86.1%).
Focused announcement/notification/leave/complaint regressions and architecture
checks passed. Migrations 1–20 full rollback/reapply and populated 19->20
upgrade/downgrade/re-upgrade preservation passed in disposable PostgreSQL.
Tests covered current recipient eligibility, concurrent archival, publication
retries, ownership, private aggregate receipts, failed-delivery recovery, strict
HTTP inputs and existing mixed-inbox behavior. Local tests used test identities
and did not enable race detection. The user reported all live Firebase-authenticated API test commands passed.
GitHub CI remains pending.
