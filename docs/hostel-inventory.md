# Hostel inventory — issue #33

Implements the read-only inventory part of SRS FR018 / UC018. The correction
addendum requires globally unique beds, unique active allocations and history.
FR019 assignment and FR020 transfer/revocation endpoints are separate work.

## Inventory contract

- Blocks have a globally unique UUID and case-insensitive unique name.
- Rooms have a globally unique UUID and a case-insensitive unique room number
  within their block. Bed numbers are unique within a room; bed UUIDs are global.
- Capacity means the number of configured beds. Empty rooms have capacity zero.
  There is no second capacity value that can disagree with the bed records.
- An allocation with `ended_at IS NULL` occupies a bed. Ending it releases the
  bed while preserving history. Unique partial indexes reject concurrent double
  allocation of either a bed or a student. Foreign keys protect history.
- Occupancy includes every active allocation, even if its student account later
  becomes inactive. Deactivation must not silently make an occupied bed available.
- Lists contain counts and identifiers, never student details or credentials.
- Issue #46 explicitly extends inventory selection to Admin and Warden.
  Student-profile permissions alone do not imply inventory access. Missing token: 401; other roles
  or inactive accounts: 403; invalid filters: 400; database failure: 503.
- Issue #46 implements assignment eligibility, atomic transfers, explicit
  revocation and actor auditing; see [allocation contract](bed-allocation.md).
  Gender zoning and maintenance-based bed exclusions are not represented by
  current inventory fields and are not enforced. Inventory creation/editing
  APIs remain outside scope.


## APIs

All routes use Firebase bearer authentication and the current database role.
Responses have `items`, `total`, `limit`, `offset`; empty items are `[]`.
Totals and page items share a repeatable-read database snapshot. Pagination is
stable for an unchanged dataset; separate requests may reflect later changes.

| Route | Supported filters |
| --- | --- |
| GET /api/v1/blocks | limit, offset |
| GET /api/v1/rooms | block_id, limit, offset |
| GET /api/v1/beds | block_id, room_id, available, limit, offset |

`limit` defaults to 20 and must be 1–100; `offset` defaults to 0 and must be
0–100000. IDs must be UUIDs. `available` must be exactly `true` or `false`.
Unknown, repeated, malformed and unsupported filters are rejected. A valid ID
with no matching records returns 200 and an empty list. Combining a room and an
unrelated block also returns an empty list. `INVENTORY_TIMEOUT` defaults to 8s.

## Automated verification

Run from the backend folder with Docker Desktop running:

```powershell
go test ./...
go vet ./...
.\scripts\test-inventory.ps1
.\scripts\test-migrations.ps1
```

The two scripts create and remove their own disposable PostgreSQL containers.
They never apply rollback tests to your development database. Inventory tests
exercise occupancy, release, history, constraints, concurrent double allocation,
filters, pagination and query cancellation. HTTP tests exercise all roles,
invalid requests, safe error responses and request deadlines.

## Fresh local startup after closing terminals

Terminal 1, in the backend folder:

