# Attendance permission checks - issue #76

This extends attendance and the #74 late-access workflow; it is not a new module.
Tasheen approved a 30-minute window on either side of the expected movement and
single use per approved request. PostgreSQL remains authoritative. Scanners
continue reporting only event_id and QR; no client time/direction/approval is trusted.

## Rules

The same HOSTEL_TIMEZONE, HOSTEL_EXIT_OPENING and HOSTEL_ENTRY_DEADLINE configuration
is used by submissions and scans. Defaults: Asia/Colombo,06:00,20:00.
- OUT before06:00 requires permission; OUT from06:00 onward does not.
- IN from20:00 onward and overnight before06:00 requires permission.
- IN at06:00 through19:59:59 does not require permission.

Only outcome=recorded is assessed as a movement. Cooldown/already-in/already-out
scans neither consume permission nor create new incidents. Existing suppression
rules and the ten-second default cooldown are unchanged.

A matching request must belong to the same Student and direction, be approved
before the scan, be unused, and have expected_at within scan time minus/plus30
minutes (inclusive, compared as absolute instants). This handles midnight and
submitted timezone offsets without guessing dates. Pending/rejected, another
Student/direction, consumed, future-decision or outside-window requests do not match.
If several approvals qualify, consume the closest expected time, then expected_at,
created_at and request UUID as deterministic tie-breakers. One event consumes one
approval; overlapping approval windows are not combined or all consumed.

A matching recorded movement consumes its grant even just before curfew: approval
for20:10 and arrival19:55 uses that permission. Otherwise a normal-hours movement
records not_required without consuming an unrelated/out-of-window approval.
A restricted recorded movement without a match records an incident. It STILL
updates actual IN/OUT presence and queues the existing Firebase projection.
An incident is a staff review record, not a door denial or proof of physical passage.

Single use is recorded with consumed_at, consumed_device_id and consumed_event_id
on late_access_requests. The original approved decision/status is preserved as
history; consumption is separate from that decision. Owning Students and staff
see consumption through the existing late-access detail/list APIs. No grant
cancellation, second use, automatic reapproval or resetting use is exposed.

## Atomicity and retries

Existing device authentication/version checks and Student eligibility locks stay
inside the scan transaction. The Student row serializes scans across readers and
staff decisions. Scan, presence, permission use, incident and notification intent
commit together. Failed intent/consumption/classification rolls the whole movement
back and returns503; the scanner can safely retry its event ID. Actual delivery is
asynchronous: a delivery outage leaves the committed movement and durable intent.

The same device/event ID and QR returns the original event, timestamp, access result,
permission and incident. A changed QR conflicts409. Approvals issued later, policy
changes and profile archival never reclassify a retry. Device rotation/deactivation
still rejects stale/currently unauthorized credentials before returning history.
Replays cannot consume another permission or create another incident/alert.

Migration22 does not backfill old scans, create retroactive incidents, or consume
old approvals. Legacy events omit access_result. Rebuild/restart the backend after
migration; the new behavior requires the new executable on each serving instance.
Downgrade discards #76 classifications/use metadata/incidents and their alerts;
recorded movement/presence and prior notification sources remain. Rollback checks
are for disposable databases, not the developer database.

## API

Existing POST /api/v1/device/scans and GET /api/v1/attendance/events add:

| access_result | Meaning |
|---|---|
| approved | This recorded movement consumed late_access_id |
| unapproved | This restricted movement created incident_id |
| not_required | Recorded normal-hours movement without a matching grant |
| not_evaluated | Cooldown/already-IN/OUT suppression; no consumption/incident |

Only applicable IDs are returned. Historical events omit these new fields.
Incident types: late_entry (IN) and early_exit (OUT).

Admin/Warden only:
- GET /api/v1/attendance/incidents
- GET /api/v1/attendance/incidents/{incidentID}

List filters: student_id, direction(IN/OUT), incident_type(late_entry/early_exit),
limit(default20,max100),offset(0..1000000). Newest occurred_at/UUID first. A single
statement snapshot preserves total on empty pages. Incompatible valid filters
return an empty page, not an input error. Unknown/duplicate/empty filters, malformed
UUIDs and bodies on GET are400. Missing login401; forbidden roles403; missing detail
404; database failure503. Responses are JSON with no-store. Archival/deactivation
preserves staff incident history; current staff account/role is rechecked.
Records include the original Student/device/event IDs, direction/time and policy
snapshot (timezone, hour thresholds in minutes, window in seconds). No edit/delete,
incident resolution, student incident API or new global analytics is claimed.

## Inbox

Each incident transaction snapshots active Admin/Warden recipients. Existing inbox
list/count/mark-read uses recipient ownership AND current Admin/Warden permission.
Students, Sub-Wardens and Security cannot receive/read these incident alerts.
- source_type attendance
- attendance_incident_id identifies the staff incident detail
- kind attendance_incident; status unapproved
- title Late entry without permission / Early exit without permission
- complaint/leave/announcement/late_access parent IDs are omitted

One incident per device/event, one notification event per incident and the existing
unique recipient inbox delivery prevent duplicates. Reading is per recipient and
keeps its first timestamp. No email/push provider or scanner firmware is added.

