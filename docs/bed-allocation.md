# Bed allocation — issue #46

FR019–FR020 extend the inventory foundation from #33. Tasheen confirmed Admin
and Warden management access for #46. Other roles, including Sub-Warden and
Security Staff, are denied. Admins now also have inventory listing access to
select beds; identity/account management stays Admin-only.

## Agreed allocation contract (D09)

- Use the students table's `student_id`, never a user ID or Firebase UID.
- Use the globally unique `bed_id`; bed numbers may repeat across rooms.
- PostgreSQL partial unique indexes enforce one active allocation per student
  and per bed. Active means `ended_at IS NULL`.
- Only non-archived profiles linked to active student accounts can be assigned
  or transferred. Missing IDs return 404; ineligible students return 409.
- Transfer creates a new allocation and closes the old allocation in one
  transaction. An occupied/missing target rolls back everything. Same-bed
  transfers and requests against ended allocations return 409; this API does
  not perform bed swaps or automatically evict another student.
- Revocation records `ended_at`, `ended_by` and `end_reason=revoked`. A repeated
  revoke returns the original ended record without changing its timestamp or
  actor. Revoking a transferred allocation returns 409 and cannot end its successor.
- Revocation remains allowed for inactive or archived students. Account
  deactivation/profile archival does not automatically release their bed; revoke
  explicitly. Account reactivation does not create a new allocation.
- Gender zoning and maintenance-based exclusions have no current schema fields
  or agreed policy; they are not enforced by this ticket. Add a separately agreed
  requirement before introducing such restrictions.
- Assignment history is retained. New API rows record `started_by`; transfers
  record `end_reason=transferred`. Audit actors are local user IDs derived from
  verified identity, never accepted in request bodies. Pre-existing rows may
  have null audit fields. Migration 10 adds audit columns without deleting history.
- The repository rechecks staff authority and student eligibility inside the
  transaction. A shared users-table lock coordinates with existing account and
  student write policies. Student locks serialize changes per student, and bed
  locks use stable ordering; unique indexes are the final concurrency guard.
- GET history is staff-only, with a stable newest-first order and a count/page
  from one database statement snapshot. No Firebase credentials or guardian data
  appear in allocation responses. Existing inventory counts derive from active rows.

## API contract

| Method / path | Body / query | Success |
|---|---|---|
| GET `/api/v1/allocations` | Optional `student_id`, `bed_id`, `active=true/false`, `limit=1..100`, `offset>=0` | 200 `{items,total,limit,offset}` |
| POST `/api/v1/allocations` | JSON `{student_id,bed_id}` | 201 allocation |
| POST `/api/v1/allocations/{allocationID}/transfer` | JSON `{bed_id}` | 200 new allocation |
| POST `/api/v1/allocations/{allocationID}/revoke` | No body | 200 ended allocation |

Omitting `active` returns both current and ended rows. Empty pages return `items: []`
and still include the filtered total. Each allocation contains `allocation_id`,
`student_id`, `bed_id`, `started_at`, nullable `ended_at`, `started_by`, `ended_by`
and `end_reason`. IDs are UUID strings and timestamps are UTC/RFC3339.

Missing/invalid token: 401. Denied or inactive actor: 403. Invalid IDs, unknown or
repeated query parameters, invalid JSON or extra fields: 400 `invalid_input`.
Wrong mutation Content-Type: 415 `unsupported_media_type`; JSON over 4096 bytes:
413 `payload_too_large`. Missing resource: 404 `not_found`. Occupied bed,
already allocated student, stale allocation or same-bed transfer: 409
`allocation_conflict`. Ineligible profile/account: 409 `active_student_required`.
Database failures: 503 `allocations_unavailable`; internal SQL is never returned.
Successful responses and safe errors use `Cache-Control: no-store`.

No idempotency key is required for these routes. Duplicate assignment requests
return 409. If an assignment/transfer response is lost, query the student's
allocation history before retrying; a retry of an old transferred ID returns 409.

## 1. Automated verification

In the backend folder, Docker Desktop running and Go/Python available:

```powershell
git branch --show-current
.\scripts\test-backend-coverage.ps1
```

