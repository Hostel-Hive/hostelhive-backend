# HostelHive Backend

[![Backend CI](https://github.com/Hostel-Hive/hostelhive-backend/actions/workflows/backend-ci.yml/badge.svg?branch=develop)](https://github.com/Hostel-Hive/hostelhive-backend/actions/workflows/backend-ci.yml?query=branch%3Adevelop)
[![Test coverage reports](https://img.shields.io/badge/coverage-CI%20artifacts-blue)](https://github.com/Hostel-Hive/hostelhive-backend/actions/workflows/backend-ci.yml?query=branch%3Adevelop)

Go REST API for HostelHive student registration, accommodation, attendance,
access requests, complaints, announcements and reporting. Firebase handles
authentication; PostgreSQL stores application data.

Part of the [HostelHive project](https://github.com/Hostel-Hive), with
[design documents and the development guideline](https://github.com/Hostel-Hive/Design-and-Development-Project)
and a separate [frontend repository](https://github.com/Hostel-Hive/hostelhive-frontend).

## Live application and API

The shared assessment environment uses the `develop` branch. The frontend is
hosted on Vercel, the backend on Render and PostgreSQL on Supabase.

| Service | Live URL |
| --- | --- |
| Student login | [Open student portal](https://web-kohl-delta-90.vercel.app/login) |
| Admin / Warden login | [Open staff portal](https://web-kohl-delta-90.vercel.app/staff/login) |
| Backend base URL | [hostelhive-backend-dev.onrender.com](https://hostelhive-backend-dev.onrender.com) |
| API base URL | [Backend `/api/v1`](https://hostelhive-backend-dev.onrender.com/api/v1) |
| Swagger API documentation | [Open Swagger](https://hostelhive-backend-dev.onrender.com/docs/) |
| OpenAPI specification | [Open API JSON](https://hostelhive-backend-dev.onrender.com/openapi.json) |
| Backend health | [Check `/health`](https://hostelhive-backend-dev.onrender.com/health) |
| Database readiness | [Check `/ready`](https://hostelhive-backend-dev.onrender.com/ready) |

The backend base URL and `/api/v1` are prefixes, not landing pages. Open Swagger
to browse endpoints. Health and readiness return JSON; readiness checks database
connectivity, not migration completeness or Firebase configuration.

## Demo accounts

These shared accounts are supplied for assessment using test data. Admin and
Warden use the staff portal; Student uses the student portal.

| Role | Email | Password | Login |
| --- | --- | --- | --- |
| Admin | `darshikatasheen99@gmail.com` | `JackCM@3003` | [Staff](https://web-kohl-delta-90.vercel.app/staff/login) |
| Warden | `navonsanjuni@gmail.com` | `Sanjuni@1234` | [Staff](https://web-kohl-delta-90.vercel.app/staff/login) |
| Student | `kavindithagya21@gmail.com` | `Thagya21` | [Student](https://web-kohl-delta-90.vercel.app/login) |

For Swagger testing, sign in through Firebase and authorize with the resulting
ID token. The account email, password and Firebase UID are not bearer tokens.

## Requirements

- Go **1.27.3 or later** (see [go.mod](go.mod)).
- PostgreSQL; Docker Desktop is supported for local development.
- A Firebase project and a service-account JSON file stored outside this repository.
- Git and PowerShell for the Windows commands below.

## Local setup

Run these commands from the repository root. On a fresh checkout, create your
local configuration:

```powershell
if (-not (Test-Path '.env.local')) {
    Copy-Item .env.example .env.local
}
notepad .env.local
```

Set these values using your own database and Firebase project:

| Setting | Purpose |
| --- | --- |
| `APP_ENV=development` | Local development mode |
| `HTTP_ADDR=127.0.0.1:18080` | Local server address |
| `DATABASE_URL` | PostgreSQL URL, including user, password, host, port and database |
| `FIREBASE_PROJECT_ID` | Firebase project ID |
| `GOOGLE_APPLICATION_CREDENTIALS` | Absolute path to the service-account JSON |

For local PostgreSQL, start Docker Desktop and create the container **once**:

```powershell
$env:POSTGRES_PASSWORD = 'local-dev-only'
docker run --name hostelhive-postgres -d `
    -p 127.0.0.1:15433:5432 `
    -e POSTGRES_PASSWORD -e POSTGRES_USER=hostelhive -e POSTGRES_DB=hostelhive `
    postgres:16-alpine
```

Use `postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable`
as the URL for this local container. Existing containers do not need recreating.

Install the migration tool, then apply migrations and start the backend:

```powershell
.\scripts\install-migrate.ps1
.\scripts\dev.ps1 -Migrate
```

For subsequent starts:

```powershell
.\scripts\dev.ps1
```

The launcher loads `.env.local`, starts the existing database container and runs
the server. Stop it with **Ctrl+C**. For a hosted or separately managed database,
use `-SkipDatabase`; add `-Migrate` when migrations need applying.
See [local development](docs/local-development.md) for all options.

## API documentation and health

With the example local address:

| URL | Purpose |
| --- | --- |
| [Swagger UI](http://127.0.0.1:18080/docs/) | Browse and test APIs by module and role |
| [OpenAPI JSON](http://127.0.0.1:18080/openapi.json) | Machine-readable API contract |
| [Health](http://127.0.0.1:18080/health) | Process liveness |
| [Readiness](http://127.0.0.1:18080/ready) | PostgreSQL connectivity |

Protected user APIs require a **Firebase ID token**. Scanner APIs use a separate
device credential. Readiness does not verify migrations or Firebase configuration.
The root URL `/` returns 404. See [Swagger instructions](docs/swagger-openapi.md)
and [first administrator setup](docs/admin-bootstrap.md).

## Configuration and deployment

[.env.example](.env.example) lists supported settings. `.env.local` is ignored by
Git; keep deployment secrets, bearer tokens and service-account files out of version control.
The Go executable reads process environment variables; the development launcher
loads the local file explicitly.

| Integration | Settings / documentation |
| --- | --- |
| Student invitations | `STUDENT_INVITATIONS_ENABLED`, `STUDENT_EMAIL_DOMAINS`; [setup and delivery](docs/student-invitations.md) |
| Private image storage | `R2_ENABLED` and R2 account, bucket and credentials; [image storage](docs/student-images.md) |
| Realtime attendance | `RTDB_ENABLED`, `FIREBASE_DATABASE_URL`; [RTDB setup](docs/attendance-realtime.md) |
| Mobile push | `FCM_ENABLED`; [FCM setup](docs/mobile-push-notifications.md) |
| Browser access | `CORS_ALLOWED_ORIGINS`; [Vercel CORS setup](docs/browser-cors.md) |

The shared backend deploys from **`develop`** on Render and uses Supabase
PostgreSQL. Configure production variables and the Firebase secret file in Render.

```text
Build command: go mod download && go build -trimpath -ldflags="-s -w" -o server ./cmd/server
Start command: ./server
HTTP_ADDR: 0.0.0.0:10000
Health check path: /ready
```

Apply pending migrations to the intended database before accepting traffic from
the new code. Set `DATABASE_URL` in the migration terminal, then run:

```powershell
.\scripts\migrate.ps1 version
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
```

The server does not run migrations automatically. Local `.env.local` changes do
not update Render settings. A successful CI run does not confirm deployment.

## Module documentation

| Area | Guides |
| --- | --- |
| Identity and staff | [Identity requirements](docs/identity-requirements.md), [staff profiles](docs/staff-profiles.md), [password flows](docs/firebase-password-flows.md) |
| Student registration | [Individual and CSV invitations](docs/student-invitations.md), [legacy profile-only CSV import](docs/student-csv-import.md) |
| Student profiles | [Own profile editing](docs/student-self-profile-editing.md), [own photos](docs/student-own-profile-photo.md), [QR codes](docs/student-qr.md) |
| Student lifecycle | [Restore profiles](docs/student-profile-restoration.md), [End hostel stay](docs/student-checkout.md), [account/profile distinctions](docs/student-lifecycle-guidance.md) |
| Accommodation | [Inventory management](docs/inventory-management.md), [bed allocation](docs/bed-allocation.md) |
| Attendance | [Scanner devices](docs/scanner-devices.md), [scans](docs/attendance-scans.md), [dashboard](docs/attendance-dashboard.md), [student history](docs/student-attendance-history.md) |
| Movement and passes | [Current movement policy](docs/hostel-movement-policy.md), [restricted-time requests](docs/late-access-requests.md), [Going Home](docs/going-home-declarations.md), [cancellation](docs/pass-cancellation.md), [combined history](docs/combined-student-pass-history.md) |
| Complaints and notices | [Complaints](docs/complaint-management.md), [announcements](docs/hostel-announcements.md), [faculty targeting](docs/faculty-announcements.md) |
| Reports | [Attendance](docs/attendance-history-reports.md), [leave history](docs/leave-request-reports.md), [complaints](docs/complaint-reports.md), [Excel](docs/excel-report-exports.md), [PDF](docs/pdf-report-exports.md) |

The current movement policy supersedes older leave-approval and yearly-allowance
behavior. See the policy guide before integrating those historical endpoints.

## Development checks

```powershell
go vet ./...
go test ./...
go build ./...
go run ./cmd/openapi -check docs/openapi.json
```

Database integration tests require an isolated migrated `TEST_DATABASE_URL`;
without it, applicable tests skip. See [CI and coverage](docs/backend-ci.md) for
the full verification workflow. After API changes, regenerate the specification
with `go run ./cmd/openapi -output docs/openapi.json`.

The coverage badge links to workflow runs containing coverage artifacts; it does
not claim a measured percentage. Open a successful run and download its coverage
artifacts for the measured results.

## Contribution workflow

Follow the [course development guideline, section 9.3](https://github.com/Hostel-Hive/Design-and-Development-Project/blob/main/documents/guidelines/SENG34213_System_Development_Project.pdf)
for repository submission. Create an issue, branch from `develop` using
`feature/<ticket>-<slug>` or `fix/<ticket>-<slug>`, use Conventional Commits and
open a pull request into `develop`. Require passing CI and peer review.
Production-ready releases move to `main` through an approved pull request;
the shared assessment deployment currently uses `develop`.

See [architecture](docs/architecture.md), [module layout](docs/module-layout.md)
and [file naming](docs/file-naming.md) for code organization.
