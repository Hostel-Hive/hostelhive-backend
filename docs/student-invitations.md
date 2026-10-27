# Student invitations and email setup (#95)

Implemented on `feature/95-student-invitations`. This is a source contract, not
proof that a shared server is deployed or that real email/Flutter setup passed.

## Process and responsibility

1. An active Admin registers an approved Student with an allowed email and profile
   details. The API atomically creates a local Student account, profile, guardians
   and durable invitation intent; it does not wait for Firebase email delivery.
2. The worker creates the reserved Firebase UID with an unpredictable internal
   bootstrap secret and records the provider password-update timestamp. The secret
   is never returned, emailed, logged or persisted by HostelHive. Existing Firebase
   accounts are never silently adopted by email or assigned another password.
3. Firebase sends its hosted password-reset/setup email to the approved address.
   The Student opens that private email and chooses a HostelHive password on the
   Firebase page. The Admin never sees the password or a usable setup link.
4. The Student signs in using email/password in Firebase, gets an ID token, calls
   `GET /api/v1/me` and, when `setup_required` is true, sends an empty authenticated
   `POST /api/v1/me/setup/complete`. The backend verifies completed setup with
   Firebase. A password sign-in/reauthentication within five minutes is required;
   merely refreshing an old token is insufficient.
5. Completion returns `setup_required:false`. Refresh `/me` and load Student APIs.
   Account activation, current role and unarchived profile remain required.

The email is an identifier; Gmail and approved university addresses are supported.
This does not connect to university SSO,
collect university passwords or change university credentials. Later HostelHive
password changes/reset use Firebase as usual. A Firebase-only public signup gets
no local role/profile/access. Existing approved accounts need no forced reset.

## Enablement and Firebase prerequisites

Apply migration `000026_student_invitations` before starting this backend revision,
even if invitation sending is disabled: account lookup reads invitation state.
Use the existing migration helper with the correct development database configured:

```powershell
.\scripts\migrate.ps1 up
$env:STUDENT_INVITATIONS_ENABLED = 'true'
$env:STUDENT_EMAIL_DOMAINS = 'gmail.com,stu.kln.ac.lk'
```

These variables must be present in the backend process environment at startup;
the example file is documentation, not automatic environment loading. The feature
flag defaults to false. The comma-separated domain allowlist matches exact domains
only, case-insensitively; no wildcard or implicit subdomain match. Confirm the
institution's actual approved domains. Existing accounts are not domain-filtered.

When `STUDENT_EMAIL_DOMAINS` is unset, the default is `gmail.com,stu.kln.ac.lk`.
An explicit setting overrides this default, including a stricter university-only
list. To enable both on an existing Render deployment, update the backend's
Environment setting to `STUDENT_EMAIL_DOMAINS=gmail.com,stu.kln.ac.lk` and redeploy.
For local development, put the setting in ignored `.env.local` and restart with
`scripts/dev.ps1`. Other email providers require their exact domain to be added;
Gmail subdomains and suffix lookalikes are not implicitly approved. The same
allowlist applies to individual and CSV invitation registration. No migration is
required, existing accounts/passwords are unchanged, and enabling invitations
still requires the Firebase prerequisites below.

Use the existing Firebase project and Application Default Credentials, e.g. the
server's configured `GOOGLE_APPLICATION_CREDENTIALS`. Never ship that credential to
Flutter or export it in Postman. Firebase Email/Password sign-in and its hosted
password reset email/action page must be configured. The service identity needs
Firebase user get/create and `firebaseauth.users.sendEmail` permission on the same
project. The worker uses OAuth Identity Toolkit; no SMTP server, extra email vendor
or Firebase web API key is required for this server adapter. FCM enablement does
not by itself verify Authentication email delivery.

The API records the connection peer IP for Firebase's password-reset request.
Forwarded headers are not trusted. Behind a proxy, this is the proxy peer; confirm
provider quota/rate-limit behavior in the actual deployment.

With the flag off, new registration/import/resend and setup completion return503;
Admin list/detail/template remain readable. Previously pending Students remain
blocked: disabling delivery never turns off setup enforcement. For a Student with
pending setup, authentication permits only `GET /api/v1/me` and
`POST /api/v1/me/setup/complete`; every other protected API returns403
`setup_required`. Inactive/revoked accounts still fail the existing checks first.

## API contract

Prefix all paths below with `/api/v1`. Use a current Firebase ID token in
`Authorization: Bearer ...`. Admin routes require an active Admin, never a Warden
or Student. Bodies/queries must contain only documented fields; do not send GET
bodies. JSON writes use `application/json`. Responses have `Cache-Control:no-store`.

| Method and path | Input | Success |
|---|---|---|
| POST /student-invitations | JSON registration below | 201 first creation; 200 exact replay |
| GET /student-invitations | Optional single `limit` (1-100, default20) and `offset` (0-1000000, default0) | 200 page |
| GET /student-invitations/{invitationID} | No query/body | 200 invitation metadata |
| POST /student-invitations/{invitationID}/resend | JSON `{"request_id":"NEW_OPERATION_UUID"}`; no query | 200 current metadata, queued when eligible |
| GET /student-invitations/import/template | No query/body | 200 UTF-8 CSV header attachment |
| POST /student-invitations/import | Raw UTF-8 `text/csv` body; no query or multipart wrapper | 200 per-record report |
| POST /me/setup/complete | Student token; no query or body (not even `{}`) | 200 setup result |