Expected branch: `feature/46-bed-allocation`. This creates and deletes its own
disposable PostgreSQL instance; it never uses your developer DATABASE_URL.
It checks formatting, dependencies, vet/build, all unit and database suites and
writes `artifacts/coverage.md`, `coverage.html` and `tests.jsonl`.
CI also runs the race detector and fresh-schema/full-rollback/reapply checks.
The allocation database package is required in the evidence report: skipped
tests cannot count as successful database verification.

Automated coverage includes full HTTP/service/database assign-transfer-revoke,
Admin/Warden access, denied roles, malformed/oversized requests, history and
empty pagination, conflicting transfers preserving the original allocation,
inventory availability, inactive/archived accounts, actor role changes and
competing requests for a single student or bed. Firebase is faked in these tests;
live provider/staging evidence remains separate.

## 2. Server terminal

Stop an older backend with Ctrl+C first. Set the known local database settings;
change credentials/port if your Docker database was configured differently.

```powershell
$env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'
$env:FIREBASE_PROJECT_ID = 'hostel-hive-152ef'
$env:GOOGLE_APPLICATION_CREDENTIALS = 'C:\Users\TASHEEN\Desktop\Projects\private\hostelhive-service-account.json'
$env:HTTP_ADDR = '127.0.0.1:18080'
$env:R2_ENABLED = 'false'
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o .\bin\hostelhive-server.exe .\cmd\server
.\bin\hostelhive-server.exe
```

Expected migration version: 10, clean. Leave this terminal running. R2 is disabled
only for this allocation check; use your normal R2 settings when testing images.

## 3. Create a dedicated local inventory fixture

Run the entire block in **pgAdmin Query Tool**, connected to the same Docker
database at port 15433. SQL is not a PowerShell command. Execute all statements,
not just the final COMMIT, and inspect both Messages and Data Output.

```sql
BEGIN;
INSERT INTO hostelhive.blocks(name) VALUES ('LOCAL-ALLOCATION-46') ON CONFLICT DO NOTHING;
INSERT INTO hostelhive.rooms(block_id,room_no)
SELECT block_id,'TEST-46' FROM hostelhive.blocks WHERE name='LOCAL-ALLOCATION-46'
ON CONFLICT DO NOTHING;
INSERT INTO hostelhive.beds(room_id,bed_no)
SELECT r.room_id,n.bed_no FROM hostelhive.rooms r JOIN hostelhive.blocks b USING(block_id)
CROSS JOIN (VALUES ('1'),('2')) n(bed_no)
WHERE b.name='LOCAL-ALLOCATION-46' AND r.room_no='TEST-46'
ON CONFLICT DO NOTHING;
COMMIT;
SELECT b.bed_id,b.bed_no FROM hostelhive.beds b
JOIN hostelhive.rooms r USING(room_id) JOIN hostelhive.blocks k USING(block_id)
WHERE k.name='LOCAL-ALLOCATION-46' ORDER BY b.bed_no;
```

Use a dedicated active test student profile with no active allocation. Existing
profile creation is documented in the README; this feature does not create a
Firebase account or student profile. Do not use real residents for mutation tests.

## 4. Client terminal: authenticate and select IDs

```powershell
$staffIDToken = .\scripts\get-firebase-token.ps1
$staffHeaders = @{ Authorization = "Bearer $staffIDToken" }
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/me' -Headers $staffHeaders
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/students?limit=100' -Headers $staffHeaders | ConvertTo-Json -Depth 6
$studentID = Read-Host 'Dedicated test student_id (students table UUID)'
$bed1 = Read-Host 'First fixture bed_id (UUID from pgAdmin)'
$bed2 = Read-Host 'Second fixture bed_id (UUID from pgAdmin)'
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/allocations?student_id=$studentID&active=true" -Headers $staffHeaders | ConvertTo-Json -Depth 5
```

Sign in as an active Admin or Warden. The API key prompt expects the Firebase
web API key, not a Firebase UID. The last response must have total 0. If the
profile already has an active allocation, choose another dedicated test profile.

## 5. Assign, reject a duplicate and transfer

```powershell
$assignBody = @{ student_id=$studentID; bed_id=$bed1 } | ConvertTo-Json
$assigned = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/allocations' -Headers $staffHeaders -ContentType 'application/json' -Body $assignBody
$assigned | ConvertTo-Json -Depth 5
$allocationID = $assigned.allocation_id
```

Expected: allocation created (HTTP 201), ended_at null, started_by your local user ID.
Verify that an exact duplicate returns HTTP 409:

