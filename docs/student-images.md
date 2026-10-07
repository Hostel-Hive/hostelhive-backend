# Student profile images — issue #30

FR008 requires profile-image uploads. SDS architecture section 1.2 specifies
Cloudflare R2 for profile photos; the selected implementation baseline requires
private objects. FR006, FR007 and FR009 already have CRUD/details/search support.

## Permission decision

On 7 October 2026, Tasheen selected **Admin and Warden** for student management.
This reconciles the SRS FR006–FR009 Warden roles with UC003's Admin role.
All student CRUD and image routes now allow these two active roles. Sub Warden,
Security Staff and Student are denied. Identity account provisioning/role changes
remain Admin-only. Repository writes recheck the current database role and active
status inside the transaction. Record this decision in the next controlled SRS/SDS
revision; it is not a claim of supervisor sign-off.

## API contract

All image requests require a Firebase bearer ID token and current staff access.
`student_id` identifies a non-deleted student profile, not its `user_id`.

| Method/path | Request | Response |
|---|---|---|
| PUT `/api/v1/students/{student_id}/image` | Raw JPEG or PNG body; matching Content-Type | 200 updated student profile |
| GET `/api/v1/students/{student_id}/image` | No body | 200 image bytes, private no-store headers |
| DELETE `/api/v1/students/{student_id}/image` | Empty body | 204; object deletion queued |

Profiles with an image include `profile_image_url`, an authenticated relative API
path. It is not a public object URL. Clients fetch it with authorization. Object
keys, credentials and signed URLs are not exposed in responses. Missing images
return 404. Removing an already absent image on an existing profile returns 204.

Uploads are limited to 5 MiB, 4096 pixels per dimension, and 12 million total
pixels. Both header and decoded format must match JPEG/PNG. Empty, corrupt,
truncated and unsupported images are rejected. Re-encoding strips original
metadata and trailing bytes. SVG/GIF/WebP and multipart bodies are unsupported.
Limits are implementation choices for FR008, not claimed wording from the SRS.

Errors: 400 invalid input/image; 401 unauthenticated; 403 unauthorized/inactive;
404 missing/deleted student or absent image; 413 oversized body; 415 unsupported
Content-Type; 503 storage/database unavailable. When R2 is disabled, image
operations return 503 `student_images_unavailable`; existing CRUD remains usable.

## Storage and failure behavior

Migration 000006 adds object metadata, a durable cleanup schedule and a
student-owned image reference. Objects have random server-generated keys; users
cannot supply keys or arbitrary URLs. The service reserves metadata before R2
upload. A transaction locks the student and reservation while uploading and
attaching; cleanup skips locked reservations. The previous reference is replaced
only after R2 acknowledges upload. Replacement schedules the old key for cleanup.

Failed/abandoned reservations become eligible after 15 minutes. Removal and
student soft deletion clear the reference and schedule cleanup immediately. A
bounded worker polls every 30 seconds and retries failed deletions. Referenced
objects are never deleted by cleanup. Jobs survive restarts; keep R2 enabled so
the worker can run. Physical removal is eventual, while API access disappears as
soon as reference removal/deletion commits.

An upload or attachment failure normally retains the old reference. A database
commit/network error can have an uncertain outcome: GET the profile before
retrying. Cleanup rechecks references, protecting successfully attached objects.
GET can return 503 if replacement removes an old object between lookup and fetch;
retry to retrieve the current image. Private R2 access must remain disabled for
public domains, and storage credentials must be restricted to the chosen bucket.

The upload/cleanup transaction holds account-write locks during bounded R2 I/O,
matching the existing serialized student-write policy. This favors correctness
for the planned small deployment; measure contention before increasing scale.

Before rolling back migration 000006 on a populated system, save the object-key
inventory and arrange object cleanup. SQL rollback drops metadata but does not
delete files from R2. Never run disposable-test cleanup SQL on real data.

## Configuration and live verification

