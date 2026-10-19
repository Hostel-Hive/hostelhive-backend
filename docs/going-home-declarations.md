# Going Home declarations - issue100

Implemented on feature/100-going-home-declarations. Confirm the deployed revision
and apply migration27 before using this endpoint. This document does not certify
Flutter integration or real scanner hardware acceptance.

## Purpose and policy

A declaration records a Student's intention to go home. It needs no Warden
approval and no return date. It does not record physical departure, authorize an
early exit or waive late-entry permission. Only accepted scanner movements change
presence, occupancy and RTDB. Existing leave/late-access approvals are separate.

Traceability: SRS UC009 (Record Leave Status), authorization FR003, scan rules
FR011-FR013, and permanent-history intent in FR023. Original UC009 says a mobile
action marks attendance OUT; issue100 follows the approved D07 scan-based amendment
instead. Update the controlled SRS/SDS accordingly; do not claim the original
wording is implemented unchanged. Use the Student/Leave and Attendance design;
no invented official requirement number is added for this extension.

## API

```http
POST /api/v1/going-home
Authorization: Bearer FIREBASE_ID_TOKEN
Content-Type: application/json
```

```json
{"request_id":"10000000-0000-4000-8000-000000000010"}
```

Use a new nonzero UUID for each intended declaration; retain it on a network retry.
UUID letter case is normalized. The Student/account owner comes from authentication.
No student_id, timestamps, reason, destination, return date, status or approval
fields are accepted. Query parameters are not supported. JSON body limit16KiB;
unknown fields, empty/invalid JSON and trailing JSON documents are rejected.

Success201 returns the declaration directly:

```json
{
  "going_home_id":"10000000-0000-4000-8000-000000000011",
  "request_id":"10000000-0000-4000-8000-000000000010",
  "student_id":"10000000-0000-4000-8000-000000000012",
  "submitted_by":"10000000-0000-4000-8000-000000000013",
  "status":"declared",
  "declared_at":"2026-10-19T08:00:00Z",
  "departed_at":null,
  "departure_device_id":null,
  "departure_event_id":null
}
```

These are fictional IDs/times. Responses are private JSON with Cache-Control:no-store.
Only an active Student with completed setup, an unarchived profile and known IN
presence may create a new declaration. The endpoint does not infer IN from an
allocation, approved leave, a previous declaration or an unknown presence row.

## Lifecycle and retries

- Exactly one outstanding declared record per Student.
- The same request_id returns200 and the same ID/original declared_at, without
  adding another record. Current lifecycle fields may now show departed.
- Replays are resolved before checking current IN/OUT, so a successful request can
  be retried after the exit without creating a second declaration. Current account,
  role, setup and profile eligibility are still checked before returning the record.
- A different request_id while an outstanding declaration exists returns409
  going_home_pending. While already OUT, new requests return409 already_outside.
- The next new recorded OUT event at/after declared_at marks the declaration
  departed and records the actual event's time/device/event IDs in the same scan
  transaction. Attendance cooldown/already-in/already-out events and old event-ID
  retries do not consume it. An older event does not consume a newer declaration.
- No scheduler, expiry, return-date workflow or automatic presence update is added.
  If the Student never leaves, the declaration stays outstanding. After an actual
  OUT and subsequent IN, a new declaration can be made with a new UUID.
- A recorded early OUT can still generate an unapproved attendance incident;
  Going Home does not replace the existing permission checks.
- Declaring creates no inbox/push event, approval, leave interval or quota charge.

Declaration creation and scans share the Student-row lock. Simultaneous identical
requests create one record; different concurrent requests cannot create two active
records. If declaration and OUT race, the intent is either committed then marked
departed by the scan, or rejected after OUT. There is no successful new declaration
left active behind an already committed exit.

## Errors

| HTTP | error | Meaning |
|---|---|---|
|400|invalid_input|Invalid/nonzero UUID, unsupported fields/query, malformed JSON|
|401|unauthorized|Missing/invalid/revoked Firebase token|
|403|forbidden|Wrong role or current account authorization|
|403|setup_required|Unfinished invitation setup blocks business APIs|
|409|active_student_required|Missing/archived/ineligible Student profile|
|409|already_outside|Known OUT; scan IN before a new declaration|
|409|presence_unknown|No authoritative presence; establish it by scanning|
|409|going_home_pending|Outstanding declaration exists|
|409|going_home_conflict|Concurrent storage conflict; refresh/retry same operation|
|413|payload_too_large|Body exceeds16KiB|
|415|unsupported_media_type|Expected application/json|
|503|going_home_unavailable|Temporary database failure or missing migration|

GET/PUT/DELETE on this path are unsupported (405). No staff decision, cancellation,
list or detail endpoint is added by this ticket. The durable table is the audit
history; retrying POST with its original request_id returns its current record.
A future read/feed contract must explicitly decide how to expose this history.

## Migration and compatibility

Migration27 creates going_home_declarations, owner/source constraints, a one-active
index and a trigger on newly inserted recorded OUT events. It does not backfill
historical scans or modify existing presence/history. Rolling27 down drops the
new declaration history and trigger; existing attendance remains unchanged.
Back up declaration history before an operational rollback. The application is
not authorized to delete source accounts, profiles or referenced departure events.

Existing APIs and extended leave/late-access statuses are unchanged. No new server
credentials or environment flags are needed. Pending setup from issue95 remains
enforced centrally, with repository defense for this operation.

## Verification

Unit/HTTP tests cover validation, retries, status codes, all roles, setup gating,
strict fields/MIME/body limits and unsupported methods. The required leave
integration package covers real scans, suppression/replay, source snapshots,
concurrent creation/exit, account/profile eligibility and ownership.

Run scripts/test-backend-coverage.ps1 for the full disposable PostgreSQL suite,
scripts/test-going-home-upgrade.ps1 for populated26->27->26->27 preservation,
and scripts/test-migrations.ps1 for full rollback/reapply. CI includes the same
upgrade fixtures. The older invitation upgrade check is pinned to migration26
so later migrations cannot cause it to roll back the wrong feature.

Local verification on19 October2026 passed80 package results and all15 required
PostgreSQL packages with no test skips. Overall statement coverage84.9%; Leave
module87.1%. New Going Home production files have merged statement coverage
73/82 (89.0%); this is not a branch-coverage measurement. Formatting, module verification, vet and server build passed. All27
migrations passed full rollback/reapply, and populated26->27 preservation plus
Windows PowerShell5.1 helpers passed. Remote CI, Flutter integration and real
scanner-hardware acceptance remain unverified; no feature branch was pushed.