Registration body (fictional data; generate a real nonzero UUID for request_id):

```json
{
  "request_id": "95000000-0000-4000-8000-000000000100",
  "email": "example-student@stu.kln.ac.lk",
  "index_no": "SE-EXAMPLE-210",
  "full_name": "Example Student",
  "faculty": "Computing",
  "year": 2,
  "contact_phone": "0771234567",
  "guardians": [
    {"name":"Example Guardian","relationship":"Parent","contact_phone":"0777654321"}
  ]
}
```

Profile and guardian fields reuse the Student service's existing validation.
Email is trimmed/lowercased; CSV cannot choose role, UID, password, active status
or setup state. Do not add those fields to JSON either. Registration body limit
is16KiB (invalid oversized JSON gives400); raw CSV limit is2MiB (413).

Registration response wraps metadata as `{"invitation":{...},"replayed":false}`.
Resend/detail return the invitation directly. Metadata contains invitation_id,
request_id, user_id, student_id, email, delivery_status, attempts, optional
last_error, created_at, last_requested_at, next_attempt_at, last_sent_at and
setup_completed_at. Nullable timestamps remain null until applicable. No password,
setup URL/action code, provider password baseline, payload hash or IP is returned.

List shape: `{"items":[],"total":0,"limit":20,"offset":0,"has_more":false}`.
Ordering is created_at descending, then invitation_id descending. Counts and page
share a database snapshot; pages fetched at different times can shift.

`GET /me` includes `setup_required:true` while unfinished. False is omitted from
that existing account response; absent means false. Completion always responds
with `{"setup_required":false,"setup_completed_at":"RFC3339_TIMESTAMP"}` for a
completed invitation. Repeating completion preserves its original timestamp.
A legacy eligible Student without an invitation receives false/null with no reset.

## Idempotency, retries and eligibility

Keep the same registration request UUID and identical normalized payload for a
network retry by the same Admin: it returns the same intended account/profile.
Changing the payload under that UUID returns409. Reusing email/index with a new
operation conflicts; it never overwrites another account or archived history.
Existing legacy provisioning reservations also block a new invitation for that email.
A replay reads current state and does not resend, reset a password or reactivate.

Generate a separate UUID per intended resend; retain it on retry. Replaying an
accepted resend by the same Admin is harmless, including after setup completed.
A different Admin cannot reuse that resend UUID on that invitation. A new resend
for a completed, inactive, archived or wrong-role account conflicts. Pending work
is not duplicated. A new send generation after sent/failed/skipped requires a
five-minute cooldown since last_requested_at;429 has `Retry-After:300`.
Resend requests may be accepted while a job is pending without another email.

The worker commits identity preparation/password baseline before sending mail.
Delivery statuses: pending, sent (Firebase request accepted), failed (terminal
attempt/error), skipped (not eligible or setup already finished). Sent does not
prove inbox receipt or completed setup. last_error exposes only safe categories:
identity_conflict, provider_unavailable or ineligible. Recheck detail for progress.

Each generation allows at most eight provider stages/attempts; preparation counts
as one. Transient errors use exponential delay from60s up to a1h base plus small
jitter, respecting longer provider Retry-After. Identity conflicts fail immediately.
Eligible Admin resend can start another generation, keeping the same UID and
baseline; it never assigns a replacement password to an existing identity. An
ambiguous provider acknowledgement can cause a repeated email on retry; exactly-once
email delivery is not promised. Inspect safe status and correct provider settings.

Expired/reused Firebase action codes are handled by the hosted Firebase flow.
Do not assume a particular code lifetime or that a resend invalidates all older
links. Code errors alone never mark setup complete. Request another invitation
when appropriate. Once setup finishes, further new setup resends conflict; normal
Firebase password recovery remains available.

Completion checks the reserved UID, exact approved email, enabled Firebase user,
unrevoked token from password sign-in, auth_time within five minutes and at/after
the current password update, and a trusted provider passwordUpdatedAt strictly
later than the committed bootstrap baseline. Client flags, token iat, domain
membership or email_verified alone are insufficient. Deactivation/profile archival
cannot be reversed through setup. If fresh sign-in or proof is missing,409
setup_not_verified leaves the account pending;503 permits bounded retry.

## Separate bulk workflow

The existing #38 `POST /students/import` remains profile-only, using existing
active `user_id` values. It does not create Firebase accounts or send email.
Do not change its template. The new invitation CSV has exactly these columns
(order may vary; no unknown/repeated/missing columns):

```csv
email,index_no,full_name,faculty,year,contact_phone,guardians
```

