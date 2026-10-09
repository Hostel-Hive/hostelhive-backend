# Staff profile management - issue #36

Traceability: SRS section 3.4.1 user names and section 3.4.6 account/profile
relationships; SDS Figure 3 (PDF page 15) STAFF with full_name/designation and
USER linkage, normalized schema on PDF pages 24 and 30. ADR-002 continues to
make Firebase the sole credential authority. No new password storage is added.

## Agreed policy and data contract

On 9 October 2026, Tasheen extended the initial Admin-management policy to
allow active staff to create/edit their own name. Admins manage all staff
profiles, including designation. Warden, Sub-Warden and Security Staff cannot
list/read/edit other staff profiles. Students cannot use staff-profile routes.
All active users read their own account through `/api/v1/me`.

`staff_profiles` has staff_id (UUID), unique user_id FK, full_name (1-200 Unicode
characters), designation (up to 120), created_at and updated_at. Admin PUT
requires a nonblank designation. Self-service creates a profile with an empty
designation until an Admin assigns it; subsequent self edits preserve it.
Text is trimmed; control/invalid UTF-8 input is rejected. Eligible staff roles
are Admin, Warden, Sub-Warden and Security Staff. Admins may manage inactive
target accounts; self-service requires the actor to be active.

Designation is descriptive text, never an authorization role. Self PUT accepts
only full_name and derives ownership from the authenticated Firebase UID.
Neither profile endpoint accepts role, email, is_active or credential fields.
Self-service does not create a user account or grant staff access.

Names are canonical in their respective profiles. `account_profiles` is a
database **view**, not a second editable profile table: current students use
their non-deleted student full_name; current staff use their staff full_name and
designation. `/me` and the Admin account list use the same view. Empty missing
names/designations are omitted from JSON; identity mutation acknowledgements
can continue to contain only base account fields. Read /me or the account list
for current profile details after changes.

Migration 8 does not invent names or designations for existing accounts. Existing
accounts can still authenticate without a profile. Use Admin PUT to populate
details, or staff self PUT to enter their own name. No staff number, department,
photo or additional
contact requirements are inferred from the STAFF diagram.

## Preservation and concurrency

- Account deactivation preserves profiles and blocks authenticated access.
- A staff-to-staff role change preserves the same staff_id and details.
- A staff-to-student change preserves the historical staff row; /me shows only
  the current student's name and no staff designation. Admin management lists
  still show the retained staff row together with its current account role.
- Admin PUT on an account whose current role is student returns 409. Changing it back
  to a staff role restores the retained staff projection; review its designation
  if the person's responsibilities have changed.
- Student-to-staff changes preserve the student record but stop using its name
  for /me. Admin or the active staff member creates the staff profile. Soft-deleted
  student names never supply the current Student /me projection.
- No DELETE staff endpoint is provided. Account deletion is restricted by the
  profile FK. Profile updates preserve staff_id and created_at; no separate
  profile version-history/audit-log requirement is claimed by this ticket.
- PUT serializes using the existing users write lock, rechecks current Admin
  status and target role, and upserts in one transaction. Self PUT rechecks the
  authenticated account status/role and ownership under the same lock. Concurrent
  writes
  yield one profile; the last successful update wins. Existing last-Admin
  protection remains in account-management code. Concurrent self/Admin edits
  never overwrite an Admin-assigned designation through self-service.

## API

| Method / path | Result |
| --- | --- |
| GET /api/v1/staff | Admin list of profiles, including retained/inactive rows |
| GET /api/v1/staff/{userID} | Admin reads profile by **users.user_id UUID** |
| PUT /api/v1/staff/{userID} | Admin creates or fully updates name/designation; 200 |
| PUT /api/v1/staff/me | Active staff creates/updates own full_name only; 200 |
| GET /api/v1/me | Active user reads own account plus current profile projection |

