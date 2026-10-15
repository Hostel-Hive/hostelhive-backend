# Complaint reports - issue #83

Implements the complaint portion of SRS FR030. Reports are read-only; complaint
submission, notes, status/reopening, images and notifications remain owned by the
existing complaint workflow. PDF/Excel exports (FR031) remain separate.

## API contract

GET /api/v1/reports/complaints is available only to active Admin and Warden,
using existing Firebase authentication and current local account checks.

| Parameter | Meaning |
|---|---|
| from | Required inclusive submission date, exactly YYYY-MM-DD |
| to | Required inclusive submission date, at most 366 calendar days including from |
| student_id | Optional canonical lowercase nonzero Student profile UUID |
| category | Optional maintenance or disciplinary |
| status | Optional pending, in_progress or resolved |
| limit | 1..100; default 20 |
| offset | 0..1000000; default 0 |

Dates select complaints.created_at, not updated_at or resolution time. Dates use
HOSTEL_TIMEZONE, Asia/Colombo by default, with local midnight on from inclusive
and local midnight AFTER to exclusive. Calendar handling is shared with attendance
and leave reports, including leap days and daylight-saving transitions.

A complaint submitted during October appears in October's submission report even
if resolved in November. Status and totals use the complaint's CURRENT state,
not its status on a past date. Later resolution/reopening can change an earlier
submission-period report. No historical status snapshot or resolution-date report
is claimed. Archived/inactive Students retain reportable complaint history.

## Response and totals

JSON fields: from, to, timezone, optional applied student_id/category/status,
summary, total, limit, offset, has_more and items. Items is always an array.

```json
{
  "summary": {
    "by_status": {"pending": 1, "in_progress": 1, "resolved": 2},
    "by_category": {"maintenance": 2, "disciplinary": 2}
  },
  "total": 4
}
```

Both breakdowns describe the SAME matching complaints. Do not add status totals
and category totals together: total = pending + in_progress + resolved, and also
= maintenance + disciplinary. Every filter affects both breakdowns and rows.
Summary covers the entire filtered period independently of pagination. A complaint
with many notes, photos or status updates is counted ONCE. Empty pages retain the
full matching total and summary. Unknown valid Student UUIDs return an empty report with HTTP 200.

Items preserve complaint_id, request_id, student_id, submitted_by, category,
description, status, version, created_at and updated_at. Ordering is created_at
DESC then complaint_id DESC. Times represent RFC3339 instants; timezone describes
the date-filter interpretation. No account credentials, contact/guardian details,
image IDs, storage keys, payload hashes or photo URLs are included.

Each item also has resolution:

```json
{
  "resolution": {
    "update_id": "original-status-update-uuid",
    "resolved_at": "2026-10-15T09:00:00Z",
    "resolved_by": "staff-user-uuid",
    "note": "Recorded resolution note"
  }
}
```

For a currently resolved complaint this is the original audit update for the latest
transition to resolved, selected by audit VERSION rather than wall-clock ordering.
Its recorded note may be empty if the workflow allowed that. It is not a later
comment or a fabricated summary. Reopened/pending complaints have resolution:null.
If resolved again, the newest resolution by version is returned. Legacy/manual
resolved source rows without matching recorded resolution also use null: their
status remains resolved, but missing actor/time/note are not invented. Full prior
resolution/reopening history stays available through existing complaint updates.
No resolution duration, average handling time or SLA statistic is claimed.

## Access, consistency and deployment

Student, Sub-Warden and Security cannot read this report. In particular,
Sub-Warden maintenance-only access remains available through existing complaint
APIs; it does not grant report access. Admin/Warden can report on both categories.
Report access does not expand private-photo access. Photos remain behind the
existing authorized image APIs; there is no direct R2 link in a report.

Missing login returns HTTP 401; valid disallowed/inactive accounts return HTTP 403. Invalid,
missing, empty or duplicated parameters, invalid dates/ranges/UUIDs/filters/page
bounds and GET bodies return HTTP 400 invalid_report_query. Direction is not a complaint
filter. Category remains invalid on attendance and leave report routes. Unknown
query keys are rejected. Database failures return HTTP 503 service_unavailable without
SQL/connection details. Success and error responses use Cache-Control:no-store.

