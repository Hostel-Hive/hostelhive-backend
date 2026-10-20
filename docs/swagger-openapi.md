# Complete backend OpenAPI and Swagger UI

The implemented API is available as **OpenAPI3.0.3**, covering107 method/path
operations on87 paths. Swagger UI5.34.0 is self-hosted and embedded in the Go
binary. It needs no CDN, external validator, production Node process or database
migration. Business handlers, role rules and data models remain unchanged.

## Open and test

Start the backend with its usual environment variables and services configured:

```powershell
cd "C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend"
go run ./cmd/server
```

For the default `HTTP_ADDR=127.0.0.1:8080`, open:

- Swagger UI: http://127.0.0.1:8080/docs/
- Downloadable specification: http://127.0.0.1:8080/openapi.json

Use the actual HTTP_ADDR/host/port if changed, e.g.18080. The UI and spec are
public documentation; business requests retain existing authentication. The server
URL `/` uses the same backend origin. Paths already include `/api/v1`; do not
append that prefix to the origin. A normal database/Firebase configuration is
still required to start the application; Swagger does not bypass configuration.

1. Sign in through Firebase using an approved test account. Obtain its current
   **ID token**, for example with the existing `scripts/get-firebase-token.ps1`.
2. Click **Authorize**, choose **FirebaseBearer**, paste the token only (without
   `Bearer `), then authorize. Do not paste the Firebase web API key, password,
   refresh token, service-account JSON or university credentials.
3. Expand a module/operation. Read its **Access** policy, body rules, schema and
   error responses. Click **Try it out**, replace fictional UUIDs/times with valid
   test values, and **Execute**. These are real API calls; writes can change data
   or queue provider work. Use dedicated test records.
4. For scanner `/device/check` and `/device/scans`, use **ScannerBearer** instead:
   the credential issued by Admin scanner registration/rotation. A Firebase token
   does not work there. Scanner credentials never grant staff/Student APIs.
5. Refresh authentication when401 appears; an inactive/wrong-role/setup-pending
   account returns403. Authorizing Swagger never changes your server-side role.

Pending invited Students can call only `/me` and `/me/setup/complete` until verified
initial setup succeeds. Student business APIs derive ownership server-side.
Tokens remain in page memory and are not persisted across reloads. External
validation and query-controlled UI configuration are disabled; the browser CSP
restricts runtime requests to the same origin. Documentation is not a login form.

## Module tags

Use **Show APIs available to** above Swagger to select Admin, Warden, Sub-Warden,
Security, Student or Scanner. **All roles** restores the complete list; **Public**
shows the unauthenticated health/readiness probes. A selected role includes shared
endpoints (for example, Admin sees Admin/Warden report APIs). Module groups remain
intact, and the existing module/path search can narrow the results further.

Every collapsed or expanded operation displays its allowed-role badges, derived
from the same `x-roles` specification metadata as the filter. These indicate route
access; ownership, setup, active-account and resource restrictions still apply.
Filtering does not sign in, switch accounts or grant permissions. Use `/api/v1/me`
to confirm the role of the current Firebase token, and replace the token in
Authorize when switching test accounts. The padlock indicates authentication,
not Admin-only access.

| Tag | Backend module |
|---|---|
| System | app: /health and /ready |
| Identity | identity: accounts, roles, invitations, setup |
| Students | student: profiles, guardians, CSV, images, QR, own profile |
| Staff | staff: managed profiles and own name |
| Inventory | inventory: blocks, rooms, beds |
| Allocation | allocation: assign, transfer, revoke and history |
| Attendance | attendance: scanners, scans, presence, incidents, RTDB, own history |
| Leave | leave: leave workflow, Going Home, annual allowance, combined passes |
| Late Access | lateaccess: one-movement permission requests |
| Complaints | complaint: requests, updates, notes and private images |
| Announcements | announcement: publication, feed and delivery summary |
| Notifications | notification: owned inbox, read/unread and FCM devices |
| Reports | report: attendance/daily/leave/complaint JSON, Excel and PDF |

Each operation belongs to exactly one owning module. `/me/*` is grouped by the
module providing the feature, not collected into an ambiguous miscellaneous tag.
The operation also includes machine-readable `x-roles`; tag metadata has `x-module`.

## Testing details

- JSON writes use the shown DTO. Unknown fields are rejected on strict endpoints.
  Do not send unimplemented department, complaint title/severity or late-access
  round-trip fields. Enums are case sensitive.
- Empty-body operations have **no requestBody** in the specification. Send no `{}`,
  `null` or whitespace on cancellation, read-state, setup completion and other
  explicitly empty actions. GET requests do not advertise a body.
- Generate a fresh request_id per intended request/action. Preserve the original
  UUID and exact normalized payload on uncertain retry. `/users` uses a required
  Idempotency-Key header instead. Source/detail IDs are separate from request IDs.
- Image uploads are raw JPEG/PNG bytes, maximum5MiB; not multipart or base64.
  The server also validates decoded dimensions/content. QR/private photos return
  bytes. Select the actual media type when uploading.
- CSV uploads use raw UTF-8 `text/csv`; download the correct template. Profile-only
  CSV accepts up to500 records; invitation CSV accepts100, both maximum2MiB.
  HTTP200 import reports may contain rejected/not-attempted records: inspect them.
- Report JSON requires paired local dates, with optional documented filters.
  Exports reject pagination and use raw XLSX/PDF responses. Excel limit5000 rows;
  PDF limit1000 rows/200 pages/16MiB. Native Sinhala/Tamil PDF shaping limitations
  remain documented in `pdf-report-exports.md`.
- Schema properties preserve nullable versus omitted Go JSON fields. `total`,
  `items`, `students`, `users` and `staff` vary by the real endpoint; do not assume
  every list has the same wrapper. Each response shows its own schema.
- The hosted page tests the current running backend. A stale binary can expose
  an older contract even when Git contains newer files. Rebuild/restart after updates.

The existing frontend/mobile guides describe workflows and integration decisions;
this specification is the executable HTTP reference. Firebase SDK login/password
reset and external RTDB subscriptions are explained but are not invented Go routes.
Newly requested mobile fields must be implemented before adding them to the spec.

## Portable file and maintenance

`docs/openapi.json` can be imported into Postman or other OpenAPI-compatible tools.
It is exported from `internal/apidocs`, whose schemas use actual public Go DTOs
and domain response types. This avoids duplicating JSON field names by hand.
Operation descriptions/constraints remain explicitly reviewed against handlers
and services; reflection alone cannot infer business policy.

Exporting/checking does not connect to PostgreSQL/Firebase or start workers:

```powershell
go run ./cmd/openapi -output docs/openapi.json
go run ./cmd/openapi -check docs/openapi.json
go test ./internal/apidocs
```

To validate against the OpenAPI standard, use Node>=22.19 (CI uses24):

```powershell
cd tools/openapi
npm ci --ignore-scripts --no-audit --no-fund
npm run validate
```

For a new/changed endpoint, update the matching `internal/apidocs/operations_*.go`
entry, response/request model registry and curated constraints, then re-export.
Route tests scan real module registration files and fail for missing/extra
operations, incorrect owning tags, unresolved schema references or stale snapshots.
The existing Go CI includes those tests and a snapshot check; a dedicated job
validates OpenAPI with the pinned Swagger Parser. UI tests verify serving, method
boundaries, security headers and no external validator/token persistence.

Swagger UI's pinned upstream license, NOTICE, source and SHA-256 hashes are kept
under `internal/apidocs/assets/`. Updating vendored assets requires deliberate
version/license/hash review. No production Go dependency was added for Swagger.
