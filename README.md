# HostelHive Backend

Go backend for HostelHive. Maintainer: Tasheen. Repository: https://github.com/Hostel-Hive/hostelhive-backend.

This foundation implements environment configuration validation, a bounded HTTP server lifecycle, structured startup/error logging and graceful shutdown. Operational endpoints provide health and startup readiness checks. Business routes are not registered yet.

## Prerequisites

- Go 1.27 or later: https://go.dev/dl/
- Git

No database, Firebase credentials or third-party Go packages are needed for this scaffold. No `go.sum` is generated until external dependencies are added.

## Structure

```text
cmd/server/main.go             Startup, signals and process exit
internal/config/              Environment parsing and validation tests
internal/server/              HTTP lifecycle and shutdown tests
.env.example                  Supported settings (no secrets)
go.mod                        Go module
```

## Start locally

From the repository directory:

```powershell
go run ./cmd/server
```

Defaults bind to `127.0.0.1:8080`. The startup log confirms the address. In another terminal:

```powershell
curl.exe -i http://127.0.0.1:8080/health
curl.exe -i http://127.0.0.1:8080/ready
```

Both endpoints return **200 OK** with Content-Type application/json. /health returns {"status":"ok"} and /ready returns {"status":"ready"}. Readiness currently covers application startup only; it does not verify database or Firebase connectivity. Unknown routes, including /, return **404 Not Found**. Press **Ctrl+C** in the server terminal to drain active requests and stop. Linux/container deployments also support SIGTERM.

Build a standalone executable:

```powershell
go build -o ./bin/hostelhive-server.exe ./cmd/server
./bin/hostelhive-server.exe
```

On Linux/macOS use `go build -o ./bin/hostelhive-server ./cmd/server` and run `./bin/hostelhive-server`.

## Configuration

The application reads **process environment variables**, not `.env` files automatically. `.env.example` is documentation for manual configuration or future Docker Compose integration. Never commit real credentials.

| Variable | Default | Valid values |
|---|---|---|
| APP_ENV | development | development, test, production |
| HTTP_ADDR | 127.0.0.1:8080 | IP/localhost/empty host plus numeric port 1-65535; IPv6 uses brackets |
| HTTP_READ_HEADER_TIMEOUT | 5s | Positive Go duration |
| HTTP_READ_TIMEOUT | 15s | Positive Go duration |
| HTTP_WRITE_TIMEOUT | 15s | Positive Go duration |
| HTTP_IDLE_TIMEOUT | 60s | Positive Go duration |
| HTTP_SHUTDOWN_TIMEOUT | 10s | Positive Go duration |

Unset variables use defaults. Explicit empty/invalid variables cause an error identifying the setting and a nonzero exit before the server listens.

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

Database/migrations (including database readiness checks), Firebase verification and authorization, scanner contracts, business modules, Docker infrastructure and CI are separate tickets. Add packages when their functionality is implemented instead of creating empty domain directories.

When using the port override above, check http://127.0.0.1:18080/health and http://127.0.0.1:18080/ready.
