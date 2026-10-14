# Realtime attendance — issue #60

PostgreSQL remains authoritative. Firebase Realtime Database distributes only
`hostelhive/attendance`: version, changed_at/published_at in epoch milliseconds,
and total/IN/OUT/UNKNOWN counts using #57's eligibility rules. A changed version
also signals clients to refetch the student list through the authorized API.
No student identities, names, credentials, QR tokens, guardian details or scan
history are published. This is recorded presence, not proof of physical passage.

## Delivery contract

Migration 14 adds a coalescing transactional outbox. Database triggers on presence,
profiles and accounts queue the latest complete snapshot in the same transaction
as a mutation, including CSV imports, archival, role changes and deactivation.
A rollback rolls back its publication intent. Intermediate snapshots may coalesce;
the authoritative attendance events are retained, and RTDB is not an event archive.

The worker runs every two seconds; five seconds is a provisional healthy-service
freshness target, not a guaranteed SLA. It claims a five-second lease and releases
the database transaction before contacting Firebase. Failed writes remain pending,
with exponential backoff capped at 30 seconds. Process restarts retry durable work.
Acknowledgements for an older snapshot cannot clear newer pending work. Firebase
conditional transactions reject version regression and tolerate repeated writes.
Every 60 seconds, a new full snapshot repairs projection drift and supplies a
freshness heartbeat. Versions are safe integer millisecond revisions with a
monotonic database increment. Keep machine/database clocks synchronized.

Use a separate database per environment. Do not restore an older application
database into an RTDB namespace carrying future/higher revisions without a
controlled reconciliation/reset plan. A remote success followed by a local crash
is safe to retry. The worker does not hold the publication row during network calls.

## Staff access and endpoints

- `POST /api/v1/attendance/realtime/session`: active Admin/Warden only. No body
  or query. Publishes a five-minute grant for the caller's verified Firebase UID,
  returning database_url, path, expires_at and renew_after_seconds (120).
  Authenticate the Firebase client with the same account. A caller cannot select
  another UID. Failed grant publication returns 503 and remains queued for retry.
- `GET /api/v1/attendance/realtime/status`: active Admin/Warden only. No body
  or query. Shows enabled, pending, version, published_version, changed_at,
  published_at, attempts and pending_grants. Disabled realtime still exposes
  its queue status; session requests return 503.

Backend-owned `hostelhive/staffAccess/<uid>` grants contain version and expires_at.
Clients cannot read grants or write any shared path. Default-deny rules allow
only granted callers to read the attendance node. No token role claims are trusted
for direct RTDB authorization. Account role removal/deactivation queues a versioned
zero-expiry tombstone in the account transaction, without deleting history.
Reactivation never automatically restores a grant; request a new session.

The worker actively writes expiry tombstones so established listeners are
cancelled, rather than assuming wall-clock expiry schedules cancellation.
Grant deletion/revocation failure is observable in pending_grants and logs and is
retried. Local API denial is immediate; cross-service RTDB revocation is eventually
consistent. Shared publication pauses while a known invalid grant awaits its
cancellation write, including during retry backoff. Existing cached data cannot
be remotely erased from a client.
Cancellation may be delayed during a Firebase outage. New reads after grant
expiry deny access. Clock expiry alone does not cancel established listeners;
even a shared projection update did not cancel one in the local emulator test.
Active worker tombstones are required, and backend outages can delay cancellation.
Workers hold the users authorization lock during bounded grant writes to prevent
an older authorized grant from being published after a committed downgrade.
This can briefly delay account mutations during a slow provider request.

Frontend integration is a later task: renew grants every 120 seconds while active,
listen to the returned path, show freshness/reconnect state, and refetch authorized
API data on version change/reconnect. Expired/revoked grants require a new session;
do not silently fall back to unauthenticated reads. Never edit business data in RTDB.

## Create Firebase RTDB before live verification

1. In Firebase Console select `hostel-hive-152ef`, then Build > Realtime Database.
2. Create a database in the team-agreed region and choose **locked mode**.
3. Copy the database root HTTPS URL; this differs from the web API key and UID.
4. In its Rules tab, replace rules with `firebase/database.rules.json` and publish.
   Preserve rules for other existing applications if sharing a database; these
   rules intentionally deny all unrelated paths. Use a dedicated database when unsure.
5. Confirm the backend service account has RTDB permission. Continue using the
   existing credentials outside the repository. Never enable public test-mode rules.

