# Student QR codes — issue #50

FR010 is implemented in the existing student module. Attendance ingestion,
ESP32 credentials, replay/cooldown rules and real-time presence are later tickets.

## API contract

| Route | Access | Success |
|---|---|---|
| GET `/api/v1/me/qr` | Active Student, own profile only | 200 image/png |
| GET `/api/v1/students/{studentID}/qr` | Active Admin/Warden | 200 image/png |

Neither endpoint takes a request body or query parameters. A staff member cannot
use the self route; a student cannot use the management route, including with
their own student ID. Students select themselves through verified Firebase identity.
Unknown UUIDs, archived profiles, inactive accounts, non-student target roles and
student accounts without a profile return 404. Missing/invalid authentication
returns 401; inactive or unauthorized actors return 403. Malformed UUIDs,
query parameters or bodies return 400. Persistence/rendering failures return 503
without internal details. Responses are `Cache-Control: no-store` with `nosniff`.

Migration 000011 creates `hostelhive.student_qr`: one student FK/primary key,
globally unique 64-character lowercase hexadecimal token, and creation timestamp.
First authorized retrieval lazily issues a cryptographically random 256-bit token.
Concurrent retrieval is serialized on the profile row and returns the same token.
Later requests, application restarts, profile edits and account reactivation keep
the identifier. Account/role changes and profile archival are rechecked inside the
transaction. Eligibility is checked for every retrieval, even if a QR already exists.
Checks describe the transaction snapshot; they do not revoke previously saved images.

PNG: 320x320 pixels, black on white, quiet border, medium error correction.
Payload: `HH1:` followed by the opaque token. It contains no name, email, index
number, database ID or Firebase credentials. Rendering is local and needs no R2
or external QR service. The encoder is pinned in go.mod; a separate test decoder
verifies the payload can be recovered from the PNG.

This is a stable identification code, not a login/access token or proof that a
person holding a copied image is its owner. Rotation and expiring/dynamic codes
are not implemented. The later device API must authenticate the device and resolve
the token against current active, unarchived student state on every accepted scan.
Do not expose a public token-to-student lookup or treat a saved image as authorization.
There is no attendance/presence mutation in these endpoints.

The SRS mixes FR010-generated codes, UC005 university ID-card QR wording and R002
dynamic-code mitigation, while the scope section excludes full copy prevention.
This ticket implements the agreed new-code generation scope. Existing university
card mapping, dynamic expiry and scanner protocol remain design clarification work;
no compatibility with university card payloads or hardware is claimed.

## 1. Automated verification

Docker Desktop must be running. In the backend directory:

```powershell
git branch --show-current
.\scripts\test-backend-coverage.ps1
```

Expected branch: `feature/50-student-qr-codes`. The script uses its own disposable
database, applies all migrations and runs format, dependency, vet, build and all
unit/integration tests. It writes ignored coverage artifacts and removes its test
container. Never run destructive migration tests against the development database.

## 2. Terminal 1: rebuild and start

Stop the old server with Ctrl+C. Docker Desktop and your development PostgreSQL
container must be running. These are the existing local development settings:

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

Expected migration version 11, clean; leave the server running.

## 3. Terminal 2: Admin retrieval

