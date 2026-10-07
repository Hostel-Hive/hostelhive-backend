# HostelHive Backend

Go backend for HostelHive. Maintainer: Tasheen. Repository: https://github.com/Hostel-Hive/hostelhive-backend.

This foundation implements environment configuration validation, a bounded HTTP server lifecycle, structured startup/error logging and graceful shutdown. Operational endpoints provide health and PostgreSQL readiness checks. Business routes are not registered yet.

## Prerequisites

- Go 1.27 or later: https://go.dev/dl/
- Git

PostgreSQL is now required for readiness. The backend uses pgxpool, with dependencies pinned in go.mod/go.sum. Firebase credentials are not needed yet.

## Structure

```text
cmd/server/main.go             Startup, signals and process exit
internal/config/              Environment parsing and validation tests
internal/database/            PostgreSQL connection pool and tests
internal/server/              HTTP lifecycle and shutdown tests
.env.example                  Supported settings (no secrets)
migrations/                   Versioned SQL migration pairs
scripts/                      Migration tool setup and verification
go.mod                        Go module
```

## Start locally

Start the local database as described below, then set DATABASE_URL in the backend terminal. From the repository directory:

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

After the baseline, version prints 1. Repeating up reports no change.
The wrapper pins version tracking to the tool-managed public.schema_migrations table, independently of the effective PostgreSQL search path.
Before any migration is applied, version reports no migration.

The baseline pair is:

- migrations/000001_baseline.up.sql: creates the empty hostelhive schema.
- migrations/000001_baseline.down.sql: removes that schema using RESTRICT.

Business tables are added in later tickets. Use explicit schema-qualified names
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
PostgreSQL 16 container, checks apply/version/repeated apply/rollback/reapply,
and removes its container afterward. It never uses your existing DATABASE_URL
as the test target and restores that environment variable when finished.

Add future migrations as consecutive six-digit version pairs:

    migrations/000002_description.up.sql
    migrations/000002_description.down.sql

Commit both files together. Do not edit an already applied migration; add a new
version. Keep real credentials and generated migration executables out of Git.

## Configuration

The application reads **process environment variables**, not `.env` files automatically. `.env.example` is documentation for manual configuration or future Docker Compose integration. Never commit real credentials.

| Variable | Default | Valid values |
|---|---|---|
| DATABASE_URL | Required; no default | PostgreSQL URL with host, user and database |
| DATABASE_CHECK_TIMEOUT | 2s | Positive Go duration; connection and readiness timeout |
| APP_ENV | development | development, test, production |
| HTTP_ADDR | 127.0.0.1:8080 | IP/localhost/empty host plus numeric port 1-65535; IPv6 uses brackets |
| HTTP_READ_HEADER_TIMEOUT | 5s | Positive Go duration |
| HTTP_READ_TIMEOUT | 15s | Positive Go duration |
| HTTP_WRITE_TIMEOUT | 15s | Positive Go duration |
| HTTP_IDLE_TIMEOUT | 60s | Positive Go duration |
| HTTP_SHUTDOWN_TIMEOUT | 10s | Positive Go duration |

Unset optional variables use defaults. DATABASE_URL is required. Explicit empty/invalid variables cause an error identifying the setting and a nonzero exit before the server listens.

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

Business table migrations, Firebase verification and authorization, scanner contracts, business modules, Docker infrastructure and CI are separate tickets. Add packages when their functionality is implemented instead of creating empty domain directories.

When using the port override above, check http://127.0.0.1:18080/health and http://127.0.0.1:18080/ready.