Official references: [Admin writes and transactions](https://firebase.google.com/docs/database/admin/save-data),
[security rules](https://firebase.google.com/docs/database/security),
[emulator testing](https://firebase.google.com/docs/emulator-suite/connect_rtdb).

## Startup — Terminal 1

Stop the old backend with Ctrl+C. Keep PostgreSQL running. Apply **up**, never down
on your development database. Copy your actual database root URL when prompted.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'
$env:FIREBASE_PROJECT_ID = 'hostel-hive-152ef'
$env:GOOGLE_APPLICATION_CREDENTIALS = 'C:\Users\TASHEEN\Desktop\Projects\private\hostelhive-service-account.json'
$env:HTTP_ADDR = '127.0.0.1:18080'
$env:R2_ENABLED = 'false'
$env:ATTENDANCE_COOLDOWN = '10s'
$env:RTDB_ENABLED = 'true'
$env:FIREBASE_DATABASE_URL = Read-Host 'Firebase RTDB root HTTPS URL'
Remove-Item Env:FIREBASE_DATABASE_EMULATOR_HOST -ErrorAction SilentlyContinue
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

Expected version 14 clean. Leave this terminal running. Before RTDB is created,
set RTDB_ENABLED=false instead and omit the URL prompt: scans/APIs still work and
the publication queue accumulates the latest state for later delivery.

## Live API and console checks — Terminal 2

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'
$adminToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders

$session = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/attendance/realtime/session" -Headers $adminHeaders
$session | ConvertTo-Json
$status = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/realtime/status" -Headers $adminHeaders
$status | ConvertTo-Json
$summary = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $adminHeaders
$summary | ConvertTo-Json
```

In Firebase Console Data, inspect hostelhive/attendance and compare summary counts
to the API while there are no concurrent changes. Console access is privileged:
it does not prove client rules. Inspect the caller's grant expiry (not credentials).
Use #55's dedicated scanner fixture for accepted IN/OUT scans, then confirm the
version and counts update within the provisional target. Ignored duplicate scans
do not change presence. Test student archival/deactivation with dedicated fixtures:
the projection must exclude them while preserving PostgreSQL attendance history.

Repeat the session call with an active Warden token: 200. With a Student token: 403.
Without a token: 401. A body `{}` or query `?uid=other` returns 400. With
RTDB_ENABLED=false, a valid staff session returns 503 and status.enabled is false.

For an outage test, stop the backend, temporarily set FIREBASE_DATABASE_URL to a
nonexistent database in your own Firebase project, rebuild/restart, then perform
one dedicated scan. The scan must commit while status.pending stays true. Restore
the correct URL and restart: the latest counts must recover without rescanning.
Do not change IAM permissions or production rules just to simulate an outage.

For staff revocation, acquire a dedicated Warden's session, deactivate that account
with Admin, then inspect pending_grants and the zero-expiry grant. Test a real
client listener for cancellation and fresh read denial. The local emulator suite
also verifies expiry and established-listener cancellation. Do not deactivate the
last Admin. Record actual live results and the CI URL on #60; do not claim live
Firebase verification solely from fakes or privileged Console inspection.

## Direct Firebase reads and live listeners

The helper `scripts/test-rtdb-live.mjs` uses Node 22+ and only process environment
tokens. It does not print tokens, authenticated URLs or provider error bodies.
Syntax and all four modes were checked with offline HTTP/SSE fakes; those are not
live Firebase evidence. Run the following in Terminal 2 after startup and obtaining
the Admin session above:

```powershell
$env:HH_TEST_API_URL = $baseURL
$env:HH_TEST_DATABASE_URL = $session.database_url
$env:HH_TEST_TOKEN = $adminToken
node .\scripts\test-rtdb-live.mjs verify
if ($LASTEXITCODE -ne 0) { throw 'Admin Firebase rules check failed.' }
```

Expected: granted read succeeds, identical-value client write is denied, broader
and anonymous reads are denied. This tests Firebase rules rather than privileged
Console access. With a fresh Student token in HH_TEST_TOKEN, run `deny`: direct
Firebase read must be denied. These rules use verified UID grants, not token roles.

For the remaining tests, sign in with an active **dedicated test Warden**:

```powershell
$wardenToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenToken" }
$wardenMe = Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $wardenHeaders
if ($wardenMe.role -ne 'warden' -or -not $wardenMe.is_active) { throw 'Use an active test Warden.' }
$wardenMe
$wardenSession = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/attendance/realtime/session" -Headers $wardenHeaders
$env:HH_TEST_TOKEN = $wardenToken
node .\scripts\test-rtdb-live.mjs verify
if ($LASTEXITCODE -ne 0) { throw 'Warden Firebase rules check failed.' }

# This mode deactivates the signed-in Warden after establishing its listener.
$env:HH_TEST_ADMIN_TOKEN = $adminToken
$env:HH_TEST_WARDEN_USER_ID = $wardenMe.user_id
node .\scripts\test-rtdb-live.mjs revoke
if ($LASTEXITCODE -ne 0) { throw 'Live listener revocation failed.' }

# Restore the dedicated test account, sign in again and acquire a new grant.
Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/users/$($wardenMe.user_id)/activate" -Headers $adminHeaders
$wardenToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenToken" }
$wardenSession = Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/attendance/realtime/session" -Headers $wardenHeaders
$env:HH_TEST_TOKEN = $wardenToken
$env:HH_TEST_EXPIRES_AT = $wardenSession.expires_at
node .\scripts\test-rtdb-live.mjs expiry
if ($LASTEXITCODE -ne 0) { throw 'Live listener expiry failed.' }

Remove-Item Env:HH_TEST_TOKEN,Env:HH_TEST_ADMIN_TOKEN,Env:HH_TEST_WARDEN_USER_ID,Env:HH_TEST_EXPIRES_AT -ErrorAction SilentlyContinue
```

The expiry test waits approximately five minutes without renewing the grant.
It must receive cancellation and deny a fresh read. Keep the backend running.
If account activation reports pending provider revocation, wait for that workflow
to complete and retry activation. Never use the last Admin as a revocation fixture.

## Automated checks and PR

Local verification on 2026-10-11: formatting, dependencies, vet, build and the
full Go test suite passed. All nine required database packages passed without
skips. Coverage was 85.5% overall (2726/3189), 84.9% attendance (724/853) and
84.0% Firebase platform code (79/94). Complete migration rollback/reapply through
14 and SQL schema checks passed in a disposable database. The final focused
integration check also passed accepted IN/OUT publication and ignored-scan
behavior. All four emulator rules/listener tests passed. Local Go tests did not
use the race detector; CI race results and live provider verification remain pending.

```powershell
.\scripts\test-backend-coverage.ps1
Push-Location firebase
npm.cmd ci --ignore-scripts --no-audit --no-fund
npm.cmd test
Pop-Location
git status
git diff --check
```

Rules tests use only the demo-hostelhive database emulator on loopback port 19000,
with pinned Node dependencies. Node 22 and Java 21 are the CI baseline. They need
no service-account key or live Firebase database. Production Auth emulator routing
remains rejected. The existing Go CI runs all new tests with PostgreSQL and race
detection; the new CI job checks RTDB rules and listener behavior.

Review changed files before staging. Commit title:
`feat(attendance): publish realtime presence updates`, body `Refs #60`.
Push `feature/60-attendance-realtime`. PR title:
`[DDP-#60] feat(attendance): publish realtime presence updates`.
Do not close #60 until its live acceptance and CI/review evidence are recorded.

After live checks, use this selected commit set (no keys, generated artifacts or
node_modules):

```powershell
git diff
git add .env.example .github/workflows/backend-ci.yml .gitignore CHANGELOG.md README.md docs/attendance-realtime.md docs/architecture.md docs/backend-ci.md docs/module-layout.md internal/app/app.go internal/config/config.go internal/config/realtime.go internal/config/realtime_test.go internal/modules/attendance/domain/attendance_realtime.go internal/modules/attendance/handler/attendance_realtime_handler.go internal/modules/attendance/repository/attendance_realtime_repository.go internal/modules/attendance/service/attendance_realtime_service.go internal/modules/attendance/attendance_routes.go internal/platform/firebase/realtime.go internal/platform/firebase/realtime_test.go internal/workers/attendance_realtime.go migrations/000014_attendance_realtime.up.sql migrations/000014_attendance_realtime.down.sql tests/integration/attendance/realtime_test.go firebase/database.rules.json firebase/firebase.json firebase/package.json firebase/package-lock.json firebase/rules.test.mjs
git add scripts/test-rtdb-live.mjs
git diff --cached --check
git diff --cached --stat
git commit -m "feat(attendance): publish realtime presence updates" -m "Refs #60"
git push -u origin feature/60-attendance-realtime
```

#57 was merged into main rather than develop; #60 starts from that merged baseline.
Coordinate synchronizing develop with main before selecting develop as PR base;
otherwise the comparison also includes #57. No develop/main synchronization or
release merge was performed as part of this implementation.
