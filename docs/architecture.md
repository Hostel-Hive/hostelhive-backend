# Backend architecture

HostelHive is a Go modular monolith. This layout implements the supplied
HostelHive backend folder-structure reference for the features currently built.
It preserves ADR-002: Firebase owns credentials and token verification;
PostgreSQL owns current application roles and account state.

## Request path

`cmd/server` starts `internal/app`, which constructs repositories, platform
adapters, services, HTTP handlers and the revocation worker. Module `routes.go`
files register protected feature routes. The app router owns `/health` and `/ready`.

Authentication middleware verifies the Firebase token and reloads the local
account for every request. Role policies explicitly list permitted roles.
Handlers validate HTTP shape, identifiers and query parameters, then call
service interfaces. Services normalize feature input and coordinate use cases.
Repositories own SQL, transactions and persistence error translation.

Atomic last-admin, actor authorization and student-account eligibility guards
stay inside repository transactions. Moving these checks into an earlier read
would introduce races. Deactivation commits local denial and durable retry work
before the service asks Firebase to disable the identity and revoke sessions.
The worker retries pending jobs and stops before the app closes the database pool.

## Implemented layers

| Location | Responsibility |
| --- | --- |
| `internal/app` | Concrete dependency wiring, probes and HTTP shutdown |
| `internal/config` | Environment configuration |
| `internal/platform/postgres` | Connection pool setup |
| `internal/platform/firebase` | Token verification, identity creation and revocation |
| `internal/shared/middleware` | Verified account context and role policies |
| `internal/shared/response` | Common safe authentication/error responses |
| `internal/shared/validation` | Common UUID, text and phone validation |
| `internal/modules/identity` | Account provisioning, lookup and management |
| `internal/modules/student` | Student and guardian profile management |
| `internal/workers` | Durable user-revocation retry orchestration |

Each feature module has `domain`, `dto`, `service`, `repository`, `handler` and
`routes.go`. Domain records and errors have no infrastructure dependencies.
DTOs describe request payloads; domain response records retain their existing
JSON contract. Persistence and provider ports live in `service/ports.go`.
Implementations satisfy these interfaces without services importing adapters.
Handlers depend on service contracts, never concrete repositories or SDK clients.

## Extending the backend

Add attendance, allocation, leave, complaint, notice and report modules when
implementing those features. Cross-module workflows should call service
interfaces; never import another module's repository. Existing student writes
use account rows in the same PostgreSQL transaction to enforce authorization
and eligibility without a separate identity-repository call.

Redis, RTDB, FCM, R2 and scanner adapters will be added when their features need
them. No empty future modules or adapters are required. The reference's
`cmd/migrate` is not implemented: existing pinned CLI scripts remain the migration
entry point, and migration history must not be renumbered for a folder refactor.
ADRs and the SRS/SDS remain in the team's
[documents repository](https://github.com/Hostel-Hive/Design-and-Development-Project).

## Verification

Unit tests stay beside the implementation. PostgreSQL suites live under
`tests/integration`, SQL constraint checks under `tests/migration`, and import
boundary checks under `tests/architecture`.

```powershell
gofmt -l ./cmd ./internal ./tests
go vet ./...
go test -count=1 -timeout=45s ./...
go build -o ./bin/hostelhive-server.exe ./cmd/server
.\scripts\test-migrations.ps1
.\scripts\test-account-management.ps1
.\scripts\test-student-profiles.ps1
```

Go integration tests skip when `TEST_DATABASE_URL` is unset. The PowerShell
verification scripts create, migrate and remove their own temporary containers;
they do not modify the developer database. Live Firebase verification uses the
existing README steps and is separate from offline adapter tests.
