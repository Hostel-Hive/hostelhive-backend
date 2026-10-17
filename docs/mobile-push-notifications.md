# Mobile push notifications - issue #89

Push is an optional prompt to open the authenticated inbox. PostgreSQL remains the
source of truth. A push is not proof of phone delivery, reading or physical movement.

## Event scope

| Event | Push recipient |
| --- | --- |
| Leave approved or rejected | Owning Student |
| Late-access request approved or rejected | Owning Student |
| Unapproved attendance incident | Eligible Admin/Warden inbox recipients |

Submissions, complaints and ordinary announcements remain inbox-only. Enrolling a
device never backfills old notifications. A current binding must predate the event.
Attendance and approved permissions retain their existing scan-based behavior.

## Deployment

1. Apply migration25 with `scripts/migrate.ps1 up` in the backend environment.
2. Rebuild/restart. `FCM_ENABLED=false` is the default: no FCM client or worker starts.
3. To enable delivery, configure `FCM_ENABLED=true`, `FIREBASE_PROJECT_ID` and
   Application Default Credentials (the existing backend service account). Enable
   the Firebase Cloud Messaging API for the same project and grant that account
   permission to send messages. Never put service-account JSON in the mobile app.
4. Keep the existing notification worker enabled through normal app startup.
   Inbox delivery creates local push jobs; the separate push worker sends them.
5. Configure Firebase Messaging in the mobile app before verifying real phones.

Registration APIs work while FCM is disabled. Eligible jobs can accumulate and be
sent if enabled before expiry; jobs expire one hour after the source event.
Existing inbox items are preserved when migrating up or down. Rolling migration25
down removes push registrations/history; stop the new binary before rollback.

## Authenticated device API

All five active roles may manage their own devices. This does not grant inbox or
source access: for example Security Staff still cannot read the notification inbox.
Use the signed-in user's Firebase ID token in the Authorization Bearer header.

| Method and path | Result |
| --- | --- |
| PUT `/api/v1/me/push-devices/{deviceID}` | 200: register or refresh this user's device |
| GET `/api/v1/me/push-devices` | 200: active owned devices and total |
| DELETE `/api/v1/me/push-devices/{deviceID}` | 204: unregister owned device; repeat is safe |

Generate a nonzero UUID once per app installation and retain it locally. PUT body:

```json
{"platform":"android","token":"THE_REAL_FCM_REGISTRATION_TOKEN"}
```

Platform is `android` or `ios`. Use the traditional registration token returned by
Firebase Messaging `getToken()`, not a Firebase ID token, installation ID or APNs
device token. Tokens must be 20-4096 ASCII letters/digits/colon/underscore/hyphen.
PUT requires application/json, at most16KiB, exactly these two fields, no duplicate
fields or trailing JSON. No query parameters or GET/DELETE bodies are accepted.

Responses expose only device_id, platform, active and updated_at. Tokens/hashes
are never returned. Limit: ten active devices per user. A token already bound to
another active registration or an exceeded cap gives409 push_device_conflict.
Unknown/another user's DELETE gives404. Anonymous requests give401; invalid inputs
400, wrong content type415, unavailable persistence503. Existing account checks
can reject an inactive identity with401/403 before registration handling.

Registration is idempotent for the same owned device/token/platform. Rotation,
reactivation and platform changes start a new generation; queued jobs for previous
generations are skipped. DELETE clears the stored raw token and disables binding.
The database stores active tokens because sending requires them; restrict database
and backup access. Do not log request bodies, bearer tokens or registration tokens.

## Mobile integration and logout

- Request the OS notification permission and obtain the current FCM token.
- After successful HostelHive login, PUT it using this installation's UUID.
- On FCM token refresh, PUT the new token for the same UUID and current account.
- Before logout/account switch, DELETE this account's registration while its login
  still works. Delete/rotate the SDK token before binding a different account.
  If offline logout prevents backend cleanup, delete the local FCM token and retry
  cleanup when possible; do not reuse the old token for a second account.
- Use the fixed notification ID to deduplicate foreground handling and navigation.
  Android notification tags/APNs collapse IDs aid replacement but cannot guarantee
  exactly-once display. Handle foreground, background and notification-tap flows.
- On tap, authenticate the CURRENT account and fetch its inbox. Locate the supplied
  notification_id and use its authorized source reference. Drop inaccessible IDs.
  Never trust push data as permission to read a leave, incident or another inbox.

