# Student CSV import — issue #38

## Contract and approved account-linking policy

Traceability: SRS UC004, PDF page 40 / printed page 32. UC004 requires Admin
bulk import, row validation, importing valid records, a summary/error report,
and skipping duplicate student IDs. Its original main flow also mentions
creating accounts. On 9 October 2026, Tasheen selected linking **existing active
student accounts** with valid-row partial import. This is an explicit change
to the account-creation part of UC004, not a claim of unchanged conformance.
Identity provisioning creates Firebase/local accounts separately; CSV contains
no credentials, emails, roles, account-status fields or profile-image URLs.
Record the change in issue #38 and the next controlled SRS/SDS revision.

The student module owns the import. No new module or database migration is needed.
Admin-only import does not change existing Admin/Warden CRUD and image access.

| Route | Result |
| --- | --- |
| GET /api/v1/students/import/template | Admin downloads a header-only CSV template |
| POST /api/v1/students/import | Admin submits raw UTF-8 CSV with Content-Type text/csv |

Use the UUID users.user_id, not firebase_uid, student_id or email. The account
must have current role student and is_active true. Existing/archived profiles
are never overwritten or restored. Existing account provisioning rules remain.

Required columns, each exactly once, in any order:

```text
user_id,index_no,full_name,faculty,year,contact_phone,guardians
```

The [checked-in template](templates/student-import.csv) contains that header.
Header names are case-sensitive. Extra/missing/duplicate columns reject the file.
UTF-8 BOM, CRLF and normal quoted CSV are accepted. Blank physical lines are
ignored by the CSV parser. Limits: 2 MiB including BOM and at most 500 data
records. Header-only, invalid UTF-8, broken quoting or wrong field counts reject
the entire document before any writes. Do not hand-escape guardian JSON; use a
CSV writer such as Export-Csv. Multipart upload is not this endpoint's contract.

Fields use existing student validation: index_no 1–64 characters, full_name
1–200, faculty 1–120, year 1–10, valid contact phone, and 1–10 guardians.
guardians contains a JSON array of objects with name, relationship and
contact_phone; unknown fields/trailing JSON are invalid. Values are trimmed.
Each valid row creates its student and all guardians in one transaction.

The first syntactically valid occurrence reserves a user_id and case-insensitive
index_no within the file; later duplicates are rejected. Invalid rows do not
reserve identifiers. Database constraints also handle existing, archived and
concurrent duplicates. No import changes Firebase or assigns an account role.
The repository rechecks active Admin authority under the existing users write
lock for every row, including after a mid-import role/status change.

## Results and retries

200 is a **report**, not a promise that every row succeeded. Inspect created,
rejected, not_attempted, optional stopped_reason and every rows entry. Rows
identify the data-record ordinal and physical starting line (quoted records
can span lines). A created row includes student_id; errors contain safe codes
without echoing names, guardian details, raw SQL or credentials.

- invalid_input: field/guardian validation failed.
- duplicate_in_file: a previously valid CSV row uses that account or index.
- duplicate_student: an existing/archived/concurrent profile reserves it.
- active_student_account_required: missing, inactive or non-student account.
- forbidden: Admin authority changed; stop further writes.
- student_profiles_unavailable or import_interrupted: processing stopped.

Earlier commits survive interruption. Later valid rows have status not_attempted;
prevalidated invalid/duplicate rows remain rejected. The request uses
STUDENT_PROFILE_TIMEOUT (default 8s); use smaller batches if needed. No background
job is implied. On client/network/database failures a commit can be uncertain:
inspect student listings and safely retry. Repeating a committed row returns a
duplicate instead of changing its profile. No Idempotency-Key is needed for
create-only profile imports; there is no persisted import-job history.

Other HTTP results: 401 missing/invalid token, 403 denied/inactive role,
415 unsupported content type/encoding, 413 oversized CSV, 400 invalid_csv for
structural/limit errors. Structural 400 writes nothing. Reports use no-store.

## Complete local verification steps

### 1. Start PostgreSQL and the rebuilt server — Terminal 1

Stop the old server using Ctrl+C first. Open Docker Desktop. All commands below
are PowerShell, from the backend repository. Do not paste SQL into PowerShell.

