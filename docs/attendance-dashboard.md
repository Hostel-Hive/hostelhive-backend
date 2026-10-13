# Attendance dashboard — issue #57

## API and scope

Active Admins and Wardens can read both endpoints. Scanner credentials and other
human roles have no dashboard access. Authority is checked again inside the read
transaction. Every response uses Cache-Control: no-store.

| Endpoint | Query | Response |
|---|---|---|
| GET /api/v1/attendance/students | optional status, q, limit, offset | items, total, limit, offset, has_more |
| GET /api/v1/attendance/summary | optional q | total, in, out, unknown |

The list defaults to all statuses, limit 20, offset 0. Status must be exactly IN,
OUT or UNKNOWN; omit it for all. Limit is 1–100; offset is 0–1,000,000. Search `q`
is a trimmed, case-insensitive literal substring of full_name or index_no, at most
200 Unicode characters without internal control characters. SQL wildcard `%`,
`_`, and escape `!` are literal text. Summary accepts the same search, but no status
or pagination parameters. Empty search means all eligible students. Malformed,
unknown and repeated parameters or GET request bodies return 400 invalid_input.

Each item contains student_id, index_no, full_name, status and updated_at. UNKNOWN
means no accepted movement has established presence; updated_at is null. Students
without generated QR or bed allocation can still appear. There are no credentials,
QR identifiers, guardian details or contact numbers in dashboard responses.

Eligibility is identical for lists and counts: unarchived profile, active linked
account, current Student role. Stale presence belonging to inactive, archived or
changed-role accounts is excluded, without deleting attendance history. Counts
come from student_presence, never from the number or most recent outcome of scan
attempts. Each summary satisfies total = in + out + unknown. List total reflects
its search and status filter, even when offset produces an empty page. Items sort
by lower(full_name), lower(index_no), student_id for deterministic pagination.

A page's rows and total share a repeatable-read snapshot. Separate HTTP requests
may see different snapshots as scans or account changes commit. These are recorded
presence counts, not proof of physical occupancy. This ticket does not publish
Firebase RTDB projections, implement a UI, or resolve leave/presence policy.
The per-student historical presence endpoint from #55 remains unchanged; it can
show saved state for archived profiles, whereas dashboard eligibility excludes them.

No new migration is introduced. Apply all existing migrations through version 13.

## Startup — Terminal 1

Stop the old server with Ctrl+C. Docker Desktop must be running; start the existing
database container if it is stopped. Environment variables belong to each terminal.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
docker start hostelhive-postgres
if ($LASTEXITCODE -ne 0) { throw 'Database startup failed.' }
docker exec hostelhive-postgres pg_isready -h 127.0.0.1 -U hostelhive -d hostelhive
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL is still starting. Wait, then rerun this startup block.' }

$env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'
$env:FIREBASE_PROJECT_ID = 'hostel-hive-152ef'
$env:GOOGLE_APPLICATION_CREDENTIALS = 'C:\Users\TASHEEN\Desktop\Projects\private\hostelhive-service-account.json'
$env:HTTP_ADDR = '127.0.0.1:18080'
$env:R2_ENABLED = 'false'
$env:ATTENDANCE_COOLDOWN = '10s'
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

Expected: version 13 clean (up may report no change), server listening at 18080.
Leave this terminal running. Never run migrations down on the development database.

## Live read verification — Terminal 2

Use Admin credentials for the token helper: Firebase web API key, email, password.
The dashboard checks below do not create accounts or change attendance.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'
$adminToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders

$summary = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $adminHeaders
$summary | ConvertTo-Json
if ($summary.total -ne ($summary.in + $summary.out + $summary.unknown)) { throw 'FAIL: count invariant.' }
$list = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?limit=100" -Headers $adminHeaders
$list | ConvertTo-Json -Depth 5
foreach ($status in @('IN','OUT','UNKNOWN')) {
    $page = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?status=$status&limit=100" -Headers $adminHeaders
    if (@($page.items | Where-Object { $_.status -ne $status }).Count -ne 0) { throw "FAIL: $status filter." }
    Write-Host "$status matching total: $($page.total)"
}

# Stable paging checks assume no simultaneous scans or profile edits during this block.
$firstPage = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?limit=1" -Headers $adminHeaders
$secondPage = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?limit=1&offset=1" -Headers $adminHeaders
if ($firstPage.items.Count -eq 1 -and $secondPage.items.Count -eq 1 -and
    $firstPage.items[0].student_id -eq $secondPage.items[0].student_id) { throw 'FAIL: repeated page item.' }
if ($firstPage.total -gt 1 -and -not $firstPage.has_more) { throw 'FAIL: has_more.' }

# Pick a unique part of an existing student's name or index number.
$search = Read-Host 'Student name/index search'
$escapedSearch = [Uri]::EscapeDataString($search)
$matches = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?q=$escapedSearch&limit=100" -Headers $adminHeaders
$searchSummary = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary?q=$escapedSearch" -Headers $adminHeaders
$matches | ConvertTo-Json -Depth 5
$searchSummary | ConvertTo-Json
if ($matches.total -ne $searchSummary.total) { throw 'FAIL: search totals (check concurrent account edits).' }

