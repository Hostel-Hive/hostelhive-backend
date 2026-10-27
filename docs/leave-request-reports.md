# Leave request reports - issue #81

Implements the leave portion of SRS FR030. Existing leave submission/decision
APIs, attendance scans and notification workflows remain authoritative and unchanged.

## API contract

GET /api/v1/reports/leaves is available only to active Admin and Warden accounts
through existing Firebase authentication and local account checks.

| Parameter | Meaning |
|---|---|
| from | Required inclusive submission date, exactly YYYY-MM-DD |
| to | Required inclusive submission date, at most 366 calendar days including from |
| student_id | Optional canonical lowercase nonzero Student profile UUID |
| status | Optional pending, approved, rejected or cancelled; case-sensitive |
| limit | 1..100; default 20 |
| offset | 0..1000000; default 0 |

Dates refer to created_at (when the Student submitted the request), using
HOSTEL_TIMEZONE, Asia/Colombo by default. The range starts at local midnight on
from and ends immediately before local midnight following to. Leave start/end
and decision dates do not determine whether a request appears. Calendar handling
is shared with attendance reports, including leap days and daylight saving.

Example: from=2026-10-01&to=2026-10-15 includes requests SUBMITTED during those
dates, even if their requested leave starts in November or staff decided later.

Status filtering and totals use each request's CURRENT status. This is not a
historical snapshot of how many requests were pending on a past date. A later
decision can therefore change the report for an earlier submission period.

## Response

JSON fields: from, to, timezone, optional applied student_id/status, summary,
total, limit, offset, has_more and items. Items is always an array, including [].
Summary contains pending, approved, rejected and cancelled counts across ALL matching
requests, independent of page size and offset. Every filter, including status,
affects both rows and summary. total = pending + approved + rejected + cancelled.

Items include leave_id, request_id, student_id, submitted_by, starts_at, ends_at,
reason, status, created_at, decided_at, decided_by and decision_note. Pending
requests retain null decided_at/decided_by and an empty decision_note. Decided
requests retain the original decision actor, time and note. Times are RFC3339
instants; the response timezone describes the date-filter interpretation.
No Firebase credentials, profile contact details or guardian data are exposed.

Ordering is created_at DESC, then leave_id DESC. A page past the end returns
empty items with the full matching total and summary and has_more=false.
An unknown valid Student UUID returns 200 with no requests and zero totals.
Archived profiles and inactive/changed-role source accounts retain their history.

Successful and error responses use Cache-Control: no-store. Missing authentication
returns 401; Student, Sub-Warden, Security and inactive accounts cannot read the
report (403 with valid account authentication). Invalid dates, reversed/oversized
ranges, invalid UUID/status/page bounds, unknown/duplicate/empty parameters and
GET request bodies return 400 invalid_report_query. The attendance-only direction
filter is not accepted. Database failures return 503 service_unavailable without
SQL or connection details.

## Consistency and deployment

The repository rechecks current staff authority in its transaction under the
account-change lock. One SQL statement snapshot selects matching rows, summary
and total, including empty pages. Separate requests may see newly committed
requests or decisions; pagination is not a multi-request export snapshot.

Apply migration 24 before starting the new server. It adds the submission-date
report index on leave_requests(created_at DESC,leave_id DESC). It changes no
request data and adds no report table. Its rollback removes only this index.
Always test rollback in a disposable database, not the developer database.

The report module shares date validation, strict HTTP query parsing, response
mapping and transaction authorization between attendance and leave reports.
Feature files remain descriptive and source-module repositories are not imported.
Reports do not write leave decisions, presence, attendance events, incidents,
notifications, permission consumption or Firebase projections. Approval continues
to leave attendance unchanged until a scan.

## Verification

Run scripts/test-backend-coverage.ps1 and scripts/test-migrations.ps1. Existing
CI requires the report PostgreSQL package (15 integration packages overall) to
pass without skips; its race-enabled Go job includes the new tests automatically.
No extra integration package or provider configuration is required.

Unit tests cover forwarding, status validation and error mapping; shared calendar
tests retain strict ranges, leap years and 23/25-hour local days. PostgreSQL/API
tests cover inclusive/exclusive midnight boundaries, submission-versus-leave dates,
current decisions made after the selected period, archived/inactive history,
nullable decision metadata, stable pagination, empty pages, combined filters,
Admin/Warden access, denied roles, fresh transaction authority, invalid inputs,
database failures and unchanged source/notification state across repeated reads.

For manual API verification, sign in as Admin and Warden and request:

```powershell
$reportURL = "$baseURL/api/v1/reports/leaves"
$fromDate = '2026-10-01'
$toDate = '2026-10-15'
$query = "from=$fromDate&to=$toDate"
$report = Invoke-RestMethod -Uri "${reportURL}?${query}&limit=20&offset=0" -Headers $adminHeaders
$report | ConvertTo-Json -Depth 8
if ($report.total -ne ($report.summary.pending + $report.summary.approved + $report.summary.rejected + $report.summary.cancelled)) {
    throw 'FAIL: status totals do not add up.'
}
```

Use dates containing your test requests' created_at values, not their leave dates.
Compare status/Student filters, a limit=1 page and an offset beyond the total;
check Warden access and exact 403 denials for other active roles. Refresh expired
Firebase tokens as the same account before retrying tests: 401 does not prove a
role restriction. In pgAdmin compare leave_requests rows and scan-based presence
before/after reads. Local automated results do not claim live Firebase login,
manual API verification, hosted HTTPS or remote CI execution.

Complaint reports and PDF/Excel exports are separate tickets.

Compare the API summary with the same submission-date range in pgAdmin:

```sql
SELECT status, count(*) AS requests
FROM hostelhive.leave_requests
WHERE created_at >= ('2026-10-01'::date::timestamp AT TIME ZONE 'Asia/Colombo')
  AND created_at < ('2026-10-16'::date::timestamp AT TIME ZONE 'Asia/Colombo')
GROUP BY status;
```

Use the API's returned timezone and the day AFTER to as the exclusive upper
boundary. Add identical Student/status predicates when comparing a filtered API
request. A missing status group means zero requests in that status.

## Automated evidence - 2026-10-15

Formatting, dependency verification, static analysis, server build, architecture
checks and all Go tests passed. All 15 required real PostgreSQL integration
packages passed without skips on a fresh schema24 database. Report statement
coverage: 173/187 (92.5%); application-wide: 4512/5301 (85.1%).

The report API tests also passed independently against PostgreSQL. Complete
baseline upgrade, schema rollback and reapply through version24 passed. A separate
populated-data check confirmed migration24 rollback removes only its index and
reapply restores it, preserving all leave-request fields exactly.

Disposable verification containers were removed. The developer database and live
Firebase accounts were not modified. Local coverage did not enable race
instrumentation; the existing GitHub CI job supplies that check.

The user reported successful live role checks: Student, Sub-Warden and Security
received 403; an anonymous request received 401. Remaining manual API checks and
remote CI results must be recorded separately.