Enter the Firebase web API key and active Admin email/password when prompted.
Choose an active, unarchived dedicated test student's `student_id`, not `user_id`
or Firebase UID. The test student does not need an allocation for QR generation.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
$baseURL = 'http://127.0.0.1:18080'
$adminToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $adminHeaders
Invoke-RestMethod -Uri "$baseURL/api/v1/students?limit=100" -Headers $adminHeaders | ConvertTo-Json -Depth 6
$studentID = Read-Host 'Active unarchived test student_id'
New-Item -ItemType Directory -Force artifacts | Out-Null
Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/students/$studentID/qr" -Headers $adminHeaders -OutFile artifacts/student-qr-admin.png
Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/students/$studentID/qr" -Headers $adminHeaders -OutFile artifacts/student-qr-repeat.png
if ((Get-FileHash artifacts/student-qr-admin.png).Hash -ne (Get-FileHash artifacts/student-qr-repeat.png).Hash) { throw 'FAIL: repeated QR changed.' }
Write-Host 'PASS: repeated retrieval is stable.'
Start-Process -FilePath (Resolve-Path artifacts/student-qr-admin.png).Path
```

Expected a scannable QR image. Read it with a local QR scanner: it starts with
`HH1:` and has 64 hexadecimal characters after that. No personal fields appear.
Files under artifacts are ignored by Git and should not be committed.

## 4. Student self retrieval

Sign in using the credentials of the SAME student selected above, not Admin.

```powershell
$studentToken = .\scripts\get-firebase-token.ps1
$studentHeaders = @{ Authorization = "Bearer $studentToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $studentHeaders
Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/me/qr" -Headers $studentHeaders -OutFile artifacts/student-qr-self.png
if ((Get-FileHash artifacts/student-qr-admin.png).Hash -ne (Get-FileHash artifacts/student-qr-self.png).Hash) { throw 'FAIL: selected profile and signed-in student do not match, or QR changed.' }
Write-Host 'PASS: Student and Admin retrieve the same QR.'
```

## 5. Permissions and validation

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
Assert-HttpStatus 401 { Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/me/qr" }
Assert-HttpStatus 403 { Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/students/$studentID/qr" -Headers $studentHeaders }
Assert-HttpStatus 403 { Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/me/qr" -Headers $adminHeaders }
Assert-HttpStatus 400 { Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/students/not-a-uuid/qr" -Headers $adminHeaders }
Assert-HttpStatus 400 { Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/me/qr?student_id=$studentID" -Headers $studentHeaders }
Assert-HttpStatus 404 { Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/students/00000000-0000-0000-0000-000000000000/qr" -Headers $adminHeaders }
```

Sign in as an active Warden and verify management retrieval:

```powershell
$wardenToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenToken" }
Invoke-RestMethod -Uri "$baseURL/api/v1/me" -Headers $wardenHeaders
Invoke-WebRequest -UseBasicParsing -Uri "$baseURL/api/v1/students/$studentID/qr" -Headers $wardenHeaders -OutFile artifacts/student-qr-warden.png
if ((Get-FileHash artifacts/student-qr-admin.png).Hash -ne (Get-FileHash artifacts/student-qr-warden.png).Hash) { throw 'FAIL: Warden QR differs.' }
Write-Host 'PASS: Warden retrieval.'
```

Expired tokens give 401, not a successful role-denial test: sign in again if needed.
Existing archived test profiles should return 404 through the staff route.
Automated database tests also verify deactivation/role change and reactivation
without needing to change your live test accounts.

Optional database check in pgAdmin's Query Tool (replace UUID):

```sql
SELECT student_id, created_at
FROM hostelhive.student_qr
WHERE student_id = 'PASTE_TEST_STUDENT_ID';
```

Expected one row after first retrieval; repeated retrieval does not add rows.

## 6. Commit and PR after live checks

```powershell
git status
git diff --check
git diff
git add go.mod go.sum migrations/000011_student_qr.up.sql migrations/000011_student_qr.down.sql internal/modules/student/service/ports.go internal/modules/student/service/qr.go internal/modules/student/service/qr_test.go internal/modules/student/repository/qr.go internal/modules/student/handler/qr.go internal/modules/student/handler/qr_test.go internal/modules/student/routes.go internal/app/app.go tests/integration/student/qr_test.go README.md CHANGELOG.md docs/student-qr.md docs/module-layout.md docs/architecture.md docs/backend-ci.md
git diff --cached --check
git diff --cached --stat
git commit -m "feat(student): add unique student QR codes" -m "Refs #50"
git push -u origin feature/50-student-qr-codes
```

PR base: develop. Compare: feature/50-student-qr-codes.
Title: `[DDP-#50] feat(student): add unique student QR codes`.

```markdown
Add stable QR PNG retrieval for students themselves and Admin/Warden.
Migration 000011 persists unique random identifiers without personal data.
Recheck role, ownership and eligibility inside the issuance transaction.

Refs #50; implements FR010 generation. ESP32 scan ingestion remains separate.

Validation:
- Automated: [actual test/coverage results].
- Live: [Admin/Warden/self retrieval, stable PNG and rejection results].
- CI: [successful run URL, including migration rollback/reapply].

Staging HTTPS remains tracked in Hostel-Hive/hostelhive-infra#1.
```

Link issue #50 through Development. Record results without credential or QR
payloads. Wait for successful CI and teammate review before merging. Keep design
clarifications and deferred staging visible; do not claim hardware verification.

After approved merge and remote feature branch deletion:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/50-student-qr-codes
```

## Recorded automated results — 10 October 2026

- Formatting, dependency verification, vet and build passed.
- Full Go unit/integration suite passed; all eight required PostgreSQL packages
  executed with no skips on a disposable database migrated to version 11.
- Independent QR decoder recovered the exact opaque payload from a 320x320 PNG.
- Database tests passed for unique/stable/concurrent issuance, service recreation,
  ownership and roles, inactive/non-student/archived profiles and reactivation.
- Migration 000011 applied, rolled back preserving student profiles, and reapplied
  successfully in a separate disposable database.
- Student statement coverage: 636/734 (86.6%); overall: 1970/2251 (87.5%).

These are local non-race results with real PostgreSQL and fake Firebase verification.
Live Firebase retrieval, hosted CI/race evidence, peer review and physical scanner
verification remain to be recorded. Staging remains pending in infrastructure #1.
