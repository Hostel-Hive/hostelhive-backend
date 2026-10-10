# Inventory management — issue #48

Tasheen approved Admin-only creation and renaming of blocks, rooms and beds.
This is an agreed extension to FR018's listing scope, based on #33 and #46.
Admin/Warden viewing and allocation policies are unchanged. Deletion, archiving,
relocation, maintenance state and gender restrictions are outside this ticket.

## Contract

All routes require Firebase authentication and an active current local Admin.
Repository transactions recheck that authority while holding a users-table SHARE
lock, coordinating with account role/status writes. Client-supplied actor fields
are rejected. Existing case-insensitive unique indexes and foreign keys are the
final concurrency/integrity guards. No new migration is required; version 10
remains the current baseline, and old migration files are not modified.

| Method/path | JSON body | Success |
| --- | --- | --- |
| POST `/api/v1/blocks` | `{ "name": "Block A" }` | 201 block |
| PUT `/api/v1/blocks/{blockID}` | `{ "name": "Block B" }` | 200 block |
| POST `/api/v1/rooms` | `{ "block_id": "UUID", "room_no": "101" }` | 201 room |
| PUT `/api/v1/rooms/{roomID}` | `{ "room_no": "102" }` | 200 room |
| POST `/api/v1/beds` | `{ "room_id": "UUID", "bed_no": "1" }` | 201 bed |
| PUT `/api/v1/beds/{bedID}` | `{ "bed_no": "2" }` | 200 bed |

Responses use the same block/room/bed records as listing endpoints. Creation
returns zero counts for a new empty block/room and `available: true` for a new bed.
Update responses include current counts/availability. Counts describe the response
snapshot; concurrent later allocations may change them.

POST does not require an idempotency key. If a creation response is lost, check
the existing inventory listing before retrying; a repeated name/number returns 409.

Names and numbers are trimmed, nonempty, valid UTF-8 without control characters.
Block names allow at most 120 characters; room/bed numbers at most 32. UUIDs are
validated and canonicalized. Duplicate block names are global; room numbers are
unique within a block, bed numbers within a room. Matching ignores case.

Updates change labels only. A room's `block_id` and a bed's `room_id` are immutable
through these endpoints; sending them in an update is rejected. Resource IDs,
creation timestamps and allocation records are preserved. Renaming an occupied
bed is permitted and leaves it occupied. Historical APIs display current inventory
labels when joined with inventory; historical label snapshots are not introduced.
Repeating a PUT with the current label succeeds; duplicate POSTs return 409.

| Failure | HTTP / error |
| --- | --- |
| Missing/invalid token | 401 `unauthorized` |
| Non-Admin/inactive actor or authority changed before write | 403 `forbidden` |
| Invalid UUID, missing/blank/long fields, control characters, query parameters, unknown JSON fields or trailing document | 400 `invalid_input` |
| Missing target or parent | 404 `not_found` |
| Case-insensitive name/number conflict | 409 `inventory_conflict` |
| Non-JSON Content-Type | 415 `unsupported_media_type` |
| Body larger than 4096 bytes | 413 `payload_too_large` |
| Database failure | 503 `inventory_unavailable` |

Responses use `Cache-Control: no-store`; database errors are not exposed.

## 1. Automated verification

From the backend directory, Docker Desktop running:

```powershell
git branch --show-current
.\scripts\test-backend-coverage.ps1
```

Expected branch: `feature/48-inventory-management`. The script creates/migrates
its own disposable PostgreSQL database, runs formatting, dependency verification,
vet, build and the full unit/integration suites, then removes that container.
It writes `artifacts/coverage.md`, `coverage.html`, `coverage.out` and `tests.jsonl`.
Inventory is included in the module summary and is a required integration package.
CI additionally runs database race tests and migration rollback/reapply checks.

## 2. Server terminal

Stop an older server with Ctrl+C. Use your existing local database settings:

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
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

Expected migration version 10, clean. Leave this terminal running. Use your normal
R2 settings separately when testing images.