Maximum100 records and2MiB UTF-8; a UTF-8 BOM is accepted. The server derives each
row's stable request UUID from normalized data and the authenticated Admin.
Retry unchanged data as the same Admin, including after reordering rows or headers.
An optional eighth `request_id` column remains supported for clients using explicit
UUIDs; preserve those UUIDs and payloads on retry. Never add user IDs or passwords.
guardians is a JSON array in one CSV field; let a CSV writer
quote it. Use `/student-invitations/import/template` or the checked-in
`docs/templates/student-invitations.csv` for the header. Never add passwords/links.

Whole-file structural failures (bad headers/CSV syntax/UTF-8, empty/header-only
file or too many records) return400 before writes. Validly structured files can
partially succeed: each normalized row is its own atomic registration. The200
report is not proof that every row succeeded or any email arrived. Inspect totals
and every row; partial outcomes are not rolled back as a batch.

Report fields: total, queued, replayed, rejected, not_attempted, retry_required,
optional stopped_reason, and rows. Each row has record (1-based data record), line
(physical CSV start line), status, optional safe error and, on successful rows,
invitation_id and student_id. A queued row confirms an account/profile and delivery
intent, not a delivered email. The worker provisions Firebase and sends the setup invitation.
Statuses: queued, replayed, rejected, retry_required, not_attempted. Invalid profile
or email data is rejected; repeated valid email/index/request UUID within a file
is duplicate_in_file. Existing email/index/reservation conflict is reported without
adopting it. If authorization is lost, remaining rows are not_attempted. Database
or ambiguous-commit failures mark the affected row retry_required and stop the
remainder. Retry that file with the same row IDs/data: committed rows replay,
uncommitted rows can create, and invalid rows still need correction. Do not replace
request IDs merely to hide an unknown result; changed payload with an old committed
ID intentionally conflicts.

## Errors and compatibility

| HTTP / error | Meaning |
|---|---|
| 400 invalid_input | Invalid fields, UUID, profile, email domain, query or body |
| 401 unauthorized | Missing/invalid/revoked Firebase authentication |
| 403 forbidden | Wrong role/current account eligibility |
| 403 setup_required | Pending setup blocks normal protected APIs |
| 404 not_found | Missing invitation or eligible own profile |
| 409 invitation_conflict | Duplicate identity/index, mismatched replay or ineligible resend |
| 409 setup_not_verified | Setup proof absent/stale; complete hosted flow and freshly sign in |
| 409 student_invitation_required | Legacy POST /users Student provisioning is disabled while invitations are enabled |
| 429 invitation_resend_too_soon | Cooldown; retain resend ID and wait |
| 413 import_too_large | Raw CSV exceeds2MiB |
| 415 unsupported_media_type | Wrong JSON/CSV type or non-UTF-8 charset |
| 503 student_invitations_unavailable | Feature off or temporary database/provider failure |

Existing active accounts/passwords and staff provisioning remain unchanged. With
invitations enabled, `POST /users` rejects Student requests with409, including old
Student provisioning replays, so new Student creation uses the approved flow.
With the flag off, that legacy endpoint retains its former behavior. Existing
profile-only CSV remains available; it cannot change invitation setup state.
No allocation, attendance movement, complaint, notification or phone push is
created by an invitation. Existing current-role and ownership rules still apply.

## Migration, rollback and verification

Migration26 adds invitation intent, resend IDs, safe audit and a Student/account
pair constraint. It does not backfill invitations or restrict legacy accounts.
Rolling it down deliberately deactivates unfinished invited accounts and queues
Firebase session revocation before dropping setup metadata. A later up does not
reactivate them or reconstruct invitation history. Preserve a backup before an
operational rollback; recover pending onboarding through an explicitly reviewed
process, never by blindly reactivating those accounts without setup state.

Local validation:80 passed Go package results, all15 required database packages
without skips,84.8% overall statement coverage (identity86.4%). Formatting,
module verification, vet and build passed. All26 migration rollback/reapply checks
and the populated invitation upgrade/rollback helper passed; the latter also ran
under Windows PowerShell5.1. CI is updated; a remote run is not yet verified.

Automated checks use fakes for Firebase. `scripts/test-backend-coverage.ps1` runs
unit/database regression tests in a disposable database; the invitation lifecycle
test covers atomicity, setup gating, concurrency, bulk outcomes and provider retries.
`scripts/test-student-invitation-upgrade.ps1` checks populated25->26 preservation
and26->25->26 fail-closed rollback. CI runs the same SQL fixtures before race tests.
`scripts/test-migrations.ps1` exercises all migrations through rollback/reapply.

Live acceptance remains separate: approved test university mailbox receives the
email, hosted setup succeeds, a fresh password sign-in completes setup, expired/
reused code handling is observed, real IAM/quota settings are correct, and Flutter
handles pending/completed/error states. No live invitations were sent during
implementation. Do not report these external checks as passed until observed.

Provider references: [OAuth email action API](https://docs.cloud.google.com/identity-platform/docs/reference/rest/v1/accounts/sendOobCode),
[trusted user lookup](https://docs.cloud.google.com/identity-platform/docs/reference/rest/v1/projects.accounts/lookup),
and [passwordUpdatedAt metadata](https://docs.cloud.google.com/identity-platform/docs/reference/rest/v1/UserInfo).