## Verification

Run scripts/test-backend-coverage.ps1; fourteen PostgreSQL packages remain required.
New attendance tests exercise both directions, matching and window exclusions,
single use, before-curfew consumption, competing readers, suppression, retry after
later approval/policy change, atomic rollback, failed delivery recovery, protected
incident APIs and consumption through late-access history. Existing attendance,
RTDB, leave, complaint and inbox regressions run against the new schema. CI adds
populated21->22 preservation and complete migration rollback/reapply checks.

### Live command-by-command preparation

Use dedicated test Students and private scanner credentials. Do not print tokens.
In Terminal1 stop the old server with Ctrl+C, keep PostgreSQL/Firebase configuration
and run this WHOLE block:

```powershell
. {
 $ErrorActionPreference = "Stop"
 cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
 .\scripts\migrate.ps1 up
 .\scripts\migrate.ps1 version #22,clean
 go build -o .\bin\hostelhive-server.exe .\cmd\server
 if ($LASTEXITCODE -ne 0) { throw "Build failed." }
 .\bin\hostelhive-server.exe
}
```

Normal-hours scans should record not_required when no matching permission exists.
To verify curfew behavior now, either test at actual restricted hours or use a
local-only controlled policy shared by both request and scan endpoints. For
example HOSTEL_EXIT_OPENING=23:58, HOSTEL_ENTRY_DEADLINE=23:59 makes early OUT and
overnight IN restricted for most of the local day (except the normal minute23:58).
Apply the test hours BEFORE restarting and submit/approve a request for15 minutes
from now. Avoid a future expected time in that one normal minute; it needs no
permission and submission correctly fails400. Restore06:00/20:00 after testing.
Do not fake occurred_at or rewrite real attendance timestamps to force a match.

In Terminal2, use one numbered block at a time with '. { ... }', stop on errors,
clear result variables before required requests, and check IDs/status before
continuing. If a login is rejected, refresh the SAME account and retry; do not
count unexpected401 as a403/404 permission-test pass. Use the backend-derived
Student ID rather than choosing one manually. A complete live test should check:

1. Student request and Admin/Warden approval15 minutes ahead, then matching scan.
2. consumed_at and device/event IDs on the original request; repeat the same scan
   and verify identical IDs/timestamp/result and unchanged use count.
3. Another IN without actual OUT: suppression, no second consumption/incident.
4. OUT then a second restricted IN: used entry permission does not match; incident
   created while presence correctly becomes IN.
5. Separate approved OUT, and unapproved OUT with correct early_exit incident.
6. Admin/Warden list/detail and their independent inbox alerts/read timestamps.
7. Student/Sub-Warden/Security denial and anonymous401; strict query validation.
8. Server restart persistence; current device rotation/deactivation denial.
9. PostgreSQL records and RTDB presence updates; restore normal test hours.

SQL, using the IDs from API responses:

```sql
SELECT event_id,device_id,student_id,direction,outcome,access_result,late_access_id,occurred_at
FROM hostelhive.attendance_events WHERE student_id='PASTE_STUDENT_ID' ORDER BY occurred_at;
SELECT late_access_id,status,expected_at,consumed_at,consumed_device_id,consumed_event_id
FROM hostelhive.late_access_requests WHERE student_id='PASTE_STUDENT_ID' ORDER BY created_at;
SELECT * FROM hostelhive.attendance_incidents WHERE student_id='PASTE_STUDENT_ID' ORDER BY occurred_at;
SELECT e.attendance_incident_id,o.recipient_id,o.delivered_at,n.notification_id,n.read_at
FROM hostelhive.notification_events e JOIN hostelhive.notification_outbox o USING(event_id)
LEFT JOIN hostelhive.notifications n ON n.event_id=o.event_id AND n.recipient_id=o.recipient_id
WHERE e.attendance_incident_id='PASTE_INCIDENT_ID';
```

Live Firebase/device checks and GitHub CI remain separate evidence. No commit/push
is performed until requested. Controlled SRS/SDS reconciliation is recorded in the
documentation repository's attendance-permission-checks amendment.
### Local automated evidence - 2026-10-14

Dependency verification, formatting, static analysis, server build, architecture
checks and all Go tests passed. All fourteen required real PostgreSQL integration
packages passed without skips on a fresh schema22 database. Attendance statement
coverage:839/988 (84.9%); application-wide coverage:4339/5112 (84.9%). Existing
complaint/leave/announcement/late-access and realtime regressions passed.
Populated21->22 preservation verifies original scans, approvals, presence and all
four prior inbox sources. Downgrade/re-upgrade and full1-22 rollback/reapply passed
in disposable databases. The final schema includes the unique event permission
reference and nullable legacy-result consistency backstop, both exercised in the
new integration tests. The existing scan retry assertions now compare event values
rather than addresses of newly optional ID fields. No real developer database,
Firebase account or scanner credential was modified by these automated checks.
Local tests do not enable race instrumentation; GitHub CI supplies race checks.
Live Firebase/device/RTDB verification and CI remain pending. No commit or push
performed. All disposable verification containers were removed.