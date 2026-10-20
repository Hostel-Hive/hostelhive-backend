# Local development configuration

Configure once: copy `.env.example` to `.env.local` and edit the copy. Keep the
Firebase service-account JSON outside the repository and put only its absolute
path in `GOOGLE_APPLICATION_CREDENTIALS`. Forward slashes work on Windows; paths
containing spaces may use matching single or double quotes. Do not add Firebase
login passwords, tokens or the JSON private key itself to this file.

The launcher supports Windows PowerShell 5.1 and PowerShell 7. Run from the repo:

```powershell
.\scripts\dev.ps1 -Check
.\scripts\dev.ps1
```

`-Check` validates the local file, required values and credential-file existence;
it does not contact Firebase, Docker or the database. The Go server still performs
its complete configuration validation at startup. Default example port: 18080;
Swagger: `http://127.0.0.1:18080/docs/`. Stop the server with Ctrl+C.

Start Docker Desktop first. The launcher starts an **existing** PostgreSQL
container named `hostelhive-postgres`, expecting localhost port15433. It never
creates, deletes or recreates a database/container or changes its password.
`pg_isready` checks readiness, not password validity or schema freshness. Check
`/ready` for API-to-database connectivity and apply pending migrations explicitly:

```powershell
.\scripts\dev.ps1 -Migrate
```

This applies `scripts/migrate.ps1 up` before starting Go and requires the pinned
migration CLI to be installed with `scripts/install-migrate.ps1`. Normal startup
does not migrate automatically. Back up valuable data before migration.

For a separately managed or hosted database, set its URL in the local file and
skip Docker startup; use appropriate TLS settings for the hosted database:

```powershell
.\scripts\dev.ps1 -SkipDatabase
```

Other options: `-EnvFile C:/private/hostelhive.env` and
`-DatabaseContainer another-local-postgres`. Relative EnvFile paths resolve from
the repository, not the caller's directory. No automatic `.env` fallback occurs.

Files contain literal `KEY=value` lines, optional matching quotes, blank lines
and full-line `#` comments. Values may contain `=`, `#`, backslashes and dollar
signs. No interpolation, escape expansion, inline comments, shell execution,
multiline values or `export` syntax is supported. URL-encode special characters
inside database URL credentials. Duplicate keys, malformed lines and settings
not listed in `.env.example` are rejected without displaying their values.

Values explicitly listed in the file override process variables for this launch;
omitted values retain the existing process environment or Go defaults. Original
process variables and working directory are restored when the launcher exits,
including failure paths. The launcher is only for development. On Heroku or
other hosted services, run the compiled backend with platform-managed environment
variables; no local dotenv file is read by the Go executable.

Optional R2, RTDB, FCM and invitation settings are listed in the example. Enable
them only when the corresponding integrations are configured. Running the backend
can process queued jobs against its configured external services. This launcher
does not provision or deploy cloud services.
