# Reactivate a scanner device

Use this Admin-only action when a deactivated scanner is ready to return to
service. It preserves the device ID, name, direction, original creator,
attendance events and presence records; it does not register another scanner.

## API contract

```http
POST /api/v1/scanner-devices/{deviceID}/reactivate
Authorization: Bearer <ADMIN_FIREBASE_ID_TOKEN>
Content-Type: application/json

{"confirmed": true}
```

Accepts only `confirmed: true`, with no query parameters. The device UUID may use
either case. The authenticated account must be an active Admin, and that
authority is rechecked inside the database transaction.

HTTP 200 uses the same response shape as registration and credential rotation:

```json
{
  "device": {
    "device_id": "11111111-1111-4111-8111-111111111111",
    "name": "Gate entry",
    "direction": "IN",
    "is_active": true,
    "credential_version": 2,
    "created_at": "2026-10-01T00:00:00Z",
    "updated_at": "2026-10-30T00:00:00Z",
    "created_by": "22222222-2222-4222-8222-222222222222",
    "updated_by": "22222222-2222-4222-8222-222222222222"
  },
  "credential": "hhd1.<device UUID>.<new random secret>"
}
```

Values above are illustrative. The new credential is returned once. Install it
securely on the scanner. Listings never reveal it; PostgreSQL stores only its
SHA-256 verifier.

| Status | Meaning |
| --- | --- |
| 400 `invalid_input` | Invalid UUID, missing/false confirmation, malformed JSON, extra fields or query parameters |
| 401 `unauthorized` | Missing or invalid Firebase identity |
| 403 `forbidden` | Inactive Admin, changed role or another role |
| 404 `not_found` | Device does not exist |
| 409 `scanner_already_active` | Already active; no state, credential or audit mutation |
| 413 `payload_too_large` | Body exceeds 2 KiB |
| 415 `unsupported_media_type` | Content type is not JSON |
| 503 `scanner_devices_unavailable` | Database, credential generation or commit could not be completed/confirmed |

## Credentials, concurrency and retries

Reactivation locks the scanner row and atomically enables it, replaces the
credential verifier, increments its version, updates the actor/time and records
a `reactivated` audit event. An audit failure rolls back the complete change.
Existing scan transactions fence requests authenticated with an older version.

All previous credentials remain invalid. Rotation of an inactive scanner still
does not reactivate it. Concurrent reactivation requests serialize: one succeeds
and another returns 409 without rotating the winner's credential. Repeating the
request while active also returns 409. Raw credentials are not stored for replay.

If a response is lost, **check the Admin device listing first**. If it is active
but the credential was not received, explicitly use the existing credential
rotation action to obtain another credential. A timeout does not prove that
activation failed. Never automatically deactivate/reactivate as a retry.

## Deployment and acceptance

Apply migration **33** before deploying this code. It extends the audit action
constraint while retaining registration, rotation, deactivation and direction
history. Rollback refuses once reactivation audit history exists; it does not
delete security history to allow rollback.

In Swagger, use **Attendance → Reactivate scanner and issue a new credential**:

1. As Admin, register a disposable scanner and retain its credential.
2. Deactivate it; confirm `/api/v1/device/check` rejects that credential.
3. Reactivate with `confirmed: true`. Verify the same ID/direction, active state
   and incremented version.
4. Confirm the new credential works and all older credentials are rejected.
5. Repeat reactivation: expect 409; the successful credential must still work.
6. Confirm non-Admins are denied and attendance history/presence are unchanged.

Unit/route tests cover confirmation, role guards, validation, errors and credential
isolation. PostgreSQL tests cover preservation, old-token rejection, repeated and
concurrent requests, audit-failure rollback, fresh Admin authority checks and the
migration rollback guard. Use a disposable database. This change adds backend
support only; no frontend action is introduced.
