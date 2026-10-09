# HostelHive Backend

Go backend for HostelHive. Maintainer: Tasheen. Repository: https://github.com/Hostel-Hive/hostelhive-backend.

This foundation implements environment configuration validation, a bounded HTTP server lifecycle, structured startup/error logging and graceful shutdown. Operational endpoints provide health and PostgreSQL readiness checks. GET /api/v1/me verifies Firebase authentication and returns the current local account.

## Prerequisites

- Go 1.27 or later: https://go.dev/dl/
- Git

PostgreSQL is now required for readiness. The backend uses pgxpool, with dependencies pinned in go.mod/go.sum. Firebase Authentication now requires a Firebase project ID and Application Default Credentials (see below).

## Structure

```text
cmd/server/                   Startup, signals and process exit
internal/app/                 Dependency wiring, operational router and shutdown
internal/config/              Environment parsing and validation
internal/platform/postgres/   PostgreSQL connection pool
internal/platform/firebase/   Authentication, provisioning and revocation adapters
internal/shared/              Middleware, responses and common validation
internal/modules/identity/    Account domain, DTOs, handlers, services and repositories
internal/modules/student/     Profile domain, DTOs, handlers, services and repositories
internal/workers/             Durable user-revocation retry loop
tests/integration/            Disposable PostgreSQL integration suites
tests/migration/              SQL constraint verification
tests/architecture/           Package dependency boundary checks
docs/architecture.md          Layer responsibilities and extension rules
migrations/                   Existing versioned SQL migration pairs
scripts/                      Migration setup and isolated verification
.env.example                  Supported settings (no secrets)
go.mod                        Go module
```

The modular layout follows the HostelHive folder-structure reference. Issue #26 reorganizes existing code; endpoint paths, JSON responses, environment settings and migration versions remain unchanged. Migration commands still use the pinned CLI through `scripts/migrate.ps1`.

## Start locally

Start the local database as described below, apply all pending migrations, then configure DATABASE_URL and Firebase credentials in the backend terminal. From the repository directory:

```powershell
go run ./cmd/server
```

Defaults bind to `127.0.0.1:8080`. The startup log confirms the address. In another terminal:

```powershell
curl.exe -i http://127.0.0.1:8080/health
curl.exe -i http://127.0.0.1:8080/ready
```

Both endpoints return **200 OK** with Content-Type application/json. /health returns {"status":"ok"} and /ready returns {"status":"ready"}. Readiness pings PostgreSQL: failures or timeouts return HTTP 503 with {"status":"not_ready"}. It checks connectivity, not schema migrations or Firebase. /health remains 200 even during a database outage. Unknown routes, including /, return **404 Not Found**. Press **Ctrl+C** in the server terminal to drain active requests and stop. Linux/container deployments also support SIGTERM.

Build a standalone executable:

```powershell
go build -o ./bin/hostelhive-server.exe ./cmd/server
./bin/hostelhive-server.exe
```

On Linux/macOS use `go build -o ./bin/hostelhive-server ./cmd/server` and run `./bin/hostelhive-server`.


## Local PostgreSQL setup

Start Docker Desktop. The official PostgreSQL image example uses disposable local
development credentials. Use a different password for shared or deployed databases.

PowerShell:

    $env:POSTGRES_PASSWORD = 'local-dev-only'
    docker run --name hostelhive-postgres -d -p 127.0.0.1:15433:5432 -e POSTGRES_PASSWORD -e POSTGRES_USER=hostelhive -e POSTGRES_DB=hostelhive postgres:16-alpine
    docker exec hostelhive-postgres pg_isready -U hostelhive -d hostelhive

Wait for accepting connections. If the container already exists, use
docker start hostelhive-postgres instead. Credentials must match those used
when its database was first initialized. This creates no application tables.

In the backend terminal, set:

    $env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'
    $env:HTTP_ADDR = '127.0.0.1:18080'
    go run ./cmd/server

The executable uses the same environment variables. .env files are not loaded
automatically. URL-encode special characters in database passwords. sslmode=disable
is for this local loopback example; configure TLS for deployed connections as required.

The pool connects lazily, so the HTTP service starts even if PostgreSQL is offline.
Readiness reports 503 until the database becomes reachable. Each check uses
DATABASE_CHECK_TIMEOUT (default 2s), including connection establishment.
Keep this timeout shorter than the HTTP write timeout and deployment probe timeout.
Database URL parsing errors are redacted. Shutdown drains HTTP requests before
closing the connection pool.

## PostgreSQL verification

Unit tests cover readiness success, outage/recovery, bounded deadlines, request
cancellation, health independence and safe configuration errors.

To run the optional real database integration test:

    $env:TEST_DATABASE_URL = $env:DATABASE_URL
    go test -count=1 -timeout=30s -v ./tests/integration/postgres
    Remove-Item Env:TEST_DATABASE_URL

This verifies PostgreSQL Ping and pool closure; it is skipped without TEST_DATABASE_URL.

With the backend running, verify an outage:

    docker stop hostelhive-postgres
    curl.exe -i http://127.0.0.1:18080/health
    curl.exe -i http://127.0.0.1:18080/ready
    docker start hostelhive-postgres

Health stays 200 and readiness returns 503. After the database accepts connections
again, /ready returns 200. Business table migrations and deployment Compose belong in
separate tickets.

## Database migrations

Migrations use golang-migrate v4.20.1, pinned in .migrate-version.
The CLI is installed separately into ignored bin/; the HTTP server does not
apply migrations automatically.

Official tool documentation:
https://github.com/golang-migrate/migrate/tree/v4.20.1/cmd/migrate