List supports only limit (default 20, range 1-100) and offset (default 0, range
0-100000), with staff array and has_more. Unknown/repeated/malformed filters are
400. Empty lists are `[]`. The request body limit is 8 KiB; oversized/invalid JSON,
extra fields, missing fields and trailing JSON are 400. Errors are 401 missing/
invalid authentication; 403 denied role/status; 404 unknown account/profile;
409 staff_account_required for Student target; 503 staff_profiles_unavailable.
Responses use no-store. Handler work uses ACCOUNT_MANAGEMENT_TIMEOUT (default 8s).

## Steps to verify locally

1. In Terminal 1, stop any previously running server with Ctrl+C. Open Docker
   Desktop and run commands from the backend directory:

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

Expect version 9 without dirty state and HTTP server listening. R2 is disabled
for this profile check only; enable it with normal settings for image tests.

2. In Terminal 2, in the same backend directory, obtain a fresh **Admin** token.
   The first prompt needs the Firebase web API key, not a user UID.

```powershell
$adminIDToken = .\scripts\get-firebase-token.ps1
$adminHeaders = @{ Authorization = "Bearer $adminIDToken" }
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $adminHeaders
$accounts = Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/users?limit=100' -Headers $adminHeaders
$accounts.users | Format-Table user_id,email,role,is_active,full_name
$staffUserID = Read-Host 'Existing test Warden user_id UUID'
```

Use an existing test Warden, not its firebase_uid or a student_id. Confirm the
Admin /me role before continuing. Test data should be agreed fictional details
for the test account, not guessed real staff information.

3. Create/update that test profile; you can rerun PUT without creating duplicates:

```powershell
$staffBody = @{ full_name='Local Test Warden'; designation='Warden' } | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:18080/api/v1/staff/$staffUserID" -Headers $adminHeaders -ContentType 'application/json' -Body $staffBody
Invoke-RestMethod "http://127.0.0.1:18080/api/v1/staff/$staffUserID" -Headers $adminHeaders
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/staff' -Headers $adminHeaders | ConvertTo-Json -Depth 5
```

Expect 200 and the same staff_id for repeated PUTs. Set a different name and
repeat PUT to verify updated_at changes and created_at remains unchanged.

4. Get a fresh token with the **Warden's credentials**. Verify own-profile
   visibility and rejected management access:

```powershell
$wardenIDToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenIDToken" }
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $wardenHeaders
curl.exe -i -H "Authorization: Bearer $wardenIDToken" 'http://127.0.0.1:18080/api/v1/staff'
curl.exe -i -X PUT -H "Authorization: Bearer $wardenIDToken" "http://127.0.0.1:18080/api/v1/staff/$staffUserID"
curl.exe -i 'http://127.0.0.1:18080/api/v1/staff'
curl.exe -i -H "Authorization: Bearer $adminIDToken" 'http://127.0.0.1:18080/api/v1/staff?limit=101'
```

Expected Warden /me: full_name and designation from step 3. Now verify self editing:

```powershell
$selfBody = @{ full_name='Local Test Warden Updated' } | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri 'http://127.0.0.1:18080/api/v1/staff/me' -Headers $wardenHeaders -ContentType 'application/json' -Body $selfBody
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $wardenHeaders
$forbiddenBody = @{ full_name='Local Test Warden'; designation='Admin' } | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri 'http://127.0.0.1:18080/api/v1/staff/me' -Headers $wardenHeaders -ContentType 'application/json' -Body $forbiddenBody
```

Expect updated name, the same staff_id and unchanged Warden designation.
The extra designation field is rejected with 400; PowerShell will show an
expected invalid_input error for this negative check. Automated tests also verify self creation without
an existing profile, which initially has an unassigned designation. The four curl
checks should return 403, 403, 401, 400. Warden inventory/student permissions
continue unchanged. Test Student /me separately: existing student name appears
and staff designation does not. Student staff-management requests return 403.

5. Optional local deactivation/reactivation on the **test Warden only**:

```powershell
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/users/$staffUserID/deactivate" -Headers $adminHeaders
Invoke-RestMethod "http://127.0.0.1:18080/api/v1/staff/$staffUserID" -Headers $adminHeaders
```