Payload title is HostelHive and body is a generic invitation to open the app. Data
contains only notification_id and source_type; names, reasons, decision details,
student identifiers and complaint photos are excluded. Android leave decisions use
normal priority; late-access decisions/incidents use high priority. APNs uses alert
pushes. Configure Android notification display/channel and iOS APNs credentials in
the mobile project; backend support alone does not complete mobile setup.

FCM may already have accepted a message when an account logs out or loses access.
That message cannot be recalled. Its generic body avoids exposing private details;
opening it always requires current server authorization.

## Queue and failures

Local push intent commits atomically with inbox insertion. Provider calls happen
in a separate worker, so FCM outages do not roll back leave decisions, scans or inbox
read state. The worker polls every two seconds, drains due jobs, and rechecks active
account, current inbox permission and device generation before sending. Per-user
and device row locks stabilize deactivation/logout during a bounded send; there is
no whole-users-table lock for provider calls.

A provider attempt has a five-second deadline, within an eight-second job context.
Transient/auth/network/quota failures retry with at least60 seconds, exponential
backoff and jitter, honoring a longer Retry-After. At most eight queue attempts are
made; the Firebase SDK may itself retry HTTP calls within an attempt. Jobs expire
one hour after the event. Deadlines/shutdown roll back unfinished acknowledgments.

UNREGISTERED disables only that registration and clears its token. INVALID_ARGUMENT
or a sender-project mismatch fails the job but does not assume the token is invalid.
Operational Firebase credential/configuration problems require correction; retries
are bounded and expiry still applies. There is no automatic replay of terminal jobs.

| State | Meaning |
| --- | --- |
| pending | Waiting for a due attempt |
| accepted | FCM accepted a message; phone receipt is unconfirmed |
| failed | Permanent rejection or attempt limit |
| skipped | Account, device or permission is no longer eligible |
| expired | The delivery deadline passed before processing |

A network timeout/crash after provider acceptance can cause a duplicate on retry.
The same notification ID is retained. There is no exactly-once delivery guarantee,
phone receipt acknowledgment or new read-state behavior in this ticket.

## Verification

Run `scripts/test-backend-coverage.ps1` with Docker, Go, Python and the installed
migration binary. It creates/deletes its own database and never uses a developer
DATABASE_URL. The notification integration suite uses simulated provider results,
not real FCM credentials. Tests exercise source workflows, registration ownership,
concurrent caps/workers, rotation/logout, invalid tokens, retries/Retry-After,
expiry, changed authority, eligible/excluded event types and unchanged inbox state.

CI adds a populated schema24->25 preservation fixture. Existing fresh migration,
rollback/reapply and all15 required database-package/race checks include this
feature. No Firebase provider secrets are needed for automated testing.

Live phone verification is PENDING: a test phone has not been configured. After the
mobile team configures FCM, test Android/iOS permissions, actual delivery for each
eligible event, foreground/background/tap flows, rotation/logout/account switching,
inactive accounts and recovery after FCM outage. Confirm ordinary announcements and
complaint updates remain inbox-only. Provider acceptance alone does not pass this
live-delivery test.

Safe database diagnostics (no tokens):

```sql
SELECT device_id, platform, active, generation, updated_at
FROM hostelhive.push_devices
WHERE user_id = 'PASTE_LOCAL_USER_UUID';

SELECT notification_id, state, attempts, next_attempt_at, expires_at,
       accepted_at, last_code
FROM hostelhive.push_deliveries
ORDER BY created_at DESC
LIMIT 100;
```

Notification read status remains `hostelhive.notifications.read_at`. Push acceptance
is separate in push_deliveries and never marks an inbox item read.

References: [FCM token lifecycle](https://firebase.google.com/docs/cloud-messaging/manage-tokens),
[FCM error handling](https://firebase.google.com/docs/cloud-messaging/error-codes),
[Android priority](https://firebase.google.com/docs/cloud-messaging/customize-messages/setting-message-priority).

## Local verification result (2026-10-15)

- Complete backend runner: dependency verification, formatting, vet, build and all
  80 package results passed. All15 required database packages passed without skips.
- Application statement coverage85.2%; notification module86.1%.
- Populated schema24->25 preservation, migration25 rollback/reapply and full
  25-version baseline upgrade/complete rollback/reapply passed on disposable DBs.
- Final credential-refresh timeout adjustment passed adapter static checks, server
  rebuild and the full suite. Linux race checks will run in GitHub CI after push.
- Live FCM phone receipt is unverified; no configured test phone was available.
