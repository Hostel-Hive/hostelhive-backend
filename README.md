# HostelHive Backend

Go backend for HostelHive. Maintainer: Tasheen. Repository: https://github.com/Hostel-Hive/hostelhive-backend.

This foundation implements environment configuration validation, a bounded HTTP server lifecycle, structured startup/error logging and graceful shutdown. Operational endpoints provide health and PostgreSQL readiness checks. GET /api/v1/me verifies Firebase authentication and returns the current local account.

## Prerequisites

- Go 1.27 or later: https://go.dev/dl/
- Git

PostgreSQL is now required for readiness. The backend uses pgxpool, with dependencies pinned in go.mod/go.sum. Firebase Authentication now requires a Firebase project ID and Application Default Credentials (see below).

## Structure

```text
cmd/server/main.go             Startup, signals and process exit
internal/config/              Environment parsing and validation tests
internal/database/            PostgreSQL connection pool and tests
internal/authentication/      Firebase verification, local accounts and middleware
internal/server/              HTTP lifecycle and shutdown tests
.env.example                  Supported settings (no secrets)
migrations/                   Versioned SQL migration pairs
scripts/                      Migration tool setup and verification
go.mod                        Go module
```

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
    go test -count=1 -timeout=30s -v ./internal/database
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

After the baseline alone, version is 1. With account management and revocation tracking, the current version is 4. Repeating up reports no change.
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

    migrations/000005_description.up.sql
    migrations/000005_description.down.sql

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
profiles will reference user_id in later migrations. Role strings above map to the
SDS human actors; scanner credentials are separate from human user accounts.
The SQL email check is basic integrity protection; full validation belongs in the
provisioning service/Firebase workflow.

Firebase handles user credentials. This table contains no password hashes or local
refresh/reset tokens. The future protected API must verify a Firebase ID token,
then retrieve the matching firebase_uid and check the current role and is_active.
An account remains inactive until the provisioning/activation workflow enables it.
Neither applying this migration nor changing is_active creates or changes a Firebase
identity; cross-system activation and revocation are separate implementation tasks.
Firebase SDK/service-account credentials are not needed to apply these SQL files.

Apply the migration to your local database with the existing commands:

    .\scripts\migrate.ps1 up
    .\scripts\migrate.ps1 version

Expected current version: 4. In pgAdmin, refresh:

    hostelhive database -> Schemas -> hostelhive -> Tables -> users

Migration rollback drops the users table and its timestamp trigger function.
It deletes local account records and refuses to cascade into dependent objects.
Use the disposable verification script to test rollback rather than an application
database containing accounts:

    .\scripts\test-migrations.ps1

Tests in tests/sql/user_accounts.sql verify all five roles, inactive defaults,
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
gofmt -l ./cmd ./internal
go vet ./...
go test -count=1 -timeout=30s -cover ./...
go build ./...
```

`gofmt -l` must print no files. Configuration tests cover defaults, environment overrides, missing/empty or malformed supplied values and timeout validation. HTTP tests verify endpoint status codes, JSON bodies, content types, route boundaries, bind failures, draining active requests and forced closure after the grace period. These tests require local loopback socket access. A teammate must independently follow this README to satisfy the ticket's fresh-setup acceptance criterion.

## Architecture and next tickets

The selected baseline uses Firebase Authentication (ADR-002), containerized PostgreSQL (ADR-003), backend-published Firebase RTDB projections (ADR-004) and Nginx HTTPS routing (ADR-005). This scaffold does not issue local user JWTs or store user passwords. ADRs live in the continued documents repository: https://github.com/Hostel-Hive/Design-and-Development-Project.

Account management, endpoint-specific permissions, scanner contracts, business modules, Docker infrastructure and CI are separate tickets. Add packages when their functionality is implemented instead of creating empty domain directories.

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
go test -count=1 -timeout=30s ./internal/authentication ./internal/server ./internal/config
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
go test -count=1 -timeout=30s -v ./internal/authentication
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

Expected migration version: 4. In a second PowerShell terminal, sign in to Firebase
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
go test -count=1 -timeout=30s -v ./internal/provisioning ./internal/server
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
