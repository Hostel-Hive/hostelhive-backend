# Pending leave and late-access cancellation (#101)

Students may withdraw their own pending leave or late-access request. Approved,
rejected and consumed requests cannot be cancelled. Going Home is outside this ticket.

## API contract

| Method | Path | Access |
|---|---|---|
| POST | /api/v1/leaves/{leaveID}/cancel | Owning active Student |
| POST | /api/v1/late-access-requests/{lateAccessID}/cancel | Owning active Student |

Send a Firebase ID token in `Authorization: Bearer ...`. Omit the request body
completely (even `{}` is invalid). No query parameters are accepted. No new
request UUID is needed: the existing request ID in the URL identifies the action.

HTTP200 returns the usual request record with `status:"cancelled"`, `cancelled_at`
(server RFC3339 timestamp) and `cancelled_by` (own account UUID). Both new fields
are explicitly null for records that are not cancelled. Existing decision fields
remain null/empty for a cancellation. Read/detail/list endpoints retain the row
and support `status=cancelled`; Admin/Warden history remains available.

Repeating cancellation returns200 and preserves the FIRST timestamp/actor. A retry
of the original submit operation also returns the existing cancelled record; it
never reopens the request. Use a NEW request UUID to submit a replacement. Cancelled
rows do not block a new leave interval or late-access movement at the same time.
There is no date cutoff beyond the pending-state rule: an overdue pending request
can be withdrawn, but cannot be approved retroactively.

| HTTP | Meaning |
|---|---|
| 400 invalid_input | Invalid UUID, any query, nonempty body |
| 401 unauthorized | Missing/invalid/expired token |
| 403 forbidden | Non-Student or inactive account (setup_required remains a separate authentication gate) |
| 404 not_found | Unknown request or another Student's request, regardless of status |
| 409 active_student_required | Missing/archived/ineligible Student profile |
| 409 leave_conflict / late_access_conflict | Request already approved/rejected, or transactional conflict |
| 503 leave_unavailable / late_access_unavailable | Database operation unavailable; refresh/retry same action |

## Transaction and audit rules

Both modules use the existing account -> Student -> request lock order. Fresh role,
active profile and ownership checks happen inside the transaction. Cancellation and
staff decision racing on the same pending request have exactly one winner; the
loser receives409. Cancellation retries serialize and retain the first audit.
Scan consumption uses the same Student lock and only selects approved requests:
pending/cancelled requests never grant permission, and approved/consumed requests
remain uncancellable. Cancellation never changes presence, scans, allocation,
Going Home records or realtime publication.

Cancellation is a withdrawal, not a staff decision. This ticket adds no new inbox
or push event. Existing submission notifications retain their original event
snapshot and read/delivery state. Open the source record to display its current
cancelled status; staff must refresh before deciding a stale pending request.

## Reports and frontend integration

Leave JSON/Excel/PDF reports accept `status=cancelled`, include cancellation audit
fields, and add `summary.cancelled`/a Cancelled summary metric. The invariant is
`total = pending + approved + rejected + cancelled`. Excel/PDF append Cancelled At
and Cancelled By after the existing decision columns. Submission-date filtering,
pagination, staff permissions and archived history are unchanged.

Show Cancel only on an owned pending request. Disable the button during the call.
After200 replace the local record and refresh relevant lists/counts; after409
reload the source. The staff UI must display Cancelled and must not offer decisions
on it. Existing clients must tolerate the added status and nullable audit fields.

## Database migration and rollback

Apply migration28 (`000028_pass_cancellation`) before starting this backend. It
widens status to9 characters and adds audit columns/constraints to both request
tables; existing records/decisions/consumption/notifications are not backfilled.
The database enforces cancellation metadata only for cancelled rows, with a finite
timestamp and the original submitting account as cancellation actor.

Rollback intentionally REFUSES while either table contains cancelled records;
silently reopening requests or deleting withdrawal history would be unsafe. Retain
migration28 after real cancellations. A failed migration CLI downgrade may leave
migration metadata dirty: restore/diagnose that controlled maintenance attempt
before restarting; never use force/down to bypass the history guard. Empty or
pre-cancellation databases can roll back normally.

## Verification

`scripts/test-pass-cancellation-upgrade.ps1` creates an isolated PostgreSQL
container and checks27->28->27->28, historical decisions/scan consumption/read
state, and invalid cancellation audit constraints. CI runs the same fixtures.
Go HTTP tests cover role/input/error handling. PostgreSQL integration tests cover
ownership, eligibility, retries, replacement submissions, decision races, parallel
cancellation, scan races and report summaries, without calling Firebase/R2/FCM.
Use `scripts/test-backend-coverage.ps1 -Race` for the full isolated suite.

Local and CI Go checks target `./cmd/... ./internal/... ./tests/...` explicitly.
These contain all project Go packages and avoid walking installed Firebase
`node_modules` dependencies when collecting packages from a local checkout.
