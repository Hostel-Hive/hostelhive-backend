# HostelHive Mobile API Integration Guide

> Student self-editing update (23 October 2026): PATCH /api/v1/me/student-profile
> supports contact_phone and complete guardian replacement for the authenticated
> Student, on mobile or web. Omit other fields; identity/academic/account fields
> remain protected. See [the full editing contract](../student-self-profile-editing.md).
> No database migration is needed. Frontend editing screens are separate work.

> Policy #118 supersedes the leave and timing instructions below: both IN and OUT
> require approval outside [06:00,20:00), Asia/Colombo. Going Home is a no-approval
> signing-book declaration with no expected return date. Leave writes and allowance
> return410; retain historical reads and remove leave actions/usage ring. Apply
> migration30 with a coordinated deployment. See [current contract](../hostel-movement-policy.md).

> Faculty announcement update (20 October 2026, migration29): Students see global
> notices plus notices for their current faculty. Inbox/read access uses the same
> scope. Admin/Warden can publish with `audience: faculty` and a `faculty` value;
> this supersedes the older all-Students-only contract below. See
> [faculty announcements](../faculty-announcements.md) and current Swagger `/docs/`.

> Pass workflow update (#101-#103, 20 October 2026): use
> `GET /api/v1/me/passes` for globally paginated leave/late-access history,
> the source-specific pending cancellation endpoints, and
> `GET /api/v1/me/leave-allowance` for the informational 18-day annual summary.
> See [the current combined-pass contract](../combined-student-pass-history.md),
> [cancellation rules](../pass-cancellation.md) and
> [allowance policy](../yearly-leave-allowance.md). These contracts supersede older
> missing-feature statements below. The dated HTML/PDF/Postman handoff packages
> remain historical snapshots; confirm that the shared server includes #103.

| Document detail | Value |
| --- | --- |
| Audience | Flutter mobile application developer |
| Reviewed | 20 October 2026, Asia/Colombo |
| Contract version | Feature-branch snapshot 1.8 (issue103 combined pass history) |
| Backend | Hostel-Hive/hostelhive-backend |
| Inspected branch | feature/103-combined-student-pass-history |
| Base revision | 56e2c39 plus issue103 working-tree implementation; deployment unverified |

This guide describes the backend that exists today. It is not a proposal to
implement the endpoint names from the earlier mobile mock repository. Examples
contain fictional IDs/data; they are not test credentials or real user records.

**Deployment status:** a shared staging HTTPS API URL has not been verified for
this handoff. Obtain the real base URL from the backend owner. Code existing in a
branch does not mean that the running server has been rebuilt or deployed.
Issue #89 phone registration/push is implemented locally with deployment still pending
and real-phone delivery is not yet verified. Confirm its availability before use.

## Mobile feature checklist: 12 planning areas

The following 12 areas group the mobile application for planning. They are not 12
official SRS requirement IDs, 12 API endpoints or a claim that all features are
finished. Status below describes BACKEND SUPPORT; actual Flutter screen completion
and deployed end-to-end behavior have not been audited in this repository.

| # | Mobile feature area | Backend status | Where this guide explains it | What the mobile developer must do / what remains |
| --- | --- | --- | --- | --- |
| 1 | Login and session handling | Available through Firebase and /me | Sections2,3 and4.1 | Use email sign-in, obtain current Firebase ID token, call /me and verify Student role. Restore/refresh safely. Student-ID login is not implemented. |
| 2 | Password reset/change and initial setup | Backend implemented in #95; live email/Flutter pending | Sections3.4,4.1 and18 | Use Firebase-hosted invitation setup, fresh password sign-in, /me setup_required and POST /me/setup/complete. Existing accounts need no reset. Separate HostelHive password; no university SSO. |
| 3 | Home dashboard | Profile, accommodation, presence and unread count available | Sections4.3 and10 | Combine the owned profile with the inbox unread count. Show UNKNOWN honestly; refresh after returning to the app. |
| 4 | Student profile and accommodation details | Available in #91 | Section4.3 | Use /me/student-profile and its authenticated image endpoint. Handle null accommodation/image and missing/archived profile errors. No Student editing endpoint is added. |
| 5 | Student QR display | Available | Section4.2 | Fetch /me/qr with Bearer auth and render PNG bytes. Require an eligible profile; handle errors and clear private images on account switch. |
| 6 | Own attendance history | Available in #93 | Section4.4 | Use /me/attendance/events for recorded IN/OUT movements. Apply local-date/direction filters and pagination; use movement_id for list identity. Staff readers remain protected. |
| 7 | Extended leave requests and history | Available for agreed request/decision model | Section5 | Create/detail through /leaves; combined history through /me/passes. Pending cancellation and informational yearly allowance are available; approval does not change presence. |
| 8 | Early-exit/late-entry requests and history | Available for one movement per request | Section6 | Use /late-access-requests with IN/OUT, expected_at and reason; /me/passes for combined history. Pending cancellation available. No round-trip payload. |
| 9 | Going Home declaration | Backend implemented #100; confirm deployment | Section6.5 | POST /going-home with request_id. Known IN required; no approval or return date. Keep presence scan-based and preserve the UUID on retries. |
| 10 | Complaints, photos and progress | Available with existing limits | Sections7 and8 | JSON create; separate timeline and authenticated photo requests. Maximum 3 JPEG/PNG images, Pending-only new photo changes. No title/severity. Handle partial uploads and versions. |
| 11 | Announcements and notification inbox | Available core feed/detail/read-one workflow | Sections9 and10 | Use announcement feed/details and /notifications. Derive isRead from read_at, navigate by source ID and refresh unread count. Dedicated notification-detail GET and read-all are absent. |
| 12 | Phone push notifications | Backend implemented in #89 feature-branch work; mobile/live testing pending | Section11 | Configure FCM/APNs as applicable, register token, handle refresh/taps/foreground and logout. Confirm running revision/migration 25/FCM enablement; observe real phone receipt. |

### How to use these statuses

- Available means the relevant backend contract exists. It does not certify that
  the mobile screen has been connected, the shared environment is deployed or
  real provider/device behavior has passed acceptance.
- Partial means some genuine data/workflow is available but the complete screen
  cannot be built from current Student endpoints alone.
- Missing means no agreed implemented Student endpoint currently supplies that
  workflow. UI mocks must not be presented as working backend integration.
- Backend implemented/mobile pending means client setup and real integration are
  still required; it does not mean another Go endpoint must be invented.

The 18-day informational summary, pending cancellation and combined history are
implemented in #101-#103. Student-ID login and bulk mark-read remain separate
requests. A teammate's mock contract does not automatically approve extra scope.
Section18 distinguishes those requests from implemented behavior. For the complete
application's intended screen design and acceptance scope, retain the SRS/SDS and
approved amendments alongside this current API reference.

## 1. Read this first

1. Flutter authenticates with Firebase Authentication. There are no Go /auth/login,
   /auth/forgot-password, /auth/reset-password or /auth/logout endpoints.
2. Every business API path starts with /api/v1. Send a Firebase ID token as Bearer.
3. /me returns account metadata. Use /me/student-profile for the Student profile, guardians, current room/bed and scan-based presence.
4. Extended leave and late access retain separate write/detail endpoints. Use
   GET /api/v1/me/passes for combined history; pending cancellation is source-specific.
   GET /api/v1/me/leave-allowance provides an informational 18-day summary.
   Going Home uses /going-home; section6.5.
5. Complaints are created as JSON. Photos upload separately as image bytes.
   Maximum THREE attached photos, not five; title and severity are unsupported.
6. The inbox is /notifications. Read state is read_at, not is_read. There is no
   notification-detail GET or mark-all-read endpoint.
7. Approval authorizes a trip; it never changes attendance. Only accepted scanner
   movements establish IN/OUT. Do not change presence optimistically in Flutter.
8. Backend APIs enforce ownership. Do not give the mobile app Admin credentials
   to work around missing Student APIs.

## 2. Connection, headers and data conventions

### 2.1 Base URL

Set one environment-specific API base URL ending in /api/v1, for example:

```text
API_BASE_URL=https://YOUR_VERIFIED_API_HOST/api/v1
```

This is a PLACEHOLDER, not an available deployment. Use it consistently; do not
append /api/v1 twice. Store configuration outside individual repository methods.

The backend owner's PC currently uses http://127.0.0.1:18080 for local testing.
On a physical phone, 127.0.0.1 refers to the PHONE, not the PC. A reachable backend
address and appropriate network configuration are required for phone-to-API tests.
A working Firebase connection alone does not make the local Go server reachable.
Use a team-approved HTTPS endpoint for shared integration; do not ship a development
cleartext exception or disable certificate verification in production.

### 2.2 Authentication and content headers

For JSON writes:

```http
Authorization: Bearer FIREBASE_ID_TOKEN
Accept: application/json
Content-Type: application/json
```

For JSON GET/empty POST/DELETE, omit Content-Type if there is no body. Send no
body, not even {}, on endpoints documented as empty-body requests.

For image upload use image/jpeg or image/png, and send the file bytes directly.
For QR/photo downloads accept image/png or the applicable image media type; always
check the HTTP status and Content-Type before attempting to display bytes.

### 2.3 IDs: do not interchange these values

| Value | Meaning | Where to use it |
| --- | --- | --- |
| Firebase UID | Firebase identity string | SDK identity; backend resolves it from the ID token |
| user_id | Local account UUID | Account references; NOT a Student profile UUID |
| student_id | Student profile UUID | Returned by /me/student-profile and leave/complaint requests; not returned by /me |
| request_id | Client-generated UUID for one logical operation | Create leave/late-access/complaint or add note; retain for exact retries |
| leave_id | Server-issued extended-leave UUID | /leaves/{leaveID} |
| late_access_id | Server-issued single-movement permission UUID | /late-access-requests/{lateAccessID} |
| complaint_id | Server-issued complaint UUID | Complaint detail/timeline/photos |
| image_id | Client-generated UUID for one logical photo | Photo upload/read/delete path |
| notification_id | Server-issued per-recipient inbox UUID | Mark read and notification navigation |
| announcement_id | Server-issued notice UUID | Announcement detail |
| device_id for push | Client installation UUID, retained locally | /me/push-devices/{deviceID}; NOT a gate scanner ID |
| FCM token | Phone push routing token | Push registration body; NOT Authorization |
| Firebase ID token | Current signed-in identity credential | Authorization: Bearer; NOT an FCM token |

Use random UUIDs for client-generated IDs. Generate once per operation, not on
 every network attempt. Never use an email, list index, QR payload or Firebase UID
as a replacement for a UUID field.

### 2.4 JSON and timestamps

- JSON property names are snake_case and enum strings are case-sensitive.
- Objects are returned directly; there is no universal data/success wrapper.
- Parse unknown response fields tolerantly. Send ONLY documented request fields;
  unknown write fields and unsupported/repeated query parameters are rejected.
- For timestamp fields, use RFC3339 with an explicit timezone: 2027-01-15T09:00:00+05:30 or
  2027-01-15T03:30:00Z. Do not send date-only or timezone-free strings for timestamp fields. The history endpoint in section4.4 instead accepts calendar-date filters as YYYY-MM-DD.
- Parse timestamps into DateTime values; compare instants rather than formatted
  strings. The backend may return another offset for the same instant.
- The default hostel policy timezone is Asia/Colombo. Flutter device timezone is
  not authoritative for permitted gate hours. Ask the backend owner if policy changes.
- Null means no value yet. Optional response fields may be absent. Do not replace
  null dates/IDs with invented defaults or assume missing presence means OUT.
- User-entered text is plain text, not trusted HTML. Render it as text.

### 2.5 Pagination

Most collection responses use:

```json
{
  "items": [],
  "total": 0,
  "limit": 20,
  "offset": 0,
  "has_more": false
}
```

Default limit 20; supported range 1-100; offset must be >=0. total is the number of
matching records, not the number on this page. Stop when has_more is false; next
offset is current offset plus received item count. Avoid an endless loop if an
unexpected empty page arrives. Reset offset to0 when changing filters/refreshing.

Each request reads current data, not a frozen multi-page session. New records may
shift offset pages; deduplicate by stable ID and support refresh. Do not append a
repeated page after retry. Photo lists and push-device lists have their own shapes.

### 2.6 Error responses

Handled API errors normally have:

```json
{"error":"invalid_input"}
```

There is no guaranteed human-readable message or per-field error list. Map known
codes to clear UI text, retain an unknown-error fallback and never display raw HTML
or internal diagnostics. Router/proxy errors, including unmatched404/405, may be
plain text; do not assume every non-2xx response is JSON.

| HTTP | Client behavior |
| --- | --- |
| 200/201 | Parse the endpoint-specific success object |
| 204 | Success with no body; do not JSON-decode |
| 400 | Fix input/query; no automatic retry loop |
| 401 | Obtain a fresh Firebase ID token once; if still denied, require sign-in |
| 403 | Account/role/permission failure; setup_required routes an invited Student to section3.5; refresh is not a permission workaround |
| 404 | Not found OR deliberately inaccessible to this owner; use a safe message |
| 409 | Business conflict, request-ID mismatch, stale version or eligibility failure; inspect error code and refresh appropriate state |
| 413 | Request exceeds body/image limit |
| 415 | Wrong Content-Type |
| 503 | Service/provider/database temporarily unavailable; preserve input and offer bounded retry |
| Network timeout | Outcome may be unknown; retry mutations with the SAME operation identity and original payload |

The same error code can represent multiple business conflicts. The server does
not currently provide an 18-day quota error, ALREADY_OUTSIDE error or the proposed
422 past-departure response.

## 3. Firebase authentication and session lifecycle

### 3.1 Project and responsibility

Use Firebase project hostel-hive-152ef. Configure Firebase in the Flutter app with
its existing registered application. Firebase owns passwords, reset emails and
client token refresh. The Go API verifies ID tokens and checks the current local
role/status on every request. A valid Firebase account by itself does not create
a HostelHive local account/profile or grant Student access.

Accounts are provisioned through the agreed Admin workflow. Do not add public
self-registration merely because the Firebase SDK supports creating an account.
No service-account JSON, R2 secret or scanner credential belongs in the mobile app.

### 3.2 Sign in, then verify application access

Flutter SDK sketch (merge into the existing AuthRepository):

```dart
final result = await FirebaseAuth.instance.signInWithEmailAndPassword(
  email: email.trim(),
  password: password,
);
final idToken = await result.user!.getIdToken();
if (idToken == null) {
  throw StateError('No Firebase ID token was returned.');
}
// GET API_BASE_URL + '/me', Authorization: Bearer idToken.
// Enter the Student app only after /me succeeds and role == 'student'.
```

Do not trim or change the password. Student-ID login is not implemented; use email.
The SDK result is not the proposed backend {token, student} response. Store the
/me account separately from Firebase's User object and the /me/student-profile response.

A revoked/expired/disabled token normally produces401; a valid Firebase identity
without an active eligible local account produces403. A transient account lookup
failure can produce503 service_unavailable. Wrong-role users must not enter the
Student workflow even if their Firebase sign-in succeeded.

Reference: [Firebase Flutter email/password authentication](https://firebase.google.com/docs/auth/flutter/password-auth).

### 3.3 Restore session and refresh

Use Firebase's authentication state to restore its signed-in user, obtain a current
ID token and call /me again. Do not assume persisted account metadata still grants
access. Retrieve tokens through the SDK rather than persisting a bearer string
forever. Keep account-specific caches scoped to the active account.

On an API401, try getIdToken(true) once and retry the same request once. For a
mutation, retain the SAME request_id/image_id and canonical body. Persistent401
requires sign-in; repeated403 requires handling account/role access. Do not
silently refresh forever or retry a decision/create with a new UUID.

Reference: [Firebase Flutter User.getIdToken](https://pub.dev/documentation/firebase_auth/latest/firebase_auth/User/getIdToken.html).

### 3.4 Reset/change password and logout

| User action | Flutter Firebase flow | Backend caveat |
| --- | --- | --- |
| Forgot password | sendPasswordResetEmail(email: email) | Show a generic completion message; Go does not provide the proposed204 endpoint |
| Open reset link | Follow the configured action handler; a custom Flutter handler verifies the action code and confirms password reset | Reset action code is not a bearer/FCM token |
| Change password | Reauthenticate if required, then currentUser.updatePassword(newPassword) | No Go change-password route or automatic account activation |
| Logout | Unregister push while authenticated, delete/rotate the FCM token, then FirebaseAuth.instance.signOut() | No Go revoke-all-sessions/logout endpoint; local sign-out is not proof old tokens were server-revoked |

After credential mutation, perform the agreed fresh-sign-in flow rather than
reusing saved API tokens. Firebase errors are SDK exceptions, not Go error JSON.
Do not report a failed reset request as successful network delivery. Never log
passwords, reset codes, bearer/refresh tokens or FCM routing tokens.

### 3.5 Admin-approved initial setup (#95)

Admin registers an approved university email and profile, individually or through
`/student-invitations/import`. The worker sends a private Firebase-hosted password
setup/reset email. These are Admin operations, not mobile self-registration.
The Student opens the email and chooses their own separate HostelHive password.
Admin does not know it. Do not request a university-issued password or implement SSO.
Use the hosted Firebase action page; no Go API accepts the new password or returns
an Admin-copyable action link. General later password recovery remains Firebase.

After choosing the password:

1. Sign in through Firebase Email/Password and obtain the current ID token.
2. Call `/me`. If `setup_required` is true, stop normal feature loading, including
   profile, QR, inbox and push registration. They return403 setup_required.
3. Send `POST /api/v1/me/setup/complete` with the Bearer token, no query and no body
   (not `{}`). Success200 contains setup_required:false and setup_completed_at.
4. Refresh `/me`, then proceed with the current-role/profile checks and API calls.

Example success (fictional timestamp):

```json
{"setup_required":false,"setup_completed_at":"2026-10-16T10:00:00Z"}
```

The backend checks the reserved UID, approved email, current eligibility, unrevoked
password sign-in and Firebase password-update metadata newer than the committed
bootstrap baseline. Password sign-in/reauthentication must be within five minutes
and at/after that password update. A forced token refresh alone does not update
Firebase auth_time. On409 setup_not_verified, complete the hosted setup or freshly
sign in/reauthenticate with the chosen password and retry once. Never set a local
completion flag to bypass this response. Do not loop password resets automatically.
On503 preserve pending state and offer a bounded retry/contact Admin. Inactive or
archived accounts cannot complete setup; password setup does not reactivate them.

Completion is idempotent and preserves the first completion timestamp. A legacy
eligible Student with no invitation gets false/null; existing accounts are not
forced through this flow. In `/me`, false setup_required is omitted; absent means
false. The completion response always includes the boolean.

Provider email acceptance is not proof of receipt. Firebase handles expired/used
action-code errors on the hosted page. Offer contact/resend through the authorized
Admin; do not promise a code lifetime or resend invalidation. Real email receipt,
action-code handling and Flutter integration remain unverified in this snapshot.

Backend prerequisites: migration26, STUDENT_INVITATIONS_ENABLED=true, approved
STUDENT_EMAIL_DOMAINS and Firebase Authentication/IAM email configuration. With the
flag off, pending accounts stay blocked and setup completion returns503. No server
credentials belong in Flutter. See [the full Admin contract](../student-invitations.md)
for registration, bulk outcomes, cooldown, retries and operational rollback.

Reference: [Firebase Flutter user management](https://firebase.google.com/docs/auth/flutter/manage-users).

## 4. Account and profile

### 4.1 GET /api/v1/me

Auth: any active provisioned application account. Body/query: none. Success 200:

```json
{
  "user_id": "11111111-1111-4111-8111-111111111111",
  "firebase_uid": "EXAMPLE_FIREBASE_UID",
  "email": "student@example.invalid",
  "role": "student",
  "is_active": true,
  "full_name": "Example Student"
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| user_id | UUID string | Local account ID |
| firebase_uid | string | Provider identity |
| email | string | Account email |
| role | string | student, admin, warden, sub_warden or security_staff |
| is_active | boolean | Current local account state |
| full_name | optional string | Current profile-derived name; may be omitted |
| designation | optional string | Staff designation where applicable; normally absent for Students |
| setup_required | optional boolean | True means invitation setup is unfinished; absent means false. Follow section3.5 before loading other APIs. |

These fields are account metadata only. Use section4.3 for full Student details.
Staff-only /students and /allocations routes remain unavailable to Students.
Faculty is the schema's field; do not silently treat it as a department.

### 4.2 GET /api/v1/me/qr

Auth: Student with an active account and unarchived Student profile.
Body/query: none. Success 200 is raw image/png bytes, 320x320 PNG; not JSON/base64.
Fetch with authentication and display from bytes, for example Image.memory(bytes).
Handle non-2xx JSON/text errors before creating the image widget.

The image encodes HH1: plus an opaque identifier. It is stable across repeated
retrievals, not a URL, Firebase token, short-lived OTP or evidence of IN/OUT status.
Do not generate another QR from student_id/email. A normal phone QR reader may
show plain text instead of a web link; gate scanners consume this application format.

A missing/archived/ineligible profile can give 404 not_found; auth403/401 and
student_profiles_unavailable503 are also possible. Respect Cache-Control:no-store;
clear displayed private data on logout/account switch. No Student QR rotation API.

### 4.3 Own Student profile, accommodation and presence (#91)

```http
GET /api/v1/me/student-profile
Authorization: Bearer FIREBASE_ID_TOKEN
```

Active Student role only. The target comes from the verified Firebase identity;
no student_id, user_id, filters, query parameters or request body are accepted.
Success 200 is a direct JSON object. Account-only /me remains unchanged.
All profile, guardians, active allocation and presence reads share one read-only
PostgreSQL repeatable-read snapshot. Reads do not update attendance or timestamps.

```json
{
  "student_id": "11111111-1111-4111-8111-111111111111",
  "index_no": "SC/2026/001",
  "full_name": "Example Student",
  "faculty": "Science",
  "year": 2,
  "contact_phone": "+94771234567",
  "email": "student@example.invalid",
  "guardians": [
    {
      "guardian_id": "22222222-2222-4222-8222-222222222222",
      "name": "Example Guardian",
      "relationship": "Parent",
      "contact_phone": "+94771234568"
    }
  ],
  "profile_image_url": "/api/v1/me/student-profile/image",
  "accommodation": {
    "allocation_id": "33333333-3333-4333-8333-333333333333",
    "block_id": "44444444-4444-4444-8444-444444444444",
    "block_name": "Block A",
    "room_id": "55555555-5555-4555-8555-555555555555",
    "room_no": "101",
    "bed_id": "66666666-6666-4666-8666-666666666666",
    "bed_no": "B1",
    "started_at": "2026-10-01T09:00:00+05:30"
  },
  "presence": {
    "status": "IN",
    "updated_at": "2026-10-15T10:00:00+05:30"
  }
}
```

All listed keys are returned. Guardian fields are strings; year is an integer.
The remaining profile fields and accommodation IDs/names are strings, with
started_at an RFC3339 timestamp. guardians is an array, empty when none exist.
accommodation is null when no current allocation exists, including after its end.
profile_image_url is null when no photo is attached. presence is always an object:
status is IN or OUT from recorded scans, or UNKNOWN with updated_at null when
there is no presence record. A known status has an RFC3339 updated_at timestamp.
This is the last recorded scan state; approval and the passage of time do not
change it. No is_inside boolean is supplied; preserve UNKNOWN in the UI.

Download the optional profile photo with a second authenticated, empty-body GET:

```http
GET /api/v1/me/student-profile/image
Authorization: Bearer FIREBASE_ID_TOKEN
```

Success 200 returns normalized JPEG/PNG bytes, with the actual Content-Type,
Cache-Control:no-store and X-Content-Type-Options:nosniff. Resolve the returned
origin-relative URL against the API origin, not by appending it to /api/v1.
Use an HTTP image loader that sends the Bearer token; do not put tokens in URLs.
The app receives no R2 object key, credentials or public bucket URL. Clear cached
private data on logout/account switch. This endpoint only reads the signed-in
Student's photo; Student upload/update/delete is not introduced by this ticket.

| Result | Meaning / client handling |
| --- | --- |
| 400 invalid_input | Query parameters or a non-empty GET body; fix the request |
| 401 unauthorized | Invalid/expired/missing Firebase login; refresh once as described above |
| 403 forbidden | Inactive account or non-Student role; stop, do not loop retries |
| 404 not_found | Missing/archived Student profile, or no photo on the image endpoint |
| 503 student_profiles_unavailable | Database or image-object read failure; bounded retry |
| 503 student_images_unavailable | An attached photo exists but image storage is not configured |

Profile JSON remains available when R2 is disabled. An image URL indicates an
attached photo, not proof that storage is currently reachable. Handle image 404
if a staff member removes it between the two requests. No profile-image bytes
are embedded in JSON. The route accepts no other Student identifier and does
not grant access to staff-only profile, allocation or attendance readers.

### 4.4 Own recorded attendance history (#93)

```http
GET /api/v1/me/attendance/events?from=2026-10-01&to=2026-10-16&direction=IN&limit=20&offset=0
Authorization: Bearer FIREBASE_ID_TOKEN
```

Active Students with an unarchived profile only. Ownership is derived from the
verified login. Do not send student_id or user_id. GET body must be empty.
This is a read-only history of accepted, recorded movements. Cooldown and
already-IN/already-OUT attempts are excluded. Repeated recorded movements, if
present in legacy data, are preserved as distinct records rather than inferred
or removed by the reader. Approval, permission consumption and current presence
are not changed by reading this endpoint.

| Query | Rules |
| --- | --- |
| from and to | Optional together; strict YYYY-MM-DD, both inclusive hostel-local dates. Start must not follow end. Maximum 366 calendar days; supported years 0001 through 9998. If neither is supplied, page through all own recorded history. |
| direction | Optional, case-sensitive IN or OUT. Omit to include both. |
| limit | Optional integer, default 20; minimum 1, maximum 100. |
| offset | Optional integer, default 0; minimum 0, maximum 1000000. |

Empty, repeated, unknown or malformed query parameters return 400. An isolated
from or to is invalid. Client timezone parameters are not accepted; use the
configured hostel timezone returned in the page (normally Asia/Colombo).
The server translates the inclusive date range to local start-of-day through
start-of-day AFTER to, using an exclusive upper bound. For example, 2026-10-15
in Asia/Colombo covers 2026-10-14T18:30:00Z inclusive through
2026-10-15T18:30:00Z exclusive. Calendar boundaries respect timezone/DST rules.

Success 200, direct JSON:

```json
{
  "items": [
    {
      "movement_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "event_id": "11111111-1111-4111-8111-111111111111",
      "direction": "IN",
      "occurred_at": "2026-10-15T09:00:00+05:30"
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0,
  "has_more": false,
  "timezone": "Asia/Colombo",
  "from": "2026-10-01",
  "to": "2026-10-16"
}
```

items is always an array, including [] when empty. total counts all matching
recorded movements, not just the current page. No matches, or an offset beyond
the last page, return 200 with an empty items array and has_more false; total
still describes all matching rows. from/to are null when dates are omitted.

movement_id is a stable opaque 64-character lowercase hexadecimal identifier.
Use it as the Flutter list/deduplication key. event_id is the scanner event UUID;
it is unique within a scanner, so event_id alone must not be treated as a globally
unique list key. No scanner/device identity or credentials, payload hashes,
processing outcomes, access results, permission or incident references are returned.

occurred_at is an RFC3339 timestamp; parse the instant and display it in the
hostel timezone. Results sort by occurred_at descending, then the internal
composite event key descending to resolve ties. Each page's count and items use
one read-only repeatable-read database snapshot. Offset pages are stable while
the dataset is unchanged. New scans between separate page requests can shift
positions; refresh/reset pagination and merge by movement_id when appropriate.

| Result | Meaning |
| --- | --- |
| 400 invalid_input | Invalid/unsupported filters, duplicate keys or non-empty GET body |
| 401 unauthorized | Missing, invalid or expired authentication |
| 403 forbidden | Inactive or non-Student account; middleware may deny revoked Firebase tokens with 401 |
| 404 not_found | Student profile missing or archived |
| 503 attendance_unavailable | Database read failed; retry with bounded backoff |

Handle 401 with the existing single-refresh policy. Empty history is not UNKNOWN
presence and must not be used to set IN or OUT. Read current presence from
/me/student-profile instead. Staff-only attendance and report routes remain
protected; this ticket adds no Student export, scan submission or editing API.

## 5. Extended leave requests

### 5.1 POST /api/v1/leaves

Auth: Student with an unarchived profile. Content-Type:application/json.
No query. JSON body limit 16 KiB. First creation AND exact creation retry return 201.

```json
{
  "request_id": "44444444-4444-4444-8444-444444444444",
  "starts_at": "2027-01-15T09:00:00+05:30",
  "ends_at": "2027-01-16T09:00:00+05:30",
  "reason": "Family visit"
}
```

| Request field | Validation |
| --- | --- |
| request_id | UUID generated once for this submission |
| starts_at | Explicit-offset timestamp; must be future for a new request |
| ends_at | Explicit-offset timestamp strictly after starts_at |
| reason | Trimmed nonblank text, maximum 1000 Unicode code points |

Ownership is derived from authentication. Do not send student_id, status, title,
destination or decision fields. Pending/approved periods for the same Student
cannot overlap. Intervals are [start,end): one request may start exactly when
another ends. Rejected/cancelled requests do not reserve a period. The 18-day
annual allowance summary is informational and does not block new requests.

Success object (also used by GET detail):

```json
{
  "leave_id": "33333333-3333-4333-8333-333333333333",
  "request_id": "44444444-4444-4444-8444-444444444444",
  "student_id": "22222222-2222-4222-8222-222222222222",
  "submitted_by": "11111111-1111-4111-8111-111111111111",
  "starts_at": "2027-01-15T03:30:00Z",
  "ends_at": "2027-01-16T03:30:00Z",
  "reason": "Family visit",
  "status": "pending",
  "created_at": "2026-10-15T14:00:00Z",
  "decided_at": null,
  "decided_by": null,
  "decision_note": ""
}
```

The server generates leave_id and timestamps. submitted_by/decided_by are local
user UUIDs, not display names. status is pending, approved, rejected or cancelled. Decision
fields remain null/empty until decided. The object does not expand Student/staff
profiles. On approval, do not mark the Student OUT in Flutter or RTDB.

### 5.2 GET /api/v1/leaves

Student sees owned requests automatically. Supported Student query: status, limit,
offset. The route also accepts a staff student_id filter, but mobile should OMIT
it; another Student's ID returns 403. Example:

```http
GET /api/v1/leaves?status=pending&limit=20&offset=0
```

Returns the standard page object with Request objects in items. Order is
created_at DESC then leave_id DESC. Omit status to include all four statuses.
Cancelled records also include cancelled_at and cancelled_by; see the #101 contract.

### 5.3 GET /api/v1/leaves/{leaveID}

Body/query:none. Success 200: the same Request object. Another Student's or missing
record returns 404. The decision is included as status/decision_note/decided_at/
decided_by. Fetching does not mark any notification read.

### 5.4 Leave errors, decisions and missing actions

| Code | Meaning |
| --- | --- |
| 400 invalid_input | Invalid dates/UUID/text/query, a past start or unsupported fields |
| 409 leave_conflict | Overlap, changed reuse of request_id, or conflicting decision |
| 409 active_student_required | Student profile/account no longer eligible for the operation |
| 404 not_found | Record missing or not accessible to this Student |
| 503 leave_unavailable | Temporary backend failure |

A retry of an already-created request retains its original record, including later
decisions/cancellation. Do not treat repeated201 as another leave. There is no
edit/delete or Student approval endpoint. Pending cancellation uses
POST /api/v1/leaves/{leaveID}/cancel with no body; yearly allowance uses
GET /api/v1/me/leave-allowance. Going Home uses /going-home; section6.5.

For test coordination only, Admin/Warden use:

```http
POST /api/v1/leaves/{leaveID}/decision
Content-Type: application/json

{"status":"approved","note":"Approved for the requested dates."}
```

Success 200: Request. Rejection requires a nonblank note. Student calls return 403.
The mobile Student app must not expose or invoke staff decision actions.

## 6. Late-access requests: early exit or late entry

### 6.1 Model and hostel hours

A request authorizes ONE expected movement in a specified direction. It is not an
extended leave interval, round-trip pass or destination record.

Default Asia/Colombo policy:

| Movement | Needs permission |
| --- | --- |
| OUT before 06:00 | Yes: early exit |
| OUT at/after 06:00, including after 20:00 | No, under this specific policy |
| IN from 06:00 until just before 20:00 | No |
| IN at/after 20:00 or overnight before 06:00 | Yes: late entry |

The backend rejects a new late-access request for a time/direction that does not
require permission. Hours are configurable; these are defaults, not a guarantee
about every deployed environment. Display hostel-local time clearly in the app.

### 6.2 POST /api/v1/late-access-requests

Auth: Student with an unarchived profile. JSON body, no query; maximum 16 KiB.
Success 201 for first creation and exact submission retry.

```json
{
  "request_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
  "direction": "IN",
  "expected_at": "2027-01-15T21:00:00+05:30",
  "reason": "Travel delay"
}
```

request_id is a client UUID. direction must be uppercase IN or OUT. expected_at
must be future for a new request and meet the restricted-hours policy. reason is
trimmed, nonblank, maximum 1000 Unicode code points. No Student/status override.
No title, purpose, departure_at, return_at or destination fields are accepted.

Example response (also the detail shape):

```json
{
  "late_access_id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
  "request_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
  "student_id": "22222222-2222-4222-8222-222222222222",
  "submitted_by": "11111111-1111-4111-8111-111111111111",
  "direction": "IN",
  "expected_at": "2027-01-15T15:30:00Z",
  "reason": "Travel delay",
  "status": "pending",
  "created_at": "2026-10-15T14:10:00Z",
  "decided_at": null,
  "decided_by": null,
  "decision_note": ""
}
```

After a matching accepted scan, optional consumed_at, consumed_device_id and
consumed_event_id fields may appear. These describe pass usage, not a fourth
request status. They are omitted until used. status remains approved; do not
expect a server status called completed, used or cancelled.

### 6.3 List and detail

```http
GET /api/v1/late-access-requests?status=approved&limit=20&offset=0
GET /api/v1/late-access-requests/{lateAccessID}
```

List supports status=pending|approved|rejected|cancelled, limit and offset; omit status for
all. Do not send student_id for Student requests. Items are owned Request objects,
ordered created_at DESC then late_access_id DESC, in the standard page envelope.
Detail200 returns one Request, including decision/optional consumption metadata.
A missing or another Student's record returns 404. GET body must be empty.

### 6.4 Matching, conflicts and approval

An approved permission can be consumed by one recorded movement of the same
Student/direction within 30 minutes before/after the expected time. A second scan
is not automatically a second authorized use. Approval/notification is not proof
that the Student physically moved. A suppressed duplicate scan does not consume
another permission. Without a matching permission, a restricted movement is still
recorded and an incident is created for staff; mobile must not describe approval
as a remotely opened door or backend denial as a physical gate lock.

Two active requests cannot use the identical Student/direction/expected-time
combination. Exact request_id retries return the original record; changed reuse
conflicts. Distinct expected times can coexist. Rejected requests allow a replacement.

| Code | Meaning |
| --- | --- |
| 400 invalid_input | Invalid/past expected_at, normal-hours request, direction, UUID, text or query |
| 409 late_access_conflict | Duplicate active movement time, changed request-ID reuse or conflicting decision |
| 409 active_student_required | Ineligible Student/profile |
| 404 not_found | Missing/not-owned request |
| 503 late_access_unavailable | Temporary failure |

Past-time validation is 400, NOT the422 in the proposed mobile contract.
For test coordination, Admin/Warden use POST /api/v1/late-access-requests/{lateAccessID}/decision
with {"status":"approved","note":"Approved."}; success 200 Request. Rejection
requires a note. Student decision calls are forbidden. Pending cancellation uses
POST /api/v1/late-access-requests/{lateAccessID}/cancel with no body. There is no
reminder API; late-access requests do not consume the yearly leave allowance.

### 6.5 Adapting PassRepository

Keep a discriminator such as leave or late_access with each mobile pass item.
Use its type plus server ID for detail/navigation; do not assume all UUIDs belong
to one collection. Map decision_note to the UI's Warden comment label. Do not
mislabel submitted_by or decided_by UUIDs as human names.

Use GET /api/v1/me/passes for one globally paginated feed (#103). It accepts optional
type=leave|late_access, status=pending|approved|rejected|cancelled, limit and offset.
Order is created_at DESC, type ASC, source_id DESC. Each item retains source_id,
request_id, status, reason, nullable leave/movement times and decision/cancellation
audit. Use (type, source_id) as its key and source-specific detail/cancel routes.
Totals and items share one snapshot per response; separate pages can change when
new requests arrive. Refresh from offset0 after mutations. Going Home is excluded.
For exact fields, errors and examples see [combined-pass API](../combined-student-pass-history.md).
Use GET /api/v1/me/leave-allowance for the real informational annual balance (#102);
it does not block requests. See [the counting policy](../yearly-leave-allowance.md).

### 6.5 Going Home declarations (#100)

```http
POST /api/v1/going-home
Authorization: Bearer FIREBASE_ID_TOKEN
Content-Type: application/json
```

```json
{"request_id":"10000000-0000-4000-8000-000000000010"}
```

An active Student with completed setup, an unarchived profile and known IN presence
can declare an intention without Warden approval or a return date. Send only the
nonzero UUID request_id; no query, owner, timestamps or form extras. Success201
returns going_home_id, request_id, student_id, submitted_by, status, declared_at,
nullable departed_at, departure_device_id and departure_event_id. The initial
status is declared. Exact retries return200 with the same ID/time and current
lifecycle, including departed if the Student has since exited.

Do not mark the Student OUT in Flutter after this response. Only the next new
recorded OUT scan changes physical presence and marks the declaration departed.
Suppressed scans and old scan retries do not consume it. Early-exit and late-entry
permissions remain separate. There is no required return date or automatic expiry;
if the Student never exits, the declaration remains outstanding.

409 already_outside means new declarations require an actual IN scan first.
409 presence_unknown means presence must first be established by a scan; do not
assume inside. 409 going_home_pending means an outstanding declaration already
exists. Missing/archived profile gives409 active_student_required; existing401,
403/setup_required,400 invalid_input,413,415 and503 going_home_unavailable apply.
Keep the same request UUID on an unknown network outcome; do not generate another
one for a retry. Replays still require current eligibility. A new declaration
after departure and return requires a new UUID.

This endpoint creates a durable declaration, not an approved extended leave or
late-access permission, notification or quota usage. No new GET/list/cancel route
is supplied here. Keep the returned declaration/request ID; future feed inclusion
needs an explicit contract. Full lifecycle: [Going Home API](../going-home-declarations.md).

## 7. Complaints: creation, lists, details and notes

### 7.1 POST /api/v1/complaints

Auth: Student with an unarchived profile. JSON, not multipart. No query;
maximum 16 KiB. Success 201 for first submission and exact creation retry.

```json
{
  "request_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
  "category": "maintenance",
  "description": "The room light is not working."
}
```

category is maintenance or disciplinary. description is trimmed, nonblank,
maximum 2000 Unicode code points. Backend text length is Unicode code points,
not UTF-8 bytes or necessarily Flutter String.length. Ordinary newlines/tabs are
allowed in these descriptions/reasons; other control characters are invalid.

No title, severity, owner, status or photos in this JSON. Do not silently encode
unsupported mandatory UI fields into another property without agreeing that UX.

Response:

```json
{
  "complaint_id": "55555555-5555-4555-8555-555555555555",
  "request_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
  "student_id": "22222222-2222-4222-8222-222222222222",
  "submitted_by": "11111111-1111-4111-8111-111111111111",
  "category": "maintenance",
  "description": "The room light is not working.",
  "status": "pending",
  "version": 1,
  "created_at": "2026-10-15T14:20:00Z",
  "updated_at": "2026-10-15T14:20:00Z"
}
```

### 7.2 GET /api/v1/complaints

Supported filters: category, status, created_from, created_to, limit, offset.
Student ownership is implicit; omit the staff student_id filter. Another Student's
ID is forbidden. Example:

```http
GET /api/v1/complaints?category=maintenance&status=pending&limit=20&offset=0
```

category is maintenance|disciplinary; status is pending|in_progress|resolved.
created_from/created_to are offset-aware timestamps with [from,to) semantics,
NOT local date-only strings. URL-encode plus signs/colons through Uri query handling;
a literal + in an unescaped query can become a space. Empty/unknown/duplicate
parameters are invalid. Returns the standard page containing Complaint objects,
ordered created_at DESC then complaint_id DESC.

### 7.3 GET /api/v1/complaints/{complaintID}

Body/query:none. Success 200 is a Complaint object. No embedded timeline or images.
Use the next endpoint for progress and section8 for images. Another Student's
complaint is 404. Students can read their own historical records; archival does not
mean deleting complaint history, but it can prevent new Student writes.

### 7.4 GET /api/v1/complaints/{complaintID}/updates

Optional query: limit, offset only. Default 20; max 100; offset>=0. Success 200 is the
standard page, with timeline Update objects ordered by version ascending.

```json
{
  "update_id": "66666666-6666-4666-8666-666666666666",
  "complaint_id": "55555555-5555-4555-8555-555555555555",
  "request_id": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
  "actor_id": "11111111-1111-4111-8111-111111111111",
  "action": "note",
  "previous_status": "pending",
  "new_status": "pending",
  "text": "The issue is still present.",
  "previous_version": 1,
  "version": 2,
  "occurred_at": "2026-10-15T14:25:00Z"
}
```

Action values currently include created, note, status, image_added and image_removed.
image_id is optional for photo actions. previous_status can be null for creation.
actor_id is a local user UUID; no expanded staff name is returned. Timeline text is
visible to the owning Student, not a confidential staff note channel.

### 7.5 POST /api/v1/complaints/{complaintID}/notes

Auth: owning eligible Student; complaint must be unresolved. JSON body:

```json
{
  "request_id": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
  "expected_version": 1,
  "text": "The issue is still present."
}
```

expected_version must be the latest known complaint version; minimum 1, below
9007199254740991. text is trimmed, nonblank, at most2000 code points. No status
field for a Student note. Success 200 is the Update object, not the full Complaint.
Refresh detail or update the known version from its response.

On409, retrieve current detail/timeline and let the user review/resubmit a genuinely
new action with a new request_id/current version. For a network retry of an action
whose outcome is unknown, retain the ORIGINAL request_id, expected_version and text;
exact successful retries return the original update even after later changes.
Do not automatically rewrite a stale note request and pretend it was the same action.
Photo changes also increment version, so refresh before posting a note afterward.

### 7.6 Status, ownership and complaint errors

Only authorized staff change status. Student status calls are403. Staff can use
pending->in_progress/resolved, in_progress->resolved, resolved->in_progress with a
reopening explanation. A Student cannot edit/delete the original description or
category, resolve, reopen or add a note after resolution. Updated status arrives
through detail/inbox refresh; do not invent a cancelled or closed status.

Admin/Warden handle both categories; Sub-Warden handles maintenance only. Security
has no complaint access. That staff restriction is not a restriction on a Student's
ability to submit/read their own disciplinary complaint.

| Code | Meaning |
| --- | --- |
| 400 invalid_input | Invalid JSON/UUID/category/text/query/version |
| 409 complaint_conflict | Changed ID reuse, stale version, forbidden lifecycle action or image conflict |
| 409 active_student_required | New Student write requires an eligible profile |
| 404 not_found | Missing/not-owned complaint/update/image |
| 503 complaints_unavailable | Complaint persistence failure |
| 503 complaint_images_unavailable | Image service/storage not configured or unavailable |

For a combined detail screen, fetch complaint, updates and image metadata as
separate calls. Each can observe a later state. Refresh if their returned versions
show intervening changes; there is no atomic combined-detail endpoint.

## 8. Private complaint photos

### 8.1 Limits and upload sequence

Maximum 3 active/reserved attachments per complaint. Each input must be nonempty
JPEG/PNG <=5,242,880 bytes (5MiB), <=4096 pixels in either dimension and <=12,000,000
pixels total. MIME type must match the actual decoded format. HEIC/WebP/GIF/SVG,
base64 JSON and multipart wrappers are unsupported. Convert/compress locally as
needed and preview the final intended JPEG/PNG before upload.

The backend decodes/re-encodes approved images, strips embedded metadata and uses
JPEG quality 85/PNG re-encoding. Do not assume downloaded bytes, embedded EXIF or
size_bytes equal the original file. A normalized image that exceeds limits is invalid.

1. Create the Complaint first and retain complaint_id.
2. Generate/retain one image_id per selected photo.
3. PUT the bytes to its endpoint, preferably sequentially for this small cap.
4. Track each photo's result; show partial success clearly.
5. GET current image list and complaint version after uploads.

Complaint creation and photo uploads are separate commits. If photo2 fails, the
complaint and photo1 may already exist. Do not create a second complaint on retry.
There is no server-side unsubmitted complaint/photo draft; pre-submit preview is
local. Do not pretend leaving the screen rolls back committed uploads.

### 8.2 PUT /api/v1/complaints/{complaintID}/images/{imageID}

Auth: owning eligible Student. Content-Type:image/jpeg or image/png. Body: raw bytes.
No query. imageID is a client UUID. The endpoint does not accept expected_version.
First completed upload201; exact completed retry200. Response:

```json
{
  "image": {
    "image_id": "77777777-7777-4777-8777-777777777777",
    "complaint_id": "55555555-5555-4555-8555-555555555555",
    "content_type": "image/jpeg",
    "size_bytes": 34567,
    "width": 1280,
    "height": 720,
    "uploaded_at": "2026-10-15T14:30:00Z",
    "attached_version": 2,
    "removed_version": null
  },
  "version": 2,
  "replayed": false
}
```

Numbers are illustrative. Keep the same image ID and bytes for a retry. Changed
content/type under that ID conflicts. A removed/expired ID cannot be resurrected;
a genuinely new photo needs a new ID. New attachments are Pending-only; an exact
retry of a previously completed upload can still acknowledge that existing result
after staff start handling the complaint. This does not permit a new attachment.

### 8.3 GET /api/v1/complaints/{complaintID}/images

Auth: owning Student or authorized staff. Body/query:none. Success 200:

```json
{
  "items": [],
  "version": 1
}
```

items contains Image metadata objects (same shape as image above), ordered by
uploaded_at then image_id. Only attached photos appear. This is not a standard
paginated envelope: there is no total/limit/offset/has_more. No public URL/object
key is returned. version is current complaint version, not an image count.

### 8.4 GET /api/v1/complaints/{complaintID}/images/{imageID}

Auth required on EVERY image request. Body/query:none. Success 200 returns raw
JPEG/PNG bytes with Content-Type, Content-Length, no-store and nosniff headers.
Retrieve through the authenticated API client; show image bytes in Flutter.
A bare Image.network URL without an Authorization header will not authenticate.
Do not store public R2 URLs or make the bucket public to display these photos.

### 8.5 DELETE /api/v1/complaints/{complaintID}/images/{imageID}

Auth: owning eligible Student. Body/query:none. New removal is Pending-only.
Success 200 JSON (NOT204):

```json
{
  "image_id": "77777777-7777-4777-8777-777777777777",
  "removed": true,
  "version": 3,
  "replayed": false
}
```

A repeat successful removal returns its stored removal version with replayed=true;
it does not add another timeline action. The raw storage object can be cleaned
asynchronously, but the removed photo is no longer an attached readable image.

New writes after Pending or a fourth reserved/attached photo return 409
complaint_conflict. Oversized HTTP upload is 413 payload_too_large, unsupported
Content-Type415 unsupported_media_type, malformed image data400 invalid_input.
Unavailable storage returns 503 complaint_images_unavailable. A reserved slot can
remain temporarily after a failed upload; retry the same ID or allow backend
cleanup. Never bypass authorization/storage errors with direct bucket access.

## 9. Announcements

### 9.1 GET /api/v1/announcements

Auth: Student with an active account/unarchived profile (Admin/Warden can also
read). Optional query: limit, offset only. Success 200 standard page containing:

```json
{
  "announcement_id": "99999999-9999-4999-8999-999999999999",
  "title": "Water supply maintenance",
  "content": "Water supply will pause from 10 AM to noon.",
  "audience": "all_students",
  "priority": "normal",
  "published_by": "ffffffff-ffff-4fff-8fff-ffffffffffff",
  "published_at": "2026-10-15T13:00:00Z"
}
```

Order: published_at DESC, announcement_id DESC. Publication is immediate and
immutable; only global all_students, normal-priority plain-text notices are
implemented. No targeted-group feed, schedules, announcement attachments or
Student publication. The feed can include older notices from before the Student
joined; this does not create retroactive inbox items for that Student.

### 9.2 GET /api/v1/announcements/{announcementID}

Body/query:none. Success 200: Announcement object above. Fetch does not automatically
mark an inbox item read. To mark a related notification, use its notification_id,
not announcement_id. A feed item without an inbox receipt has nothing to mark read.
The staff delivery-summary API is not a Student endpoint.

Errors include400 invalid_input,401 unauthorized,403 forbidden,404 not_found and
503 announcements_unavailable. A profile becoming archived can remove current
announcement access without deleting older complaint/leave history.

## 10. Notification inbox

### 10.1 GET /api/v1/notifications

Auth: Student sees eligible owned items. Query: limit, offset and unread=true|false
only. Default unread=false means ALL items, not only read items. There is no server
read-only filter, notification-ID lookup filter or source filter in this endpoint.

```http
GET /api/v1/notifications?unread=true&limit=20&offset=0
```

Success 200 standard page with Notification objects:

```json
{
  "notification_id": "88888888-8888-4888-8888-888888888888",
  "source_type": "leave",
  "leave_id": "33333333-3333-4333-8333-333333333333",
  "kind": "leave_decided",
  "status": "approved",
  "title": "Your leave request is approved",
  "occurred_at": "2026-10-15T14:40:00Z",
  "created_at": "2026-10-15T14:40:02Z",
  "read_at": null
}
```

| Field | Meaning |
| --- | --- |
| notification_id | Per-recipient inbox item ID; use for mark read |
| source_type | Source category; choose the appropriate detail repository |
| leave_id/late_access_id/complaint_id/announcement_id | Optional source reference; normally one applicable field |
| attendance_incident_id | Optional staff incident reference, not a Student-readable source |
| kind | Specific event type |
| category | Optional complaint category |
| status | Status associated with the EVENT, not guaranteed current source status |
| title | Server-provided plain-text summary |
| occurred_at | When the original event occurred |
| created_at | When inbox delivery created this item |
| read_at | Null until deliberately marked read |

Order: occurred_at DESC then notification_id DESC. A notification can describe
an earlier status even if a later decision/update has occurred; fetch the source
for current details. There is no notification body/message field containing the
full private complaint/leave reason.

### 10.2 Source navigation and Student event types

| source_type | Student kind | Detail request |
| --- | --- | --- |
| leave | leave_decided | GET /api/v1/leaves/{leave_id} |
| late_access | late_access_decided | GET /api/v1/late-access-requests/{late_access_id} |
| complaint | status | GET /api/v1/complaints/{complaint_id}, optionally /updates |
| announcement | announcement_published | GET /api/v1/announcements/{announcement_id} |

Staff inboxes additionally include complaint created, leave_submitted,
late_access_submitted and attendance_incident events. A Student must not receive
or try to open another role's incident data. Treat unknown source/kind values
with a safe generic UI rather than crashing. A removed permission/source can give 404.
Student notes/photo actions are in the complaint timeline, not all separate inbox
notifications. Do not promise a new inbox item for every timeline event.

### 10.3 GET /api/v1/notifications/unread-count

Body/query:none. Success 200:

```json
{"unread": 3}
```

This is the eligible owned inbox count, not a push-delivery count or number of
unapproved passes. Refresh after reading and on appropriate app foreground refresh.
The backend currently provides no Student inbox RTDB/WebSocket subscription API.
Use manual refresh or a team-agreed bounded polling strategy; push is a prompt to
refresh, not the source of truth or a guarantee every inbox event generates push.

### 10.4 POST /api/v1/notifications/{notificationID}/read

Body/query:none. Success 200: Notification object with nonnull read_at. Exact repeats
preserve its FIRST read_at. One Student reading does not mark another recipient's
receipt read. Another owner's or unavailable notification is 404 not_found.

Map isRead in the Flutter model to read_at != null. When marking from an unread
list, remove/update that item and then refresh the count. Do not mark every item
read merely because it appeared in a list. Fetching source detail alone does not
mutate read state; explicitly call this endpoint for the intended read action.

### 10.5 Unsupported inbox operations and delivery timing

There is no GET /notifications/{id}, POST /notifications/read-all, /inbox alias,
notification deletion or archive endpoint. Source-detail navigation is the supported
alternative to a dedicated notification detail route. If the UI needs a true
server bulk-read action, keep it pending a new contract. A loop of single-item calls
is not an atomic read-all operation and cannot claim unseen pages were marked.

Inbox delivery is asynchronous and durable. A successfully submitted/decided source
may not appear in a recipient's inbox in the same response instant. Refresh with a
bounded delay rather than generating the operation again. A transient provider
outage does not erase the source record or mean an inbox item was read.

Errors:400 invalid_input,401 unauthorized,403 forbidden,404 not_found,
503 notifications_unavailable, plus authentication-layer service_unavailable503.

## 11. Mobile push registration and receipt - issue #89

**Availability:** implemented in the inspected working tree; confirm merge,
migration 25, running binary and environment before using these routes. Backend
sending defaults OFF (FCM_ENABLED=false). Registration can succeed while sending
is disabled, so API200 alone does not prove phone notifications are enabled.

### 11.1 What sends phone push

| Source event | Who receives push |
| --- | --- |
| Leave approved/rejected | Owning Student's eligible registered phones |
| Late-access approved/rejected | Owning Student's eligible registered phones |
| Unapproved early-exit/late-entry incident | Eligible Admin/Warden phones, NOT the Student app |

Complaint updates and ordinary announcements remain inbox-only by agreement.
Do not show a failed-delivery error because those events produced no phone push.
No push replay/backfill is created when a phone is newly enrolled. Its current
binding must predate the event. Generate fresh test decisions AFTER registration.

### 11.2 PUT /api/v1/me/push-devices/{deviceID}

Auth: current active account. deviceID is a nonzero installation UUID generated
and persisted by Flutter; keep it across launches/token refresh for this installation.
JSON body, no query; only two fields:

```json
{
  "platform": "android",
  "token": "REPLACE_WITH_REAL_FCM_REGISTRATION_TOKEN"
}
```

platform is android or ios. Use the registration token from Firebase Messaging
getToken(), not Firebase UID, an installation ID/FID, APNs token or ID token. The
current Go adapter sends registration-token messages, not FID-targeted messages.
Token validation:20-4096 ASCII letters/digits/colon/underscore/hyphen; no whitespace.
Body maximum 16 KiB; duplicate/unknown JSON fields are rejected.

Success 200 (same for first registration and refresh):

```json
{
  "device_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  "platform": "android",
  "active": true,
  "updated_at": "2026-10-15T14:50:00Z"
}
```

No raw token or hash is returned. Maximum 10 active devices per user. The same active
FCM token cannot be bound to two registrations/accounts simultaneously. Same owned
installation/token/platform refresh is idempotent; changed token starts a new
binding generation so old queued jobs cannot target the updated registration.

### 11.3 GET /api/v1/me/push-devices

Body/query:none. Success 200:

```json
{"items": [], "total": 0}
```

Items use the metadata shape above. Only this account's ACTIVE registrations are
listed. No pagination parameters, token values or another account's devices.

### 11.4 DELETE /api/v1/me/push-devices/{deviceID}

Body/query:none. Success 204 empty body. An already inactive owned registration can
be deleted repeatedly. Unknown/another owner's registration is 404 not_found.
Unregister before Firebase sign-out while authentication still works. This clears
the stored routing token and prevents unsent jobs using that binding. It does not
revoke a Firebase login token or recall messages already accepted by FCM.

Errors:400 invalid_input,401 unauthorized,403 forbidden,404 not_found,
409 push_device_conflict (cap or active token collision),415 unsupported_media_type,
503 notifications_unavailable. Oversized/malformed push JSON is currently400
invalid_input; do not assume all endpoints use the same413 handling.

### 11.5 Flutter lifecycle

- Configure Firebase Messaging for the same Firebase project, obtain OS permission,
  then obtain the FCM token. For iOS, configure APNs/capabilities and wait for its
  token as required by the SDK. Use the Android setup first for this handoff test.
- After successful application login, register the installation token for THAT account.
- Subscribe to token refresh and PUT new tokens with the same installation UUID.
  Coordinate async callbacks with the current auth session; an old-account callback
  must not register its token after another account has logged in.
- Before account switch/logout, DELETE the old account registration, then delete/
  rotate the SDK FCM token and clear account data. If offline cleanup fails, record
  the limitation and delete the local SDK token; do not reuse the old token under
  the next account. Device registration may409 until ownership is safely cleared.
- Handle foreground messages, background notification display and both cold-start
  and background notification taps. Foreground display requires explicit handling.
- On tap, use the CURRENT account to refresh its inbox. Match notification_id to
  an authorized item and navigate using that source reference. Drop inaccessible
  IDs; do not treat push as permission or open another account's cached detail.

The payload uses generic visible text:

```json
{
  "notification": {
    "title": "HostelHive",
    "body": "You have a new notification. Open HostelHive to view it."
  },
  "data": {
    "notification_id": "88888888-8888-4888-8888-888888888888",
    "source_type": "leave"
  }
}
```

This shows the logical message content, not the complete SDK RemoteMessage envelope.
Do not expect leave_id, decision reason, Student name or private source details
inside push data. Use the inbox item. Device-level duplicate delivery is possible;
deduplicate client handling by notification_id. Android tags/APNs collapse IDs help
but do not guarantee exactly-once display. A generic already-accepted message can
arrive after logout; source retrieval must still require current authorization.

The backend retries transient failures with bounded backoff and honors longer
Retry-After; jobs expire one hour after the event. FCM acceptance is not phone
receipt, reading or a physical movement. UNREGISTERED invalidates that registration;
INVALID_ARGUMENT does not by itself prove the token is bad. Real Android/iOS receipt
remains a separate integration test, not established by backend fake-provider tests.

References: [Flutter FCM setup](https://firebase.google.com/docs/cloud-messaging/flutter/get-started),
[message/tap handling](https://firebase.google.com/docs/cloud-messaging/flutter/receive-messages).

## 12. Endpoint inventory for Student integration

Every path below is complete. Replace braces with the correct UUID; do not send
the braces. Staff approval actions in sections5/6 are deliberately absent here.

| Method | Actual path | Success | Response |
| --- | --- | --- | --- |
| GET | /api/v1/me | 200 | Account JSON |
| GET | /api/v1/me/attendance/events | 200 | Own recorded movement page |
| GET | /api/v1/me/student-profile | 200 | Owned Student profile, accommodation and presence |
| GET | /api/v1/me/student-profile/image | 200 | Own JPEG/PNG bytes |
| POST | /api/v1/me/setup/complete | 200 | SetupResult; empty body; section3.5 |
| GET | /api/v1/me/qr | 200 | PNG bytes |
| POST | /api/v1/going-home | 201 first /200 replay | GoingHomeDeclaration; section6.5 |
| POST | /api/v1/leaves | 201 | Leave Request |
| GET | /api/v1/leaves | 200 | Request page |
| GET | /api/v1/leaves/{leaveID} | 200 | Leave Request |
| POST | /api/v1/late-access-requests | 201 | Late-access Request |
| GET | /api/v1/late-access-requests | 200 | Request page |
| GET | /api/v1/late-access-requests/{lateAccessID} | 200 | Late-access Request |
| POST | /api/v1/complaints | 201 | Complaint |
| GET | /api/v1/complaints | 200 | Complaint page |
| GET | /api/v1/complaints/{complaintID} | 200 | Complaint |
| GET | /api/v1/complaints/{complaintID}/updates | 200 | Update page |
| POST | /api/v1/complaints/{complaintID}/notes | 200 | Update |
| PUT | /api/v1/complaints/{complaintID}/images/{imageID} | 201 or200 retry | ImageResult |
| GET | /api/v1/complaints/{complaintID}/images | 200 | items/version |
| GET | /api/v1/complaints/{complaintID}/images/{imageID} | 200 | JPEG/PNG bytes |
| DELETE | /api/v1/complaints/{complaintID}/images/{imageID} | 200 | ImageRemoval JSON |
| GET | /api/v1/announcements | 200 | Announcement page |
| GET | /api/v1/announcements/{announcementID} | 200 | Announcement |
| GET | /api/v1/notifications | 200 | Notification page |
| GET | /api/v1/notifications/unread-count | 200 | unread count |
| POST | /api/v1/notifications/{notificationID}/read | 200 | Notification |
| PUT | /api/v1/me/push-devices/{deviceID} | 200 | PushDevice metadata; requires #89 |
| GET | /api/v1/me/push-devices | 200 | items/total; requires #89 |
| DELETE | /api/v1/me/push-devices/{deviceID} | 204 | Empty; requires #89 |

All require current Firebase authentication. Student ownership/profile conditions
are described per feature. No Student RTDB grants, direct database access, scanner
secret or staff report endpoint is required for these mobile calls.

## 13. Mapping the proposed Flutter repositories

These are compatibility decisions, not permission to invent new backend routes.

| Proposed mobile method/path | Use today | Remaining work |
| --- | --- | --- |
| AuthRepository POST /auth/login | Firebase SDK sign-in then GET /api/v1/me | Email-only; Student-ID resolution and combined token/student payload absent |
| POST /auth/change-password | Firebase password change; initial setup uses hosted invitation and POST /me/setup/complete | No Go password-taking endpoint; see section3.5 |
| POST /auth/forgot-password | Firebase reset email | Do not expect Go 204 |
| POST /auth/reset-password | Firebase action-code reset | No Go reset endpoint |
| POST /auth/logout | Firebase sign-out plus push cleanup | No backend revoke-session/logout API |
| ProfileRepository GET /me with full profile | GET /api/v1/me/student-profile; /me remains account-only | Map nullable accommodation/image and UNKNOWN presence; section4.3 |
| PassRepository GET /passes | GET /api/v1/me/passes | Combined global pagination, type/source_id; see current #103 contract |
| GET /passes/{id} | Type-specific detail path | Retain type discriminator |
| GET /passes/quota | GET /api/v1/me/leave-allowance | Informational 18-day calendar-year summary; no enforcement |
| POST /passes/late-access | POST /api/v1/late-access-requests | Replace interval/extra fields with one direction/expected_at/reason and request_id |
| POST /passes/going-home | POST /api/v1/going-home | request_id only; known IN required; no optimistic OUT; section6.5 |
| POST /passes/{id}/cancel | Source-specific /leaves/{id}/cancel or /late-access-requests/{id}/cancel | Owning Student, pending only, empty body; preserve type |
| ComplaintRepository GET /complaints | GET /api/v1/complaints | Decode pagination and ownership |
| GET /complaints/{id} with progress/photos | Detail plus /updates plus /images | Three calls, not one aggregate |
| POST /complaints multipart+title+severity+5images | JSON create then raw image PUTs | Remove unsupported fields; maximum 3; handle partial uploads |
| InboxRepository GET /inbox | GET /api/v1/notifications | read_at -> isRead; paginated |
| GET /inbox/{id} | Use listed item's authorized source reference | Dedicated notification-detail GET absent |
| POST /inbox/{id}/read | POST /api/v1/notifications/{notificationID}/read | Empty body; decode returned Notification |
| POST /inbox/read-all | None | No bulk-read endpoint |

Own profile and photo retrieval are supplied by section4.3; recorded attendance
history is supplied by section4.4. Do not call staff-only Student/allocation/attendance
routes or fill real screens from permanent mocks.
A missing feature should be visibly unavailable until its contract is agreed.

## 14. Recommended client structure and operation state

This is an integration suggestion, not an instruction to rename an established
Flutter architecture. Adapt existing repositories behind their interfaces.

| Component | Responsibility |
| --- | --- |
| AuthRepository | Firebase sign-in/reset/change/sign-out and current auth state |
| AuthenticatedApiClient | Environment base URL, Bearer token, timeout, safe error decoding, one refresh/retry, account scoping |
| AccountRepository | /me, pending setup and /me/setup/complete; current account role/status |
| QrRepository | Authenticated /me/qr byte download |
| LeaveRepository | Extended leave create/list/detail and Going Home declarations |
| LateAccessRepository | Single-movement permission create/list/detail |
| ComplaintRepository | Complaint JSON, timeline/notes, image metadata/bytes/upload/removal |
| InboxRepository | Notifications, unread count, mark one read; source navigation |
| AnnouncementRepository | Global notice feed/detail |
| PushRegistrationService | Installation ID, OS permission/token refresh, account binding and logout cleanup |

### 14.1 Retry rules

Maintain logical operation states such as editing, submitting, outcome_unknown,
confirmed and conflict. Disable accidental repeated taps while a request is in
flight, but rely on backend operation identity for network retries.

| Operation | Stable retry identity | What not to change on a retry |
| --- | --- | --- |
| Going Home declaration | request_id within Student | Same request ID; no extra payload |
| Create leave | request_id within Student | Dates and reason |
| Create late access | request_id within Student | Direction, expected_at and reason |
| Create complaint | request_id within Student | Category and description |
| Add complaint note | request_id within actor/complaint | expected_version and text |
| Upload photo | image_id within complaint | File content and MIME type |
| Mark notification read | notification_id | No body; original timestamp is retained |
| Register push | account plus installation device_id | Same token/platform for an exact refresh; new token is an intentional update |
| Delete photo | complaint_id plus image_id | No body; never reuse removed image ID for a new upload |

A changed form is a NEW operation; generate a new request_id after user confirmation.
When recovering a saved draft after process death, preserve the operation UUID/body
securely if automatic retry is offered. Do not replay another account's draft.
Never label a timeout as definitely failed if the server might have committed.

### 14.2 Cache, media and state freshness

- Scope memory/disk data to the local account, clear it on account switch and honor
  no-store responses. Do not persist private QR/photo bytes in a shared global cache.
- Fetch image bytes with the current Authorization header and check status first.
  JSON error text is not a corrupted image; surface the actual failure.
- Keep loading, empty, unavailable and permission-denied states distinct. An empty
  list is not the same as a failed request or a Student who is outside.
- Student account active state and unarchived-profile eligibility are different.
  Account login can succeed while a new leave/QR/photo operation is not eligible.
- Ownership404 should not reveal another Student's record. Do not enumerate IDs
  or use admin tokens to make that404 disappear.
- Refresh private data after successful changes and when the app resumes as needed.
  A local notification/read update is not a write to attendance or room allocation.

## 15. Using the accompanying Postman collection

File: HostelHive-Mobile-API-2026-10-15.postman_collection.json.
The collection contains 30 implemented Student-facing requests, grouped by feature;
it does not add the missing proposed routes or perform Firebase password login.

1. Import the JSON collection in Postman.
2. Set collection variable api_base_url to the actual reachable API base ending
   /api/v1. The supplied hostname is deliberately a non-working placeholder.
3. Obtain a TEST Student's Firebase ID token through the configured client/helper.
   Put it in a private LOCAL firebase_id_token value; never share/export a collection
   containing a real token. Refresh when expired; Postman does not run Flutter auth.
4. Set leave_id, late_access_id, complaint_id, image_id, notification_id,
   announcement_id and push_device_id as needed. push_device_id is a retained UUID.
5. Create-body request_id placeholders must be replaced with generated UUIDs.
   Keep those values for exact retries. Example future dates may need adjustment.
6. For photo PUT choose a local JPEG/PNG as the binary body and set the matching
   media type. The collection defaults to image/jpeg; no file is bundled.
7. Inspect responses and manually transfer returned IDs/version into local variables.
   No collection script silently creates accounts, changes roles, decides requests,
   writes database records outside the request you choose, or sends emails.
8. Send one request at a time against dedicated test records. The collection includes
   mutations; do not run an uncontrolled collection runner on real Student data.

The Markdown and HTML guides are self-contained handoff copies. HTML opens in a
browser and supports printing/searching/copying code. Share the guide plus collection;
share environment URLs and test identities privately through the team's usual process.

## 16. Integration acceptance checklist

These are manual integration checks to perform, not claims they already passed.
Backend local automated evidence is described in section19.

| Area | Check and expected result |
| --- | --- |
| Environment | Known backend revision is running; HTTPS URL is reachable from the phone |
| Login | Firebase sign-in followed by /me succeeds as Student; no fake /auth/login |
| Access | Wrong role and inactive/local-unprovisioned accounts cannot enter Student workflows |
| Refresh | Expired token gets one fresh-token attempt; persistent auth failure returns to sign-in |
| Account/profile | /me remains account-only; /me/student-profile supplies owned details, null allocation/image and UNKNOWN presence |
| QR | Authenticated PNG displays; switching accounts does not show prior Student's QR |
| Leave | Create future non-overlapping request; exact retry yields same leave_id; overlap returns 409 |
| Leave decision | Staff changes status; Student sees decision_note after refresh; presence remains scan-based |
| Late access | IN at 21:00 or OUT at 05:00 permitted by default policy; normal-hours request rejected400 |
| Pass usage | Approved pass consumption metadata renders without inventing a new status |
| Ownership | Student B cannot fetch Student A's leave, late-access, complaint, photo or inbox item |
| Complaint | JSON create succeeds; unknown title/severity/multipart body is not sent |
| Timeline | Progress and photos load through separate calls; timeline pages remain version-ordered |
| Note | Current expected_version succeeds; stale version409 is handled; exact retry does not duplicate |
| Photos | JPEG/PNG within dimensions/size upload; fourth photo and new uploads after Pending are blocked |
| Partial upload | Complaint/photo1 remain visible if photo2 fails; retry does not create another complaint |
| Image privacy | No public R2 access; private byte download uses current authentication |
| Announcements | Feed/detail load; opening historical feed item does not fabricate a notification receipt |
| Inbox | read_at maps to isRead, source navigation works, unread badge updates after explicit read |
| Read retry | Repeated mark-read retains its first timestamp and changes only this recipient |
| Push availability | #89 running/migration 25/FCM enabled and mobile project configuration confirmed |
| Push receipt | Fresh leave/late-access decision reaches registered Student phone; actual receipt observed |
| Push lifecycle | Foreground/background/cold start, token refresh and logout/account switch tested |
| Push scope | Complaint/ordinary announcement remain inbox-only; staff incidents are not Student push |
| Pass workflows | Real combined history, cancellation, annual allowance and Going Home contracts used; no fabricated bulk-read success |

Use two dedicated Student accounts for ownership tests and an Admin/Warden test
account only for staff-side coordination. Keep those credentials out of source,
screenshots, screenshots of consoles, Postman exports and version control.

## 17. Troubleshooting

| Symptom | Likely cause and next check |
| --- | --- |
|404 page not found on a documented route| Wrong prefix, old server binary, different deployed branch or unavailable #89 |
|404 not_found JSON for another Student's ID| Ownership protection is working; do not bypass it |
|401 after earlier success| ID token expired/revoked/disabled; refresh once, then sign in if necessary |
|403 despite Firebase login| Local account missing/inactive, wrong role or feature eligibility |
|400 on otherwise valid JSON| Unknown fields, missing UUID/offset timezone, invalid enum, past time or unsupported query |
|400 on late access at noon| That movement does not need permission under the default policy |
|409 leave_conflict| Overlapping pending/approved leave or changed reuse of request_id |
|409 after adding a photo then a note| Photo changed complaint version; fetch latest detail before a NEW note |
|415 submitting complaint| Multipart/text sent to JSON endpoint, or wrong image media type |
|413 uploading phone photo| Input >5MiB; resize/compress before sending |
|400 image validation| Invalid bytes/MIME mismatch, dimensions/pixel count or normalized-size limit |
|503 complaint_images_unavailable| R2 disabled/misconfigured/unavailable; JSON complaint can still exist |
|Image widget decode error| Error response was decoded as image; check status and Content-Type |
|Notification list briefly missing a new event| Inbox worker delivers asynchronously; bounded refresh, not another create |
|Announcement appears in feed but no inbox item| Student joined after publication or no eligible delivery snapshot |
|Push registration200 but no phone message| Worker disabled, event predates binding, wrong project/token, OS settings or provider failure |
|FCM job accepted but nothing displayed| Provider acceptance is not receipt; check mobile permission/foreground handling and actual device setup |
|409 push_device_conflict after account switch| Old token still bound, duplicate installation entry or cap; complete cleanup and rotate token |
|Phone cannot reach127.0.0.1| That address points to the phone; obtain reachable backend URL |
|Calling /me does not return room/guardians| /me is account-only; call /me/student-profile (section4.3) |

Do not fix endpoint mismatches by weakening authentication, making storage public,
hardcoding Admin credentials or storing passwords in PostgreSQL/the mobile app.

## 18. Explicit pending decisions and features

| Item | Current status | Required next action |
| --- | --- | --- |
| Full Student profile/home aggregate | Implemented in #91; deployment must be confirmed | Use /me/student-profile and authenticated /image; section4.3 |
| Student invitation and initial password setup | Backend implemented #95; live email/Flutter pending | Admin-approved university email, hosted Firebase setup, fresh password sign-in and /me/setup/complete; section3.5. No SSO. |
| Student-ID login | Not implemented | Selected onboarding uses university email; an alternative Student-ID login requires a separate scope decision |
| 18-day yearly quota| Informational summary implemented #102 | Asia/Colombo calendar year, approved local dates, no blocking enforcement |
| Going Home declaration | Backend implemented #100; deployment unverified | Use /going-home; no approval/return date, actual OUT stays scan-based; section6.5 |
| Cancel pending pass | Implemented #101 | Source-specific cancellation with ownership, pending-state and audit rules |
| Combined pass feed | Implemented #103 | GET /me/passes with deterministic ordering, combined pagination and consistent totals |
| Notification detail GET | No dedicated route | Use source detail now, or agree aggregate behavior |
| Mark all read | Missing | Add explicit recipient-scoped bulk contract if required |
| Student-owned attendance history | Implemented in #93; confirm deployment | Use /me/attendance/events, section4.4 |
| Complaint title/severity/five photos | Not supported | Align Flutter to approved category/description/three-photo model, or approve changed contract |
| Shared staging URL | Not verified in this handoff | Backend/infra owner provides running revision and reachable environment |
| Live mobile FCM receipt | Pending | Complete Flutter/device setup and observe actual delivery |

This list distinguishes requests from existing implementation. It does not authorize
or schedule new features, define an unapproved quota or claim they are all required
for the same release. Resolve them together before the mobile developer depends on
a mocked endpoint. Do not label a future API as available in a production repository.

## 19. Evidence, source files and maintenance

Source of truth for this snapshot: backend route files, DTOs, domain models,
handlers, services and repositories reviewed through 19 October 2026. No backend code or
cloud settings were changed to write this guide. Source paths below are relative
to the backend repository, not links to an already deployed API.

| Area | Principal implementation sources |
| --- | --- |
| Auth/account | internal/shared/middleware/authentication.go; internal/modules/identity/identity_routes.go; handler/current_account_handler.go; domain/user_account.go |
| Own profile | student/domain/student_own_profile.go; handler/student_own_profile_handler.go; repository/student_own_profile_repository.go |
| Own attendance history | attendance/domain/student_attendance_history.go; handler/student_attendance_history_handler.go; repository/student_attendance_history_repository.go |
| QR | internal/modules/student/student_routes.go; handler/student_qr_handler.go; service/student_qr_service.go |
| Leave | internal/modules/leave/leave_routes.go; dto/leave_requests.go; domain/leave_request.go; handler/leave_handler.go |
| Late access | internal/modules/lateaccess/late_access_routes.go; dto/late_access_requests.go; domain/late_access_request.go; handler/late_access_handler.go |
| Complaints | internal/modules/complaint/complaint_routes.go; dto/complaint_requests.go; domain/complaint.go; handler/complaint_handler.go |
| Images | internal/modules/complaint/domain/complaint_image.go; handler/complaint_image_handler.go; internal/shared/imagevalidation/image_normalization.go |
| Announcements | internal/modules/announcement/announcement_routes.go; domain/announcement.go; handler/announcement_handler.go |
| Inbox | internal/modules/notification/notification_routes.go; domain/notification.go; handler/notification_inbox_handler.go |
| Push | notification domain/notification_push.go; handler/notification_push_devices_handler.go; internal/platform/firebase/fcm_push_sender.go; migration000025 |

For paths starting with handler/domain/service/dto in a row, resolve them under
that row's module directory. Read the backend docs/*.md feature guide for staff-side
manual tests and deployment prerequisites. Earlier ticket docs may say a later
feature was future work; this guide incorporates subsequent implementation through #93.

Historical issue95 verification on16 October passed80 package results, all15 required
database packages without test skips, and reported84.8% statement coverage. The
populated invitation upgrade/rollback and all26 migration rollback/reapply checks
also passed, including the new helper under Windows PowerShell5.1. These numbers
are not feature completeness, live Firebase/R2/FCM behavior, remote CI or deployed
acceptance. Firebase adapter tests use fakes; real setup email and Flutter remain
pending. No live invitation accounts or emails were created by those checks.

Local issue100 verification on19 October passed80 package results and all15
required PostgreSQL packages without test skips. Overall statement coverage84.9%
and Leave87.1%; all27 migrations passed rollback/reapply and the populated Going
Home preservation check passed. This is automated backend evidence, not live
Flutter/device or remote CI acceptance.

Before changing a route/payload/status/limit, update this guide, the collection and
both repositories' mapping together. Confirm the running server revision, migrations
and environment with the backend owner. Never infer availability from a screenshot,
mock repository or an issue marked complete alone.

### External SDK references

- Firebase Flutter sign-in: https://firebase.google.com/docs/auth/flutter/password-auth
- Firebase Flutter user management: https://firebase.google.com/docs/auth/flutter/manage-users
- Firebase ID token API: https://pub.dev/documentation/firebase_auth/latest/firebase_auth/User/getIdToken.html
- Flutter FCM configuration: https://firebase.google.com/docs/cloud-messaging/flutter/get-started
- Flutter FCM receipt/taps: https://firebase.google.com/docs/cloud-messaging/flutter/receive-messages

The SDK references explain Firebase behavior; the backend code defines HostelHive's
paths, business rules and permission checks. Use compatible SDK versions in the
mobile lockfile and verify compilation within the actual Flutter project.
