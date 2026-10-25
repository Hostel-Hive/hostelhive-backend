# Restore an archived student profile

`POST /api/v1/students/{studentID}/restore` is restricted to active Admins.
Send a Firebase bearer token, no request body and no query parameters.
Migration 31 must be applied before use.

HTTP 200 returns `{"student": { ...profile and guardians... }, "restored": true}`.
The original student/account IDs, index number and guardian IDs are preserved.
Normal student listing and detail endpoints can read the restored profile again.
An eligible profile that is already current returns `restored: false`, without
changing its timestamp or creating a duplicate restoration audit event.

The transaction rechecks current Admin authority and requires the linked account
to be active with the student role. An inactive or changed-role account returns
409 `active_student_account_required`; reactivate or correct that account through
the separate account-management workflow first. Unknown student IDs return 404;
invalid UUIDs, any body or query parameters return 400. Wrong roles return 403.
Database failures return 503; retry using the same student ID and empty body.

The same database transaction stores the previous archive timestamp/actor and
the restoration timestamp/actor in `hostelhive.student_profile_restorations`.
Restore does not recover a removed photo, change attendance/presence, assign or
release beds, reactivate accounts, or send invitations. Existing allocation and
attendance history are retained. Existing user/index uniqueness constraints
continue to reserve student identity even while archived.

`GET /api/v1/students/archived` supplies the Admin-only archived directory.
It accepts the same `q`, `faculty`, `year`, `limit` and `offset` filters as the
current directory and returns the same page shape. It excludes current profiles.
The web portal exposes Current students / Archived students views; an Admin can
confirm Restore profile from an archived row and return to the refreshed list.
Unconfirmed restores can retry the same endpoint safely. Existing archive
permissions remain: Admin and Warden may archive, but only Admin may restore.
