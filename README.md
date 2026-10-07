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

Start the local database as described below, apply migrations through version 2, then configure DATABASE_URL and Firebase credentials in the backend terminal. From the repository directory:

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

After the baseline alone, version is 1. With the user-account migration, the current version is 2. Repeating up reports no change.
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

    migrations/000003_description.up.sql
    migrations/000003_description.down.sql

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

Expected current version: 2. In pgAdmin, refresh:

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

Account provisioning, role-specific authorization, scanner contracts, business modules, Docker infrastructure and CI are separate tickets. Add packages when their functionality is implemented instead of creating empty domain directories.

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
identity alone receives HTTP 403; this ticket does not provision or activate
accounts automatically. Account provisioning is a separate ticket. For live
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
migrated to version 2; its fixture is rolled back automatically. Live Firebase
valid/expired/revoked/disabled token checks require the project's credentials and
test identities and are not claimed by the offline tests.
