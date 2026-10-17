# Attendance history reports - issue #78

SRS FR017 and the attendance portion of FR030. This starts the report module;
leave/complaint reports and PDF/Excel exports (FR031) remain separate tickets.

## API contract

Admin/Warden only, using the existing Firebase login and local account checks:

- GET /api/v1/reports/attendance
- GET /api/v1/reports/attendance/daily

Both accept the same strict query parameters:

| Parameter | Meaning |
|---|---|
| from | Required inclusive hostel-local date, exactly YYYY-MM-DD |
| to | Required inclusive hostel-local date, at most 366 calendar days including from |
| student_id | Optional canonical lowercase nonzero profile UUID |
| direction | Optional IN or OUT |
| limit | 1..100; default 20 |
| offset | 0..1000000; default 0 |

Unknown, duplicated, empty or malformed parameters, reversed/oversized ranges
and bodies on GET return 400. No login returns 401; Student/Sub-Warden/Security
return 403. An unknown valid Student UUID returns an empty movement page and
zero-valued daily rows, not a profile existence oracle. Database failures return
503 without exposing SQL. Successful responses are JSON and Cache-Control:no-store.

Dates use HOSTEL_TIMEZONE (Asia/Colombo by default), reported as timezone in each
response. from starts at local midnight; to ends at the following local midnight,
exclusive. Explicit calendar conversion handles UTC offsets, leap days and daylight
saving rather than assuming every local day lasts 24 hours. Input timestamps are
not accepted in date parameters. Dates may be in the future and show zero activity.

## What is counted

Only attendance_events.outcome='recorded' is included. Cooldown, already_in and
already_out attempts are excluded. No current active/profile filter is applied:
archived/deactivated Students and disabled scanners retain their recorded history.

Movement items include event_id,device_id,student_id,direction,occurred_at and,
where originally recorded, access_result,late_access_id,incident_id. Raw QR data,
payload hashes, device credentials, contact information and guardian data are
never returned. Legacy access_result=NULL is omitted; absence means unspecified,
not approved or not_required. Historical timestamps and classifications are not
rewritten. Movement ordering: occurred_at descending, then device_id,event_id
ascending as deterministic tie-breakers.

Both responses include from,to,timezone, optional applied student_id/direction,
summary,total,limit,offset,has_more and items. summary covers the ENTIRE filtered
period, independently of pagination, and contains entries,exits,distinct_students.
Its distinct_students counts each Student once across the whole period; summing
daily distinct counts is not the same measurement.

For the movement endpoint total is the count of matching recorded movements.
For the daily endpoint total is the count of selected calendar dates, including
zero-activity days. Daily items contain date,entries,exits,distinct_students and
sort newest date first. Pagination applies to movements or days respectively.
Empty pages keep their full matching total and summary; items is always an array.

These are scan-derived movement statistics. They are not historical occupancy,
time spent in the hostel, proof of passage, or a reconstruction of missed scans.
An unmatched entry/exit is reported as recorded; no missing movement is invented.
The report does not change attendance, permission use, incidents, notifications
or Firebase projections.

## Consistency, architecture and deployment

Each report rechecks current Admin/Warden authority inside its read transaction,
under the same users-table lock used by existing account changes. Rows, totals
and summary are selected in ONE SQL statement snapshot. Different API calls may
observe newer committed movements, so pages from separate calls are not an
export snapshot. No multi-request snapshot or stored report job is claimed.

The report module follows domain/dto/handler/service/repository/routes boundaries.
App wiring injects the report repository, service and configured hostel timezone.
It reads source tables directly without importing attendance's concrete repository.

Apply migration 23 before starting this version. It adds a partial date-range index
over recorded attendance events; no history is backfilled/reclassified and there
is no new report table. Its down migration removes only that index. Test downgrade
and complete rollback in disposable databases, not the developer database.

## Local verification

Run scripts/test-backend-coverage.ps1. Report is the fifteenth required integration
package; CI cannot claim success when that package is absent/skipped. Service
tests cover strict dates, 366-day limits, leap years and 23/25-hour local days.
Real PostgreSQL/API tests cover midnight boundaries, recorded-only statistics,
approved/unapproved/legacy permission results, zero days, full-period distinct
counts, filters, pagination and empty-page totals, historical profile retention,
role restrictions, repository authority rechecks and database unavailability.
Run scripts/test-migrations.ps1 for fresh apply/rollback/reapply through version23.

For manual verification, keep the existing PostgreSQL/Firebase configuration,
apply migrations, rebuild and restart the backend. Use an Admin/Warden login.

Example:
GET /api/v1/reports/attendance?from=2026-10-01&to=2026-10-15&limit=20
GET /api/v1/reports/attendance/daily?from=2026-10-01&to=2026-10-15&direction=IN

Compare against PostgreSQL using the exact API timezone and half-open bounds:

SELECT direction, count(*) AS recorded_movements
FROM hostelhive.attendance_events
WHERE outcome='recorded'
  AND occurred_at >= ('2026-10-01'::date::timestamp AT TIME ZONE 'Asia/Colombo')
  AND occurred_at < ('2026-10-16'::date::timestamp AT TIME ZONE 'Asia/Colombo')
GROUP BY direction;

Do not compare report totals to the current IN/OUT presence summary: they measure
different things. Keep live manual results and GitHub CI links with ticket #78.

## Automated evidence - 2026-10-15

Formatting, dependency verification, static analysis, server build, architecture
checks and all Go tests passed. All fifteen required real PostgreSQL integration
packages passed without skips on a fresh schema23 database. Report statement
coverage:134/145 (92.4%); application-wide:4473/5258 (85.1%). The report API tests
also ran independently against real PostgreSQL before the full coverage run.
Complete migration rollback and reapply through version23 passed separately.

No developer database, live Firebase records or application accounts were
modified. All disposable verification databases were removed. Local coverage
did not enable race instrumentation; GitHub CI supplies race checks. Manual
live API verification and GitHub CI remain pending.