## 3. Client terminal: sign in and create a fixture through the API

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$adminIDToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminIDToken" }
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/me' -Headers $adminHeaders
```

Use the Firebase web API key and active Admin email/password. Verify role `admin`
and `is_active: true`. A Warden can view inventory but cannot create or rename it.

```powershell
$fixtureName = 'LOCAL-INVENTORY-48-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
$blockBody = @{ name = $fixtureName } | ConvertTo-Json
$block = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/blocks' -Headers $adminHeaders -ContentType 'application/json' -Body $blockBody
$blockID = $block.block_id

$roomBody = @{ block_id = $blockID; room_no = 'TEST-101' } | ConvertTo-Json
$room = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/rooms' -Headers $adminHeaders -ContentType 'application/json' -Body $roomBody
$roomID = $room.room_id

$bedBody = @{ room_id = $roomID; bed_no = '1' } | ConvertTo-Json
$bed = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/beds' -Headers $adminHeaders -ContentType 'application/json' -Body $bedBody
$bedID = $bed.bed_id
$bed | ConvertTo-Json -Depth 5
```

Expected three created UUIDs; new bed available. No SQL fixture insertion is needed.

## 4. Rename all three resources

```powershell
$renamedBlockBody = @{ name = "$fixtureName-RENAMED" } | ConvertTo-Json
$renamedBlock = Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:18080/api/v1/blocks/$blockID" -Headers $adminHeaders -ContentType 'application/json' -Body $renamedBlockBody

$renamedRoomBody = @{ room_no = 'TEST-102' } | ConvertTo-Json
$renamedRoom = Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:18080/api/v1/rooms/$roomID" -Headers $adminHeaders -ContentType 'application/json' -Body $renamedRoomBody

$renamedBedBody = @{ bed_no = '2' } | ConvertTo-Json
$renamedBed = Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:18080/api/v1/beds/$bedID" -Headers $adminHeaders -ContentType 'application/json' -Body $renamedBedBody

if ($renamedBlock.block_id -ne $blockID -or $renamedRoom.room_id -ne $roomID -or $renamedBed.bed_id -ne $bedID) { throw 'FAIL: IDs changed.' }
if ($renamedRoom.block_id -ne $blockID -or $renamedBed.room_id -ne $roomID) { throw 'FAIL: parent relationships changed.' }

Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/rooms?block_id=$blockID" -Headers $adminHeaders | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/beds?room_id=$roomID" -Headers $adminHeaders | ConvertTo-Json -Depth 5
```

Expected renamed values, same IDs and parents, capacity 1 in the room.

## 5. Negative checks

Define a helper that checks actual HTTP status without treating unrelated failures
as successful rejection:

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

# Duplicate renamed bed number in the same room: 409.
$duplicateBody = @{ room_id = $roomID; bed_no = '2' } | ConvertTo-Json
Assert-HttpStatus 409 { Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/beds' -Headers $adminHeaders -ContentType 'application/json' -Body $duplicateBody -ErrorAction Stop }

# Parent changes are not accepted in update requests: 400.
$moveBody = @{ bed_no = '3'; room_id = $roomID } | ConvertTo-Json
Assert-HttpStatus 400 { Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:18080/api/v1/beds/$bedID" -Headers $adminHeaders -ContentType 'application/json' -Body $moveBody -ErrorAction Stop }

# Missing target: 404.
Assert-HttpStatus 404 { Invoke-RestMethod -Method Put -Uri 'http://127.0.0.1:18080/api/v1/beds/00000000-0000-0000-0000-000000000000' -Headers $adminHeaders -ContentType 'application/json' -Body $renamedBedBody -ErrorAction Stop }

# Missing authentication: 401.
Assert-HttpStatus 401 { Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/blocks' -ContentType 'application/json' -Body $blockBody -ErrorAction Stop }
```

Sign in as an active Warden; verify read success and write denial:

```powershell
$wardenIDToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenIDToken" }
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/blocks' -Headers $wardenHeaders
Assert-HttpStatus 403 { Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/blocks' -Headers $wardenHeaders -ContentType 'application/json' -Body $blockBody -ErrorAction Stop }
```

Student/Sub-Warden/Security Staff modification is also denied (automated tests
cover each). An expired token gives 401; obtain a fresh token rather than counting
that as a role-permission check.