```powershell
Set-Location 'C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend'
git branch --show-current
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

Expected branch: feature/38-student-csv-import. Latest migration remains 9;
the server should log HTTP server listening. R2 is disabled only for this check.

### 2. Obtain fresh Admin authentication — Terminal 2

```powershell
Set-Location 'C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend'
curl.exe -i 'http://127.0.0.1:18080/health'
curl.exe -i 'http://127.0.0.1:18080/ready'
$adminIDToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminIDToken" }
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $adminHeaders
$accounts = Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/users?limit=100' -Headers $adminHeaders
$accounts.users | Where-Object { $_.role -eq 'student' -and $_.is_active } | Format-Table user_id,email,role,is_active
$studentUserID = Read-Host 'Existing active TEST student user_id UUID without a student profile'
$csvPath = Join-Path $env:TEMP ('hostelhive-import-' + [Guid]::NewGuid().ToString('N') + '.csv')
$templatePath = Join-Path $env:TEMP 'hostelhive-student-template.csv'
Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:18080/api/v1/students/import/template' -Headers $adminHeaders -OutFile $templatePath
Get-Content -LiteralPath $templatePath
```

Use the Firebase **web API key** at the helper's first prompt, not a Firebase UID.
/me must show admin and active. Use an agreed test account, not a real student.
If all listed students already have profiles, provision/activate a new test
student using the existing identity guide first. Never delete real profiles to
make room. Get a fresh token if it expires; terminal variables are not shared.

### 3. Build a correctly quoted CSV and submit it

```powershell
$guardianJSON = ConvertTo-Json -Compress -InputObject @(
    @{ name='Local Test Guardian'; relationship='Parent'; contact_phone='0771234567' }
)
$indexNo = 'CSV-TEST-' + [Guid]::NewGuid().ToString('N').Substring(0,12)
$validRow = [pscustomobject][ordered]@{
    user_id=$studentUserID
    index_no=$indexNo
    full_name='Local CSV Test Student'
    faculty='Science'
    year=1
    contact_phone='0771234567'
    guardians=$guardianJSON
}
$invalidRow = $validRow.PSObject.Copy()
$invalidRow.index_no = $indexNo + '-INVALID'
$invalidRow.year = 99
$duplicateRow = $validRow.PSObject.Copy()
@($validRow, $invalidRow, $duplicateRow) | Export-Csv -LiteralPath $csvPath -NoTypeInformation -Encoding UTF8
$report = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/students/import' -Headers $adminHeaders -ContentType 'text/csv; charset=utf-8' -InFile $csvPath
$report | ConvertTo-Json -Depth 6
$createdStudentID = ($report.rows | Where-Object status -eq 'created' | Select-Object -First 1).student_id
Invoke-RestMethod "http://127.0.0.1:18080/api/v1/students/$createdStudentID" -Headers $adminHeaders | ConvertTo-Json -Depth 6
```

Expected first import: total 3, created 1, rejected 2, not_attempted 0.
Record 2 is invalid_input; record 3 is duplicate_in_file. Read back the created
profile and verify its guardian. If there is no created row, inspect the report
before executing the GET; the account might already have a profile.

### 4. Verify repeat and permission handling

```powershell
$repeat = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/students/import' -Headers $adminHeaders -ContentType 'text/csv' -InFile $csvPath
$repeat | ConvertTo-Json -Depth 6
$wardenIDToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenIDToken" }
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $wardenHeaders
Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/api/v1/students/import' -Headers $wardenHeaders -ContentType 'text/csv' -InFile $csvPath
curl.exe -i -X POST -H 'Content-Type: text/csv' --data-binary "@$csvPath" 'http://127.0.0.1:18080/api/v1/students/import'
```

For the helper use the test Warden's credentials this time; /me must show warden.
Repeat: created 0, rejected 3; record 1 duplicate_student. Existing profile and
guardian identities remain unchanged. Warden import returns 403 (an expected
PowerShell error); unauthenticated import returns 401. The Warden's ordinary
student CRUD permissions remain unchanged. Automated tests cover malformed files,
multi-guardian rows, stale authority and concurrent imports on disposable data.

### 5. Run checks and record live evidence

```powershell
go test ./...
go vet ./...
.\scripts\test-student-profiles.ps1
git diff --check
Remove-Item -LiteralPath $csvPath, $templatePath
```

The student test script creates/removes its own random disposable PostgreSQL
container and runs both existing profile/image and new CSV integration tests.
It does not mutate the developer database. The test profile created above stays
in the local developer database; cleanup is optional through existing test-only
student deletion, which preserves archived identifiers.

Add a progress comment to issue #38, with actual results (not a description rewrite):

```markdown
Policy confirmed: CSV imports link existing active student accounts; Firebase
account creation stays in identity provisioning. Valid rows import independently.
This updates the account-creation step in UC004.

Live verification: [template download; 1 created/2 rejected; guardian readback;
repeat produces no duplicates; Warden 403; unauthenticated 401].
Automated checks: [actual test/vet/build and PostgreSQL results].
```

## Commit, PR, review and merge

### 6. Commit and push after live verification

```powershell
git branch --show-current
git status --short
git diff --check
git add README.md docs/identity-requirements.md docs/student-csv-import.md docs/templates/student-import.csv internal/app/app.go internal/modules/student/domain/import.go internal/modules/student/service/import.go internal/modules/student/service/import_test.go internal/modules/student/handler/import.go internal/modules/student/handler/import_test.go internal/modules/student/repository/postgres.go internal/modules/student/routes.go tests/integration/student/import_test.go
git diff --cached --stat
git diff --cached --check
git commit -m "feat(student): add Admin CSV profile import" -m "Refs #38"
git push -u origin feature/38-student-csv-import
```

### 7. Open a PR into develop

Base develop; compare feature/38-student-csv-import.
Title: `feat(student): add Admin CSV profile import`.

```markdown
Add Admin CSV imports linked to existing active student accounts, with per-row
results, guardian validation and duplicate protection. Account provisioning
remains in identity; existing student CRUD permissions are unchanged.

Validation: [actual automated and live results].
Refs #38
```

### 8. Review, merge, close and clean up

Request teammate review, resolve feedback, merge into develop after approval.
Record the individual PR URL, test results and reviewer in an issue comment.
Close #38 / mark Done only after verification and review. Delete the remote
feature branch using the merged PR. Stop your server, then run:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/38-student-csv-import
```

If safe deletion refuses after a squash merge, verify the contents before
deciding what to do; do not blindly force-delete. Merge to main at an agreed
release/milestone, not automatically for this feature.