```powershell
docker start hostelhive-postgres
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

Expect the latest migration version (10 after issue #46), not dirty, and `HTTP server listening`. R2 is
disabled only for this inventory check; configure it normally when testing images.

Terminal 2, in the backend folder, sign in as an **active Warden**:

```powershell
$wardenIDToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenIDToken" }
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $wardenHeaders
$blocks = Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/blocks' -Headers $wardenHeaders
$blocks | ConvertTo-Json -Depth 5
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/rooms' -Headers $wardenHeaders | ConvertTo-Json -Depth 5
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/beds?available=true' -Headers $wardenHeaders | ConvertTo-Json -Depth 5
curl.exe -i 'http://127.0.0.1:18080/api/v1/blocks'
curl.exe -i -H "Authorization: Bearer $wardenIDToken" 'http://127.0.0.1:18080/api/v1/rooms?limit=101'
```

Confirm `/me` reports `warden`. Empty lists are correct on a new database:
migrations do not invent production rooms or beds. The last two requests should
return 401 and 400 respectively. Sign in as a Student and repeat a list
request to verify 403; Admin listing now returns 200 under issue #46. Never post tokens or passwords in the ticket.

If there is no Warden account, provision a separate local test Warden. First
sign in as your existing Admin in Terminal 2; do not change the sole Admin's role.
Use a test email you control and remember the password for subsequent sign-in:

```powershell
$adminIDToken = .\scripts\get-firebase-token.ps1
$wardenEmail = Read-Host 'New local test Warden email'
$wardenPassword = Read-Host 'Password (12-128 characters)' -AsSecureString
$wardenBody = @{
    email = $wardenEmail
    role = 'warden'
    password = ([System.Net.NetworkCredential]::new('', $wardenPassword)).Password
} | ConvertTo-Json
$createHeaders = @{
    Authorization = "Bearer $adminIDToken"
    'Idempotency-Key' = [Guid]::NewGuid().ToString()
}
try {
    Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/users' -Headers $createHeaders -ContentType 'application/json' -Body $wardenBody
} finally {
    Remove-Variable wardenBody, wardenPassword -ErrorAction SilentlyContinue
}
```

After successful creation, obtain `$wardenIDToken` with the helper and the new
Warden credentials, then rebuild `$wardenHeaders`. If creation reports an
in-progress/unavailable operation, retry with the same idempotency key; do not
create a new key for that operation. Existing accounts should simply sign in.

### Optional fictional local inventory fixture

To see nonempty lists, run this **SQL in pgAdmin Query Tool connected to the
local HostelHive Docker database**, not in PowerShell. It creates fictional
inventory only and is repeatable. Never use it for production inventory.

```sql
BEGIN;
INSERT INTO hostelhive.blocks(name) VALUES ('LOCAL-TEST-BLOCK') ON CONFLICT DO NOTHING;
INSERT INTO hostelhive.rooms(block_id,room_no)
SELECT block_id,'TEST-101' FROM hostelhive.blocks WHERE name='LOCAL-TEST-BLOCK'
ON CONFLICT DO NOTHING;
INSERT INTO hostelhive.beds(room_id,bed_no)
SELECT r.room_id,n.bed_no FROM hostelhive.rooms r
JOIN hostelhive.blocks b USING(block_id)
CROSS JOIN (VALUES ('1'),('2')) n(bed_no)
WHERE b.name='LOCAL-TEST-BLOCK' AND r.room_no='TEST-101'
ON CONFLICT DO NOTHING;
COMMIT;
```

Repeat the lists: the fixture room has capacity 2, occupied 0, available 2.
Use returned IDs to test parent filters. Allocation/release changes are already
verified in the isolated automated tests; do not assign real students via SQL.

## Completion evidence

Keep the issue description short. Add a comment after verification with the PR
URL, automated checks, live Warden/Admin results, rejected Student requests,
and reviewer confirmation. Record actual results only. Merge the feature PR
into `develop`, close #33 when all required verification/review is complete,
then remove the merged feature branch. Keep `develop` and `main`.

## Commit and PR steps, in order

These commands are for after the live checks above. The branch has already been
created by the implementation; do not create it again.

```powershell
git branch --show-current
git status --short
git diff --check
git add .env.example README.md internal/app/app.go internal/config/config.go internal/config/config_test.go internal/modules/inventory migrations/000007_hostel_inventory.up.sql migrations/000007_hostel_inventory.down.sql scripts/test-inventory.ps1 scripts/test-migrations.ps1 tests/integration/inventory docs/hostel-inventory.md
git diff --cached --stat
git diff --cached --check
git commit -m "feat(inventory): add hostel inventory listings"
git push -u origin feature/33-hostel-inventory
```

Confirm the current branch is `feature/33-hostel-inventory`. Open GitHub's
Pull requests tab and create a PR with **base `develop`**, **compare
`feature/33-hostel-inventory`**.

Title: `feat(inventory): add hostel inventory listings`

Suggested short description:

```markdown
Add block, room and bed listings (Warden initially; Admin added in #46) with capacity and availability,
allocation constraints and retained history.

Validation: Go tests, vet, build, disposable PostgreSQL inventory tests and
migration rollback/reapply passed. Live verification: [add actual result].

Refs #33
```

Request a teammate review. Address any requested changes on this same feature
branch, rerun affected checks, commit and push. After approval and successful
checks, merge into `develop`. Copy the URL of the **individual PR**, not `/pulls`.

Add a comment on issue #33 using actual results:

```markdown
Implemented hostel inventory listings.
- PR: [paste individual PR URL]
- Automated: tests, vet, build, inventory integration and migration rollback/reapply passed.
- Live: Warden lists [result]; capacity/availability [result]; missing token [result]; invalid filter [result]; Admin access 200 [result]; Student access 403 [result].
- Review: [reviewer and result]
- Merged into develop: [yes/no]
```

Close #33 and mark the project item Done only when verification and review are
complete. Then click Delete branch on the merged PR, stop the running server
with Ctrl+C in Terminal 1, and update your local checkout:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/33-hostel-inventory
```

If `git branch -d` refuses after a squash merge, leave the local branch until
its contents are verified in `develop`; do not blindly force-delete it. Do not
merge `develop` into `main` just to finish this feature ticket.
