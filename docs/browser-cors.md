# Browser CORS and the Vercel frontend

For browsers calling the Render API directly from the deployed web portal,
configure this on the **backend** in Render's Environment settings:

```ini
CORS_ALLOWED_ORIGINS=https://web-kohl-delta-90.vercel.app
```

Deploy this backend version and restart/redeploy after saving the setting.
The frontend can keep:

```ini
VITE_API_BASE_URL=https://hostelhive-backend-dev.onrender.com
```

No Supabase credentials, Firebase service-account file or R2 secrets belong in
the frontend. No database migration is needed. This change does not deploy itself
to Render or change its environment settings.

## Policy

- An empty/unset setting preserves existing behavior with CORS disabled.
- Multiple exact origins can be comma-separated, for example a production domain
  and `http://localhost:5173`. Paths, credentials, queries, fragments, wildcard
  origins and `null` are rejected at configuration load. A trailing slash is
  normalized away. Include the scheme and any nondefault port.
- No automatic permission is given to other `vercel.app` domains or previews.
  Add a reviewed preview/custom hostname explicitly when needed.
- Valid preflight OPTIONS requests return 204 with an empty body **before**
  route matching, Firebase verification and account/database lookups.
- Allowed methods: GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS. This advertises
  transport support; actual routes retain their existing methods and permissions.
- Allowed requested headers: Authorization, Content-Type, Idempotency-Key,
  Accept, Cache-Control, Pragma. Header matching is case-insensitive.
- Unknown origins/methods/requested headers on preflights return 403 without a
  CORS grant. Duplicate Origin or requested-method headers are also denied.
- Actual responses for an allowed origin include its exact
  Access-Control-Allow-Origin, including 401/403/404/405/503 errors. Browser
  clients can read Content-Disposition, Retry-After and WWW-Authenticate.
- Preflight permission can be cached by the browser for 600 seconds. Vary headers
  distinguish Origin and both preflight request headers. Existing Vary values
  and application Cache-Control headers are preserved.
- Cookie credentials are not enabled. The web client supplies the existing
  Firebase Authorization bearer header and uses `credentials: 'omit'`.
- Requests without an allowed Origin retain ordinary routing/authentication.
  In particular, same-origin Swagger and non-browser mobile/scanner/CLI clients
  are unaffected. Unlisted origins receive no cross-origin response-read grant.

CORS is not authentication or a network access control list. An allowed browser
still needs a valid Firebase token, active account, completed required setup and
the correct role/ownership. Preflight success alone does not authorize an API call.

## Deployment verification

After merging and deploying this backend, use PowerShell:

```powershell
$corsHeaders = @{
    Origin = 'https://web-kohl-delta-90.vercel.app'
    'Access-Control-Request-Method' = 'GET'
    'Access-Control-Request-Headers' = 'authorization,content-type'
}
$corsResult = Invoke-WebRequest -Method Options -Uri 'https://hostelhive-backend-dev.onrender.com/api/v1/me' -Headers $corsHeaders
$corsResult.StatusCode
$corsResult.Headers
```

Expect 204, Access-Control-Allow-Origin equal to the exact Vercel origin, and
Authorization/Content-Type in Access-Control-Allow-Headers. Do not send a token
on this preflight. Then sign in through the actual Vercel site and confirm the
subsequent authenticated GET /api/v1/me succeeds. A preflight for a different,
unlisted hostname must not return an Access-Control-Allow-Origin grant.

The middleware applies to uploads and downloads too. Existing file size,
content-type, database, storage and authorization errors remain independent of
CORS. This does not change existing Swagger operations or response DTOs.
