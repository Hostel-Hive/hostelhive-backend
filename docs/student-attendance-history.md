# Student-owned attendance history - issue #93

GET /api/v1/me/attendance/events requires a current Firebase ID token and an
active Student account with an unarchived profile. Ownership comes from the
verified UID. Missing/archived profiles return 404, staff/inactive accounts 403,
missing/invalid authentication 401. GET body must be empty.

## Filters and response

Optional paired from/to: strict YYYY-MM-DD inclusive local dates, maximum 366
calendar days, years 0001..9998. Neither date means all history, paginated.
Optional direction: IN or OUT, case-sensitive. limit defaults 20, range 1..100;
offset defaults 0, range 0..1000000. Unknown, repeated, empty or malformed
parameters (including student_id/user_id) return 400 invalid_input.

Response 200: items,total,limit,offset,has_more,timezone,from,to. Dates are null
when omitted. Each item has movement_id,event_id,direction,occurred_at.
Only outcome=recorded qualifies. Empty history returns items:[], total:0.
An offset beyond the end returns items:[], has_more:false, with total preserved.

movement_id is an opaque stable 64-character lowercase SHA-256 hexadecimal key
derived from the complete (device_id,event_id) identity. event_id alone can
repeat across scanners. No device ID, payload hash, processing outcome,
credentials, access result, permission or incident IDs are exposed.

Sort: occurred_at DESC, device_id DESC, event_id DESC. Counts and rows share a
read-only repeatable-read snapshot. Offset pagination is stable for unchanged
data; new scans can shift pages between requests. Mobile clients should reset
paging on refresh and use movement_id to identify items. RFC3339 movement times
represent instants; format them in the returned hostel timezone for display.

Date bounds are local midnight through midnight after to, with the upper bound
exclusive. Use HOSTEL_TIMEZONE (default Asia/Colombo), including DST calendar
boundaries. This reader does not infer presence, authorize trips or alter
attendance events, current presence, late-access requests or notification intent.
The existing per-student attendance index supports reads; no migration or new
configuration is required. Existing staff routes and scanner authentication stay
unchanged. Database failure returns 503 attendance_unavailable, without SQL detail.

## Verification

Run scripts/test-backend-coverage.ps1 with Docker Desktop running. It creates
its own disposable migrated database. TestStudentAttendanceHistoryPostgres
covers ownership, all four staff-role denials, missing/archived/inactive/role-
changed accounts, no-event profiles, recorded-only filtering, legacy rows,
local-midnight inclusivity/exclusivity, stable tied pagination including reused
event IDs on different devices, direction filters, empty/out-of-range pages,
private-field exclusion and preservation of events/presence/approval records.
Unit tests cover strict parsing, defaults, bounds, leap years, the 366-day maximum,
DST days, error mapping and empty GET body enforcement. Existing attendance
integration tests continue to cover staff readers, scanners and permission use.

The mobile guide section4.4 has the full response example and client guidance.
The Postman collection includes an empty-body Student history request.