The Admin read still shows the same profile and is_active false. If revocation
is pending, let the worker retry; do not declare completion or immediately
force reactivation. Once revocation completes:

```powershell
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/users/$staffUserID/activate" -Headers $adminHeaders
$wardenIDToken = .\scripts\get-firebase-token.ps1
$wardenHeaders = @{ Authorization = "Bearer $wardenIDToken" }
Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/me' -Headers $wardenHeaders
```

Expect fresh-sign-in success and preserved profile. Automated tests exercise
role changes on disposable accounts; there is no need to change the roles of
your real accounts for this ticket. Refresh Admin token if it expires.

6. Run verification commands in Terminal 2:

```powershell
go test ./...
go vet ./...
.\scripts\test-staff-profiles.ps1
.\scripts\test-account-management.ps1
.\scripts\test-student-profiles.ps1
.\scripts\test-migrations.ps1
```

Integration and migration scripts create/remove their own disposable databases.
They never roll back the developer database. Tests cover all roles, invalid
requests, Unicode/length bounds, stale Admin authority, profile identity and
timestamps, inactive targets, role roundtrips, current student name projection,
retained staff data, own-profile creation/editing, forbidden self fields,
fresh ownership/status checks, and concurrent self/Admin upserts.

## Commit, PR and issue steps

7. Review and commit the feature after successful live checks:

```powershell
git branch --show-current
git status --short
git diff --check
git add README.md docs/architecture.md docs/identity-requirements.md docs/staff-profiles.md internal/app/app.go internal/modules/staff internal/modules/identity/domain/user.go internal/modules/identity/repository/accounts.go internal/modules/identity/repository/lookup.go internal/modules/identity/repository/lookup_test.go migrations/000008_staff_profiles.up.sql migrations/000008_staff_profiles.down.sql migrations/000009_staff_self_service.up.sql migrations/000009_staff_self_service.down.sql scripts/test-staff-profiles.ps1 scripts/test-migrations.ps1 tests/integration/staff
git diff --cached --stat
git diff --cached --check
git commit -m "feat(staff): add staff profile management"
git push -u origin feature/36-staff-profile-management
```

The branch is already created. Do not create another one. Keep credential JSON,
passwords and ID tokens out of commits, screenshots and issue comments.

8. Create a GitHub PR: base **develop**, compare
   **feature/36-staff-profile-management**.

Title: `feat(staff): add staff profile management`

```markdown
Add staff profiles with Admin management and staff self-service name editing.
Expose current profile details on /me and account lists; preserve profiles across
account status/role changes. Self editing cannot change designation or permissions.

Validation: [record actual automated and live results].
Refs #36
```

9. Request teammate review. Address feedback, rerun affected checks, then merge
   into develop after approval. Record results as an issue comment, not a rewrite
   of the original description:

```markdown
Implemented staff profile management.
- PR: [individual PR URL]
- Automated: [test/vet/build/integration/migration results]
- Live: [Admin PUT/GET/list; staff own /me and self PUT; designation preserved; denied management access]
- Preservation: [deactivation/reactivation result, if tested live]
- Reviewer: [name and result]
- Merged into develop: [yes/no]
```

10. When verified and reviewed, close #36, mark Done and delete the remote
    feature branch using the merged PR. Stop the server (Ctrl+C), then:

```powershell
git switch develop
git pull --ff-only origin develop
git fetch --prune
git branch -d feature/36-staff-profile-management
```

If safe deletion refuses after a squash merge, retain the branch until its
contents are verified in develop. Do not blindly force-delete or merge to main.
UC004 CSV import remains the next missing student workflow; frontend password
reset/change and provider-security evidence remain separate work.

## Migration rollback

Migration 9 adds the unassigned designation default without rewriting migration 8.
Its down migration refuses rollback if any profile has an empty designation.
An Admin must assign real designations before reverting it; the rollback does
not delete profiles or invent values. Rollback tests use disposable databases.
