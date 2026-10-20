# Combined Student pass history (#103)

`GET /api/v1/me/passes` returns the signed-in Student's leave and late-access
requests together. Rebuild/restart the backend; no new migration or configuration
is required beyond migration28. This is a read projection, not a new request
workflow. Going Home declarations are not included.

## Authentication and query

Send `Authorization: Bearer <Firebase ID token>`. Access requires an active,
setup-complete Student account with an unarchived Student profile. Ownership comes
from the authenticated account, never a supplied Student/account UUID. Staff roles
cannot call this endpoint. Send no request body, including `{}` or whitespace.

| Parameter | Optional values/default |
|---|---|
| type | `leave` or `late_access`; omit for both |
| status | `pending`, `approved`, `rejected`, `cancelled`; omit for all |
| limit | Integer1..100; default20 |
| offset | Integer0..1000000; default0 |

Unknown parameters (including `student_id`, `user_id`, dates and direction),
duplicate keys, empty values and invalid filters return400. Filters are exact and
case sensitive. `status=consumed` is invalid: a consumed late-access approval
remains `approved`, with `consumed_at` populated.

Example: `GET /api/v1/me/passes?type=late_access&status=approved&limit=20&offset=0`.

## Response

HTTP200, JSON, `Cache-Control: no-store`:

```json
{
  "items": [
    {
      "type": "late_access",
      "source_id": "11111111-1111-4111-8111-111111111111",
      "request_id": "22222222-2222-4222-8222-222222222222",
      "student_id": "33333333-3333-4333-8333-333333333333",
      "reason": "Returning from an event",
      "status": "pending",
      "created_at": "2026-10-20T10:00:00Z",
      "starts_at": null,
      "ends_at": null,
      "direction": "IN",
      "expected_at": "2026-10-21T21:00:00+05:30",
      "decided_at": null,
      "decided_by": null,
      "decision_note": "",
      "cancelled_at": null,
      "cancelled_by": null,
      "consumed_at": null
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0,
  "has_more": false
}
```

All items have the same keys. IDs are UUID strings; timestamps are RFC3339
instants (equivalent offsets are possible). Actor UUIDs are not staff names.

| Fields | Meaning |
|---|---|
| type, source_id | Original request type and primary ID; together form the list key |
| request_id | Original submission idempotency UUID; not the detail/cancel URL ID |
| student_id | Owning Student profile UUID |
| reason, status, created_at | Stored reason, current status and submission timestamp |
| starts_at, ends_at | Leave interval; both null for late access |
| direction, expected_at | One late-access movement (`IN`/`OUT`) and its expected instant; null for leave |
| decided_at, decided_by, decision_note | Original recorded approval/rejection; timestamps/actor null and note empty when not decided |
| cancelled_at, cancelled_by | Original cancellation audit; null when not cancelled |
| consumed_at | Late-access scan consumption timestamp; null for unused approvals and all leave |

Order is **created_at descending, type ascending using ASCII/C ordering, source_id
descending**. Submission time determines order, not travel/decision time. Identical
UUIDs in different source tables remain distinct items. No deduplication or grouping
removes history. Inactive/archived Students cannot use this own-feed route; existing
authorized staff history endpoints retain their current behaviour.

`total` counts ALL matching requests across both types before pagination. `items`
is the globally ordered slice. `has_more` is `offset + items.length < total`.
An empty feed is `items:[]`, `total:0`, `has_more:false`. An offset beyond the end
returns an empty array with the correct full matching total and `has_more:false`.

Authorization, profile eligibility, total and items share a read-only repeatable-read
database snapshot. Each HTTP response is internally consistent. Separate HTTP pages
are separate snapshots: new submissions/cancellations between requests can shift
offsets or filter membership. Refresh from offset0 after changes and deduplicate
by `(type, source_id)` when appending pages. There is no cross-request snapshot token.

## Flutter integration

1. Load this route for the Passes screen; do not fetch and merge independently
   paginated leave and late-access pages.
2. Keep `type` and `source_id`; display current status and type-specific times.
3. For details, use `GET /api/v1/leaves/{source_id}` for `leave`, or
   `GET /api/v1/late-access-requests/{source_id}` for `late_access`.
4. For an owned pending request, use `POST /api/v1/leaves/{source_id}/cancel` or
   `POST /api/v1/late-access-requests/{source_id}/cancel`, with no body. Then refresh
   the combined feed. After409 reload details before showing another action.
5. Keep the yearly ring separate: `GET /api/v1/me/leave-allowance?year=2026`.
   The 18-day allowance is informational; history does not calculate or enforce it.
6. Refresh current presence through the existing Student profile/attendance APIs.
   Reading history never changes IN/OUT or consumes an approval.

Submission and decision endpoints, cancellation retries, notification snapshots,
permission windows and scan consumption remain compatible. This ticket adds no
combined detail/submission/cancellation route, no round-trip late-access fields,
and no Going Home history API.

## Errors and verification

| HTTP / error | Meaning |
|---|---|
| 400 invalid_input | Invalid query, duplicate/unknown key or nonempty GET body |
| 401 unauthorized | Missing/invalid/expired Firebase token |
| 403 forbidden | Non-Student or inactive account |
| 403 setup_required | Initial invitation setup is not complete |
| 409 active_student_required | Missing or archived Student profile |
| 503 leave_unavailable | Temporary read failure; retry/refresh |
| 405 | Unsupported write method |

Service/HTTP tests cover bounds, filters, body validation, authentication, all
staff roles and sanitized errors. PostgreSQL tests cover interleaved types,
identical timestamps/IDs, combined pagination/empty pages, filtering, cancellation
and decision audits, consumed approvals, ownership, setup/archive restrictions,
unchanged requests/attendance/notifications, and consistent totals during writes.
No Firebase/R2/FCM provider is called by the disposable database suite. Existing
CI Go checks automatically discover these tests through the usual package paths.
