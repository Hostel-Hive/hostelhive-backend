# HostelHive Backend

[![Backend CI](https://github.com/Hostel-Hive/hostelhive-backend/actions/workflows/backend-ci.yml/badge.svg?branch=develop)](https://github.com/Hostel-Hive/hostelhive-backend/actions/workflows/backend-ci.yml?query=branch%3Adevelop)
[![Coverage reports](https://img.shields.io/badge/coverage-CI%20artifacts-blue)](https://github.com/Hostel-Hive/hostelhive-backend/actions/workflows/backend-ci.yml?query=branch%3Adevelop)

Go REST API for student registration, accommodation, attendance, access requests,
complaints, announcements and reports. Uses Firebase Authentication and PostgreSQL.
Part of [HostelHive](https://github.com/Hostel-Hive):
[frontend](https://github.com/Hostel-Hive/hostelhive-frontend) ·
[project documents](https://github.com/Hostel-Hive/Design-and-Development-Project).

## Live application

| Service | URL |
| --- | --- |
| Student login | [Student portal](https://web-kohl-delta-90.vercel.app/login) |
| Admin / Warden login | [Staff portal](https://web-kohl-delta-90.vercel.app/staff/login) |
| Backend | [Base URL](https://hostelhive-backend-dev.onrender.com) · [API prefix `/api/v1`](https://hostelhive-backend-dev.onrender.com/api/v1) |
| API documentation | [Swagger](https://hostelhive-backend-dev.onrender.com/docs/) · [OpenAPI JSON](https://hostelhive-backend-dev.onrender.com/openapi.json) |
| Service checks | [Health](https://hostelhive-backend-dev.onrender.com/health) · [Database readiness](https://hostelhive-backend-dev.onrender.com/ready) |

Assessment deployment: `develop` → Render backend, Vercel frontend and Supabase
PostgreSQL. API prefixes are not landing pages; use Swagger to explore endpoints.

## Demo accounts

Shared assessment accounts for test data. Admin and Warden use the staff portal.

| Role | Email | Password |
| --- | --- | --- |
| Admin | `darshikatasheen99@gmail.com` | `JackCM@3003` |
| Warden | `navonsanjuni@gmail.com` | `Sanjuni@1234` |
| Student | `kavindithagya21@gmail.com` | `Thagya21` |

Swagger requires a Firebase **ID token**, not an email, password or Firebase UID.

## Run locally

Requirements: **Go 1.27.3+**, Git, PowerShell, PostgreSQL and Firebase credentials.
For Docker-based development, start Docker Desktop and create this container once:

```powershell
docker run --name hostelhive-postgres -d -p 127.0.0.1:15433:5432 -e POSTGRES_PASSWORD=local-dev-only -e POSTGRES_USER=hostelhive -e POSTGRES_DB=hostelhive postgres:16-alpine
```

From the repository root:

```powershell
if (-not (Test-Path '.env.local')) { Copy-Item .env.example .env.local }
notepad .env.local
```

Configure `DATABASE_URL`, `FIREBASE_PROJECT_ID` and
`GOOGLE_APPLICATION_CREDENTIALS` (path to a service-account file outside the repo).
For this local container, use
`postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable`.
Set `APP_ENV=development` and `HTTP_ADDR=127.0.0.1:18080`.

```powershell
.\scripts\install-migrate.ps1
.\scripts\dev.ps1 -Migrate
```

Then start with `.\scripts\dev.ps1`; stop with **Ctrl+C**. Swagger opens at
[localhost:18080/docs/](http://127.0.0.1:18080/docs/). Use `-SkipDatabase` for a
hosted database. Migrations are explicit; normal startup does not apply them.

## Checks and documentation

```powershell
go vet ./...
go test ./...
go build ./...
go run ./cmd/openapi -check docs/openapi.json
```

Database integration tests need an isolated migrated `TEST_DATABASE_URL`.
Download coverage artifacts from successful CI runs; the badge is a report link,
not a measured percentage.

- [Local configuration and startup options](docs/local-development.md)
- [API testing](docs/swagger-openapi.md) and [first Admin setup](docs/admin-bootstrap.md)
- [CI and coverage](docs/backend-ci.md)
- [Architecture](docs/architecture.md) and [module guides](docs/)

Open pull requests into `develop` with passing CI and peer review. Keep deployment
secrets, bearer tokens and service-account files outside version control.
