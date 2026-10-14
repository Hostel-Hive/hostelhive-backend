# Complaint image attachments - issue #66

Optional private JPEG/PNG photos for complaints. Core text requests and progress
continue working independently of object storage. Notifications are follow-up work.

## API and policy

| Endpoint | Purpose |
|---|---|
| PUT /api/v1/complaints/{complaintID}/images/{imageID} | Student upload; raw JPEG/PNG bytes |
| GET /api/v1/complaints/{complaintID}/images | Authorised metadata list and current version |
| GET /api/v1/complaints/{complaintID}/images/{imageID} | Authorised image bytes |
| DELETE /api/v1/complaints/{complaintID}/images/{imageID} | Student removal; empty body |

Student writes require an active account, unarchived profile, ownership and Pending
status. Staff cannot upload/remove Student photos. Reads use existing complaint
scope: Admin/Warden both categories; Sub-Warden maintenance only; Student own.
Security/anonymous denied. Active Students may still read their archived profile's
complaint history/evidence. Inactive Students cannot access it; staff can.

Up to three images, each raw and normalized file <=5 MiB; JPEG/PNG only, 4096 max
width/height, 12 million max pixels. The shared validator decodes/re-encodes and
strips unnecessary metadata; Student profile photo behavior stays compatible.

imageID is a client UUID. First PUT returns 201 with image metadata, version and
replayed=false. Exact retry (same normalized content/type) returns 200 with the
original attachment/action version and replayed=true; it does not write R2 again.
Changed reuse, removed/expired IDs, fourth slot or frozen new mutation returns 409.
Do not confuse a replay's original action version with current complaint version:
refetch the complaint/list before changing status. DELETE returns removed=true,
image_id, action version; exact retry preserves its original version/history.

No public URL/key/hash is exposed. GET serves image bytes with no-store/nosniff.
List is bounded by the three-image limit and needs no pagination. No endpoint takes
query parameters. Invalid UUID/content 400, unsupported content type 415, oversized
raw body 413, wrong role 403, out-of-scope/missing image 404, provider/disabled upload
or read 503 complaint_images_unavailable. Image metadata/removal can operate while
R2 is disabled; physical cleanup waits for the enabled worker.

## Storage lifecycle and concurrency

Migration 17 adds complaint_images and image_id on history records. Add/remove
increment the complaint version and append image_added/image_removed entries.
Preserve all previous description/status history. Photos freeze at In Progress or
Resolved; reopening to In Progress does not unlock Student image changes. Exact
completed retries after freezing are read-only and may return the original result.

Reservation commits before R2 writes. Reserved and attached rows consume slots.
Pending-only authority is rechecked under locks before upload/attach. Bounded R2
calls hold row/authorization locks, so staff changes can briefly wait; outdated
expected_version requests then return 409. Provider success and PostgreSQL commit
are not one distributed transaction. Unattached successful writes remain tracked
for eventual cleanup after a lost response/crash. Failed attempts can be retried
with the same image ID/content until the reservation is retired.

Removed images become immediately unavailable through the API. The cleanup worker
runs every 30 seconds; failed/abandoned reservations become eligible after 15 minutes.
It skips locked rows, retries provider failures and keeps tombstones/audit references.
Cleanup outages delay physical deletion. Expired IDs conflict; use a new image UUID
for a fresh attempt once a slot is available. Attached evidence is never selected
for automatic cleanup. Only explicit Student removal while Pending queues it.

## R2 setup and startup - Terminal 1

Use a private development bucket. Confirm public r2.dev access and public custom
bucket domains are disabled. API checks do not prove the bucket is private. Keep
service credentials out of Git and screenshots; use bucket-scoped object read/write
credentials. Reuse your existing R2 configuration if it is already set correctly.

Stop the old server with Ctrl+C. Set the same DATABASE_URL, Firebase project/service
account, HTTP_ADDR and attendance settings you used for #64. If those variables
were lost, restore them using that guide before migrating. Only run up on dev data.

```powershell
cd C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend
$ErrorActionPreference = 'Stop'
git branch --show-current # feature/66-complaint-images
$env:R2_ENABLED = 'true'
$env:R2_ACCOUNT_ID = Read-Host 'R2 account ID (32 lowercase hex characters)'
$env:R2_BUCKET = Read-Host 'Private development bucket name'
$access = Read-Host 'R2 Access Key ID' -AsSecureString
$secret = Read-Host 'R2 Secret Access Key' -AsSecureString
$env:R2_ACCESS_KEY_ID = ([System.Net.NetworkCredential]::new('', $access)).Password
$env:R2_SECRET_ACCESS_KEY = ([System.Net.NetworkCredential]::new('', $secret)).Password
Remove-Variable access,secret
.\scripts\migrate.ps1 up
.\scripts\migrate.ps1 version # 17, clean
go build -o .\bin\hostelhive-server.exe .\cmd\server
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
.\bin\hostelhive-server.exe
```

## Live verification - Terminal 2

Get fresh Student/Admin/Warden/Sub-Warden headers and the Assert-HttpError helper
from the #64 guide. The Student must have an unarchived profile. Run /me for each
account to confirm role/is_active; refresh expired tokens before interpreting errors.

Create a NEW Pending complaint; do not reuse a resolved #64 test fixture:

```powershell
$payload=@{request_id=[Guid]::NewGuid().ToString();category='maintenance';description='LOCAL-TEST-66 photo evidence'} | ConvertTo-Json
$c=Invoke-RestMethod -Method Post -Uri "$baseURL/api/v1/complaints" -Headers $studentHeaders -ContentType 'application/json' -Body $payload
$complaintID=$c.complaint_id
$studentID=$c.student_id
$imageID=[Guid]::NewGuid().ToString()
New-Item -ItemType Directory -Force artifacts | Out-Null
# A tiny local fixture avoids uploading personal photos during verification.
Add-Type -AssemblyName System.Drawing
$pngPath=Join-Path (Get-Location) 'artifacts/complaint-66-source.png'
$bitmap=New-Object System.Drawing.Bitmap(8,8)
try { $bitmap.SetPixel(0,0,[System.Drawing.Color]::Red); $bitmap.Save($pngPath,[System.Drawing.Imaging.ImageFormat]::Png) }
finally { $bitmap.Dispose() }
$uri="$baseURL/api/v1/complaints/$complaintID/images/$imageID"
$uploaded=Invoke-RestMethod -Method Put -Uri $uri -Headers $studentHeaders -ContentType 'image/png' -InFile $pngPath
$uploaded | ConvertTo-Json -Depth 5
if ($uploaded.replayed -or $uploaded.version -ne 2) { throw 'Incorrect first attachment.' }
$retry=Invoke-RestMethod -Method Put -Uri $uri -Headers $studentHeaders -ContentType 'image/png' -InFile $pngPath
if (-not $retry.replayed -or $retry.version -ne $uploaded.version) { throw 'Retry changed attachment history.' }
$list=Invoke-RestMethod -Uri "$baseURL/api/v1/complaints/$complaintID/images" -Headers $studentHeaders
if ($list.items.Count -ne 1) { throw 'Duplicate attachment.' }
Invoke-WebRequest -UseBasicParsing -Uri $uri -Headers $studentHeaders -OutFile artifacts/complaint-66-student.png
Invoke-WebRequest -UseBasicParsing -Uri $uri -Headers $adminHeaders -OutFile artifacts/complaint-66-admin.png
if ((Get-FileHash artifacts/complaint-66-student.png).Hash -ne (Get-FileHash artifacts/complaint-66-admin.png).Hash) { throw 'Same attachment differs by viewer.' }
# The normalized download need not equal original input bytes.
```

Additional checks in order:

1. Add two more image UUIDs to the same Pending complaint. Same file is acceptable
   for this cap test. A fourth UUID returns 409. Exact retries do not consume slots.
2. Different valid image content reusing imageID returns 409. Corrupt content 400,
   PNG declared JPEG 400, unsupported type 415, raw body over 5 MiB 413, bad UUID 400.
3. Remove first image with DELETE and empty body: 200. Exact retry preserves the
   original action version. Subsequent GET 404. PUT with removed ID 409. A new ID
   can occupy its freed slot. Check Cloudflare object disappearance after cleanup.
4. Get current complaint.version, then Warden changes status to in_progress using
   that expected_version and a new request_id. New Student upload/removal returns
   409. Existing photos remain readable to authorised viewers. An exact completed
   PUT retry is still a no-op. Older staff versions must return 409.
5. Create a new Pending disciplinary complaint and attach one photo. Sub-Warden
   byte read/list 404. Admin/Warden 200. Different Student metadata/bytes 404;
   unauthenticated 401; Security 403. Staff upload/removal 403.
6. History GET /complaints/{id}/updates preserves image_added/image_removed with
   actor, time and image_id alongside status/notes. Public metadata must not include
   object_key, payload_hash or public image URLs.
7. Storage-disabled test: stop server, set R2_ENABLED=false, restart executable.
   New text complaint submission still succeeds. Upload to that Pending complaint
   returns 503 without reserving a slot. Metadata list remains available. Restore
   R2_ENABLED=true and restart before further image tests.
8. Simulate provider outage only with dedicated dev settings: stop server, save
   correct R2_BUCKET, set a nonexistent bucket name, restart and attempt a valid
   Pending upload. It returns 503 while text reads/submission work. Restore bucket,
   restart and retry SAME image ID/file before cleanup retires the reservation.
   Never change production bucket privacy/IAM permissions just to simulate failure.
9. Check private bucket settings separately and verify image bytes cannot be read
   anonymously through a public bucket URL. Do not publish screenshots of credentials.

SQL (pgAdmin query tool, not PowerShell) to inspect your fixture:

```sql
SELECT image_id, state, content_type, size_bytes, uploaded_at, cleanup_at
FROM hostelhive.complaint_images
WHERE complaint_id = 'PASTE_COMPLAINT_ID';
SELECT action, image_id, actor_id, version, occurred_at
FROM hostelhive.complaint_updates
WHERE complaint_id = 'PASTE_COMPLAINT_ID'
ORDER BY version;
```

Retain frozen evidence/audit. Do not use database deletion as normal cleanup.
Student can remove photos only from dedicated Pending fixtures. Record actual
live checks; automated provider fakes are not evidence of real R2 privacy/access.

## Automated checks and completion

```powershell
.\scripts\test-backend-coverage.ps1
```

Existing CI automatically includes the new complaint image integration test,
shared normalization and migration 17 rollback/reapply. Tests use PostgreSQL plus
storage fakes for failed/lost writes, deletion retry, cleanup, cap races, integrity,
roles, archived/deactivated users and frozen evidence. No real R2 credentials in CI.
Local verification on 2026-10-13 passed formatting, dependencies, vet, build and
the full Go test suite. All eleven required PostgreSQL integration packages passed
without skips. Statement coverage: 84.8% overall (3571/4212), 83.9% complaint
(568/677). Focused photo/legacy Student image tests and full migration 1-17
apply/rollback/reapply passed. Local runs did not use race detection; GitHub CI,
live R2 reads/uploads, bucket privacy and IAM verification remain pending.

Commit: feat(complaint): add private image attachments
Body: Refs #66
PR: [DDP-#66] feat(complaint): add image attachments with access controls
Base develop. Review/test before committing; no automatic commit/push performed.