```powershell
try {
    Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/allocations' -Headers $staffHeaders -ContentType 'application/json' -Body $assignBody -ErrorAction Stop | Out-Null
    throw 'FAIL: duplicate allocation accepted.'
} catch {
    if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne 409) { throw }
    Write-Host 'PASS: duplicate allocation rejected (409).'
}
$transferBody = @{ bed_id=$bed2 } | ConvertTo-Json
$transferred = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/allocations/$allocationID/transfer" -Headers $staffHeaders -ContentType 'application/json' -Body $transferBody
$transferred | ConvertTo-Json -Depth 5
$newAllocationID = $transferred.allocation_id
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/allocations?student_id=$studentID" -Headers $staffHeaders | ConvertTo-Json -Depth 6
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/beds?available=true' -Headers $staffHeaders | ConvertTo-Json -Depth 5
```

Expected: new allocation ID on bed2; old row ended with reason transferred;
bed1 is available and bed2 occupied. Repeat with another test student on the
occupied bed to confirm 409. Automated tests additionally prove a failed transfer
to an occupied bed leaves the original allocation active.

## 6. Revoke and verify history

```powershell
$revoked = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/allocations/$newAllocationID/revoke" -Headers $staffHeaders
$revoked | ConvertTo-Json -Depth 5
$retry = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/allocations/$newAllocationID/revoke" -Headers $staffHeaders
if ($retry.ended_at -ne $revoked.ended_at) { throw 'FAIL: revocation retry changed history.' }
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/allocations?student_id=$studentID&active=true" -Headers $staffHeaders | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/allocations?student_id=$studentID&active=false" -Headers $staffHeaders | ConvertTo-Json -Depth 6
```

Expected: active total 0, history contains both ended rows, and both fixture beds
are available. Retain history; do not delete allocations to clear occupancy.

## 7. Verify access

Repeat a GET list with a fresh token from the other permitted role (Admin/Warden):
200. A Student, Sub-Warden or Security Staff token: 403. Without a token: 401.
Never include tokens, credentials or personal student details in ticket evidence.

```powershell
curl.exe -i http://127.0.0.1:18080/api/v1/allocations
```

## 8. Commit and PR

Only commit after reviewing your diff and completing the local checks above:

```powershell
git status
git diff --check
git add internal/modules/allocation internal/modules/inventory internal/modules/identity internal/modules/student internal/modules/staff internal/app/app.go migrations/000010_allocation_audit.up.sql migrations/000010_allocation_audit.down.sql tests/integration/allocation scripts/coverage-report.py scripts/test_coverage_report.py docs/bed-allocation.md docs/hostel-inventory.md docs/architecture.md docs/module-layout.md docs/backend-ci.md README.md CHANGELOG.md
git diff --cached --check
git commit -m "feat(allocation): assign, transfer and revoke student beds" -m "Refs #46"
git push -u origin feature/46-bed-allocation
```

Create PR base **develop**, compare **feature/46-bed-allocation**. Title:
`[DDP-#46] feat(allocation): assign, transfer and revoke student bed allocations`.
Description:

```markdown
Add Admin/Warden bed assignment, atomic transfers and revocation with preserved history and actor auditing. Extend inventory selection to Admins.

Organize module records, errors and service ports by responsibility. Split inventory resource queries and staff self-service into separate files; preserve their API contracts and transaction guards.

Refs #46. Requirements: FR019–FR020; D09. Depends on #33.
Validation: [actual local verification results and CI link].
Staging HTTPS verification remains pending under Hostel-Hive/hostelhive-infra#1.
```

Link #46 under the PR/issue Development section. Wait for CI and teammate review;
do not claim live or staging checks that have not been executed. Comment on #46
with actual results, PR URL and evidence links. Agree any staging exception with
the team before marking Done under the existing Definition of Done.

After approved merge into develop, delete the remote feature branch, then:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/46-bed-allocation
```

## Recorded automated results — 10 October 2026

Formatting, dependency verification, vet, build and all Go/database tests passed.
All eight required PostgreSQL integration packages passed with no skipped tests.
Allocation statement coverage: 271/291 (93.1%); all application code: 1751/2011
(87.1%). Migration 10 passed Linux fresh installation, full rollback and reapply.
These are local non-race results with fake Firebase verification; live checks,
PR CI race results, teammate review and staging verification remain pending.
