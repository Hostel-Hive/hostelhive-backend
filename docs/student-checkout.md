# #132 — End hostel stay

## API

`POST /api/v1/students/{studentID}/checkout` is restricted to active Admins.
Send the Admin Firebase bearer token and `Content-Type: application/json`.
No query parameters are accepted. Migration 32 must be applied first.

```json
{
  "request_id": "11111111-1111-4111-8111-111111111111",
  "reason": "graduated",
  "notes": "Completed hostel clearance.",
  "deactivate_account": true,
  "confirmed": true
}
```

Use a new operation UUID for each intended departure, and retain it and the
original body for exact retries. Both the student ID and request ID must be
nonzero canonical UUIDs. Reasons are `graduated`, `withdrawn`, `moved_out`,
`accommodation_cancelled`, or `other`. Reason and optional notes are trimmed;
notes have at most 1,000 characters without control characters. The confirmation
must be true and the account choice must explicitly be true or false.

Temporary trips home, holidays and access suspensions do not use checkout.
Wardens may continue to perform their existing allocation/archive operations,
but checkout is Admin-only because it can deactivate an account.

## Atomic departure

The database transaction rechecks current Admin authority, then:

1. Ends the student's current bed allocation, if present, with `end_reason=checkout`
   and the staff actor/time. The bed is immediately available after commit.
2. Archives the profile and queues the current private photo for cleanup,
   matching existing archive behavior. Guardians, student identity and history remain.
3. If explicitly requested, deactivates local account access and upserts the
   existing durable Firebase disable/session-revocation job.
4. Stores the immutable departure snapshot, reason, notes, operation UUID,
   normalized payload hash and staff actor/time in `student_checkouts`.

Any database failure rolls back all four steps. The same user-authority lock as
existing account/profile writes prevents allocation/transfer from racing checkout.
No attendance event or presence change is invented. It does not delete accounts,
erase history, send invitations, or call Firebase/R2 inside the transaction.

Students without beds and previously suspended student accounts can check out.
False for `deactivate_account` preserves the current account state; it never
reactivates a suspended account. A manually archived stay not yet checked out
can be finalized to release a forgotten bed, including a later stay after return.
Its original archive actor
and timestamp are preserved. A changed-role account cannot be checked out.

## Response and replay

HTTP 200 returns:

- `checkout`: immutable snapshot with checkout/request/student/user IDs, reason,
  notes, explicit deactivation choice, `account_active_after`,
  `profile_was_archived`, `archived_at`, nullable `released_allocation_id` and
  `released_bed_id`, and `performed_by`/`performed_at`.
- `replayed`: false on first commit, true on the same Admin's exact retry.
- `revocation_pending`: current state of the shared durable revocation job.

The snapshot represents the original departure, not live state after a later
return. Retrying an old checkout after restoration does not archive the new stay,
release its bed or request another revocation. A newly restored stay uses a new
operation UUID for its eventual departure. Restoring/reactivating/assigning a bed
remain separate authorized workflows.

Local access is denied as soon as a deactivation commits. The existing worker
disables Firebase and revokes sessions asynchronously, retrying failures durably.
A provider outage can leave `revocation_pending=true`; it does not undo a committed
checkout. Replaying the same request can read the updated pending state without
repeating the departure.

## Errors

| Status/code | Meaning |
|---|---|
| 400 `invalid_input` | Invalid identifiers/reason/notes, missing confirmation/account choice, unknown body fields or query parameters |
| 401 `unauthorized` | Missing or invalid authentication |
| 403 `forbidden` | Actor is not a current active Admin |
| 404 `not_found` | No student profile exists for that ID |
| 409 `checkout_conflict` | Request UUID reused with another actor, student or normalized payload |
| 409 `student_already_checked_out` | Fresh request for an archived stay that already has a departure record |
| 409 `student_account_required` | Linked account no longer has the student role |
| 415 `unsupported_media_type` | Body is not application/json |
| 503 `student_profiles_unavailable` | Database failure or unconfirmed commit; retry the original UUID/body |

Migration 32 can roll back only before any departure audit or checkout allocation
history exists. It refuses to discard those records or rewrite their reasons.

## Verification

Unit/HTTP tests cover role boundaries, validation, explicit choices, canonical
payload hashing, error contracts and timeout propagation. Isolated PostgreSQL
tests exercise bed release, archival/photo cleanup, retained QR/guardians,
deactivation and fake-provider revocation retry, audit failure rollback, missing
beds, inactive accounts, legacy archives, concurrent replays, allocation races,
immutable old-request replay after a new stay, and rollback protection.
The separate frontend checkout ticket supplies the staff confirmation form.