From the backend repository in PowerShell, install the pinned CLI:

    .\scripts\install-migrate.ps1

Go must be on PATH. This installs only the PostgreSQL-enabled migration tool.
Afterwards, set DATABASE_URL in the same terminal using your local credentials.
The local example uses port 15433 because 15432 was already occupied.

    $env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'

Apply all pending migrations, then show the database migration version:

    .\scripts\migrate.ps1 up
    .\scripts\migrate.ps1 version

After the baseline alone, version is 1. With student profiles, the current version is 5. Repeating up reports no change.
The wrapper pins version tracking to the tool-managed public.schema_migrations table, independently of the effective PostgreSQL search path.
Before any migration is applied, version reports no migration.

The baseline pair is:

- migrations/000001_baseline.up.sql: creates the empty hostelhive schema.
- migrations/000001_baseline.down.sql: removes that schema using RESTRICT.

User accounts are added by migration 000002; remaining business tables follow in later tickets. Use explicit schema-qualified names
such as hostelhive.table_name in those migrations and database queries.
The baseline does not change search_path settings. PostgreSQL may resolve the new schema via its default user-based search path; use explicit schema-qualified names.
It intentionally fails if an unmanaged hostelhive schema already exists.

Rollback one migration on a disposable development database:

    .\scripts\migrate.ps1 down

This wrapper rolls back exactly one version. The baseline rollback refuses to
drop a non-empty schema. Roll back later migrations first; do not add CASCADE.
Review down SQL before using it on a database containing data. A failed migration
can leave the version dirty: inspect and repair the failed SQL/database before
changing migration metadata. Do not blindly force a version.

Verify the migration cycle in an isolated database:

    .\scripts\test-migrations.ps1

Docker Desktop must be running. The script creates a uniquely named temporary
PostgreSQL 16 container, checks user constraints, version-1 upgrade and migration rollback/reapply,
and removes its container afterward. It never uses your existing DATABASE_URL
as the test target and restores that environment variable when finished.

Add future migrations as consecutive six-digit version pairs:

    migrations/000006_description.up.sql
    migrations/000006_description.down.sql

Commit both files together. Do not edit an already applied migration; add a new
version. Keep real credentials and generated migration executables out of Git.

## User accounts (issue #13)

Migration 000002 creates hostelhive.users, implementing the SDS USER entity
with the Firebase identity amendment in ADR-002.

| Column | Behavior |
|---|---|
| user_id | UUID primary key, generated by PostgreSQL |
| firebase_uid | Required, unique, case-sensitive Firebase UID; maximum 128 characters; blank values rejected |
| email | Required, maximum 254 characters; basic shape/no-whitespace checks; unique ignoring case |
| role | Required: admin, warden, sub_warden, security_staff or student |
| is_active | Required boolean, defaults to false |
| created_at | Timestamp with time zone, defaults to the current transaction time |
| updated_at | Timestamp with time zone, automatically refreshed by a trigger on updates |

Email and role remain on the common account table, as in the SDS. Student and staff
profiles reference user_id through migrations 5 and 8. Role strings above map to the
SDS human actors; scanner credentials are separate from human user accounts.
The SQL email check is basic integrity protection; full validation belongs in the
provisioning service/Firebase workflow.

Firebase handles user credentials. This table contains no password hashes or local
refresh/reset tokens. The protected API verifies a Firebase ID token,
then retrieves the matching firebase_uid and checks the current role and is_active.
An account remains inactive until the provisioning/activation workflow enables it.
Neither applying this migration nor changing is_active creates or changes a Firebase
identity; cross-system activation and revocation are separate implementation tasks.
Firebase SDK/service-account credentials are not needed to apply these SQL files.

Apply the migration to your local database with the existing commands:

    .\scripts\migrate.ps1 up
    .\scripts\migrate.ps1 version

Expected current version: 5. In pgAdmin, refresh:

    hostelhive database -> Schemas -> hostelhive -> Tables -> users

Migration rollback drops the users table and its timestamp trigger function.
It deletes local account records and refuses to cascade into dependent objects.
Use the disposable verification script to test rollback rather than an application
database containing accounts:

    .\scripts\test-migrations.ps1

Tests in tests/migration/user_accounts.sql verify all five roles, inactive defaults,
generated UUIDs/timestamps, UID boundaries/case sensitivity, duplicate identities,
duplicate emails ignoring case, missing required values, malformed email,
invalid role inserts/updates, activation/deactivation and automatic update timestamps.
They verify that credential/token columns are absent. Fixtures are rolled back.
The PowerShell script verifies fresh apply, upgrade from version 1, unchanged repeated
apply, account rollback preserving the baseline, complete rollback, and reapply.
It runs the constraint tests again after reapplying migration 2.

Design reference:
https://github.com/Hostel-Hive/Design-and-Development-Project/blob/main/documents/adr/ADR-002-firebase-authentication.md

## Configuration

The application reads **process environment variables**, not `.env` files automatically. `.env.example` is documentation for manual configuration or future Docker Compose integration. Never commit real credentials.

