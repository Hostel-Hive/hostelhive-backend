# Student-owned profile photos

Both mobile and web clients use these endpoints with a current Firebase ID token.
Only active Students who have completed required invitation setup can write.
The server derives the student from authentication. No student ID, user ID or
query parameter is accepted. Admin/Warden photo management stays unchanged.

## Upload or replace

`PUT /api/v1/me/student-profile/image`

- `Authorization: Bearer <Firebase ID token>`
- `Content-Type: image/jpeg` or `image/png`
- Body: raw file bytes, **not** JSON, base64 or multipart form data.
- Maximum input: 5 MiB. Decoded image: at most 12 million pixels and 4096 per axis.
- The server checks the actual format and normalizes the image, stripping metadata.

HTTP 200:

```json
{
  "student_id": "11111111-1111-4111-8111-111111111111",
  "profile_image_url": "/api/v1/me/student-profile/image"
}
```

Fetch this path from the backend with the student's Authorization header.
It is not a public R2 URL. Browser clients can fetch a Blob and display an object
URL, revoking it when replaced or when the component/account is cleared.

## Remove

`DELETE /api/v1/me/student-profile/image`

Send Authorization only; omit body and query parameters. Success is HTTP 204 with
no response body. An eligible profile with no photo also returns 204. After removal,
the existing GET image endpoint returns 404. Account/profile deletion is separate.

## Errors and consistency

| HTTP | Meaning |
|---|---|
| 400 | Invalid image, format mismatch, query selector or DELETE body |
| 401 | Missing, invalid or expired authentication |
| 403 | Inactive account, non-Student or incomplete setup (`setup_required`) |
| 404 | Student profile missing/archived; GET also returns this for a missing photo |
| 413 | Input exceeds 5 MiB |
| 415 | Unsupported or missing upload Content-Type |
| 503 | Storage disabled (`student_images_unavailable`), database/storage failure or timeout |

Responses are `Cache-Control: no-store`. Validation or failed replacement preserves
the current reference. Transactions serialize changes to the same student and
recheck role, activity, setup and ownership before storage writes. Concurrent
successful changes take effect in commit order. A timeout or disconnected response
can have an uncertain outcome: refetch the profile/image before retrying. PUT retries
may create a new object; there is no operation-ID replay contract.

Replacing or removing a photo queues old objects for the existing durable cleanup
worker. Failed upload reservations also remain discoverable for cleanup. Storage
deletion is asynchronous; the authenticated download reference is removed immediately.
Academic/contact/guardian/allocation and scan-derived presence are unchanged.
The existing Student update trigger may refresh a derived realtime projection.

## Deployment and frontend integration

No migration or new environment variable is required; use the existing schema and
private R2 configuration (`R2_ENABLED=true` plus the existing R2 credentials).
Both write endpoints return 503 when storage is disabled. Deploy the updated backend
before connecting UI actions. Cross-origin web clients still require the existing
approved proxy/CORS arrangement. Never embed R2 credentials in either frontend.

Use a file picker, show upload/removal progress, prevent duplicate clicks, handle
authentication refresh, and refresh the own profile/photo after success. Profile
photo controls in mobile/web are separate frontend work. Swagger documents the
binary upload under the **Students** tag; supply the FirebaseBearer authorization.

PowerShell example with an existing `$studentToken`:

```powershell
$baseURL = 'https://hostelhive-backend-dev.onrender.com'
$headers = @{ Authorization = "Bearer $studentToken" }
Invoke-RestMethod -Method Put -Uri "$baseURL/api/v1/me/student-profile/image" -Headers $headers -ContentType 'image/jpeg' -InFile 'C:\path\photo.jpg'
Invoke-RestMethod -Method Delete -Uri "$baseURL/api/v1/me/student-profile/image" -Headers $headers
```
