# Faculty-targeted announcements

Admin and Warden can publish to all Students or to one faculty. This extends the
existing announcement API; there is no department field and no new login flow.
Faculty names come from existing Student profiles, not a new fixed university
enum. Frontends should use the institution's agreed faculty names consistently.

## Publish

`POST /api/v1/announcements` with the publisher's Firebase bearer token:

```json
{
  "request_id": "GENERATE-A-NEW-UUID",
  "title": "Science faculty hostel meeting",
  "content": "Science faculty residents: meet in the common room at 7 PM.",
  "audience": "faculty",
  "faculty": "Science"
}
```

Faculty is required for `audience: faculty`, single-line, 1-120 Unicode characters
after trimming, and stored lowercase. It must match at least one unarchived
Student profile's faculty, ignoring case. There is no automatic aliasing:
`Arts` and `Art` are different.
Unknown/empty/invalid faculty or a conflicting all-Student target returns400
`invalid_input`. An existing faculty whose Students are all inactive can validly
receive zero alerts. Do not send recipient IDs, department or priority.

For a global broadcast, omit faculty and omit audience or set `all_students`.
Old clients remain compatible. Response `announcement.faculty` is the normalized
name for a targeted notice and `null` for a global notice. First publication
returns201; exact publisher-scoped retries return200/replayed=true. Changing
faculty, audience or content under the same request UUID returns409
`announcement_conflict`. Retry matching targets do not resnapshot recipients,
even if faculty membership has changed since publication.

## Read and delivery rules

- Admin/Warden see all notices and aggregate delivery/read summaries.
- Eligible Students see global notices plus notices for their current faculty.
  Both list totals and pagination are scoped. Detail for another faculty is404.
- Notifications are sent only to active Student-role users with unarchived
  profiles in the target at publication time. Missing, inactive, archived and
  other-faculty Students receive no delivery intent for that publication.
- Inbox listing, unread counts and mark-as-read recheck current faculty/profile
  eligibility. Leaving a faculty hides its notices and alerts; attempting to
  mark an inaccessible alert read returns404. Returning restores the existing
  read timestamp, rather than creating a new receipt.
- Joining a faculty permits reading historical faculty notices, but creates no
  retroactive alert. Concurrent faculty transfers are stabilized by profile locks.
- Summary counts describe the original recipient snapshot. Later transfers,
  deactivation or archival do not erase historical delivery/read receipts.
- Faculty targeting changes inbox delivery only. Ordinary announcements do not
  gain FCM phone push, email or SMS delivery through this feature.

## Migration and verification

Apply migration29 with the pinned migration CLI before running this backend:

```powershell
.\scripts\dev.ps1 -Migrate
```

The migration adds nullable `announcements.faculty`, audience consistency checks
and faculty-aware recipient selection. Existing global notices, IDs, timestamps,
read state and queued jobs are preserved; there is no alert backfill.

Rollback deliberately refuses while faculty-only publications exist, protecting
the recorded audience. Older backend binaries do not enforce faculty visibility:
stop all older API instances before publishing targeted notices, and do not roll
back application code to an incompatible version against targeted data. Retain
the schema and a compatible backend; do not change targets or delete published
records to force a production rollback. Disposable test fixtures are cleaned up
before rollback/reapply checks. Fresh databases still support complete rollback.

Automated tests cover both audiences, validation, HTTP role boundaries, scoped
feed/detail/inbox/unread/read operations, publication replay, faculty transfer,
recipient exclusions and migration/receipt preservation. CI also verifies that
rollback cannot widen an audience. Swagger remains at `/docs/` with updated models.