The server reads process environment variables; it does not automatically load
`.env`. R2 is disabled by default. When enabled, all four settings are required:
`R2_ACCOUNT_ID`, `R2_BUCKET`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`.
The SDK uses the account's HTTPS S3 endpoint, region `auto` and private requests.
Use a bucket-specific Object Read & Write R2 API token, not a Firebase key or
Cloudflare global API key. Keep the bucket private; browser CORS is unnecessary
because the Go API proxies upload/download.

1. Create a private R2 bucket and scoped S3 credentials in Cloudflare.
2. Set the normal database/Firebase startup settings plus the R2 settings below.
3. Apply migrations, build and restart the server.

```powershell
$env:DATABASE_URL = 'postgres://hostelhive:local-dev-only@127.0.0.1:15433/hostelhive?sslmode=disable'
$env:FIREBASE_PROJECT_ID = 'hostel-hive-152ef'
$env:GOOGLE_APPLICATION_CREDENTIALS = 'C:\Users\TASHEEN\Desktop\Projects\private\hostelhive-service-account.json'
$env:HTTP_ADDR = '127.0.0.1:18080'
$env:R2_ENABLED = 'true'
$env:R2_ACCOUNT_ID = Read-Host 'Cloudflare account ID'
$env:R2_BUCKET = Read-Host 'Private R2 bucket name'
$env:R2_ACCESS_KEY_ID = Read-Host 'R2 access key ID'
$r2Secret = Read-Host 'R2 secret access key' -AsSecureString
$env:R2_SECRET_ACCESS_KEY = ([System.Net.NetworkCredential]::new('', $r2Secret)).Password
Remove-Variable r2Secret
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version
go build -o ./bin/hostelhive-server.exe ./cmd/server
.\bin\hostelhive-server.exe
```

Expected migration version: 6, clean. In a separate test terminal, sign in as an
Admin using the included interactive helper; the token must be in this terminal.
Choose an existing disposable student **profile** from the student listing:

```powershell
$firebaseIDToken = .\scripts\get-firebase-token.ps1
$staffHeaders = @{ Authorization = "Bearer $firebaseIDToken" }
$page = Invoke-RestMethod -Uri 'http://127.0.0.1:18080/api/v1/students?limit=100' -Headers $staffHeaders
$page.students | Format-Table student_id, index_no, full_name
$studentProfileID = Read-Host 'Disposable student profile student_id'
$imagePath = Read-Host 'Full path to a test PNG image'
curl.exe --fail-with-body -i -X PUT "http://127.0.0.1:18080/api/v1/students/$studentProfileID/image" -H "Authorization: Bearer $firebaseIDToken" -H 'Content-Type: image/png' --data-binary "@$imagePath"
Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/students/$studentProfileID" -Headers $staffHeaders
$downloadPath = Join-Path $env:TEMP 'hostelhive-student-image.png'
Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:18080/api/v1/students/$studentProfileID/image" -Headers $staffHeaders -OutFile $downloadPath
```

Expect upload 200, an authenticated `profile_image_url` and a valid downloaded
image. Upload a different PNG to the same endpoint; verify replacement and that
the old R2 object disappears after the cleanup worker runs. Re-encoding means
original and downloaded file hashes may differ. For JPEG use `image/jpeg`.

Repeat with a Warden token: listing, CRUD and image operations must succeed.
Repeat an image request with a Student token: expect 403. Without a token: 401.
Try text bytes with `image/png`: 400; SVG content type: 415; over-5-MiB body: 413.
Never modify SQL roles or enable real accounts merely to bypass these checks.

```powershell
Invoke-RestMethod -Method Delete -Uri "http://127.0.0.1:18080/api/v1/students/$studentProfileID/image" -Headers $staffHeaders
```

Expect 204, then image GET 404. The R2 object is deleted asynchronously. Verify
an existing image is preserved when storage access temporarily fails, restore
access and check cleanup recovery. Use a dedicated test bucket for failure tests.

Automated checks: `go test ./...`, `go vet ./...`, `go build ./...`,
`scripts/test-student-profiles.ps1` and `scripts/test-migrations.ps1`. Integration
tests use disposable PostgreSQL and in-memory objects; R2 SDK contract tests use
an offline HTTP transport. Live R2 credentials/behavior and peer review remain
explicit checks before closing #30. No live R2 test was performed by Codex.

References: [R2 S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/),
[R2 API token setup](https://developers.cloudflare.com/r2/api/tokens/).