Rows, full-period totals and resolution audit data use ONE SQL statement snapshot.
The transaction rechecks current staff authority under the existing account lock.
Source Student eligibility does not hide historical complaints. Different requests
can observe new complaints/status changes; pages are not a multi-request export
snapshot. Report reads do not modify complaints, versions, notes, images,
notification events/outbox/read state, attendance or Firebase projections.

No new migration: schema 24 is sufficient. Existing complaints_history,
complaints_student and per-complaint version indexes support these queries.
Build and restart the backend to load the new route. No new R2/RTDB settings,
credentials or dependency changes are needed. The Node/npm development-startup
proposal remains separate and is not included in this ticket.

## Verification

Run scripts/test-backend-coverage.ps1. Existing CI runs all Go tests with race
instrumentation and requires all 15 PostgreSQL integration packages without skips.
The report package already exists; complaint report tests extend that required
package rather than adding another required integration package. Existing migration
checks remain at schema 24 because this ticket changes no migrations.

Unit tests cover all category/status combinations, shared period validation,
forwarding and safe error mapping. PostgreSQL/API tests cover local midnight
boundaries, current status/late decisions, multiple updates counted once,
resolution/reopen/re-resolution, version ordering despite clock changes, unspecified
legacy resolution, archive/inactive history, combined filters, pagination/empty
pages, independent status/category totals, role/ownership/category restrictions,
private image metadata exclusion, fresh staff authority and database failures.
Repeated reads compare source rows, image state, queued/delivered notifications,
and recorded attendance/presence for preservation. Existing attendance and leave
report tests also run, including route-specific filter rejection.

For live manual checks, use test accounts and create complaints with distinct
statuses/categories. Choose from/to using created_at, compare totals with SQL,
exercise Admin/Warden success and other-role403, and compare history/presence
before/after reads. An expired token401 is not a passed role restriction; refresh
as the same account and check the expected403. GET-body and database-failure cases
are covered automatically without disrupting the running developer server.

```powershell
$reportURL = "$baseURL/api/v1/reports/complaints"
$query = 'from=2026-10-01&to=2026-10-15'
$report = Invoke-RestMethod -Uri "${reportURL}?${query}&limit=20" -Headers $adminHeaders
$report | ConvertTo-Json -Depth 8
```

Compare identical filters using the API timezone and the day after to:

```sql
SELECT count(*) AS total,
 count(*) FILTER (WHERE status='pending') AS pending,
 count(*) FILTER (WHERE status='in_progress') AS in_progress,
 count(*) FILTER (WHERE status='resolved') AS resolved,
 count(*) FILTER (WHERE category='maintenance') AS maintenance,
 count(*) FILTER (WHERE category='disciplinary') AS disciplinary
FROM hostelhive.complaints
WHERE created_at >= ('2026-10-01'::date::timestamp AT TIME ZONE 'Asia/Colombo')
 AND created_at < ('2026-10-16'::date::timestamp AT TIME ZONE 'Asia/Colombo');
```

Add identical optional Student/category/status predicates when comparing a filtered
report. Source updated_at and resolution dates must not replace created_at here.
Local automated results do not claim live Firebase verification or remote CI.

## Automated evidence - 2026-10-15

Formatting, dependency verification, static analysis, server build, architecture
checks and all Go tests passed. All 15 required real PostgreSQL integration
packages passed without skips against a fresh schema 24 database. Report statement
coverage: 214/231 (92.6%); application-wide: 4555/5346 (85.2%).

The final complaint report PostgreSQL suite includes the legacy/manual resolution
after reopening case, re-resolution by audit version, preserved complaint/image
scope and unchanged source/notification/attendance state. Existing attendance and
leave report regressions passed in the same full run. No migration files changed;
the verification database successfully applied all existing 24 migrations.

Disposable verification containers were removed. The developer database and live
Firebase accounts were not modified. Local verification did not enable race
instrumentation; existing GitHub CI supplies that check. Live manual API tests
and remote CI results must be recorded separately.