$noMatch = [Uri]::EscapeDataString('NO-MATCH-57-' + [Guid]::NewGuid().ToString('N'))
$empty = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?q=$noMatch" -Headers $adminHeaders
$emptySummary = Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary?q=$noMatch" -Headers $adminHeaders
if ($empty.total -ne 0 -or $empty.items.Count -ne 0 -or $emptySummary.total -ne 0) { throw 'FAIL: empty search.' }
Write-Host 'PASS: presence lists, paging, search and summary.'
```

UNKNOWN may be zero if all eligible students have already scanned. An empty list
for a status is valid. To test known IN/OUT students, use #55's scan guide on a
dedicated active test profile, then reread the dashboard. Automated disposable
fixtures exercise all statuses, inactive/archived/role-changed exclusions and
concurrent snapshot behaviour; do not edit production-like data just to force counts.

## Permissions and input errors

```powershell
function Assert-HttpStatus {
    param([int]$Expected, [scriptblock]$Request)
    try {
        & $Request | Out-Null
        throw "FAIL: expected HTTP $Expected, but request succeeded."
    } catch {
        if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne $Expected) { throw }
        Write-Host "PASS: HTTP $Expected."
    }
}

Assert-HttpStatus 401 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" }
Assert-HttpStatus 400 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?status=SIDE" -Headers $adminHeaders }
Assert-HttpStatus 400 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?limit=0" -Headers $adminHeaders }
Assert-HttpStatus 400 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?offset=-1" -Headers $adminHeaders }
Assert-HttpStatus 400 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?status=IN&status=OUT" -Headers $adminHeaders }
Assert-HttpStatus 400 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary?status=IN" -Headers $adminHeaders }

# Sign in as Warden: both reads must succeed.
$wardenToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $wardenHeaders
Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $wardenHeaders
Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students?limit=20" -Headers $wardenHeaders | ConvertTo-Json -Depth 5

# Sign in as Student: both reads must be forbidden.
$studentToken = .\scripts\get-firebase-token.ps1
$studentHeaders = @{ Authorization = "Bearer $studentToken" }
Assert-HttpStatus 403 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/summary" -Headers $studentHeaders }
Assert-HttpStatus 403 { Invoke-RestMethod -Uri "$baseURL/api/v1/attendance/students" -Headers $studentHeaders }
Write-Host 'PASS: dashboard permissions and invalid inputs.'
```

An expired Firebase token returns 401, not the 403 role check. Obtain a fresh token
if needed. Scanner and Security Staff/Sub Warden exclusion are also automated.

## Compare with PostgreSQL — pgAdmin Query Tool

Execute each query separately. These are SELECTs; no scanner needs to be active
to read previously recorded presence. API reads still require an active staff account.
Run without concurrent data changes when comparing separate API/SQL requests.

```sql
SELECT count(*) AS total,
       count(*) FILTER (WHERE p.status='IN') AS in_count,
       count(*) FILTER (WHERE p.status='OUT') AS out_count,
       count(*) FILTER (WHERE p.student_id IS NULL) AS unknown_count
FROM hostelhive.students s
JOIN hostelhive.users u USING(user_id)
LEFT JOIN hostelhive.student_presence p USING(student_id)
WHERE s.deleted_at IS NULL AND u.is_active AND u.role='student';

SELECT s.student_id,s.index_no,s.full_name,
       COALESCE(p.status,'UNKNOWN') AS status,p.updated_at
FROM hostelhive.students s
JOIN hostelhive.users u USING(user_id)
LEFT JOIN hostelhive.student_presence p USING(student_id)
WHERE s.deleted_at IS NULL AND u.is_active AND u.role='student'
ORDER BY lower(s.full_name),lower(s.index_no),s.student_id;
```

Record actual live results and a CI URL on #57 before closing it. Do not treat
this read API as Firebase realtime delivery or evidence of physical passage.

## Automated checks, commit and PR

Local verification on 2026-10-11 passed formatting, dependency verification,
static checks, build, coverage-report checks and the full Go test suite. All nine
required PostgreSQL integration packages passed without skips, including the new
dashboard test for filtering, permissions and concurrent-read consistency.
Statement coverage was 87.0% overall (2574/2959) and 85.9% for attendance
(602/701). The disposable database applied migrations 1–13.

This local run did not use the race detector. CI race checks, live Firebase/API
verification and staging HTTPS verification remain separate evidence.

```powershell
.\scripts\test-backend-coverage.ps1
git status
git diff --check
git diff
git add internal/app/app.go internal/modules/attendance/domain/attendance_dashboard.go internal/modules/attendance/dto/attendance_dashboard_requests.go internal/modules/attendance/handler/attendance_dashboard_handler.go internal/modules/attendance/handler/attendance_dashboard_handler_test.go internal/modules/attendance/service/attendance_dashboard_service.go internal/modules/attendance/service/attendance_dashboard_service_test.go internal/modules/attendance/service/attendance_dependencies.go internal/modules/attendance/repository/attendance_dashboard_repository.go internal/modules/attendance/attendance_routes.go tests/integration/attendance/dashboard_test.go docs/attendance-dashboard.md docs/architecture.md docs/module-layout.md docs/backend-ci.md README.md CHANGELOG.md
git diff --cached --check
git diff --cached --stat
git commit -m "feat(attendance): add presence lists and occupancy counts" -m "Refs #57"
git push -u origin feature/57-attendance-dashboard
```

PR base develop; compare feature/57-attendance-dashboard. Title:
`[DDP-#57] feat(attendance): add presence lists and occupancy counts`.
Describe recorded-presence semantics, identical eligibility, UNKNOWN handling,
search/paging and snapshot consistency. Include actual tests, live results and
`Refs #57`. Link the issue in Development, wait for CI/race tests and review, then
merge. Firebase RTDB projection remains a subsequent ticket; staging is infra #1.

After merge and remote branch deletion:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/57-attendance-dashboard
```
