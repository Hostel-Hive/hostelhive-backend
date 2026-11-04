# Faculty management

Migration **34** adds a central catalogue. It imports faculties from existing
Students (including archived profiles) and historical announcements. Case and
surrounding whitespace variants share one ID. It does not invent default entries.
Migration 33 is an earlier dependency in this checkout; deploy matching migrations
and backend code before using the new routes.

## Endpoints

All routes require a current Firebase ID token and active local account.
Swagger tag: **Faculties**.

| Method and path | Role | Purpose |
| --- | --- | --- |
| GET `/api/v1/faculties` | Admin, Warden | List active choices by default |
| GET `/api/v1/faculties/{facultyID}` | Admin, Warden | Read one current/inactive faculty |
| POST `/api/v1/faculties` | Admin | Create an active faculty |
| PATCH `/api/v1/faculties/{facultyID}` | Admin | Rename the same faculty |
| POST `/api/v1/faculties/{facultyID}/deactivate` | Admin | Stop new selections |
| POST `/api/v1/faculties/{facultyID}/activate` | Admin | Make the same faculty selectable again |

Create and rename use `Content-Type: application/json` with only:

```json
{"name":"Science"}
```

Names are trimmed, 1–120 Unicode characters, without control characters. Unknown
or duplicate JSON fields, invalid UTF-8 and trailing JSON are rejected. Maximum
JSON body is 8 KiB. Activate/deactivate and GET use no body. Other than list,
these routes reject query parameters. There is no faculty delete endpoint.

Create returns **201**. Read, rename, activate and deactivate return **200**:

```json
{
  "faculty_id":"11111111-1111-4111-8111-111111111111",
  "name":"Science",
  "is_active":true,
  "created_at":"2026-10-30T10:00:00Z",
  "updated_at":"2026-10-30T10:00:00Z"
}
```

The UUID above is fictional; use the actual creation/list response.

## Dropdowns and filtering

`GET /api/v1/faculties?limit=20&offset=0` returns
`items`, `limit`, `offset`, `has_more`; follow pages until `has_more=false`.
Names sort case-insensitively, followed by ID. Maximum limit is 100; maximum
offset is 100000. Supported `active` values are `true` (default), `false`, `all`.
Use default active choices for registration and announcement forms. Administration
and historical Student filters may explicitly request `active=all`.

Existing registration, profile and announcement payloads still use the **current
name** in `faculty`. They do not switch to `faculty_id` in this release. Select
the name from the catalogue instead of accepting arbitrary input. Student list
filtering and all-Student announcements retain their existing contracts.

## Deactivation and activation

For example, deactivating Science removes it from default active lists. New
Student profiles, Student invitations (including invitation CSV), profile CSV,
transfers into Science and new Science-targeted announcements are rejected.
Unknown and former renamed names are also rejected for new assignments/notices.

Existing Science Students remain assigned. Staff may edit a profile while keeping
its existing inactive faculty. Self-service contact edits are unaffected. Existing
announcements and inbox items remain readable by eligible Students. Global notices
still reach eligible Students, including those assigned to inactive faculties.

Activation restores selection of the same current name with the **same ID**.
It does not create a replacement faculty, change presence, reactivate accounts,
reassign beds or send retroactive notifications.

## Renaming and history

Renaming changes existing Student faculty labels, including archived profiles,
inside the same transaction. Old announcement content and faculty-name snapshots
are preserved. Reserved historical names link those old audiences to the same
faculty identity, so eligible Students retain access and other faculties do not
gain access. Announcement and notification reads apply this same identity rule.

An old faculty name cannot be reused for an unrelated faculty. It also cannot be
selected for new registrations/notices; use the current catalogue name. An Admin
may rename the original faculty back to its own former name.

## Retries and errors

- Repeating activation/deactivation, or renaming to the exact current display
  name, returns 200 without another state/audit change.
- Create does not accept a request UUID. Name uniqueness prevents duplicates;
  a repeated create returns 409. After an uncertain response, check the catalogue.
- Existing exact invitation/publication retries are checked before the active
  faculty requirement, so a committed result can be recovered after deactivation
  or rename. Do not replace their original request UUID/body when retrying.
- 400 `invalid_input`: malformed input or unknown/inactive/current-name mismatch
  in Student registration/new targeted announcements. CSV reports this per row.
- 401 `unauthorized`: missing/invalid credential.
- 403 `forbidden` (or authentication `setup_required`): account/role not allowed.
- 404 `not_found`: faculty ID is absent.
- 409 `faculty_name_conflict`: current or historical name belongs to a faculty.
- 413 `request_too_large`, 415 `unsupported_media_type`, or
  503 `faculties_unavailable` as applicable.

All faculty lifecycle writes recheck current Admin authority in the database,
serialize against account/registration/publication changes, and write an audit
entry atomically. Audit failure rolls back state, renamed Student labels and
historical-name reservation. Use the backend APIs for catalogue/assignment writes;
direct SQL edits bypass application validation.

Migration 34 can roll back before lifecycle use. It refuses rollback after audit,
deactivation or historical rename aliases exist, protecting identity/history.

## Frontend follow-up

This backend change does not add a web faculty management screen. The frontend
needs an Admin screen and active-faculty dropdowns using these APIs. Existing
free-text forms must be updated to select a current active catalogue name.