## 6. Check renaming an occupied bed

Choose a dedicated active test student with no active allocation, using their
`student_id`, not local user ID or Firebase UID:

```powershell
Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/students?limit=100' -Headers $adminHeaders | ConvertTo-Json -Depth 6
$studentID = Read-Host 'Dedicated test student_id with no active allocation'
$assignBody = @{ student_id = $studentID; bed_id = $bedID } | ConvertTo-Json
$allocation = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/allocations' -Headers $adminHeaders -ContentType 'application/json' -Body $assignBody
$allocationID = $allocation.allocation_id

$occupiedRenameBody = @{ bed_no = '3' } | ConvertTo-Json
$occupiedBed = Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:18080/api/v1/beds/$bedID" -Headers $adminHeaders -ContentType 'application/json' -Body $occupiedRenameBody
if ($occupiedBed.available -ne $false) { throw 'FAIL: rename released occupied bed.' }

$active = Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/allocations?student_id=$studentID&active=true" -Headers $adminHeaders
if ($active.total -ne 1 -or $active.items[0].allocation_id -ne $allocationID -or $active.items[0].bed_id -ne $bedID) { throw 'FAIL: rename changed allocation.' }

# Release the test occupancy while retaining history.
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/allocations/$allocationID/revoke" -Headers $adminHeaders
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/allocations?student_id=$studentID&active=false" -Headers $adminHeaders | ConvertTo-Json -Depth 6
```

Retain the test inventory and allocation history; no delete endpoint is added.
Automated tests additionally compare whole allocation rows before/after renaming,
including ended rows, and verify concurrent uniqueness and fresh-role checks.

## 7. Commit and PR

Review the diff and finish local verification first:

```powershell
git status
git diff --check
git diff
git add internal/modules/inventory tests/integration/inventory/management_test.go scripts/coverage-report.py README.md CHANGELOG.md docs/inventory-management.md docs/hostel-inventory.md docs/module-layout.md docs/architecture.md docs/backend-ci.md
git diff --cached --check
git diff --cached --stat
git commit -m "feat(inventory): add Admin creation and renaming" -m "Refs #48"
git push -u origin feature/48-inventory-management
```

Create a PR with base `develop`, compare `feature/48-inventory-management`.
Title: `[DDP-#48] feat(inventory): add block, room and bed management`.

```markdown
Add Admin-only creation and renaming APIs for blocks, rooms and beds.
Preserve resource IDs, parent relationships, occupancy and allocation history.
Recheck Admin authority inside write transactions and enforce scoped uniqueness.

Refs #48. Builds on #33 and #46; approved extension of FR018 listing scope.

Validation:
- Automated results: [actual results and inventory coverage].
- Live create/rename, permission, conflict and occupied-bed checks: [actual results].
- CI: [successful run URL].

Staging HTTPS verification remains tracked in Hostel-Hive/hostelhive-infra#1.
```

Link #48 through Development; wait for CI and at least one teammate approval.
Record actual evidence on the issue without tokens, passwords or service-account
contents. Staging remains pending until the team resolves infrastructure issue #1;
agree any deferral before declaring the applicable Definition of Done complete.

After approved merge into develop and remote feature-branch deletion:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/48-inventory-management
```

The design repository's inventory-management amendment records the agreed scope;
it is a separate repository change, not part of the backend commit above.

## Recorded automated results — 10 October 2026

- Formatting, dependency verification, vet and build passed.
- HTTP validation and permission tests passed for all six new routes.
- All eight required PostgreSQL integration packages passed, with no skips.
- Inventory statement coverage: 242/266 (91.0%).
- Overall application statement coverage: 1897/2168 (87.5%).
- Database tests verified case-insensitive/scoped uniqueness, conflict rollback,
  missing resources/parents, preserved active/ended allocation records and counts,
  concurrent creates, authority changes during blocked writes and inactive actors.

These are local non-race results using fake Firebase verification with real
PostgreSQL. Live Firebase smoke checks, hosted CI race results and teammate review
remain to be recorded. Staging remains pending in infrastructure issue #1.