| Variable | Default | Valid values |
|---|---|---|
| DATABASE_URL | Required; no default | PostgreSQL URL with host, user and database |
| FIREBASE_PROJECT_ID | Required | Firebase project ID, not its display name |
| AUTH_TIMEOUT | 5s | Positive Go duration for verification and account lookup |
| PROVISIONING_TIMEOUT | 8s | Positive Go duration for provisioning after authentication |
| ACCOUNT_MANAGEMENT_TIMEOUT | 8s | Positive Go duration for account management and each revocation retry |
| STUDENT_PROFILE_TIMEOUT | 8s | Positive Go duration for student-profile operations after authentication |
| GOOGLE_APPLICATION_CREDENTIALS | ADC | Optional path to an external service-account JSON file; otherwise use platform ADC |
| DATABASE_CHECK_TIMEOUT | 2s | Positive Go duration; connection and readiness timeout |
| APP_ENV | development | development, test, production |
| HTTP_ADDR | 127.0.0.1:8080 | IP/localhost/empty host plus numeric port 1-65535; IPv6 uses brackets |
| HTTP_READ_HEADER_TIMEOUT | 5s | Positive Go duration |
| HTTP_READ_TIMEOUT | 15s | Positive Go duration |
| HTTP_WRITE_TIMEOUT | 15s | Positive Go duration |
| HTTP_IDLE_TIMEOUT | 60s | Positive Go duration |
| HTTP_SHUTDOWN_TIMEOUT | 10s | Positive Go duration |

Unset optional variables use defaults. DATABASE_URL and FIREBASE_PROJECT_ID are required. Explicit empty/invalid variables cause an error identifying the setting and a nonzero exit before the server listens.

PowerShell override example:

```powershell
$env:HTTP_ADDR = '127.0.0.1:18080'
$env:HTTP_SHUTDOWN_TIMEOUT = '5s'
go run ./cmd/server
# Remove overrides after stopping:
Remove-Item Env:HTTP_ADDR, Env:HTTP_SHUTDOWN_TIMEOUT
```

For containers, set `HTTP_ADDR=0.0.0.0:8080` and route through the private deployment network/Nginx. This service does not terminate production TLS itself. If the shutdown deadline expires, remaining connections are closed and the process exits with an error.

## Verification

```powershell
gofmt -l ./cmd ./internal ./tests
go vet ./...
go test -count=1 -timeout=30s -cover ./...
go build ./...
```

`gofmt -l` must print no files. Configuration tests cover defaults, environment overrides, missing/empty or malformed supplied values and timeout validation. HTTP tests verify endpoint status codes, JSON bodies, content types, route boundaries, bind failures, draining active requests and forced closure after the grace period. These tests require local loopback socket access. A teammate must independently follow this README to satisfy the ticket's fresh-setup acceptance criterion.

## Architecture and next tickets

The selected baseline uses Firebase Authentication (ADR-002), containerized PostgreSQL (ADR-003), backend-published Firebase RTDB projections (ADR-004) and Nginx HTTPS routing (ADR-005). This scaffold does not issue local user JWTs or store user passwords. ADRs live in the continued documents repository: https://github.com/Hostel-Hive/Design-and-Development-Project.

Identity management and student profiles are implemented modules. Attendance, allocation, leave, complaint, notice, report and scanner features will extend this layout in their own tickets. Add adapters and modules when their functionality is implemented. See [architecture.md](docs/architecture.md) for dependency rules.

When using the port override above, check http://127.0.0.1:18080/health and http://127.0.0.1:18080/ready.

## Firebase authentication (issue #15)

Protected requests follow ADR-002: the Firebase Admin SDK verifies the ID token
with VerifyIDTokenAndCheckRevoked, then PostgreSQL supplies the current role and
active status on every request. Client role/custom claims do not override local
account state. No passwords, local JWT issuer or refresh/reset tokens are stored.
The Firebase Auth emulator is rejected because it accepts unsigned tokens.

Use your team's Firebase project and enable the chosen sign-in provider. For
local development, obtain a service-account credential with permission to read
Firebase Authentication users, so revocation and disabled-account checks work.
Keep the JSON file outside this repository and never send it to a teammate via
an issue, PR or chat. On deployed Google infrastructure, use workload credentials
instead of a downloaded key when available.

Official documentation:
- https://firebase.google.com/docs/admin/setup
- https://firebase.google.com/docs/auth/admin/verify-id-tokens
- https://firebase.google.com/docs/auth/admin/manage-sessions

Set these in the same PowerShell terminal as the server (replace placeholders):

```powershell
$env:FIREBASE_PROJECT_ID = 'YOUR_FIREBASE_PROJECT_ID'
$env:GOOGLE_APPLICATION_CREDENTIALS = 'C:/private/hostelhive-service-account.json'
$env:AUTH_TIMEOUT = '5s'
$env:HTTP_ADDR = '127.0.0.1:18080'
# Set DATABASE_URL as shown in the PostgreSQL section and apply migrations first.
go run ./cmd/server
```

Startup fails with a generic, credential-safe error if Firebase initialization
fails. Startup does not verify that credentials have sufficient remote IAM
permissions; requests fail closed if verification cannot complete. Outbound
access to Google token certificates, OAuth and Firebase Auth APIs is required.
AUTH_TIMEOUT bounds both verification and the local account lookup; choose it
below HTTP_WRITE_TIMEOUT. /health and /ready do not call Firebase.

### Test the protected endpoint

Without a token:

```powershell
curl.exe -i http://127.0.0.1:18080/api/v1/me
```

Expected: HTTP 401 and {"error":"unauthorized"}. In the web/mobile Firebase
client, sign in and obtain an **ID token** (not a custom token). Keep it out of
logs and GitHub. In a second terminal, use the token held in a local variable:

```powershell
# Set $firebaseIDToken locally to the client-issued ID token.
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/me' -Headers @{ Authorization = "Bearer $firebaseIDToken" }
```

