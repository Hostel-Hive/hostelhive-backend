# Initialize the first administrator

There is no public bootstrap endpoint. A project owner with database
administrative access initializes the first local Admin once. First verify the
intended UID and email in **Firebase Authentication > Users**, and confirm that
the identity is enabled.

After applying migrations, run the following SQL against the intended database
using Supabase SQL Editor or pgAdmin. Replace both placeholders with the verified
Firebase UID and email. The transaction inserts or promotes the matching account
only while no active Admin exists.

```sql
BEGIN;
LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE;
INSERT INTO hostelhive.users (firebase_uid, email, role, is_active)
SELECT 'YOUR_VERIFIED_FIREBASE_UID', lower('YOUR_VERIFIED_EMAIL'), 'admin', TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM hostelhive.users WHERE role = 'admin' AND is_active
)
ON CONFLICT (firebase_uid) DO UPDATE
SET role = 'admin', is_active = TRUE
WHERE lower(hostelhive.users.email) = lower(EXCLUDED.email)
RETURNING user_id, firebase_uid, email, role, is_active;
COMMIT;
```

If no row is returned, an active Admin already exists or the UID/email pair does
not match. Investigate the existing records rather than removing the guard.

Sign in through Firebase and call `GET /api/v1/me` with the resulting ID token.
Verify `role: admin` and `is_active: true` before provisioning other accounts.
This SQL changes application access only; it does not create a Firebase identity
or set a password. Later account changes use the Admin APIs documented in
[Swagger](swagger-openapi.md).
