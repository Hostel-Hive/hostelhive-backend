# Student self-service profile editing

Photo editing uses separate [own photo PUT/DELETE endpoints](student-own-profile-photo.md).
The profile PATCH body continues to accept contact phone and guardians only.

`PATCH /api/v1/me/student-profile` lets an active Student update their own contact
phone and guardian details from mobile or web. It uses the current Firebase ID
token, not an account or Student ID supplied by the client. The existing staff
`PUT /api/v1/students/{studentID}` remains unchanged.

## Request

Send `Authorization: Bearer <Firebase ID token>` and
`Content-Type: application/json`. No query parameters are accepted. Maximum body
size is16KiB. Supply at least one of the following fields:

| Field | Meaning |
|---|---|
| contact_phone | Replace the Student contact number; omitted means preserve |
| guardians | Replace the complete guardian list; omitted means preserve |

Phone-only example:

```json
{"contact_phone":"0771234567"}
```

Guardian replacement example:

```json
{
  "guardians": [
    {
      "name": "Example Guardian",
      "relationship": "Mother",
      "contact_phone": "0712345678"
    }
  ]
}
```

Both fields may be sent together in one atomic update. Strings are trimmed.
Phones follow the existing staff profile rules:7-32 permitted characters
(`+`, parentheses, digits, spaces, hyphens), with at least seven digits.
There must be1-10 complete guardians when that field is supplied. Each guardian
requires a nonblank name (max200 Unicode characters), relationship (max80) and a
valid phone. Control characters and invalid UTF-8 are rejected.

Do not send `guardian_id` or copy an entire GET response into PATCH. Guardian
replacement creates new guardian IDs; use the returned list. Phone-only updates
preserve existing guardian IDs. Guardian ordering follows the existing reader's
ID order, not a user-defined priority. Concurrent edits to different omitted
fields do not overwrite each other; for the same field/list, the last committed
edit wins. There is no per-guardian PATCH, ETag, version precondition or new
idempotency-key contract.

Empty objects, explicit nulls, empty guardian lists, duplicate JSON keys at any
level, unknown properties, trailing JSON and query selectors return400.
`user_id`, `student_id`, `index_no`, `full_name`, `faculty`, `year`, `email`,
`password`, `role`, `profile_image_url`, accommodation and presence cannot be
changed through this endpoint. If any part is invalid, nothing is updated.

## Response and access

HTTP200 returns the same safe Student aggregate as
`GET /api/v1/me/student-profile`: Student ID, index/name/faculty/year, contact phone,
email, guardians, optional image URL, accommodation and scanned presence. Internal
account IDs, Firebase UID, R2 object keys and account flags are not returned.
Responses use Cache-Control:no-store. Refresh the local profile from this response.

| Status | Meaning |
|---|---|
| 200 | Complete update committed; updated own profile returned |
| 400 invalid_input | Invalid/empty/unknown/null/duplicate input or any query |
| 401 unauthorized | Missing, invalid or expired Firebase login |
| 403 forbidden | Non-Student or inactive account; repository rechecks current access |
| 403 setup_required | Middleware requires Student invitation setup completion |
| 404 not_found | No unarchived Student profile belongs to the account |
| 413 payload_too_large | Body exceeds16KiB |
| 415 unsupported_media_type | Content-Type is not application/json |
| 503 student_profiles_unavailable | Database/transaction failure; no partial update |

Account role/activity and pending invitation setup are rechecked inside the write
transaction. A Student-row lock serializes edits with scans, archival and other
profile writers. Phone changes, guardian replacement and response assembly happen
in one transaction. Failure rolls back the entire change, including guardian IDs
and timestamps. Existing profile update triggers can queue an RTDB projection
refresh; this never records attendance or changes scanned presence. No account,
email/password, bed allocation, image or notification workflow is changed.

## Client integration and deployment

- Both mobile and web can use this same endpoint; authorization is based on the
  account, never on a claimed platform or User-Agent.
- Add editing screens in the respective frontend repositories. This backend
  change does not implement those screens or alter existing cross-origin/CORS
  deployment configuration. A separate-origin browser client must already have
  an allowed API route/proxy/CORS configuration.
- Keep staff-managed fields read-only in the Student form. Submit only changed
  fields, and preserve all desired guardians when replacing their list.
- No database migration or new environment variable is required. Existing schema
  version30 remains sufficient. Deploy the backend and refresh Swagger before
  enabling the new client action.

PowerShell example using a current **Student** token:

```powershell
$baseURL = 'https://hostelhive-backend-dev.onrender.com'
$body = @{ contact_phone = '0771234567' } | ConvertTo-Json
Invoke-RestMethod -Uri "$baseURL/api/v1/me/student-profile" `
    -Method Patch -Headers @{ Authorization = "Bearer $studentToken" } `
    -ContentType 'application/json' -Body $body -ErrorAction Stop
```

Verification covers roles and account state, ownership, strict parsing, partial
updates, guardian limits, transaction rollback, concurrency, preserved attendance
and accommodation, and compatibility with Admin/Warden editing. Use disposable
test accounts for live acceptance. No Supabase mutation or Render deployment is
performed by implementing the source changes.
