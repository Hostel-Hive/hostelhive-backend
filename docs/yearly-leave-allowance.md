# Yearly Student leave allowance (#102)

> Retired by #118: GET /api/v1/me/leave-allowance now returns410
> leave_workflow_retired after authentication/role checks. Remove the usage ring.
> The calculation below is historical, not current hostel policy. See
> [current movement policy](hostel-movement-policy.md).

Students can read their own calendar-year allowance summary for the mobile Passes
usage ring. This is informational: reaching 18 days does not block submission or
staff approval. It does not change attendance or consume an approved permission.

## Endpoint

`GET /api/v1/me/leave-allowance?year=2026`

Requires an active, setup-complete Student account and an unarchived Student
profile. Use the Firebase ID token in `Authorization: Bearer ...`. Staff roles
cannot use this self-service endpoint. Do not send a body or a Student/account ID.
The response uses `Cache-Control: no-store`.

`year` is optional. Omit it for the current year in Asia/Colombo. An explicit year
must be exactly four digits, from `0001` to `9998`. Empty, duplicate, unknown and
malformed query parameters are rejected.

Illustrative response (replace the fictional ID):

```json
{
  "student_id": "11111111-1111-4111-8111-111111111111",
  "year": 2026,
  "timezone": "Asia/Colombo",
  "total_days": 18,
  "used_days": 7,
  "remaining_days": 11,
  "exceeded_days": 0
}
```

## Counting policy

- January 1 through December 31 in **Asia/Colombo**, independent of the phone,
  server or PostgreSQL session timezone. The returned timezone is authoritative.
- Count only leave requests whose CURRENT status is `approved`, including future
  approved leave in that year. Use the leave period, not submission/decision dates.
- Each local date touched by `[starts_at, ends_at)` costs one day, even for a
  partial day. An exact-midnight end excludes the following date.
- Merge overlapping dates across all approved requests; count each date once.
- Split a request crossing New Year between its respective calendar years.
- Pending, rejected and cancelled leave, late access and Going Home do not count.
- `used_days` is the actual distinct-date total and may exceed 18.
  `remaining_days = max(18 - used_days, 0)` and
  `exceeded_days = max(used_days - 18, 0)`; remaining days are never negative.

Examples: Jan2 10:00 -> Jan3 00:00 costs one day; Jan2 10:00 -> Jan3 01:00 costs
two. Two approved periods touching Jan2 still charge Jan2 once. Dec31 23:00 ->
Jan1 01:00 charges one date in each year. Feb28 noon -> Mar1 midnight in a leap
year charges Feb28 and Feb29.

## Frontend integration

Load this endpoint when opening Passes and refresh after a relevant request or
decision is refreshed. Display, for example, `7 / 18 days used` and `11 remaining`.
For the visual ring clamp `used_days / total_days` to a maximum of 1, while still
displaying actual usage and any `exceeded_days`. Do not use this informational
summary to block a request or invent approval/attendance state.

The endpoint derives ownership from the authenticated account. A parameter such
as `student_id` is invalid rather than a way to choose another Student.

## Responses

| HTTP | Meaning |
|---|---|
| 200 | Current summary for the authenticated Student and selected year |
| 400 invalid_input | Invalid/unknown/duplicate query or a nonempty GET body |
| 401 unauthorized | Missing, invalid or expired Firebase ID token |
| 403 forbidden | Non-Student or inactive account |
| 403 setup_required | Account password setup has not been completed |
| 409 active_student_required | Missing or archived Student profile |
| 405 | Unsupported method |
| 503 leave_unavailable | The summary could not be read; no fabricated zero usage |

## Implementation and verification

No migration, allowance table, background job or external provider call is added.
The leave repository reads existing requests. It clips periods to the selected
year, builds local date ranges and unions those ranges before summing their days.
One aggregate statement supplies the usage total. Existing account/Student locks
recheck authority and profile eligibility through the read.

Unit tests cover Colombo year rollover, remaining/excess calculation, input and
role restrictions. PostgreSQL HTTP tests cover midnight/microsecond boundaries,
overlaps, leap years, multi-year requests, ignored statuses, ownership, archived
and inactive profiles, different database timezones and unchanged source records.
An over-allowance test confirms that submission and approval remain available.
