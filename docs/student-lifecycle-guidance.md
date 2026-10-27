# Student lifecycle guidance (#134)

Admin `GET /api/v1/users` responses now include three optional fields for student accounts with an existing current or archived profile:

| Field | Meaning |
| --- | --- |
| `student_profile_id` | Original student profile UUID |
| `student_profile_archived` | Whether that profile is archived |
| `student_has_active_bed` | Whether an allocation without an end time exists |

All three fields are omitted for non-student accounts and accounts without a student profile. False boolean values are included when a profile exists. These fields are populated by the Admin account-list query; they are not added to `/me` or account-mutation responses. They are read-only snapshots for staff guidance and do not change permissions or mutation eligibility checks.

Deactivation suspends access and retains profile/accommodation state. Activation does not restore an archived profile or assign a bed. Restoration requires an active student account and does not assign accommodation. Archival retains account access and bed allocations; permanent departures should use the atomic End hostel stay endpoint documented in `student-checkout.md`.

No new migration or configuration variable is required for #134. Restart the backend after updating code. Existing restoration and checkout migrations remain prerequisites for their own endpoints.

PostgreSQL integration coverage checks lifecycle metadata before departure, after checkout and after restoration with a new allocation. Non-student account metadata is omitted. Swagger describes the optional fields and the separation of account/profile/accommodation actions.