The matching row in hostelhive.users must already exist and be active. A Firebase
identity alone receives HTTP 403; authentication does not provision or activate accounts automatically. Administrators
provision new accounts with POST /api/v1/users (see issue #20 below). For live
verification, use a dedicated test identity and its approved local account.

Successful JSON contains user_id, firebase_uid, email, role and is_active. Error
responses contain no token, SDK message, database detail or credential paths:

| Condition | HTTP response |
| --- | --- |
| Missing/malformed bearer header, invalid/expired/revoked token, disabled Firebase user, verifier failure or timeout | 401 unauthorized |
| No matching local account, inactive account or unsupported local role | 403 forbidden |
| Local account lookup failure or timeout | 503 service_unavailable |
| Verified identity and active local account | 200 with current local account |

Responses use Cache-Control: no-store. After changing the local role or active
status, the next protected request sees the change. Authentication middleware
provides AccountFromContext to future protected handlers; each business endpoint
must still enforce its role and resource-ownership permissions.

### Authentication verification

```powershell
go test -count=1 -timeout=30s ./internal/shared/middleware ./internal/platform/firebase ./internal/modules/identity/... ./internal/app ./internal/config
go vet ./...
go test -count=1 -timeout=30s ./...
go build -o ./bin/hostelhive-server.exe ./cmd/server
```

Deterministic tests use injected verifier/account dependencies, with no real
credentials. They cover denied tokens, mandatory revocation-check adapter usage,
UID-based account lookup, SQL parameter binding, fresh roles/deactivation, safe
errors, duplicate headers, deadlines/cancellation and public probe boundaries.
An optional real PostgreSQL lookup test uses TEST_DATABASE_URL against a database
migrated through the current version; its fixture is rolled back automatically. Live Firebase
valid/expired/revoked/disabled token checks require the project's credentials and
test identities and are not claimed by the offline tests.

## Role-based authorization (issue #18)

RequireRoles restricts a handler to explicitly listed local PostgreSQL roles.
Wrap authentication around authorization so that the verified, active local
account is loaded before its role is checked:

```go
// Illustrative registration for a future business endpoint; not a live route.
mux.Handle("GET /api/v1/warden-example", authenticate(
    authentication.RequireRoles(
        authentication.RoleWarden,
        authentication.RoleSubWarden,
    )(wardenHandler),
))
```

Here authenticate is the middleware constructed with authentication.Middleware,
and wardenHandler is that endpoint's http.Handler. Authentication runs first,
then RequireRoles, then the business handler if the role is allowed.

Supported constants map to migration 2: RoleAdmin (admin), RoleWarden (warden),
RoleSubWarden (sub_warden), RoleSecurityStaff (security_staff), RoleStudent (student).
An admin is permitted only when RoleAdmin is explicitly listed. Roles are exact
and case-sensitive; there is no implicit role hierarchy. Empty policies or any
unsupported configured role deny access to all authenticated callers.

| Condition | Result |
| --- | --- |
| No authenticated account in the request context | 401 unauthorized |
| Current local role is not in the endpoint's allowed roles | 403 forbidden |
| Inactive account or invalid role/policy | 403 forbidden |
| Active account with an explicitly allowed role | Business handler runs |

Denials use JSON with Cache-Control: no-store. Client query/body/header roles
and Firebase custom claims do not override the verified local account. Every
protected request rechecks PostgreSQL through authentication, so role changes
and deactivation take effect on the next request without requiring a new token.
Resource ownership (for example, a student accessing only their own records)
remains a separate check inside each business feature.

This ticket adds reusable authorization middleware and its tests. It does not
invent business routes or permission assignments. GET /api/v1/me remains
available to every authenticated active account; /health and /ready remain public.
Business tickets must declare their role policies when registering endpoints.

Run the role authorization matrix and authentication integration tests:

```powershell
go test -count=1 -timeout=30s -v ./internal/shared/middleware
go vet ./...
go test -count=1 -timeout=30s ./...
go build -o ./bin/hostelhive-server.exe ./cmd/server
```

The deterministic tests exercise all five roles, policies with multiple roles,
missing/failed authentication, denied requests never reaching business handlers,
unsafe or empty policies, caller mutation of a policy, client role spoofing and
fresh local role/active-state changes. No Firebase credentials are needed for
these tests. No new environment variables or database migrations are required.

## Administrator user provisioning (issue #20)

POST /api/v1/users requires a verified, active local admin account. Other roles
receive 403 and missing/invalid authentication receives 401. Apply migration 3
before using this route. Firebase service credentials need Authentication user
read/create/update permissions. No credentials are needed for unit tests.

### Request and responses

Provide Content-Type: application/json, Authorization: Bearer <ID token> and
Idempotency-Key: <request identifier>. Keys must contain 8-128 ASCII letters,
digits, hyphens or underscores; a UUID generated once per new account is suitable.
The JSON body is limited to 16 KiB and rejects unknown fields/trailing JSON:

```json
{
  "email": "new.student@example.com",
  "role": "student",
  "password": "REPLACE_WITH_A_PRIVATE_INITIAL_PASSWORD"
}
```

Emails are normalized to lowercase and roles must be admin, warden, sub_warden,
security_staff or student. Initial passwords must contain 12-128 Unicode
characters. The password is sent only to Firebase, not persisted in PostgreSQL
or returned in responses. Coordinate confidential initial-password delivery
with the recipient. This ticket does not send email or implement password-reset
flows. Use HTTPS when calling deployed endpoints; the loopback example is local.

| Result | HTTP response |
| --- | --- |
| Newly completed provisioning | 201, account plus replayed: false |
| Completed request replay | 200, current account plus replayed: true |
| Invalid body, role, email, password or request key | 400 invalid_input |
| Changed key intent/creator, existing email or unrelated identity | 409 conflict |
| Another worker holds the reservation | 503 request_in_progress, Retry-After: 1 |
| Database/Firebase failure or timeout | 503 provisioning_unavailable |

Error responses never expose submitted passwords, tokens, raw Firebase errors
or database details. Successful responses contain user_id, firebase_uid, email,
role and is_active under account; they do not return Firebase user-record objects.

### Retry and partial-failure behavior

1. Persist a reservation in hostelhive.user_provisioning before contacting Firebase.
   It records the request key, verified administrator UID, normalized email,
   requested role and a random, stable Firebase UID. No credential fields exist.
2. Lock the reservation and insert an inactive local account inside a transaction.
3. Create the reserved Firebase identity disabled, or resume that same UID after
   an earlier partial attempt. An existing identity with the email but a different
   UID is rejected; it is never adopted or assigned a new role.
4. Enable the reserved identity and verify Firebase's acknowledgement.
5. Atomically commit local activation and completion metadata.

On failure the transaction rolls back, leaving the durable reservation for retry.
An enabled Firebase identity without an active local account still cannot access
protected application endpoints. If a commit acknowledgement is lost, retrying
resolves the persisted completion state. A completed replay never changes the
password, current role or active state; it cannot reactivate a deactivated user.

Retry transient failures with the SAME key and original request from the SAME
administrator. Email/role changes or another administrator using that key return
409. Passwords are not fingerprinted or saved: once the reserved Firebase identity
exists, retries ignore the submitted password instead of resetting it. Keep the
original password for sign-in. Use a new key for a genuinely different account.
Pending reservations remain until recovered; persistent conflicts require trusted
operator investigation, not automatic deletion of unrelated Firebase identities.
Completed retry metadata should be retained while its replay guarantees are needed.

PROVISIONING_TIMEOUT defaults to 8s. Authentication has its own deadline before
this workflow; configure HTTP/proxy/client deadlines above their combined budget.
The row lock is held across the bounded workflow to prevent concurrent mutation.
There is no background recovery worker in this ticket: the caller resumes pending
work by repeating the request. Future account deletion must handle the reservation's
RESTRICT foreign key explicitly. Migration 3 rollback drops retry metadata only;
it does not delete local/Firebase accounts and loses pending/replay information.

### Initialize the first administrator

There is no public bootstrap endpoint. A trusted project owner with database
administrative access must initialize the first local admin once. First verify
in Firebase Authentication > Users that the intended UID/email belongs to the
owner and the identity is enabled. The account previously used for authentication
testing can be used if it is the intended project administrator.

Run this SQL in pgAdmin's Query Tool on the intended database, replacing both
placeholders with the verified Firebase UID and email. It inserts a new local
account or promotes that exact existing UID/email only while no active admin exists:

```sql
BEGIN;
LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE;
INSERT INTO hostelhive.users (firebase_uid, email, role, is_active)
SELECT 'YOUR_VERIFIED_FIREBASE_UID', lower('YOUR_VERIFIED_EMAIL'), 'admin', TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM hostelhive.users WHERE role = 'admin' AND is_active
)
ON CONFLICT (firebase_uid) DO UPDATE
SET role = 'admin', is_active = TRUE
WHERE lower(hostelhive.users.email) = lower(EXCLUDED.email)
RETURNING user_id, firebase_uid, email, role, is_active;
COMMIT;
```

If no row is returned, an active administrator already exists or the UID/email
pair does not match. Investigate; do not remove the guard. Sign in as the owner
and call /api/v1/me to verify role: admin before provisioning other accounts.
This SQL changes only local access; it does not create Firebase credentials.

### Local manual verification

Stop the old server with Ctrl+C, apply migrations and rebuild it, then start it
with your existing database/Firebase environment settings:

```powershell
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o ./bin/hostelhive-server.exe ./cmd/server
.\bin\hostelhive-server.exe
```

Expected migration version: 5. In a second PowerShell terminal, sign in to Firebase
as the initialized administrator using the existing sign-in procedure and keep
its ID token in $firebaseIDToken. Create a request key ONCE and enter a new user's
email and private initial password (do not use the administrator's email):

```powershell
$provisioningKey = [Guid]::NewGuid().ToString()
$newUserEmail = Read-Host 'New user email'
$newUserPassword = Read-Host 'New user initial password (12-128 characters)' -AsSecureString
$provisioningBody = @{
    email = $newUserEmail
    role = 'student'
    password = ([System.Net.NetworkCredential]::new('', $newUserPassword)).Password
} | ConvertTo-Json
$provisioningHeaders = @{
    Authorization = "Bearer $firebaseIDToken"
    'Idempotency-Key' = $provisioningKey
}
Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/users' -ContentType 'application/json' -Headers $provisioningHeaders -Body $provisioningBody
```

First success returns the active new account. Repeat only the last command with
unchanged headers/body to verify replayed: true and the same user_id/firebase_uid.
Check Firebase Users and PostgreSQL for exactly one account. After successful
verification, clear local credential variables:

```powershell
Remove-Variable provisioningBody, newUserPassword
```

Use a non-admin test user's ID token to verify 403; without a token expect 401.
Malformed input should return 400; changing the email or role with the same key
should return 409. Keep all passwords, ID tokens and credential JSON out of issues.
Unit fault-injection tests cover failures and retries; do not simulate outages
against a shared production Firebase project to test recovery.

### Provisioning tests

```powershell
go test -count=1 -timeout=30s -v ./internal/modules/identity/... ./internal/platform/firebase ./internal/app
.\scripts\test-migrations.ps1
go vet ./...
go test -count=1 -timeout=30s ./...
go build ./...
```

Unit tests cover input validation, all route roles, JSON/body limits, deadlines,
creation/replay/conflicts, fresh state, and failures before/after external side
effects. An in-memory HTTP transport tests the real pinned Firebase SDK's disabled
creation, activation, email-conflict mapping and UID-race recovery without live
credentials. The optional TestProvisioningPostgresIntegration test requires
TEST_DATABASE_URL pointed at a disposable database migrated through version 3;
it tests persisted retries, role/active-state preservation, partial failures,
row locks and removal of its own fixtures. Live Firebase creation still requires
manual verification with the project's credentials.

Firebase Admin user-management reference:
https://firebase.google.com/docs/auth/admin/manage-users


## Administrator account management (issue #22)

Apply migration 4 before starting this version. All endpoints below require a
verified Firebase Bearer ID token and a current active local `admin` account.
Responses use `Cache-Control: no-store` and never include credentials.

| Method and path | Request | Result |
| --- | --- | --- |
| GET /api/v1/users?limit=20&offset=0 | No body; limit 1-100, offset 0-1000000 | 200: users, limit, offset, has_more |
| PATCH /api/v1/users/{user_id}/role | JSON: `{"role":"warden"}` | 200: updated account |
| POST /api/v1/users/{user_id}/deactivate | Empty body | 200: account, revocation_pending=false; 202: local access blocked, Firebase revocation pending |

Roles: `admin`, `warden`, `sub_warden`, `security_staff`, `student`. IDs are local
UUID `user_id` values, not Firebase UIDs. Unknown accounts return 404; invalid
input returns 400; missing/invalid token returns 401; denied roles return 403.
Removing the last active admin's role or access returns 409 `last_active_admin`.
Database failures return generic 503 errors. Mutations recheck the actor's current
local role under a transaction lock, preventing stale administrator privileges.
Concurrent mutations cannot remove all active administrators. Role updates are
visible on subsequent protected requests without new tokens or server restarts.
Pagination orders by created_at/user_id; concurrent inserts or updates can change
pages, so this is not an export snapshot. Account reactivation and deletion are
outside this ticket.

Deactivation commits `is_active=false` and a durable revocation job atomically.
The backend then disables the Firebase identity and revokes refresh tokens. Only
acknowledged Firebase completion returns revocation_pending=false. On errors or
timeouts the local block remains; 202 indicates unfinished work, not full workflow
success. Missing Firebase identities are already unable to authenticate and are
treated as complete. The worker checks immediately at startup, then every 30s,
with up to 10 attempts per batch and ACCOUNT_MANAGEMENT_TIMEOUT (default 8s) per
attempt. Failed jobs become due after 30s and survive restarts. Multiple processes
use row locks to avoid simultaneous processing. Repeating deactivation is safe
and requests a fresh revocation. No passwords, tokens or raw provider errors are
stored in job metadata. Never reactivate blocked accounts manually while a job
is pending. There are currently no implemented RTDB grants; removal of future
realtime grants must be integrated before enabling direct realtime access.

ACCOUNT_MANAGEMENT_TIMEOUT also bounds each listing/change request after
authentication. Keep AUTH_TIMEOUT plus operation timeout below HTTP_WRITE_TIMEOUT
with adequate margin. Existing defaults are 5s + 8s under a 15s write timeout.

After live verification, use a disposable test account, not your administrator:

```powershell
$adminHeaders = @{ Authorization = "Bearer $firebaseIDToken" }
$page = Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/users?limit=20&offset=0' -Headers $adminHeaders
$page.users | Format-Table user_id, email, role, is_active
$managedUserID = Read-Host 'Disposable test account user_id from the listing'
$roleBody = @{ role = 'warden' } | ConvertTo-Json
Invoke-RestMethod -Method Patch -Uri "http://127.0.0.1:18080/api/v1/users/$managedUserID/role" -ContentType 'application/json' -Headers $adminHeaders -Body $roleBody
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/users/$managedUserID/deactivate" -Headers $adminHeaders
```

Before deactivating, sign in as the disposable test user and retain its ID token
privately in another terminal. `/api/v1/me` should show its updated role before
deactivation. Afterwards its existing token must receive 401 (Firebase disabled/
revoked) or 403 (local block while Firebase is pending). New Firebase sign-in must
fail after completion. Test a non-admin token against management routes for 403,
and try demoting/deactivating the sole admin for 409. Never put tokens in tickets.

To inspect pending work, run SQL in pgAdmin on the intended database:

```sql
SELECT u.email, j.requested_at, j.attempts, j.next_attempt_at, j.completed_at
FROM hostelhive.user_revocations j
JOIN hostelhive.users u ON u.user_id = j.user_id
ORDER BY j.requested_at DESC;
```

A null completed_at means pending. Keep the backend running and restore Firebase
connectivity/permissions; it retries automatically. Do not simulate outages in a
shared production project. Migration 4 down refuses to discard unfinished jobs;
once all jobs finish it drops metadata only and leaves accounts disabled.

```powershell
go test ./...
go vet ./...
.\scripts\test-account-management.ps1
.\scripts\test-migrations.ps1
go build ./...
```

The account-management script creates and removes its own disposable PostgreSQL
container, applies migrations, and runs real transaction/concurrency/recovery
tests. It never uses the development DATABASE_URL. Unit tests use an in-memory
transport with the pinned Firebase SDK and no live credentials; live Firebase
deactivation still needs the manual checks above.

Firebase session reference: https://firebase.google.com/docs/auth/admin/manage-sessions


## Student profile management (issue #24)

Traceability: FR006, FR007, FR009; SRS UC003 (PDF page 39); SDS Figure 7
(PDF page 38), and normalized STUDENT/GUARDIAN entities. UC003 and Figure 7
identify Admin as the actor. Tasheen resolved the FR006-FR009/UC003 discrepancy
by permitting active Admin AND Warden accounts in issue #30. Sub-wardens,
security staff and students cannot use these management routes.
Account credentials remain in Firebase; the linked users table supplies email
and account status. Profile-image uploads are implemented in issue #30;
see [student image setup and verification](docs/student-images.md). QR generation
and the SRS UC004 CSV import remain separate, unimplemented features.

Apply migration 5 before starting this version. `students.student_id` is an
internal UUID; `index_no` is the human student identification/index number.
Each profile has exactly one linked user account and one or more guardian rows.
Create requires an existing active `student` account from provisioning; it
does not create Firebase credentials or reactivate accounts. Updates keep the
user_id immutable and preserve the student_id. Changes use transactions and
recheck administrator privileges against current local account state.

| Method/path | Behavior |
| --- | --- |
| POST /api/v1/students | Create profile; 201 with Location header |
| GET /api/v1/students | Paginated summaries; 200 |
| GET /api/v1/students/{student_id} | Full profile with guardians; 200 |
| PUT /api/v1/students/{student_id} | Replace all editable profile fields and guardian list; 200 |
| DELETE /api/v1/students/{student_id} | Soft-delete profile; empty body required; 204 |

POST body (use the actual local user_id of an active student account):

```json
{
  "user_id": "11111111-1111-1111-1111-111111111111",
  "index_no": "SC/2026/001",
  "full_name": "Test Student",
  "faculty": "Science",
  "year": 1,
  "contact_phone": "0771234567",
  "guardians": [
    {"name": "Test Guardian", "relationship": "Parent", "contact_phone": "0779876543"}
  ]
}
```

PUT uses the same fields except user_id. Every editable field is required; this
is a full replacement, not PATCH. Guardian rows are replaced atomically, so their
UUIDs may change; do not use guardian IDs as external historical identifiers.
Creating/updating returns the full profile including guardians. Listing excludes
guardian details and supports `limit` (default 20, 1-100), `offset` (default 0,
0-1000000), `q` (literal case-insensitive substring of index_no/full_name),
`faculty` (case-insensitive exact match), and `year` (1-10). Filters combine with
AND. Blank, duplicate or unknown query parameters are rejected. Summaries include
email/account_active from the linked account. Sorting uses created_at/student_id;
pagination is not a snapshot across concurrent changes.

Implementation validation bounds: trimmed nonblank index_no up to 64 characters,
full_name and guardian name up to 200, faculty up to 120, relationship up to 80;
year 1-10; 1-10 guardians; phones 7-32 ASCII characters using digits, spaces,
parentheses, plus or hyphen, with at least 7 digits. Unicode names are supported;
control characters and invalid UTF-8 are rejected. These are implementation
bounds rather than numeric limits specified in the SRS. Frontends must display
these fields as escaped text. Payload limit 16 KiB; unknown JSON fields and extra
JSON documents are rejected. STUDENT_PROFILE_TIMEOUT defaults to 8s; keep it plus
AUTH_TIMEOUT below HTTP_WRITE_TIMEOUT with a margin.

Deletion sets deleted_at/deleted_by and hides the profile from GET/list/update.
It retains student/guardian rows and historical foreign-key references; index_no
and user_id remain reserved even after deletion. Repeating DELETE returns 204;
an unknown UUID returns 404. This is not irreversible data erasure or account
deactivation. Use the account deactivation endpoint separately if access must be
blocked. No profile restoration endpoint is implemented. Future attendance,
allocation and leave features must reject archived profiles for new activity
while retaining historical references. Migration rollback removes the new tables
and their data; it is intended for disposable verification before rollout, with
RESTRICT protecting future dependent tables.

Other errors: 400 invalid_input, 401 unauthorized, 403 forbidden,
409 duplicate_student (case-insensitive index or linked-account collision),
409 active_student_account_required (missing/inactive/non-student account),
503 student_profiles_unavailable. Responses use no-store and generic errors.

### Local verification

Stop the old server, set your existing database/Firebase environment settings,
run migrations (expected version 5), rebuild and restart:

```powershell
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o ./bin/hostelhive-server.exe ./cmd/server
.\bin\hostelhive-server.exe
```

In the other terminal use a fresh administrator ID token in $firebaseIDToken.
Provision a NEW active student test account using POST /api/v1/users first;
the test account deactivated in issue #22 cannot be used for profile creation.

```powershell
$adminHeaders = @{ Authorization = "Bearer $firebaseIDToken" }
$studentAccountID = Read-Host 'Active student account user_id'
$profileInput = @{
    user_id = $studentAccountID
    index_no = 'TEST-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
    full_name = 'Test Student'
    faculty = 'Science'
    year = 1
    contact_phone = '0771234567'
    guardians = @(@{ name='Test Guardian'; relationship='Parent'; contact_phone='0779876543' })
}
$student = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/students' -Headers $adminHeaders -ContentType 'application/json' -Body ($profileInput | ConvertTo-Json -Depth 5)
$student | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/students/$($student.student_id)" -Headers $adminHeaders
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/students?faculty=Science&year=1&limit=10' -Headers $adminHeaders
# Full update: omit the immutable account link.
$profileInput.Remove('user_id')
$profileInput.full_name = 'Updated Test Student'
Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:18080/api/v1/students/$($student.student_id)" -Headers $adminHeaders -ContentType 'application/json' -Body ($profileInput | ConvertTo-Json -Depth 5)
Invoke-RestMethod -Method Delete -Uri "http://127.0.0.1:18080/api/v1/students/$($student.student_id)" -Headers $adminHeaders
# Expect 404 after deletion:
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/students/$($student.student_id)" -Headers $adminHeaders
```

Before deletion, repeat the original POST with the same user_id/index_no for 409,
and test with a Student or other non-staff token for 403. Never include tokens or real guardian
information in issue comments. Use fictional profile details for verification.

```powershell
go test ./...
go vet ./...
.\scripts\test-student-profiles.ps1
.\scripts\test-migrations.ps1
go build ./...
```

The student verification script creates its own disposable PostgreSQL container,
applies migrations and runs CRUD, duplicate-race, account eligibility, filtering,
transaction and history-reference tests. It never uses the developer database
and removes its container. Unit tests cover all role/route combinations, input
validation and safe responses. Live API checks remain a separate manual step.

## Administrator account reactivation

`POST /api/v1/users/{user_id}/activate` requires an active administrator's
Firebase bearer ID token and an **empty request body**. It returns HTTP 200
with the account object (`user_id`, `firebase_uid`, `email`, `role`, `is_active`).
The role and password are preserved. Repeating an activation for an already
active account returns its current state without changing Firebase again.

The transaction rechecks the administrator and serializes local account writes.
It rejects pending deactivation jobs (`409 revocation_pending`) and unfinished
provisioning (`409 provisioning_incomplete`). For inactive accounts, Firebase
UID and email must match the local account. Old sessions are revoked before
Firebase sign-in is enabled; local access is enabled only after acknowledgment.
Revocation-row locks prevent a retry worker from disabling a reactivated account.
No new database migration is needed.

Other responses: 400 `invalid_input`, 401 `unauthorized`, 403 `forbidden`,
404 `not_found`, 409 `firebase_identity_missing` or `firebase_identity_mismatch`,
and 503 `account_management_unavailable`. The operation uses
`ACCOUNT_MANAGEMENT_TIMEOUT`. A provider failure leaves local access blocked.
If Firebase succeeds but the database commit fails, local access remains blocked;
retry the administrator request after recovery. Missing/mismatched identities
require investigation; this endpoint never recreates credentials.

### Verify reactivation locally

Rebuild and restart the server using the startup instructions above. Use a
disposable test account, not the only administrator. In the test terminal, obtain
a fresh administrator ID token using the Firebase sign-in instructions, then:

```powershell
$adminHeaders = @{ Authorization = "Bearer $firebaseIDToken" }
$managedUserID = Read-Host 'Disposable test account user_id'
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/users/$managedUserID/deactivate" -Headers $adminHeaders
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/users/$managedUserID/activate" -Headers $adminHeaders
```

If activation returns `revocation_pending`, wait for the revocation worker to
finish and retry. Expect `is_active: True` with the same role. Repeat activation
to verify idempotence. The test user's old sessions remain revoked: sign in
again after activation and verify `/api/v1/me` with that user's new ID token.
A non-admin token must receive 403; a request without a token must receive 401.
Do not update `is_active` directly in SQL or manually enable Firebase to bypass
the lifecycle. Keep tokens and passwords out of issue comments.

Automated verification:

```powershell
go test ./...
go vet ./...
go build ./...
.\scripts\test-account-management.ps1
```

The account integration script uses disposable PostgreSQL and mocked Firebase
responses; SDK transport tests exercise the pinned Firebase SDK without network
credentials. Real Firebase sign-in and teammate review remain manual gates.
See [identity requirements](docs/identity-requirements.md) for module readiness.

## Student profile images (issue #30)

Admin and Warden can manage student profiles and images under the agreed
7 October 2026 permission decision. Other roles remain denied. This does not
grant Warden access to administrator identity/account-management endpoints.

Migration 000006 supports private Cloudflare R2 image upload, retrieval,
replacement/removal and durable cleanup. JPEG/PNG images are validated and
re-encoded. Uploads are limited to 5 MiB, 4096 pixels per dimension and 12 million
pixels. `profile_image_url` is an authenticated API path, not a public R2 URL.

R2 defaults to disabled. Set `R2_ENABLED=true`, `R2_ACCOUNT_ID`, `R2_BUCKET`,
`R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY` in the server terminal to enable it.
Credentials must stay outside Git. Apply migrations before running the updated
server even when R2 is disabled; existing student queries now use migration 6.
Image endpoints return 503 when disabled; other endpoints remain available.

See [student image setup and verification](docs/student-images.md) for complete
PowerShell commands, API/error contracts, cleanup/retry behavior and permission
verification. Automated lifecycle tests run with `scripts/test-student-profiles.ps1`.
Live R2 verification and teammate review are required before closing issue #30.

## Hostel inventory (issue #33)

Warden-only block, room and bed listings, with derived capacity and availability.
See [inventory contract, startup and verification](docs/hostel-inventory.md).
Apply migration 000007 and run `scripts/test-inventory.ps1` before review.

## Staff profiles (issue #36)

Apply migrations 000008 and 000009 before starting this version. Admins manage
staff names/designations through the staff management routes. Active staff can
create/edit their own name through PUT `/api/v1/staff/me` and read their current
profile through `/api/v1/me`. Self edits preserve Admin-assigned designations.
New self-created profiles leave designation unassigned until an Admin fills it.
Student names remain in student profiles. Existing accounts remain valid without
a profile. See [staff contract and complete verification/PR steps](docs/staff-profiles.md).
